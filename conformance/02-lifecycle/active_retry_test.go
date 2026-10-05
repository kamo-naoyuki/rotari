package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestRetryFinalJobInsideActiveRun(t *testing.T) {
	covers(t, "RUN-15", "SAFE-2", "SAFE-11", "STATE-5")
	for _, viaWeb := range []bool{false, true} {
		t.Run(fmt.Sprintf("web=%t", viaWeb), func(t *testing.T) {
			e := support.NewEnv(t)
			project := "active-retry"
			marker := filepath.Join(e.Root, "fail-once")
			failed := support.AddedJobID(t, e.MustRotari("add", "-p", project, "--job-name", "fail-once", "--", "sh", "-c", "if [ ! -f \""+marker+"\" ]; then touch \""+marker+"\"; exit 9; fi; exit 0"))
			slow := support.AddedJobID(t, e.MustRotari("add", "-p", project, "--job-name", "slow", "--", "sh", "-c", "sleep 6"))
			e.MustRotari("run", "-p", project, "--async", "--quiet")
			var run struct {
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", project, "--json").Stdout), &run); err != nil || run.RunID == "" {
				t.Fatalf("active run ID missing: %v", err)
			}
			runDir := filepath.Join(e.Base, "projects", project, "runs", run.RunID)
			support.WaitUntil(t, 15*time.Second, func() (bool, string) {
				finalResults, err := filepath.Glob(filepath.Join(runDir, failed, "attempts", "*", "final_result.json"))
				return err == nil && len(finalResults) > 0 && support.JobProcesses(t, e.Root, slow) > 0, "failed job has not finalized while slow job is active"
			})

			var accepted []string
			if viaWeb {
				got := e.HTTPPostJSON(e.StartWeb()+"/api/retry-active", map[string]any{
					"project_name": project, "run_id": run.RunID, "job_ids": []string{failed}, "selection": "failed", "partial_array": true,
				})
				if got.Status != 200 {
					t.Fatalf("active retry API: status %d: %s", got.Status, got.Body)
				}
				var response struct {
					Accepted []string `json:"accepted_job_ids"`
				}
				if err := json.Unmarshal([]byte(got.Body), &response); err != nil {
					t.Fatal(err)
				}
				accepted = response.Accepted
			} else {
				optionResult := e.Rotari("retry", "-p", project, "--job-id", failed, "--retry", "1")
				if optionResult.Code == 0 || !strings.Contains(optionResult.Stderr, "cannot be changed by retry inside an active run") {
					t.Fatalf("active retry accepted run-level --retry: %s", optionResult)
				}
				result := e.MustRotari("retry", "-p", project, "--job-id", failed)
				if !strings.Contains(result.Stdout, "run_id="+run.RunID) || !strings.Contains(result.Stdout, "accepted=1") {
					t.Fatalf("active retry output = %q; want accepted same run %s", result.Stdout, run.RunID)
				}
				accepted = []string{failed}
			}
			if len(accepted) != 1 || accepted[0] != failed {
				t.Fatalf("accepted jobs = %v, want [%s]", accepted, failed)
			}
			requestFiles, err := filepath.Glob(filepath.Join(runDir, "retry_*.json"))
			if err != nil || len(requestFiles) < 3 {
				t.Fatalf("retry protocol files = %v, %v; want accepting, request, and response", requestFiles, err)
			}
			for _, protocolFile := range requestFiles {
				data, err := os.ReadFile(protocolFile)
				if err != nil {
					t.Fatal(err)
				}
				var versioned struct {
					StateVersion int `json:"state_version"`
				}
				if err := json.Unmarshal(data, &versioned); err != nil || versioned.StateVersion != 3 {
					t.Fatalf("protocol file %s version = %d, %v; want 3", protocolFile, versioned.StateVersion, err)
				}
			}
			if entries, err := os.ReadDir(runDir); err == nil {
				for _, entry := range entries {
					if entry.IsDir() && entry.Name() != failed && entry.Name() != slow {
						t.Fatalf("run directory has non-job subdirectory %q", entry.Name())
					}
				}
			}
			attemptDirs, err := filepath.Glob(filepath.Join(runDir, failed, "attempts", "*"))
			if err != nil || len(attemptDirs) != 2 {
				t.Fatalf("failed job attempts = %d, want 2: %v", len(attemptDirs), err)
			}
			if current := e.MustRotari("show", "-p", project, "--json").Stdout; !strings.Contains(current, `"run_id":"`+run.RunID+`"`) {
				t.Fatalf("retry created a different run: %s", current)
			}
			if waited := e.Rotari("wait", "-p", project, "--timeout", "30s"); waited.Code != 0 {
				t.Fatalf("wait for retried run failed: %s", waited)
			}
		})
	}
}
