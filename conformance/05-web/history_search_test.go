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
	e.FinishedJobRun("alpha")
	e.FinishedJobRun("beta")
	base := e.StartWeb()
	absBase, err := filepath.Abs(e.Base)
	if err != nil {
		t.Fatal(err)
	}
	response := e.HTTPPostJSON(base+"/api/history-search", map[string]any{
		"basedir_ids": []string{base64.RawURLEncoding.EncodeToString([]byte(filepath.Clean(absBase)))},
		"filters":     []map[string]string{{"target": "job", "field": "command", "word": "true"}},
		"limit":       50,
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
}
