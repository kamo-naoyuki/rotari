package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

type fingerprintPayload struct {
	Command          []string           `json:"command"`
	Environment      []fingerprintEnv   `json:"environment,omitempty"`
	WorkingDirectory string             `json:"working_directory,omitempty"`
	Matrix           []fingerprintParam `json:"matrix,omitempty"`
	ArrayTask        *int               `json:"array_task,omitempty"`
}

type fingerprintEnv struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type fingerprintParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type FingerprintJob struct {
	ID          string
	Fingerprint string
}

type FingerprintMatch struct {
	CurrentID string
	SourceID  string
	Method    string
}

// ClassifyFingerprintJobs identifies current execution units that are new or
// changed relative to a source queue using the selected matching mode.
func ClassifyFingerprintJobs(current, source []FingerprintJob, mode string) (changed, newJobs map[string]bool) {
	changed = make(map[string]bool)
	newJobs = make(map[string]bool)
	matches := MatchFingerprintJobsByMode(current, source, mode)
	sourceByID := make(map[string]FingerprintJob, len(source))
	for _, job := range source {
		sourceByID[job.ID] = job
	}
	matched := make(map[string]bool, len(matches))
	for _, match := range matches {
		matched[match.CurrentID] = true
		if sourceJob, ok := sourceByID[match.SourceID]; ok {
			currentJob := findFingerprintJob(current, match.CurrentID)
			if currentJob.Fingerprint != sourceJob.Fingerprint {
				changed[match.CurrentID] = true
			}
		}
	}
	for _, job := range current {
		if !matched[job.ID] {
			newJobs[job.ID] = true
		}
	}
	return changed, newJobs
}

func findFingerprintJob(jobs []FingerprintJob, id string) FingerprintJob {
	for _, job := range jobs {
		if job.ID == id {
			return job
		}
	}
	return FingerprintJob{}
}

const (
	MatchByJobID            = "job-id"
	MatchByFingerprint      = "fingerprint"
	MatchByIDAndFingerprint = "id-and-fingerprint"
)

// Fingerprint returns the SHA-256 fingerprint of one expanded execution unit.
// It uses only persisted job inputs that users explicitly put in the queue.
func Fingerprint(command QueuedCommand, arrayTask *int) (string, error) {
	payload := fingerprintPayload{
		Command:          append([]string(nil), command.Command...),
		Environment:      fingerprintEnvironment(command.Environment),
		WorkingDirectory: fingerprintWorkingDirectory(command.WorkingDirectory),
		Matrix:           fingerprintMatrix(command.Matrix),
	}
	if arrayTask != nil {
		task := *arrayTask
		payload.ArrayTask = &task
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

// QueueFingerprintJobs expands queue commands into the execution units used
// by fingerprint matching. Array tasks are separate units; matrix values are
// already present on their expanded commands.
func QueueFingerprintJobs(queue Queue) ([]FingerprintJob, error) {
	jobs := make([]FingerprintJob, 0)
	for _, command := range queue.Commands {
		if command.Array == nil {
			fingerprint, err := Fingerprint(command, nil)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, FingerprintJob{ID: command.ID, Fingerprint: fingerprint})
			continue
		}
		for _, task := range ArrayTaskIDs(command.Array) {
			taskID := taskID(command.ID, task)
			fingerprint, err := Fingerprint(command, &task)
			if err != nil {
				return nil, err
			}
			jobs = append(jobs, FingerprintJob{ID: taskID, Fingerprint: fingerprint})
		}
	}
	return jobs, nil
}

// MatchFingerprintJobs matches current queue units to source units. Job IDs
// are consumed first. Remaining units are grouped by fingerprint; a group is
// matched by occurrence only when both sides have the same count.
func MatchFingerprintJobs(current, source []FingerprintJob) []FingerprintMatch {
	return MatchFingerprintJobsByMode(current, source, MatchByIDAndFingerprint)
}

// MatchFingerprintJobsByMode applies the selected identity sources. In
// fingerprint mode, job IDs are ignored; in the combined mode they are
// consumed before fingerprint candidates are counted.
func MatchFingerprintJobsByMode(current, source []FingerprintJob, mode string) []FingerprintMatch {
	sourceByID := make(map[string]int, len(source))
	for index, job := range source {
		sourceByID[job.ID] = index
	}
	usedSource := make([]bool, len(source))
	usedCurrent := make([]bool, len(current))
	matches := make([]FingerprintMatch, 0)
	if mode == MatchByFingerprint {
		return matchFingerprintOnly(current, source)
	}
	for index, job := range current {
		sourceIndex, ok := sourceByID[job.ID]
		if !ok || usedSource[sourceIndex] {
			continue
		}
		usedCurrent[index] = true
		usedSource[sourceIndex] = true
		matches = append(matches, FingerprintMatch{CurrentID: job.ID, SourceID: source[sourceIndex].ID, Method: "job-id"})
	}
	if mode == MatchByJobID {
		return matches
	}

	currentByFingerprint := fingerprintGroups(current, usedCurrent)
	sourceByFingerprint := fingerprintGroups(source, usedSource)
	for fingerprint, currentIndexes := range currentByFingerprint {
		sourceIndexes := sourceByFingerprint[fingerprint]
		if len(currentIndexes) != len(sourceIndexes) {
			continue
		}
		for index := range currentIndexes {
			currentIndex := currentIndexes[index]
			sourceIndex := sourceIndexes[index]
			matches = append(matches, FingerprintMatch{CurrentID: current[currentIndex].ID, SourceID: source[sourceIndex].ID, Method: "fingerprint"})
		}
	}
	return matches
}

func matchFingerprintOnly(current, source []FingerprintJob) []FingerprintMatch {
	used := make([]bool, len(current))
	sourceUsed := make([]bool, len(source))
	return matchRemainingFingerprintJobs(current, source, used, sourceUsed)
}

func matchRemainingFingerprintJobs(current, source []FingerprintJob, usedCurrent, usedSource []bool) []FingerprintMatch {
	matches := make([]FingerprintMatch, 0)
	currentByFingerprint := fingerprintGroups(current, usedCurrent)
	sourceByFingerprint := fingerprintGroups(source, usedSource)
	for fingerprint, currentIndexes := range currentByFingerprint {
		sourceIndexes := sourceByFingerprint[fingerprint]
		if len(currentIndexes) != len(sourceIndexes) {
			continue
		}
		for index := range currentIndexes {
			matches = append(matches, FingerprintMatch{CurrentID: current[currentIndexes[index]].ID, SourceID: source[sourceIndexes[index]].ID, Method: "fingerprint"})
		}
	}
	return matches
}

func fingerprintGroups(jobs []FingerprintJob, used []bool) map[string][]int {
	groups := make(map[string][]int)
	for index, job := range jobs {
		if !used[index] {
			groups[job.Fingerprint] = append(groups[job.Fingerprint], index)
		}
	}
	return groups
}

func taskID(commandID string, task int) string {
	return fmt.Sprintf("%s-%d", commandID, task)
}

func fingerprintEnvironment(values []string) []fingerprintEnv {
	last := make(map[string]string)
	for _, value := range values {
		name, environmentValue, ok := strings.Cut(value, "=")
		if ok {
			last[name] = environmentValue
		}
	}
	names := make([]string, 0, len(last))
	for name := range last {
		names = append(names, name)
	}
	sort.Strings(names)
	environment := make([]fingerprintEnv, 0, len(names))
	for _, name := range names {
		environment = append(environment, fingerprintEnv{Name: name, Value: last[name]})
	}
	return environment
}

func fingerprintWorkingDirectory(value string) string {
	if value == "" {
		return ""
	}
	return path.Clean(value)
}

func fingerprintMatrix(matrix *MatrixSpec) []fingerprintParam {
	if matrix == nil {
		return nil
	}
	values := append([]MatrixValue(nil), matrix.Values...)
	sort.Slice(values, func(left, right int) bool { return values[left].Name < values[right].Name })
	parameters := make([]fingerprintParam, 0, len(values))
	for _, value := range values {
		parameters = append(parameters, fingerprintParam{Name: value.Name, Value: value.Value})
	}
	return parameters
}
