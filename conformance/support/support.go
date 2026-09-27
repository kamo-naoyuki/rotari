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
	"testing"
	"time"
)

var binary string

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
	e.MustRotari("run", "-p", project, "--quiet")
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				AttemptID string `json:"attempt_id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", project, "--json").Stdout), &shown); err != nil || shown.RunID == "" || len(shown.Summary.Results) == 0 {
		e.T.Fatalf("show --json did not describe the run of %s: %v", project, err)
	}
	return shown.RunID, shown.Summary.Results[0].AttemptID
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
