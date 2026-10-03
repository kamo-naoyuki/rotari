package lifecycle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestWholeRunCancelFinishesRun(t *testing.T) {
	covers(t, "CAN-1", "CAN-2")
	for _, test := range []struct {
		name  string
		async bool
		web   bool
	}{
		{"sync run, CLI", false, false}, {"sync run, Web", false, true},
		{"async run, CLI", true, false}, {"async run, Web", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			run := e.StartRun("live", 2, test.async)
			if test.web {
				got := e.HTTPPostJSON(e.StartWeb()+"/api/cancel-run", map[string]any{"project_name": run.Project, "run_id": run.RunID})
				if got.Status != 200 {
					t.Fatalf("cancel-run: status %d: %s", got.Status, got.Body)
				}
			} else {
				e.MustRotari("cancel", "-p", run.Project)
			}
			support.WaitUntil(t, 30*time.Second, func() (bool, string) {
				alive := support.JobProcesses(t, e.Root, "")
				return alive == 0, fmt.Sprintf("%d job processes still running", alive)
			})
			support.WaitUntil(t, 30*time.Second, func() (bool, string) {
				check := e.Rotari("check", run.Project).Stdout
				return projectFinished(check), "check: " + check
			})
			results := runResults(t, e, run)
			if len(results) != len(run.Jobs) {
				t.Errorf("summary records %d results, want %d", len(results), len(run.Jobs))
			}
			e.MustRotari("add", "-p", run.Project, "--", "true")
		})
	}
}

func TestCancelWaitReturnsAfterRunFinishes(t *testing.T) {
	covers(t, "CAN-3")
	for _, async := range []bool{false, true} {
		t.Run(fmt.Sprintf("async=%t", async), func(t *testing.T) {
			e := support.NewEnv(t)
			run := e.StartRun("live", 1, async)
			if r := e.Rotari("cancel", "-p", run.Project, "--wait"); r.Code != 0 {
				t.Fatalf("cancel --wait failed: %s", r)
			}
			if check := e.Rotari("check", run.Project).Stdout; !projectFinished(check) {
				t.Errorf("run not finished: %s", check)
			}
		})
	}
}

func TestCancelJobStopsOnlyThatJob(t *testing.T) {
	covers(t, "CAN-4")
	for _, web := range []bool{false, true} {
		t.Run(fmt.Sprintf("web=%t", web), func(t *testing.T) {
			e := support.NewEnv(t)
			run := e.StartRun("live", 2, false, "--retry", "1")
			cancelled, other := run.Jobs[0], run.Jobs[1]
			if web {
				got := e.HTTPPostJSON(e.StartWeb()+"/api/cancel-job", map[string]any{"project_name": run.Project, "run_id": run.RunID, "job_id": cancelled})
				if got.Status != 200 {
					t.Fatalf("cancel-job: status %d: %s", got.Status, got.Body)
				}
			} else {
				e.MustRotari("cancel", "-p", run.Project, cancelled)
			}
			support.WaitUntil(t, 30*time.Second, func() (bool, string) {
				return support.JobProcesses(t, e.Root, cancelled) == 0, "cancelled job is still running"
			})
			time.Sleep(2 * time.Second)
			if alive := support.JobProcesses(t, e.Root, other); alive == 0 {
				t.Errorf("job %s stopped too; only %s was cancelled", other, cancelled)
			}
			if attempts := jobAttempts(t, e, run); attempts != 1 {
				t.Errorf("cancelled job has %d attempts, want 1", attempts)
			}
		})
	}
}

func projectFinished(check string) bool {
	return strings.Contains(check, "lock=none") && !strings.Contains(check, "state=interrupted") && !strings.Contains(check, "state=running")
}

func runResults(t *testing.T, e *support.Env, run support.ActiveRun) []json.RawMessage {
	t.Helper()
	var shown struct {
		Summary *struct {
			Results []json.RawMessage `json:"results"`
		} `json:"summary"`
	}
	r := e.MustRotari("show", "-p", run.Project, "--run-id", run.RunID, "--json")
	if err := json.Unmarshal([]byte(r.Stdout), &shown); err != nil || shown.Summary == nil {
		t.Fatalf("run %s has no summary: %s", run.RunID, r)
	}
	return shown.Summary.Results
}

func jobAttempts(t *testing.T, e *support.Env, run support.ActiveRun) int {
	t.Helper()
	return strings.Count(e.MustRotari("jobs", run.Project, "--format", "%a").Stdout, "-"+run.Jobs[0]+"-")
}

func TestCancelledJobsReadAsCancelled(t *testing.T) {
	covers(t, "CAN-5")
	for _, test := range []struct {
		name string
		// jobFirst cancels the first job alone before the whole run.
		jobFirst bool
	}{{"whole run", false}, {"one job, then the run", true}} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			run := e.StartRun("live", 2, true)
			if test.jobFirst {
				e.MustRotari("cancel", "-p", run.Project, run.Jobs[0])
				support.WaitUntil(t, 30*time.Second, func() (bool, string) {
					return support.JobProcesses(t, e.Root, run.Jobs[0]) == 0, "cancelled job is still running"
				})
			}
			e.MustRotari("cancel", "-p", run.Project, "--wait")

			var summary struct {
				Failures []struct {
					Kind string `json:"kind"`
					Jobs []struct {
						ID string `json:"id"`
					} `json:"jobs"`
				} `json:"failures"`
			}
			if err := json.Unmarshal([]byte(e.MustRotari("lineage", "-p", run.Project, "--json", run.RunID).Stdout), &summary); err != nil {
				t.Fatal(err)
			}
			var cancelled []string
			for _, group := range summary.Failures {
				if group.Kind != "cancelled" {
					t.Errorf("failure group %+v, want every stopped job cancelled", group)
				}
				for _, job := range group.Jobs {
					cancelled = append(cancelled, job.ID)
				}
			}
			sort.Strings(cancelled)
			want := append([]string(nil), run.Jobs...)
			sort.Strings(want)
			if strings.Join(cancelled, ",") != strings.Join(want, ",") {
				t.Fatalf("cancelled jobs = %v, want %v", cancelled, want)
			}
			table := e.MustRotari("show", "-p", run.Project, "--run-id", run.RunID, "--filter-failure-kind", "cancelled").Stdout
			for _, job := range run.Jobs {
				if !strings.Contains(table, job) {
					t.Errorf("--filter-failure-kind cancelled does not list %s:\n%s", job, table)
				}
			}
		})
	}
}

// TestCancelledPendingJobNeverStarts cancels a job that waits for another,
// then the job it waits for, and checks that the waiting job never ran and
// records a cancelled result.
func TestCancelledPendingJobNeverStarts(t *testing.T) {
	covers(t, "CAN-6")
	e := support.NewEnv(t)
	marker := filepath.Join(e.Root, "after-ran")
	slow := support.AddedJobID(t, e.MustRotari("add", "-p", "pending", "--job-name", "slow", "--", "sleep", "30"))
	after := support.AddedJobID(t, e.MustRotari("add", "-p", "pending", "--job-name", "after", "--depends-on-finished", "slow", "--", "touch", marker))
	e.MustRotari("run", "-p", "pending", "--async", "--quiet")
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, slow) == 1, "the slow job did not start"
	})
	e.MustRotari("cancel", "-p", "pending", after)
	e.MustRotari("cancel", "-p", "pending", slow)
	e.Rotari("wait", "-p", "pending", "--timeout", "30s")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the cancelled pending job ran")
	}
	var shown struct {
		Summary struct {
			Results []struct {
				ID    string `json:"id"`
				Error string `json:"error"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "pending", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	for _, result := range shown.Summary.Results {
		if result.ID == after && !strings.Contains(result.Error, "cancelled") {
			t.Fatalf("the cancelled pending job's result: %+v", result)
		}
	}
}
