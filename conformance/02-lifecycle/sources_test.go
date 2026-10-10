package lifecycle

import (
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func gitRepo(t *testing.T, dir, script string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "job.sh"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "first")
	return git(t, dir, "rev-parse", "HEAD")
}

type recordedSource struct {
	Root     string `json:"root"`
	VCS      string `json:"vcs"`
	CommitID string `json:"commit_id"`
	Dirty    bool   `json:"dirty"`
}

func runSources(t *testing.T, e *support.Env, runID string) []recordedSource {
	t.Helper()
	var summary struct {
		Run struct {
			Sources []recordedSource `json:"sources"`
		} `json:"run"`
	}
	output := e.MustRotari("lineage", runID, "--json").Stdout
	if err := json.Unmarshal([]byte(output), &summary); err != nil {
		t.Fatalf("lineage %s --json: %v\n%s", runID, err, output)
	}
	return summary.Run.Sources
}

// latestRun returns the project's latest run, the one show prints.
func latestRun(t *testing.T, e *support.Env, project string) string {
	t.Helper()
	var shown struct {
		RunID string `json:"run_id"`
	}
	output := e.MustRotari("show", "-p", project, "--json").Stdout
	if err := json.Unmarshal([]byte(output), &shown); err != nil || shown.RunID == "" {
		t.Fatalf("show -p %s --json: %v\n%s", project, err, output)
	}
	return shown.RunID
}

// TestRunRecordsTheSourceItExecuted runs a failing job in one git repository
// and a passing job in another, commits a fix to the first, and retries. Each
// run records the commit of the repositories its executed jobs ran from; the
// retry, which carries the passing job, records only the first. show,
// lineage, and the Web API report the recorded commits, and lineage compares
// them.
func TestRunRecordsTheSourceItExecuted(t *testing.T) {
	covers(t, "RUN-15")
	e := support.NewEnv(t)
	fixed := filepath.Join(e.Root, "fixed")
	carried := filepath.Join(e.Root, "carried")
	firstCommit := gitRepo(t, fixed, "exit 3\n")
	carriedCommit := gitRepo(t, carried, "exit 0\n")
	fixedJob := support.AddedJobID(t, e.MustRotari("add", "-p", "src", "--job-name", "fixed", "--working-directory", fixed, "--", "sh", "job.sh"))
	e.MustRotari("add", "-p", "src", "--job-name", "carried", "--working-directory", carried, "--", "sh", "job.sh")
	e.Rotari("run", "-p", "src", "--quiet")
	firstRun := latestRun(t, e, "src")

	sources := runSources(t, e, firstRun)
	if len(sources) != 2 || sources[0].Root != fixed || sources[0].CommitID != firstCommit || sources[1].Root != carried || sources[1].CommitID != carriedCommit || sources[0].Dirty || sources[0].VCS != "git" {
		t.Fatalf("first run sources = %+v, want %s at %s and %s at %s", sources, fixed, firstCommit, carried, carriedCommit)
	}

	if err := os.WriteFile(filepath.Join(fixed, "job.sh"), []byte("exit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, fixed, "commit", "-qam", "fix")
	secondCommit := git(t, fixed, "rev-parse", "HEAD")
	e.MustRotari("retry", "-p", "src", "--quiet")
	secondRun := latestRun(t, e, "src")
	sources = runSources(t, e, secondRun)
	if len(sources) != 1 || sources[0].Root != fixed || sources[0].CommitID != secondCommit {
		t.Fatalf("retry sources = %+v, want only %s at %s", sources, fixed, secondCommit)
	}

	short := func(id string) string { return id[:12] }
	runView := e.MustRotari("show", "-r", secondRun, "--no-pager").Stdout
	if !strings.Contains(runView, "Source: git "+short(secondCommit)+" in "+fixed) {
		t.Fatalf("show for the retry does not name its source:\n%s", runView)
	}
	jobView := e.MustRotari("show", "-r", secondRun, "-j", fixedJob, "--no-pager").Stdout
	if !strings.Contains(jobView, "Source: git "+short(secondCommit)+" in "+fixed) {
		t.Fatalf("show -j does not name the job's source:\n%s", jobView)
	}
	comparison := e.MustRotari("lineage", firstRun, secondRun).Stdout
	for _, want := range []string{
		"Source: changed git " + short(firstCommit) + " -> git " + short(secondCommit) + " in " + fixed,
		"Source: unknown git " + short(carriedCommit) + " -> not recorded in " + carried,
	} {
		if !strings.Contains(comparison, want) {
			t.Fatalf("lineage comparison lacks %q:\n%s", want, comparison)
		}
	}

	response := e.HTTPGet(e.StartWeb() + "/api/run?project_name=src&run_id=" + url.QueryEscape(secondRun))
	if response.Status != 200 {
		t.Fatalf("GET run: status %d: %s", response.Status, response.Body)
	}
	var detail struct {
		Sources      []recordedSource `json:"sources"`
		SourceLabels []string         `json:"source_labels"`
	}
	if err := json.Unmarshal([]byte(response.Body), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail.Sources) != 1 || detail.Sources[0].CommitID != secondCommit || len(detail.SourceLabels) != 1 || detail.SourceLabels[0] != "git "+short(secondCommit)+" in "+fixed {
		t.Fatalf("Web run sources = %+v, labels %q", detail.Sources, detail.SourceLabels)
	}
}

// TestRunRecordsUncommittedGitChanges edits a tracked file without
// committing: the run records the commit as dirty, and lineage cannot tell
// whether the code changed.
func TestRunRecordsUncommittedGitChanges(t *testing.T) {
	covers(t, "RUN-15")
	e := support.NewEnv(t)
	repo := filepath.Join(e.Root, "repo")
	commit := gitRepo(t, repo, "exit 0\n")
	e.In(repo).MustRotari("add", "-p", "dirty", "--", "sh", "job.sh")
	e.In(repo).MustRotari("run", "-p", "dirty", "--quiet")
	older := latestRun(t, e, "dirty")
	if err := os.WriteFile(filepath.Join(repo, "job.sh"), []byte("echo edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.In(repo).MustRotari("add", "-p", "dirty", "--", "sh", "job.sh")
	e.In(repo).MustRotari("run", "-p", "dirty", "--quiet")
	newer := latestRun(t, e, "dirty")
	sources := runSources(t, e, newer)
	if len(sources) != 1 || sources[0].CommitID != commit || !sources[0].Dirty {
		t.Fatalf("sources = %+v, want %s dirty", sources, commit)
	}
	comparison := e.MustRotari("lineage", older, newer).Stdout
	want := "Source: unknown git " + commit[:12] + " -> git " + commit[:12] + " (uncommitted changes) in " + repo
	if !strings.Contains(comparison, want) {
		t.Fatalf("lineage comparison lacks %q:\n%s", want, comparison)
	}
}

// TestRunRecordsAJJWorkingCopy snapshots a jj working copy, so a run records
// a new commit ID, in the same change, after an edit nobody committed.
func TestRunRecordsAJJWorkingCopy(t *testing.T) {
	covers(t, "RUN-15")
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj is not installed")
	}
	e := support.NewEnv(t).WithVar("JJ_USER", "t").WithVar("JJ_EMAIL", "t@example.invalid")
	repo := filepath.Join(e.Root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("jj", "git", "init")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "HOME="+filepath.Join(e.Root, "home"), "JJ_USER=t", "JJ_EMAIL=t@example.invalid")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jj git init: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(repo, "job.sh"), []byte("exit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.In(repo).MustRotari("add", "-p", "jj", "--", "sh", "job.sh")
	e.In(repo).MustRotari("run", "-p", "jj", "--quiet")
	older := latestRun(t, e, "jj")
	if err := os.WriteFile(filepath.Join(repo, "job.sh"), []byte("echo edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.In(repo).MustRotari("add", "-p", "jj", "--", "sh", "job.sh")
	e.In(repo).MustRotari("run", "-p", "jj", "--quiet")
	newer := latestRun(t, e, "jj")
	before, after := runSources(t, e, older), runSources(t, e, newer)
	if len(before) != 1 || len(after) != 1 || before[0].VCS != "jj" || before[0].CommitID == "" || before[0].CommitID == after[0].CommitID {
		t.Fatalf("jj sources before %+v, after %+v; want two jj commits", before, after)
	}
	if comparison := e.MustRotari("lineage", older, newer).Stdout; !strings.Contains(comparison, "Source: changed jj ") {
		t.Fatalf("lineage comparison does not report the change:\n%s", comparison)
	}
}
