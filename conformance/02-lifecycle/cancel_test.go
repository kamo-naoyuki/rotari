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
	return strings.Count(e.MustRotari("jobs", "--basedir", e.Base, run.Project, "--format", "%a").Stdout, "-"+run.Jobs[0]+"-")
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
				Diagnoses []struct {
					Name string `json:"name"`
				} `json:"diagnoses"`
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
			// Rules did not analyze a cancelled job, so the diagnosis summary
			// does not count it, and the text never prints no_match.
			if len(summary.Diagnoses) != 0 {
				t.Errorf("diagnoses = %+v, want none for cancelled jobs", summary.Diagnoses)
			}
			if text := e.MustRotari("lineage", "-p", run.Project, run.RunID).Stdout; strings.Contains(text, "no_match") {
				t.Errorf("lineage text prints no_match:\n%s", text)
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

// TestAsyncStartHintsWork starts a run with --async and runs the commands
// its message suggests, as printed: the message ends its last line, its
// cancel command names the run and cancels it, and its wait command waits
// for that run.
func TestAsyncStartHintsWork(t *testing.T) {
	covers(t, "CAN-7", "DUR-8")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "hints", "--", "sleep", "30")
	started := e.MustRotari("run", "-p", "hints", "--async").Stdout
	if !strings.HasSuffix(started, "\n") {
		t.Fatalf("the start message does not end its last line: %q", started)
	}
	hint := func(prefix string) []string {
		for _, line := range strings.Split(started, "\n") {
			if fields := strings.Fields(line); len(fields) > 1 && fields[0] == "rotari" && fields[1] == prefix {
				for index := range fields {
					fields[index] = strings.Trim(fields[index], "'")
				}
				return fields[1:]
			}
		}
		t.Fatalf("the start message suggests no rotari %s:\n%s", prefix, started)
		return nil
	}
	cancel, wait := hint("cancel"), hint("wait")
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "hints", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	runID := shown.RunID
	assertAsyncDetachedClientLabels(t, e, runID)
	var runStatus struct {
		Lifecycle string `json:"lifecycle"`
		Client    struct {
			Mode  string `json:"mode"`
			State string `json:"state"`
		} `json:"client_status"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "hints", "--run-id", runID, "--json").Stdout), &runStatus); err != nil {
		t.Fatal(err)
	}
	if runStatus.Lifecycle != "running" || runStatus.Client.Mode != "async" || runStatus.Client.State != "detached" {
		t.Fatalf("async run lifecycle/client = %q/%+v", runStatus.Lifecycle, runStatus.Client)
	}
	if cancel[len(cancel)-1] != runID {
		t.Fatalf("the cancel hint %v does not name run %s", cancel, runID)
	}
	e.MustRotari(cancel...)
	if result := e.Rotari(wait...); !strings.Contains(result.Stdout+result.Stderr, runID) {
		t.Fatalf("the wait hint %v did not wait for run %s: %s", wait, runID, result)
	}
	if check := e.Rotari("check", "hints").Stdout; !projectFinished(check) {
		t.Fatalf("the run did not finish after the hinted cancel and wait: %s", check)
	}
}

func assertAsyncDetachedClientLabels(t *testing.T, e *support.Env, runID string) {
	t.Helper()
	for _, args := range [][]string{
		{"runs", "--project-name", "hints"},
		{"show", "-p", "hints", "--run-id", runID},
	} {
		if out := e.MustRotari(args...).Stdout; !strings.Contains(out, "detached (async)") {
			t.Errorf("%v does not distinguish an async detached run:\n%s", args, out)
		}
	}
	web := e.StartWeb()
	for _, endpoint := range []string{"/api/run?project_name=hints&run_id=" + runID, "/api/project?project_name=hints"} {
		body := e.HTTPGet(web + endpoint).Body
		if !strings.Contains(body, `"client_label":"detached (async)"`) {
			t.Errorf("%s omitted the shared async detached label:\n%s", endpoint, body)
		}
	}
}

// TestWholeRunCancelRecordsCancelledRun checks that a whole-run cancel, from
// the CLI or the Web UI, records the run as cancelled with exit code 1, which
// every view shows, while a run whose jobs were cancelled one by one stays
// failed.
func TestWholeRunCancelRecordsCancelledRun(t *testing.T) {
	covers(t, "CAN-8")
	for _, test := range []struct {
		name   string
		cancel func(e *support.Env, run support.ActiveRun)
		want   string
	}{
		{"whole run, CLI", func(e *support.Env, run support.ActiveRun) { e.MustRotari("cancel", "-p", run.Project) }, "cancelled"},
		{"whole run, Web", func(e *support.Env, run support.ActiveRun) {
			got := e.HTTPPostJSON(e.StartWeb()+"/api/cancel-run", map[string]any{"project_name": run.Project, "run_id": run.RunID})
			if got.Status != 200 {
				t.Fatalf("cancel-run: status %d: %s", got.Status, got.Body)
			}
		}, "cancelled"},
		{"each job", func(e *support.Env, run support.ActiveRun) {
			for _, job := range run.Jobs {
				e.MustRotari("cancel", "-p", run.Project, job)
			}
		}, "failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			run := e.StartRun("live", 2, true)
			test.cancel(e, run)
			waited := e.Rotari("wait", "-p", run.Project, "--timeout", "30s")
			title := "=== Run " + test.want + " ==="
			if waited.Code != 1 || !strings.Contains(waited.Stdout, title) {
				t.Fatalf("wait = exit %d, want 1 and %q: %s", waited.Code, title, waited)
			}
			var shown struct {
				Lifecycle string `json:"lifecycle"`
				Summary   *struct {
					Status   string `json:"status"`
					ExitCode int    `json:"exit_code"`
				} `json:"summary"`
			}
			if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", run.Project, "--run-id", run.RunID, "--json").Stdout), &shown); err != nil || shown.Summary == nil {
				t.Fatalf("show --json: %v", err)
			}
			if shown.Summary.Status != test.want || shown.Summary.ExitCode != 1 || shown.Lifecycle != test.want {
				t.Errorf("show --json status %q exit %d lifecycle %q, want %s and exit 1", shown.Summary.Status, shown.Summary.ExitCode, shown.Lifecycle, test.want)
			}
			var runs []struct {
				RunID     string `json:"run_id"`
				Lifecycle string `json:"lifecycle"`
			}
			if err := json.Unmarshal([]byte(e.MustRotari("runs", "--basedir", e.Base, run.Project, "--json").Stdout), &runs); err != nil || len(runs) != 1 || runs[0].Lifecycle != test.want {
				t.Errorf("runs --json = %+v, %v; want lifecycle %s", runs, err, test.want)
			}
		})
	}
}
