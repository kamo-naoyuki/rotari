package web

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMain(m *testing.M) { os.Exit(support.Run(m)) }

func covers(t *testing.T, _ ...string) { t.Helper() }

func TestHistorySearchAcrossProjects(t *testing.T) {
	covers(t, "WEB-2")
	e := support.NewEnv(t)
	alphaRunID, _ := e.FinishedJobRun("alpha")
	e.FinishedJobRun("beta")
	alphaSummaryPath := filepath.Join(e.Base, "projects", "alpha", "runs", alphaRunID, "summary.json")
	alphaSummaryData, err := os.ReadFile(alphaSummaryPath)
	if err != nil {
		t.Fatal(err)
	}
	var alphaSummary map[string]any
	if err := json.Unmarshal(alphaSummaryData, &alphaSummary); err != nil {
		t.Fatal(err)
	}
	results := alphaSummary["results"].([]any)
	results[0].(map[string]any)["diagnoses"] = []map[string]string{{"name": "Permission denied"}, {"name": "Legacy custom diagnosis"}}
	alphaSummaryData, err = json.Marshal(alphaSummary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alphaSummaryPath, alphaSummaryData, 0o600); err != nil {
		t.Fatal(err)
	}
	base := e.StartWeb()
	absBase, err := filepath.Abs(e.Base)
	if err != nil {
		t.Fatal(err)
	}
	response := e.HTTPPostJSON(base+"/api/history-search", map[string]any{
		"scopes":  []map[string]string{{"basedir_id": base64.RawURLEncoding.EncodeToString([]byte(filepath.Clean(absBase)))}},
		"filters": []map[string]string{{"target": "job", "field": "command", "word": "true"}},
		"limit":   50,
	})
	if response.Status != 200 {
		t.Fatalf("history search: status %d: %s", response.Status, response.Body)
	}
	var result struct {
		Total int `json:"total"`
		Rows  []struct {
			ProjectName string `json:"project_name"`
			Target      string `json:"target"`
			JobID       string `json:"job_id"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || len(result.Rows) != 2 {
		t.Fatalf("history search result = %#v, want jobs from both projects", result)
	}
	projects := map[string]bool{}
	for _, row := range result.Rows {
		if row.Target != "job" || row.JobID == "" {
			t.Errorf("history search row = %#v, want a job result", row)
		}
		projects[row.ProjectName] = true
	}
	if !projects["alpha"] || !projects["beta"] {
		t.Errorf("cross-project results = %#v, want alpha and beta", projects)
	}
	searchTotal := func(word string, caseSensitive, fuzzy bool) int {
		t.Helper()
		response := e.HTTPPostJSON(base+"/api/history-search", map[string]any{
			"scopes":         []map[string]string{{"basedir_id": base64.RawURLEncoding.EncodeToString([]byte(filepath.Clean(absBase)))}},
			"target":         "job",
			"case_sensitive": caseSensitive,
			"fuzzy":          fuzzy,
			"filters":        []map[string]string{{"target": "job", "field": "command", "word": word}},
		})
		if response.Status != 200 {
			t.Fatalf("history search for %q: status %d: %s", word, response.Status, response.Body)
		}
		var got struct {
			Total int `json:"total"`
		}
		if err := json.Unmarshal([]byte(response.Body), &got); err != nil {
			t.Fatal(err)
		}
		return got.Total
	}
	if got := searchTotal("TRUE", false, false); got != 2 {
		t.Errorf("default ignore-case total = %d, want 2", got)
	}
	if got := searchTotal("TRUE", true, false); got != 0 {
		t.Errorf("case-sensitive total = %d, want 0", got)
	}
	if got := searchTotal("ture", false, true); got != 2 {
		t.Errorf("fuzzy typo total = %d, want 2", got)
	}
	if got := searchTotal("ture", false, false); got != 0 {
		t.Errorf("exact typo total = %d, want 0", got)
	}
	searchRange := map[string]string{"basedir_id": base64.RawURLEncoding.EncodeToString([]byte(filepath.Clean(absBase)))}
	diagnosisOptions := e.HTTPGet(base + "/api/history-search-diagnoses")
	if diagnosisOptions.Status != 200 {
		t.Fatalf("diagnosis options: status %d: %s", diagnosisOptions.Status, diagnosisOptions.Body)
	}
	var options struct {
		Diagnoses []string `json:"diagnoses"`
	}
	if err := json.Unmarshal([]byte(diagnosisOptions.Body), &options); err != nil {
		t.Fatal(err)
	}
	containsDiagnosis := false
	containsLegacyDiagnosis := false
	for _, name := range options.Diagnoses {
		containsDiagnosis = containsDiagnosis || name == "Permission denied"
		containsLegacyDiagnosis = containsLegacyDiagnosis || name == "Legacy custom diagnosis"
	}
	if !containsDiagnosis || containsLegacyDiagnosis {
		t.Fatalf("diagnosis options = %#v, want standard names without saved custom labels", options.Diagnoses)
	}
	diagnosisSearch := e.HTTPPostJSON(base+"/api/history-search", map[string]any{
		"scopes":  []map[string]string{searchRange},
		"target":  "job",
		"filters": []map[string]string{{"target": "job", "field": "diagnosis", "word": "Permission denied"}},
	})
	if diagnosisSearch.Status != 200 {
		t.Fatalf("diagnosis search: status %d: %s", diagnosisSearch.Status, diagnosisSearch.Body)
	}
	var diagnosisResult struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(diagnosisSearch.Body), &diagnosisResult); err != nil {
		t.Fatal(err)
	}
	if diagnosisResult.Total != 1 {
		t.Errorf("diagnosis search total = %d, want 1", diagnosisResult.Total)
	}
}
