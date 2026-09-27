package support

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
}

func Run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "rotari-conformance-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	binary = filepath.Join(dir, "rotari")
	build := exec.Command("go", "build", "-o", binary, "github.com/kamo-naoyuki/rotari/cmd/rotari")
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build rotari: %v\n%s", err, output)
		return 1
	}
	return m.Run()
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
	cmd := exec.Command(binary, args...)
	cmd.Env = e.vars
	cmd.Dir = e.Root
	return cmd
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

func (e *Env) WithVar(name, value string) *Env {
	copied := e.Without(name)
	copied.vars = append(copied.vars, name+"="+value)
	return copied
}

func (e *Env) FinishedJobRun(project string) (runID, attemptID string) {
	e.T.Helper()
	RequireUnixSockets(e.T)
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
	RequireUnixSockets(e.T)
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
	RequireUnixSockets(e.T)
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

func JobProcesses(t *testing.T, root, job string) int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Skipf("cannot list processes: %v", err)
	}
	count := 0
	for _, entry := range entries {
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || !bytes.Contains(cmdline, []byte(root+string(filepath.Separator))) || !bytes.Contains(cmdline, []byte("local-wrapper.sh")) {
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

func RequireUnixSockets(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	_ = listener.Close()
}

func AddedJobID(t *testing.T, result Result) string {
	t.Helper()
	match := addedJobPattern.FindStringSubmatch(result.Stdout)
	if match == nil {
		t.Fatalf("add did not print a job ID: %s", result)
	}
	return strings.TrimSpace(match[1])
}
