package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

// These tests run the rotari binary inside the scheduler container, so the
// run engine, the job wrappers, and the Slurm or PBS executor all work
// against a real scheduler. SCHEDULER_STATE_DIR is the host directory mounted
// at /state in the container.

var (
	containerRotariOnce sync.Once
	containerRotariErr  error
)

// containerRotari builds a static rotari into the shared state directory once
// and returns its path inside the container.
func containerRotari(t *testing.T) string {
	t.Helper()
	stateDir := os.Getenv("SCHEDULER_STATE_DIR")
	if stateDir == "" {
		t.Fatal("SCHEDULER_STATE_DIR must name the host directory mounted at /state")
	}
	containerRotariOnce.Do(func() {
		binDir := filepath.Join(stateDir, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			containerRotariErr = err
			return
		}
		build := exec.Command("go", "build", "-o", filepath.Join(binDir, "rotari"), ".")
		build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
		if output, err := build.CombinedOutput(); err != nil {
			containerRotariErr = fmt.Errorf("go build: %v\n%s", err, output)
		}
	})
	if containerRotariErr != nil {
		t.Fatal(containerRotariErr)
	}
	return "/state/bin/rotari"
}

// containerProject is one rotari basedir inside the container.
type containerProject struct {
	t       *testing.T
	config  schedulerContainerTestConfig
	rotari  string
	baseDir string
}

func newContainerProject(t *testing.T) containerProject {
	t.Helper()
	config := requireSchedulerContainerTest(t)
	project := containerProject{t: t, config: config, rotari: containerRotari(t), baseDir: fmt.Sprintf("/state/r%d", time.Now().UnixNano())}
	project.shell(30*time.Second, "mkdir -p "+executor.ShellQuote(project.baseDir))
	return project
}

// shell runs script in the container and returns its output and exit code.
func (project containerProject) shell(timeout time.Duration, script string) (string, int) {
	project.t.Helper()
	args := []string{"exec"}
	if project.config.user != "" {
		args = append(args, "--user", project.config.user)
	}
	args = append(args, project.config.container, "sh", "-lc", script)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if ctx.Err() != nil {
		project.t.Fatalf("docker exec timed out after %s: %s\n%s", timeout, script, output)
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		return string(output), exitError.ExitCode()
	}
	if err != nil {
		project.t.Fatalf("docker exec failed: %v\n%s", err, output)
	}
	return string(output), 0
}

// rotariCommand runs rotari with the project's basedir.
func (project containerProject) rotariCommand(timeout time.Duration, args ...string) (string, int) {
	project.t.Helper()
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, executor.ShellQuote(arg))
	}
	env := "ROTARI_BASEDIR=" + executor.ShellQuote(project.baseDir) + " ROTARI_MASTERDIR=" + executor.ShellQuote(project.baseDir+"/master")
	return project.shell(timeout, env+" "+executor.ShellQuote(project.rotari)+" "+strings.Join(quoted, " "))
}

// add queues a job for the scheduler under test.
func (project containerProject) add(args ...string) {
	project.t.Helper()
	full := append([]string{"add", "-p", "p", "--quiet", "--executor", project.config.executor}, args...)
	if output, code := project.rotariCommand(30*time.Second, full...); code != 0 {
		project.t.Fatalf("rotari %v exit = %d\n%s", full, code, output)
	}
}

// run starts the queue synchronously and returns its exit code.
func (project containerProject) run(args ...string) int {
	project.t.Helper()
	full := append([]string{"run", "-p", "p"}, args...)
	output, code := project.rotariCommand(4*time.Minute, full...)
	project.t.Logf("rotari %v exit = %d\n%s", full, code, output)
	return code
}

// results returns the latest run's results by job name, or by job ID for
// unnamed jobs.
func (project containerProject) results() map[string]model.JobResult {
	project.t.Helper()
	output, code := project.rotariCommand(30*time.Second, "show", "-p", "p", "-r", "latest", "--json")
	if code != 0 {
		project.t.Fatalf("rotari show exit = %d\n%s", code, output)
	}
	var shown showJSON
	if err := json.Unmarshal([]byte(output), &shown); err != nil || shown.Summary == nil {
		project.t.Fatalf("show --json = %v\n%s", err, output)
	}
	names := make(map[string]string)
	for _, job := range model.QueueToJobs(shown.Commands.Commands) {
		names[job.ID] = job.Name
	}
	results := make(map[string]model.JobResult)
	for _, result := range shown.Summary.Results {
		key := names[result.ID]
		if key == "" {
			key = result.ID
		}
		results[key] = result
	}
	return results
}

// attempts counts the attempts recorded for jobs whose ID matches pattern.
func (project containerProject) attempts(jobPattern string) int {
	project.t.Helper()
	output, _ := project.shell(30*time.Second, fmt.Sprintf("ls -d %s/projects/p/runs/*/%s/attempts/* 2>/dev/null | wc -l", project.baseDir, jobPattern))
	var count int
	fmt.Sscan(strings.TrimSpace(output), &count)
	return count
}

func (project containerProject) file(name string) string {
	return project.baseDir + "/" + name
}

func TestSchedulerContainerRetriesFailedJobImmediately(t *testing.T) {
	project := newContainerProject(t)
	marker := project.file("flaky.marker")
	project.add("--job-name", "flaky", "--retry", "1", "--", "sh", "-c", fmt.Sprintf("if [ -f %[1]s ]; then exit 0; fi; touch %[1]s; exit 1", executor.ShellQuote(marker)))
	if code := project.run(); code != 0 {
		t.Fatalf("run exit = %d, want the retry to succeed", code)
	}
	if result := project.results()["flaky"]; result.ExitCode != 0 {
		t.Fatalf("flaky = %#v, want success on retry", result)
	}
	if attempts := project.attempts("*"); attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

// jobIDs returns the latest run's job IDs by job name.
func (project containerProject) jobIDs() map[string]string {
	project.t.Helper()
	output, code := project.rotariCommand(30*time.Second, "show", "-p", "p", "-r", "latest", "--json")
	if code != 0 {
		project.t.Fatalf("rotari show exit = %d\n%s", code, output)
	}
	var shown showJSON
	if err := json.Unmarshal([]byte(output), &shown); err != nil {
		project.t.Fatalf("show --json = %v\n%s", err, output)
	}
	ids := make(map[string]string)
	for _, job := range model.QueueToJobs(shown.Commands.Commands) {
		ids[job.Name] = job.ID
	}
	return ids
}

// attemptTimestamp reads a timestamp field from a JSON file of a job's
// attempt, such as submitted_at from job.json or finished_at from
// status.json. The files belong to the container user, so they are read
// inside the container.
func (project containerProject) attemptTimestamp(jobID, file, field string) string {
	project.t.Helper()
	output, _ := project.shell(30*time.Second, fmt.Sprintf("cat %s/projects/p/runs/*/%s/attempts/*/%s", project.baseDir, jobID, file))
	var values map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &values); err != nil {
		project.t.Fatalf("%s of %s: %v\n%s", file, jobID, err, output)
	}
	value, _ := values[field].(string)
	if value == "" {
		project.t.Fatalf("%s of %s has no %s:\n%s", file, jobID, field, output)
	}
	return value
}

func TestSchedulerContainerRefillsConcurrencySlots(t *testing.T) {
	project := newContainerProject(t)
	project.add("--job-name", "slow", "--", "sleep", "25")
	for _, name := range []string{"quick-1", "quick-2", "quick-3"} {
		project.add("--job-name", name, "--", "sleep", "1")
	}
	if code := project.run("--batch-concurrency", "2"); code != 0 {
		t.Fatalf("run exit = %d", code)
	}
	// rotari keeps two jobs submitted. When quick-1 finishes, quick-2 takes
	// its slot while slow still runs; waiting for whole batches would submit
	// quick-2 only after slow finished. The check uses rotari's submission
	// time, not execution order, because the test scheduler may run only one
	// job at a time.
	ids := project.jobIDs()
	submitted := project.attemptTimestamp(ids["quick-2"], "job.json", "submitted_at")
	slowFinished := project.attemptTimestamp(ids["slow"], "status.json", "finished_at")
	if submitted >= slowFinished {
		t.Fatalf("quick-2 was submitted at %s, not before slow finished at %s", submitted, slowFinished)
	}
}

func TestSchedulerContainerStopsTimedOutJob(t *testing.T) {
	project := newContainerProject(t)
	project.add("--job-name", "hang", "--timeout", "3s", "--", "sleep", "300")
	started := time.Now()
	if code := project.run(); code == 0 {
		t.Fatal("run exit = 0, want the timed-out job to fail the run")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Minute {
		t.Fatalf("run with a 3s timeout took %s", elapsed)
	}
	if result := project.results()["hang"]; result.ExitCode != 124 || !strings.Contains(result.Error, "timed out after 3s") {
		t.Fatalf("hang = %#v, want exit 124 and a timeout error", result)
	}
}

func TestSchedulerContainerResolvesDependencies(t *testing.T) {
	project := newContainerProject(t)
	project.add("--job-name", "prepare", "--", "false")
	project.add("--job-name", "train", "--depends-on", "prepare", "--", "true")
	project.add("--job-name", "collect", "--depends-on-finished", "prepare", "--", "true")
	project.run()
	results := project.results()
	if results["prepare"].ExitCode == 0 || results["train"].Error != "blocked by failed dependency" || results["collect"].ExitCode != 0 {
		t.Fatalf("results = %#v, want train blocked and collect run after prepare failed", results)
	}
}

func TestSchedulerContainerRetriesOnlyFailedArrayTask(t *testing.T) {
	project := newContainerProject(t)
	marker := project.file("task2.marker")
	project.add("--job-name", "sweep", "--array", "1-3", "--retry", "1", "--", "sh", "-c",
		fmt.Sprintf(`if [ "$ROTARI_ARRAY_TASK_ID" = 2 ] && [ ! -f %[1]s ]; then touch %[1]s; exit 1; fi`, executor.ShellQuote(marker)))
	if code := project.run(); code != 0 {
		t.Fatalf("run exit = %d, want every task to succeed", code)
	}
	for _, name := range []string{"sweep[1]", "sweep[2]", "sweep[3]"} {
		if result := project.results()[name]; result.ExitCode != 0 {
			t.Fatalf("%s = %#v, want success", name, result)
		}
	}
	if attempts := project.attempts("*-2"); attempts != 2 {
		t.Fatalf("task 2 attempts = %d, want 2", attempts)
	}
	if attempts := project.attempts("*-1"); attempts != 1 {
		t.Fatalf("task 1 attempts = %d, want 1 (only the failed task is retried)", attempts)
	}
}
