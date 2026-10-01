package web

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	HistorySearchProject     = "project"
	HistorySearchRun         = "run"
	HistorySearchJob         = "job"
	HistorySearchPageSize    = 50
	HistorySearchMaxPageSize = 200
)

type HistorySearchFilter struct {
	Target string `json:"target"`
	Field  string `json:"field"`
	Word   string `json:"word"`
	Join   string `json:"join,omitempty"`
}

type HistorySearchRequest struct {
	Target        string                `json:"target,omitempty"`
	CaseSensitive bool                  `json:"case_sensitive,omitempty"`
	Fuzzy         bool                  `json:"fuzzy,omitempty"`
	Filters       []HistorySearchFilter `json:"filters"`
	From          string                `json:"from,omitempty"`
	To            string                `json:"to,omitempty"`
	Offset        int                   `json:"offset,omitempty"`
	Limit         int                   `json:"limit,omitempty"`
}

type HistorySearchScope struct {
	BaseDirID   string `json:"basedir_id"`
	ProjectName string `json:"project_name,omitempty"`
	RunID       string `json:"run_id,omitempty"`
}

type HistorySearchRecord struct {
	BaseDirID   string
	BaseDirPath string
	ProjectName string
	RunID       string
	RunName     string
	RunStatus   string
	RunExitCode *int
	RunStarted  string
	RunFinished string
	JobID       string
	JobName     string
	JobStatus   string
	JobStage    string
	Command     string
	Executor    string
	AttemptID   string
	JobExitCode *int
	Timestamp   string
}

type HistorySearchRow struct {
	Target      string `json:"target"`
	BaseDirID   string `json:"basedir_id"`
	BaseDirPath string `json:"basedir_path"`
	ProjectName string `json:"project_name"`
	RunID       string `json:"run_id,omitempty"`
	RunName     string `json:"run_name,omitempty"`
	RunStatus   string `json:"run_status,omitempty"`
	JobID       string `json:"job_id,omitempty"`
	JobName     string `json:"job_name,omitempty"`
	JobStatus   string `json:"job_status,omitempty"`
	Command     string `json:"command,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
}

type HistorySearchResponse struct {
	Rows   []HistorySearchRow `json:"rows"`
	Total  int                `json:"total"`
	Offset int                `json:"offset"`
	Limit  int                `json:"limit"`
}

func ValidateHistorySearchRequest(request HistorySearchRequest) error {
	if len(request.Filters) == 0 || len(request.Filters) > 20 {
		return fmt.Errorf("history search requires between 1 and 20 conditions")
	}
	if request.Offset < 0 {
		return fmt.Errorf("history search offset must not be negative")
	}
	if request.Offset > 10000000 {
		return fmt.Errorf("history search offset exceeds the maximum")
	}
	if request.Limit < 0 || request.Limit > HistorySearchMaxPageSize {
		return fmt.Errorf("history search limit must be 0 (default) or at most %d", HistorySearchMaxPageSize)
	}
	from, err := parseHistorySearchTime(request.From, "start")
	if err != nil {
		return err
	}
	to, err := parseHistorySearchTime(request.To, "end")
	if err != nil {
		return err
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return fmt.Errorf("history search start time must not be after end time")
	}
	if request.Target != "" && historySearchTargetRank(request.Target) < 0 {
		return fmt.Errorf("invalid history search result target %q", request.Target)
	}
	for index, filter := range request.Filters {
		if err := validateHistorySearchFilter(index, filter); err != nil {
			return err
		}
		if request.Target != "" && historySearchTargetRank(filter.Target) > historySearchTargetRank(request.Target) {
			return fmt.Errorf("history search condition %d is more specific than result target %q", index+1, request.Target)
		}
	}
	return nil
}

func validateHistorySearchFilter(index int, filter HistorySearchFilter) error {
	if strings.TrimSpace(filter.Word) == "" || len(filter.Word) > 256 {
		return fmt.Errorf("history search condition %d requires a word of 1 to 256 characters", index+1)
	}
	fields, ok := historySearchFields[filter.Target]
	if !ok {
		return fmt.Errorf("invalid history search target %q", filter.Target)
	}
	if !fields[filter.Field] {
		return fmt.Errorf("invalid history search field %q for %s", filter.Field, filter.Target)
	}
	if index > 0 && filter.Join != "and" && filter.Join != "or" {
		return fmt.Errorf("history search condition %d must join with and or or", index+1)
	}
	return nil
}

func parseHistorySearchTime(value, label string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid history search %s time", label)
	}
	return parsed, nil
}

var historySearchFields = map[string]map[string]bool{
	HistorySearchProject: {"project_name": true},
	HistorySearchRun:     {"run_id": true, "run_name": true, "status": true, "exit_code": true},
	HistorySearchJob:     {"job_id": true, "job_name": true, "status": true, "command": true, "stage": true, "executor": true, "attempt_id": true, "exit_code": true},
}

// SearchHistory evaluates conditions against the selected hierarchy level.
// When conditions target multiple levels, results use the most specific level;
// parent-level conditions are evaluated against each result's ancestors.
func SearchHistory(records []HistorySearchRecord, request HistorySearchRequest) (HistorySearchResponse, error) {
	if err := ValidateHistorySearchRequest(request); err != nil {
		return HistorySearchResponse{}, err
	}
	from, err := parseHistorySearchTime(request.From, "start")
	if err != nil {
		return HistorySearchResponse{}, err
	}
	to, err := parseHistorySearchTime(request.To, "end")
	if err != nil {
		return HistorySearchResponse{}, err
	}
	target := historySearchResultTarget(request)
	rows := collectHistorySearchRows(records, request, target, from, to)
	total := len(rows)
	limit := request.Limit
	if limit == 0 {
		limit = HistorySearchPageSize
	}
	rows = pageHistorySearchRows(rows, request.Offset, limit)
	return HistorySearchResponse{Rows: rows, Total: total, Offset: request.Offset, Limit: limit}, nil
}

func historySearchResultTarget(request HistorySearchRequest) string {
	if request.Target != "" {
		return request.Target
	}
	target := HistorySearchProject
	for _, filter := range request.Filters {
		if historySearchTargetRank(filter.Target) > historySearchTargetRank(target) {
			target = filter.Target
		}
	}
	return target
}

func collectHistorySearchRows(records []HistorySearchRecord, request HistorySearchRequest, target string, from, to time.Time) []HistorySearchRow {
	unique := make(map[string]HistorySearchRow)
	for _, record := range records {
		if !historySearchRecordMatches(record, request, target, from, to) {
			continue
		}
		row := historySearchRow(record, target)
		key := historySearchRowKey(row)
		if existing, ok := unique[key]; !ok || historyTimestampAfter(row.Timestamp, existing.Timestamp) {
			unique[key] = row
		}
	}
	rows := make([]HistorySearchRow, 0, len(unique))
	for _, row := range unique {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Timestamp != rows[j].Timestamp {
			return rows[i].Timestamp > rows[j].Timestamp
		}
		if rows[i].BaseDirID != rows[j].BaseDirID {
			return rows[i].BaseDirID < rows[j].BaseDirID
		}
		if rows[i].ProjectName != rows[j].ProjectName {
			return rows[i].ProjectName < rows[j].ProjectName
		}
		if rows[i].RunID != rows[j].RunID {
			return rows[i].RunID < rows[j].RunID
		}
		return rows[i].JobID < rows[j].JobID
	})
	return rows
}

func historySearchRecordMatches(record HistorySearchRecord, request HistorySearchRequest, target string, from, to time.Time) bool {
	return historySearchInWindow(record, target, from, to) && historySearchMatchesWithOptions(record, target, request.Filters, !request.CaseSensitive, request.Fuzzy)
}

func pageHistorySearchRows(rows []HistorySearchRow, offset, limit int) []HistorySearchRow {
	if offset >= len(rows) {
		return []HistorySearchRow{}
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end]
}

func historySearchInWindow(record HistorySearchRecord, target string, from, to time.Time) bool {
	value := record.Timestamp
	if target == HistorySearchRun {
		value = historyLatestTimestamp(record.RunFinished, record.RunStarted)
	} else if target == HistorySearchProject {
		value = historyLatestTimestamp(record.Timestamp, record.RunFinished, record.RunStarted)
	}
	if value == "" {
		return from.IsZero() && to.IsZero()
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return from.IsZero() && to.IsZero()
	}
	return (from.IsZero() || !parsed.Before(from)) && (to.IsZero() || !parsed.After(to))
}

func historyLatestTimestamp(values ...string) string {
	latest := ""
	var latestTime time.Time
	for _, value := range values {
		parsed, err := time.Parse(time.RFC3339, value)
		if err == nil && (latest == "" || parsed.After(latestTime)) {
			latest = value
			latestTime = parsed
		}
	}
	return latest
}

func historyTimestampAfter(value, previous string) bool {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value > previous
	}
	previousTime, err := time.Parse(time.RFC3339, previous)
	return err != nil || parsed.After(previousTime)
}

func historySearchMatches(record HistorySearchRecord, target string, filters []HistorySearchFilter) bool {
	return historySearchMatchesWithOptions(record, target, filters, true, false)
}

func historySearchMatchesWithOptions(record HistorySearchRecord, target string, filters []HistorySearchFilter, ignoreCase, fuzzy bool) bool {
	matched := historySearchFilterMatchesWithOptions(record, target, filters[0], ignoreCase, fuzzy)
	for _, filter := range filters[1:] {
		next := historySearchFilterMatchesWithOptions(record, target, filter, ignoreCase, fuzzy)
		if filter.Join == "or" {
			matched = matched || next
		} else {
			matched = matched && next
		}
	}
	return matched
}

func historySearchFilterMatches(record HistorySearchRecord, resultTarget string, filter HistorySearchFilter) bool {
	return historySearchFilterMatchesWithOptions(record, resultTarget, filter, true, false)
}

func historySearchFilterMatchesWithOptions(record HistorySearchRecord, resultTarget string, filter HistorySearchFilter, ignoreCase, fuzzy bool) bool {
	if historySearchTargetRank(filter.Target) > historySearchTargetRank(resultTarget) {
		return false
	}
	value := historySearchFieldValue(record, filter.Target, filter.Field)
	word := strings.TrimSpace(filter.Word)
	if ignoreCase {
		value, word = strings.ToLower(value), strings.ToLower(word)
	}
	if historySearchIsEnumeratedField(filter) {
		return value == word
	}
	if strings.Contains(value, word) {
		return true
	}
	return fuzzy && historySearchFuzzyMatch(value, word)
}

func historySearchIsEnumeratedField(filter HistorySearchFilter) bool {
	return filter.Field == "status" || (filter.Target == HistorySearchJob && filter.Field == "executor")
}

func historySearchFuzzyMatch(value, word string) bool {
	queryTokens := historySearchTokens(word)
	valueTokens := historySearchTokens(value)
	if len(queryTokens) == 0 || len(valueTokens) == 0 {
		return false
	}
	for _, query := range queryTokens {
		queryRunes := []rune(query)
		if len(queryRunes) < 4 {
			return false
		}
		maxDistance := 1
		if len(queryRunes) >= 8 {
			maxDistance = 2
		}
		found := false
		for _, candidate := range valueTokens {
			candidateRunes := []rune(candidate)
			if absInt(len(queryRunes)-len(candidateRunes)) > maxDistance {
				continue
			}
			if historySearchEditDistanceAtMost(queryRunes, candidateRunes, maxDistance) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func historySearchTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func historySearchEditDistanceAtMost(left, right []rune, limit int) bool {
	if absInt(len(left)-len(right)) > limit {
		return false
	}
	previous := make([]int, len(right)+1)
	var previousPrevious []int
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range left {
		current := make([]int, len(right)+1)
		current[0] = leftIndex + 1
		rowMinimum := current[0]
		for rightIndex, rightRune := range right {
			cost := 1
			if leftRune == rightRune {
				cost = 0
			}
			current[rightIndex+1] = min(
				current[rightIndex]+1,
				previous[rightIndex+1]+1,
				previous[rightIndex]+cost,
			)
			if leftIndex > 0 && rightIndex > 0 && left[leftIndex] == right[rightIndex-1] && left[leftIndex-1] == right[rightIndex] {
				current[rightIndex+1] = min(current[rightIndex+1], previousPrevious[rightIndex-1]+1)
			}
			rowMinimum = min(rowMinimum, current[rightIndex+1])
		}
		if rowMinimum > limit {
			return false
		}
		previousPrevious, previous = previous, current
	}
	return previous[len(right)] <= limit
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func historySearchFieldValue(record HistorySearchRecord, target, field string) string {
	switch target {
	case HistorySearchProject:
		return record.ProjectName
	case HistorySearchRun:
		switch field {
		case "run_id":
			return record.RunID
		case "run_name":
			return record.RunName
		case "status":
			return record.RunStatus
		case "exit_code":
			if record.RunExitCode != nil {
				return strconv.Itoa(*record.RunExitCode)
			}
		}
	case HistorySearchJob:
		switch field {
		case "job_id":
			return record.JobID
		case "job_name":
			return record.JobName
		case "status":
			return record.JobStatus
		case "command":
			return record.Command
		case "stage":
			return record.JobStage
		case "executor":
			return record.Executor
		case "attempt_id":
			return record.AttemptID
		case "exit_code":
			if record.JobExitCode != nil {
				return strconv.Itoa(*record.JobExitCode)
			}
		}
	}
	return ""
}

func historySearchTargetRank(target string) int {
	switch target {
	case HistorySearchProject:
		return 0
	case HistorySearchRun:
		return 1
	case HistorySearchJob:
		return 2
	default:
		return -1
	}
}

func historySearchRow(record HistorySearchRecord, target string) HistorySearchRow {
	row := HistorySearchRow{
		Target: target, BaseDirID: record.BaseDirID, BaseDirPath: record.BaseDirPath,
		ProjectName: record.ProjectName, Timestamp: record.Timestamp,
	}
	if target == HistorySearchRun || target == HistorySearchJob {
		row.RunID, row.RunName, row.RunStatus = record.RunID, record.RunName, record.RunStatus
	}
	if target == HistorySearchRun {
		row.Timestamp = historyLatestTimestamp(record.RunFinished, record.RunStarted)
		if row.Timestamp == "" {
			row.Timestamp = record.Timestamp
		}
	}
	if target == HistorySearchJob {
		row.JobID, row.JobName, row.JobStatus, row.Command = record.JobID, record.JobName, record.JobStatus, record.Command
	}
	return row
}

func historySearchRowKey(row HistorySearchRow) string {
	switch row.Target {
	case HistorySearchProject:
		return row.BaseDirID + "\x00" + row.ProjectName
	case HistorySearchRun:
		return row.BaseDirID + "\x00" + row.ProjectName + "\x00" + row.RunID
	default:
		return row.BaseDirID + "\x00" + row.ProjectName + "\x00" + row.RunID + "\x00" + row.JobID
	}
}
