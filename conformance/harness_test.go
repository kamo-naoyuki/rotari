package conformance

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

// rotariBin is the binary built by TestMain.
var rotariBin string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "rotari-conformance-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	rotariBin = filepath.Join(dir, "rotari")
	build := exec.Command("go", "build", "-o", rotariBin, "github.com/kamo-naoyuki/rotari/cmd/rotari")
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build rotari: %v\n%s", err, output)
		return 1
	}
	return m.Run()
}

// covers declares the contract IDs from docs/contracts/ that the calling
// test checks. It does nothing at run time: TestContractStatus reads these
// calls from the source and matches them against the status table in
// docs/CONTRACTS.md. Pass the IDs as string literals.
func covers(t *testing.T, ids ...string) {
	t.Helper()
}

// env is one isolated rotari installation: a base directory, a master
// directory, and a home with its own config directories.
type env struct {
	t      *testing.T
	root   string
	base   string
	master string
	vars   []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{t: t, root: root, base: filepath.Join(root, "base"), master: filepath.Join(root, "master")}
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
		"ROTARI_BASEDIR=" + e.base,
		"ROTARI_MASTERDIR=" + e.master,
	}
	return e
}

// in returns e reporting to t, for use inside a subtest.
func (e *env) in(t *testing.T) *env {
	copied := *e
	copied.t = t
	return &copied
}

// withVar returns e with the environment variable name set to value.
func (e *env) withVar(name, value string) *env {
	copied := *e
	copied.vars = nil
	for _, item := range e.vars {
		if !strings.HasPrefix(item, name+"=") {
			copied.vars = append(copied.vars, item)
		}
	}
	copied.vars = append(copied.vars, name+"="+value)
	return &copied
}

type result struct {
	args   []string
	code   int
	stdout string
	stderr string
}

func (r result) String() string {
	return fmt.Sprintf("rotari %q: exit %d\nstdout:\n%s\nstderr:\n%s", r.args, r.code, r.stdout, r.stderr)
}

func (e *env) command(args ...string) *exec.Cmd {
	cmd := exec.Command(rotariBin, args...)
	cmd.Env = e.vars
	cmd.Dir = e.root
	return cmd
}

// rotari runs the binary to completion.
func (e *env) rotari(args ...string) result {
	e.t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := e.command(args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		e.t.Fatalf("rotari %q: %v", args, err)
	}
	return result{args: args, code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// mustRotari runs the binary and fails the test unless it exits 0.
func (e *env) mustRotari(args ...string) result {
	e.t.Helper()
	r := e.rotari(args...)
	if r.code != 0 {
		e.t.Fatalf("unexpected failure: %s", r)
	}
	return r
}

var listeningPattern = regexp.MustCompile(`listening at (http://\S+)`)

// startWeb starts `rotari web` on a free loopback port and returns its URL.
// The server is stopped when the test ends.
func (e *env) startWeb(args ...string) string {
	e.t.Helper()
	cmd := e.command(append([]string{"web", "--port", "0"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		e.t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	found := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if match := listeningPattern.FindStringSubmatch(scanner.Text()); match != nil {
				found <- match[1]
				break
			}
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()
	select {
	case url := <-found:
		return url
	case <-time.After(10 * time.Second):
		e.t.Fatalf("rotari web did not report its address; stderr:\n%s", stderr.String())
		return ""
	}
}

type httpResult struct {
	status int
	body   string
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func (e *env) httpGet(url string) httpResult {
	e.t.Helper()
	response, err := httpClient.Get(url)
	if err != nil {
		e.t.Fatalf("GET %s: %v", url, err)
	}
	return readResponse(e.t, response)
}

func (e *env) httpPostJSON(url string, body any) httpResult {
	e.t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		e.t.Fatal(err)
	}
	response, err := httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		e.t.Fatalf("POST %s: %v", url, err)
	}
	return readResponse(e.t, response)
}

func readResponse(t *testing.T, response *http.Response) httpResult {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return httpResult{status: response.StatusCode, body: string(body)}
}

// requireUnixSockets skips a test that runs jobs, which go through the
// supervisor's Unix socket, where the platform or sandbox does not allow one.
func requireUnixSockets(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "probe.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	_ = listener.Close()
}

// finishedRun is a run of project "p1" with one successful and one failed
// job, created through the CLI.
type finishedRun struct {
	project string
	runID   string
	okJob   string
	badJob  string
}

var addedJobPattern = regexp.MustCompile(`job_id=(\S+)`)

func (e *env) createFinishedRun() finishedRun {
	e.t.Helper()
	requireUnixSockets(e.t)
	run := finishedRun{project: "p1"}
	run.okJob = addedJobID(e.t, e.mustRotari("add", "-p", run.project, "--job-name", "ok", "--", "sh", "-c", "echo hello"))
	run.badJob = addedJobID(e.t, e.mustRotari("add", "-p", run.project, "--job-name", "bad", "--", "sh", "-c", "echo broken >&2; exit 3"))
	if r := e.rotari("run", "-p", run.project, "--quiet"); r.code != 1 {
		e.t.Fatalf("run with a failing job should exit 1: %s", r)
	}
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "-p", run.project, "--json").stdout), &shown); err != nil || shown.RunID == "" {
		e.t.Fatalf("show --json did not name the run: %v", err)
	}
	run.runID = shown.RunID
	return run
}

func addedJobID(t *testing.T, r result) string {
	t.Helper()
	match := addedJobPattern.FindStringSubmatch(r.stdout)
	if match == nil {
		t.Fatalf("add did not print a job ID: %s", r)
	}
	return strings.TrimSpace(match[1])
}
