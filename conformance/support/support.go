package support

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

var binary string

const (
	flagJSON    = "--json"
	flagJobName = "--job-name"
	flagQuiet   = "--quiet"
)

var addedJobPattern = regexp.MustCompile(`job_id=(\S+)`)
var listeningPattern = regexp.MustCompile(`listening at (http://\S+)`)

type Result struct {
	Args   []string
	Code   int
	Stdout string
	Stderr string
}

type HTTPResult struct {
	Status int
	Body   string
}

type ActiveRun struct {
	Project string
	RunID   string
	Jobs    []string
}

type FinishedRun struct {
	Project    string
	RunID      string
	OKJob      string
	BadJob     string
	BadAttempt string
}

func (r Result) String() string {
	return fmt.Sprintf("rotari %q: exit %d\nstdout:\n%s\nstderr:\n%s", r.Args, r.Code, r.Stdout, r.Stderr)
}

type Env struct {
	T      *testing.T
	Root   string
	Base   string
	Master string
	vars   []string
	// dir is the working directory of commands; empty means Root.
	dir string
}

func Run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "rotari-conformance-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	binary = filepath.Join(dir, "rotari")
	if err := BuildRotari(binary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

// BuildRotari builds cmd/rotari at path for a conformance test binary.
// Commands that run the binary must call TrackBuildInputs.
func BuildRotari(path string) error {
	build := exec.Command("go", "build", "-o", path, "github.com/kamo-naoyuki/rotari/cmd/rotari")
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build rotari: %w\n%s", err, output)
	}
	return nil
}

var trackBuildInputs = sync.OnceValue(statBuildInputs)

// TrackBuildInputs makes a cached test result depend on rotari's sources.
//
// BuildRotari runs `go build` in a subprocess, which Go's test cache does not
// observe, so a cached conformance result would survive a change to rotari.
// The cache does record files the test process stats while tests run (not
// in TestMain), so every command that runs the built binary calls this: it
// stats each file under the module's cmd and internal directories, plus
// go.mod and go.sum, once per test binary.
func TrackBuildInputs(t testing.TB) {
	t.Helper()
	if err := trackBuildInputs(); err != nil {
		t.Fatalf("track rotari sources: %v", err)
	}
}

// statBuildInputs stats the files a rotari build reads; see TrackBuildInputs.
func statBuildInputs() error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			return err
		}
	}
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			_, err = os.Stat(path)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// moduleRoot finds the directory holding go.mod above the working directory,
// which go test sets to the test package's directory.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above the working directory")
		}
		dir = parent
	}
}

func NewEnv(t *testing.T) *Env {
	t.Helper()
	root := t.TempDir()
	e := &Env{T: t, Root: root, Base: filepath.Join(root, "base"), Master: filepath.Join(root, "master")}
	for _, dir := range []string{"home", "config", "state", "cache", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.vars = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + filepath.Join(root, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"XDG_CACHE_HOME=" + filepath.Join(root, "cache"),
		"TMPDIR=" + filepath.Join(root, "tmp"),
		"TZ=UTC",
		"ROTARI_BASEDIR=" + e.Base,
		"ROTARI_MASTERDIR=" + e.Master,
	}
	t.Cleanup(func() { _ = e.command("server", "shutdown").Run() })
	return e
}

func (e *Env) command(args ...string) *exec.Cmd {
	TrackBuildInputs(e.T)
	cmd := exec.Command(binary, args...)
	cmd.Env = e.vars
	cmd.Dir = e.Root
	if e.dir != "" {
		cmd.Dir = e.dir
	}
	return cmd
}

// Command returns an unstarted rotari command in e's environment, for a
// test that talks to the process itself, such as over its stdin.
func (e *Env) Command(args ...string) *exec.Cmd {
	return e.command(args...)
}

func (e *Env) Rotari(args ...string) Result {
	e.T.Helper()
	var stdout, stderr bytes.Buffer
	cmd := e.command(args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		e.T.Fatal(err)
	}
	return Result{Args: args, Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
}

func (e *Env) MustRotari(args ...string) Result {
	e.T.Helper()
	r := e.Rotari(args...)
	if r.Code != 0 {
		e.T.Fatalf("unexpected failure: %s", r)
	}
	return r
}

func (e *Env) Without(name string) *Env {
	copied := *e
	copied.vars = nil
	for _, item := range e.vars {
		if !strings.HasPrefix(item, name+"=") {
			copied.vars = append(copied.vars, item)
		}
	}
	return &copied
}

// In returns a copy of e whose commands run in dir.
func (e *Env) In(dir string) *Env {
	copied := *e
	copied.dir = dir
	return &copied
}

func (e *Env) WithVar(name, value string) *Env {
	copied := e.Without(name)
	copied.vars = append(copied.vars, name+"="+value)
	return copied
}

func (e *Env) FinishedJobRun(project string) (runID, attemptID string) {
	e.T.Helper()
	e.MustRotari("add", "-p", project, "--", "true")
	e.MustRotari("run", "-p", project, flagQuiet)
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", project, flagJSON).Stdout), &shown); err != nil || shown.RunID == "" || len(shown.Summary.Results) == 0 {
		e.T.Fatalf("show --json did not describe the run of %s: %v", project, err)
	}
	return shown.RunID, shown.Summary.Results[0].AttemptID
}

func (e *Env) CreateFinishedRun() FinishedRun {
	e.T.Helper()
	run := FinishedRun{Project: "p1"}
	run.OKJob = AddedJobID(e.T, e.MustRotari("add", "-p", run.Project, flagJobName, "ok", "--", "sh", "-c", "echo hello"))
	run.BadJob = AddedJobID(e.T, e.MustRotari("add", "-p", run.Project, flagJobName, "bad", "--", "sh", "-c", "exit 3"))
	if result := e.Rotari("run", "-p", run.Project, flagQuiet); result.Code == 0 {
		e.T.Fatalf("run with a failing job should exit 1: %s", result)
	}
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				ID        string `json:"id"`
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", run.Project, flagJSON).Stdout), &shown); err != nil || shown.RunID == "" {
		e.T.Fatalf("show --json did not name the run: %v", err)
	}
	run.RunID = shown.RunID
	for _, result := range shown.Summary.Results {
		if result.ID == run.BadJob {
			run.BadAttempt = result.AttemptID
		}
	}
	return run
}

func (e *Env) StartWeb(args ...string) string {
	e.T.Helper()
	cmd := e.command(append([]string{"web", "--port", "0"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.T.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		e.T.Fatal(err)
	}
	e.T.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	found := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if match := listeningPattern.FindStringSubmatch(scanner.Text()); match != nil {
				found <- match[1]
				return
			}
		}
		_ = scanner.Err()
	}()
	select {
	case url := <-found:
		return url
	case <-time.After(10 * time.Second):
		e.T.Fatal("rotari web did not report its address")
		return ""
	}
}

func (e *Env) HTTPGet(url string) HTTPResult {
	e.T.Helper()
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(url)
	if err != nil {
		e.T.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		e.T.Fatal(err)
	}
	return HTTPResult{Status: response.StatusCode, Body: string(body)}
}

func (e *Env) HTTPPostJSON(url string, body any) HTTPResult {
	e.T.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		e.T.Fatal(err)
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		e.T.Fatal(err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil {
		e.T.Fatal(err)
	}
	return HTTPResult{Status: response.StatusCode, Body: string(result)}
}

func (e *Env) StartRun(project string, count int, async bool, runArgs ...string) ActiveRun {
	e.T.Helper()
	run := ActiveRun{Project: project}
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("hold%d", i)
		run.Jobs = append(run.Jobs, AddedJobID(e.T, e.MustRotari("add", "-p", project, flagJobName, name, "--", "sleep", "300")))
	}
	args := append([]string{"run", "-p", project, flagQuiet}, runArgs...)
	if async {
		e.MustRotari(append(args, "--async")...)
		e.T.Cleanup(func() { _ = e.command("cancel", "-p", project, "--wait").Run() })
	} else {
		client := e.command(args...)
		if err := client.Start(); err != nil {
			e.T.Fatal(err)
		}
		e.T.Cleanup(func() {
			_ = client.Process.Kill()
			_ = client.Wait()
			_ = e.command("wait", "-p", project, "--timeout", "30s").Run()
			KillStrays(e.T, e.Root)
		})
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		running := strings.Count(e.Rotari("jobs", project, "--format", "%a %s").Stdout, " running")
		var shown struct {
			RunID string `json:"run_id"`
		}
		_ = json.Unmarshal([]byte(e.Rotari("show", "-p", project, flagJSON).Stdout), &shown)
		if running == count && shown.RunID != "" {
			run.RunID = shown.RunID
			return run
		}
		if time.Now().After(deadline) {
			e.T.Fatalf("run did not start: running=%d, want=%d", running, count)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (e *Env) CheckState(project string) string {
	e.T.Helper()
	for _, field := range strings.Fields(e.Rotari("check", project).Stdout) {
		if state, ok := strings.CutPrefix(field, "state="); ok {
			return state
		}
	}
	return ""
}

func (e *Env) ExportFinishedRun(project string) string {
	e.T.Helper()
	runID, _ := e.FinishedJobRun(project)
	manifest := filepath.Join(e.Root, "manifest.yaml")
	e.MustRotari("export", runID, manifest)
	return manifest
}

func GuardedCommands(project, manifest string, run ActiveRun) map[string][]string {
	return map[string][]string{
		"run":    {"run", "-p", project},
		"add":    {"add", "-p", project, "--", "true"},
		"copy":   {"copy", "-p", project, "--run-id", run.RunID, "--overwrite"},
		"change": {"change", "-p", project, "--job-id", run.Jobs[0], "--timeout", "1m"},
		"delete": {"delete", "-p", project, "--all"},
		"remove": {"remove", "-p", project, run.Jobs[0]},
		"import": {"import", manifest, project, "--overwrite"},
	}
}

func (e *Env) OrphanRun(project, script string) string {
	e.T.Helper()
	jobID := AddedJobID(e.T, e.MustRotari("add", "-p", project, "--", "sh", "-c", script))
	client := e.command("run", "-p", project, flagQuiet)
	if err := client.Start(); err != nil {
		e.T.Fatal(err)
	}
	e.T.Cleanup(func() { _ = client.Wait() })
	WaitUntil(e.T, 15*time.Second, func() (bool, string) { return JobProcesses(e.T, e.Root, jobID) == 1, "the job did not start" })
	var lock struct {
		PID int `json:"pid"`
	}
	data, err := os.ReadFile(filepath.Join(e.Base, "projects", project, "running.lock"))
	if err != nil || json.Unmarshal(data, &lock) != nil || lock.PID <= 0 {
		e.T.Fatalf("running.lock does not name supervisor: %s", data)
	}
	if err := syscall.Kill(lock.PID, syscall.SIGKILL); err != nil {
		e.T.Fatal(err)
	}
	WaitForInterrupted(e.T, e, project)
	return jobID
}

func (e *Env) JobExitStatus(project, jobID string) string {
	e.T.Helper()
	var status string
	WaitUntil(e.T, 30*time.Second, func() (bool, string) {
		out := e.Rotari("show", "-p", project, "--run-id", "latest", "--job-id", jobID).Stdout
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Status:") {
				status = line
			}
		}
		return strings.Contains(status, "7"), "show reports " + status
	})
	return status
}

// KillSupervisors kills the supervisor processes of runs under root, as a
// crash would, and leaves their jobs running.
func KillSupervisors(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Skipf("cannot list processes: %v", err)
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || !bytes.Contains(cmdline, []byte(root+string(filepath.Separator))) || !bytes.Contains(cmdline, []byte("__server\x00")) {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

func KillStrays(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || !bytes.Contains(cmdline, []byte(root+string(filepath.Separator))) {
			continue
		}
		pgid, err := syscall.Getpgid(pid)
		if err == nil && pgid != syscall.Getpgrp() {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}
}

func WaitForInterrupted(t *testing.T, e *Env, project string) {
	WaitUntil(t, 15*time.Second, func() (bool, string) {
		state := e.CheckState(project)
		return state == "interrupted", "state " + state + ", want interrupted"
	})
}

func JobProcesses(t *testing.T, root, job string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Skipf("cannot list processes: %v", err)
	}
	count := 0
	for _, entry := range entries {
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || !bytes.Contains(cmdline, []byte(root+string(filepath.Separator))) {
			continue
		}
		if job == "" && !bytes.Contains(cmdline, []byte("local-wrapper.sh")) {
			continue
		}
		if job == "" || bytes.Contains(cmdline, []byte(string(filepath.Separator)+job+string(filepath.Separator))) {
			count++
		}
	}
	return count
}

func WaitUntil(t *testing.T, timeout time.Duration, done func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if ok, message := done(); ok {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("after %s: %s", timeout, message)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func KillProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func AddedJobID(t *testing.T, result Result) string {
	t.Helper()
	match := addedJobPattern.FindStringSubmatch(result.Stdout)
	if match == nil {
		t.Fatalf("add did not print a job ID: %s", result)
	}
	return strings.TrimSpace(match[1])
}
