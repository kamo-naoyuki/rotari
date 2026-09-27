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
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var rotariBin string
var listeningPattern = regexp.MustCompile(`listening at (http://\S+)`)
var addedJobPattern = regexp.MustCompile(`job_id=(\S+)`)

type env struct {
	t                  *testing.T
	root, base, master string
	vars               []string
}
type result struct {
	args           []string
	code           int
	stdout, stderr string
}
type activeRun struct {
	project, runID string
	jobs           []string
}
type httpResult struct {
	status int
	body   string
}

func TestMain(m *testing.M) { os.Exit(runTests(m)) }
func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "rotari-selector-")
	if err != nil {
		return 1
	}
	defer os.RemoveAll(dir)
	rotariBin = filepath.Join(dir, "rotari")
	b := exec.Command("go", "build", "-o", rotariBin, "github.com/kamo-naoyuki/rotari/cmd/rotari")
	if out, err := b.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}
func covers(t *testing.T, _ ...string) { t.Helper() }
func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{t: t, root: root, base: filepath.Join(root, "base"), master: filepath.Join(root, "master")}
	for _, d := range []string{"home", "config", "state", "cache", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	e.vars = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(root, "home"), "XDG_CONFIG_HOME=" + filepath.Join(root, "config"), "XDG_STATE_HOME=" + filepath.Join(root, "state"), "XDG_CACHE_HOME=" + filepath.Join(root, "cache"), "TMPDIR=" + filepath.Join(root, "tmp"), "TZ=UTC", "ROTARI_BASEDIR=" + e.base, "ROTARI_MASTERDIR=" + e.master}
	t.Cleanup(func() { killStrays(t, root) })
	t.Cleanup(func() { _ = e.command("server", "shutdown").Run() })
	return e
}
func (e *env) command(a ...string) *exec.Cmd {
	c := exec.Command(rotariBin, a...)
	c.Env = e.vars
	c.Dir = e.root
	return c
}
func (e *env) rotari(a ...string) result {
	e.t.Helper()
	var o, s bytes.Buffer
	c := e.command(a...)
	c.Stdout = &o
	c.Stderr = &s
	err := c.Run()
	code := 0
	if x, ok := err.(*exec.ExitError); ok {
		code = x.ExitCode()
	} else if err != nil {
		e.t.Fatal(err)
	}
	return result{a, code, o.String(), s.String()}
}
func (e *env) mustRotari(a ...string) result {
	r := e.rotari(a...)
	if r.code != 0 {
		e.t.Fatalf("unexpected failure: %s", r)
	}
	return r
}
func (r result) String() string {
	return fmt.Sprintf("rotari %q: exit %d\nstdout:\n%s\nstderr:\n%s", r.args, r.code, r.stdout, r.stderr)
}
func (e *env) without(n string) *env {
	c := *e
	c.vars = nil
	for _, v := range e.vars {
		if !strings.HasPrefix(v, n+"=") {
			c.vars = append(c.vars, v)
		}
	}
	return &c
}
func (e *env) withVar(n, v string) *env {
	c := e.without(n)
	c.vars = append(c.vars, n+"="+v)
	return c
}
func requireUnixSockets(t *testing.T) {
	p := filepath.Join(t.TempDir(), "probe.sock")
	l, err := net.Listen("unix", p)
	if err != nil {
		t.Skipf("Unix sockets unavailable: %v", err)
	}
	l.Close()
}
func addedJobID(t *testing.T, r result) string {
	m := addedJobPattern.FindStringSubmatch(r.stdout)
	if m == nil {
		t.Fatalf("add did not print job ID: %s", r)
	}
	return m[1]
}
func (e *env) startWeb(a ...string) string {
	cmd := e.command(append([]string{"web", "--port", "0"}, a...)...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		e.t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	found := make(chan string, 1)
	go func() {
		s := bufio.NewScanner(out)
		for s.Scan() {
			if m := listeningPattern.FindStringSubmatch(s.Text()); m != nil {
				found <- m[1]
				return
			}
		}
		_ = s.Err()
		_, _ = io.Copy(io.Discard, out)
	}()
	select {
	case u := <-found:
		return u
	case <-time.After(10 * time.Second):
		e.t.Fatal("web did not start")
		return ""
	}
}
func (e *env) httpGet(u string) httpResult {
	r, err := (&http.Client{Timeout: 10 * time.Second}).Get(u)
	if err != nil {
		e.t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return httpResult{r.StatusCode, string(b)}
}
func (e *env) httpPostJSON(u string, v any) httpResult {
	b, _ := json.Marshal(v)
	r, err := http.Post(u, "application/json", bytes.NewReader(b))
	if err != nil {
		e.t.Fatal(err)
	}
	defer r.Body.Close()
	body, _ := io.ReadAll(r.Body)
	return httpResult{r.StatusCode, string(body)}
}
func (e *env) startActiveRun(p string, n int) activeRun { return e.startRun(p, n, false) }
func (e *env) startRun(p string, n int, async bool, extra ...string) activeRun {
	requireUnixSockets(e.t)
	r := activeRun{project: p}
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("hold%d", i)
		r.jobs = append(r.jobs, addedJobID(e.t, e.mustRotari("add", "-p", p, "--job-name", name, "--", "sleep", "300")))
	}
	args := append([]string{"run", "-p", p, "--quiet"}, extra...)
	if async {
		e.mustRotari(append(args, "--async")...)
		e.t.Cleanup(func() { _ = e.command("cancel", "-p", p, "--wait").Run() })
	} else {
		c := e.command(args...)
		if err := c.Start(); err != nil {
			e.t.Fatal(err)
		}
		e.t.Cleanup(func() {
			_ = c.Process.Kill()
			_ = c.Wait()
			_ = e.command("wait", "-p", p, "--timeout", "30s").Run()
			killStrays(e.t, e.root)
		})
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		running := strings.Count(e.rotari("jobs", p, "--format", "%a %s").stdout, " running")
		var shown struct {
			RunID string `json:"run_id"`
		}
		_ = json.Unmarshal([]byte(e.rotari("show", "-p", p, "--json").stdout), &shown)
		if running == n && shown.RunID != "" {
			r.runID = shown.RunID
			return r
		}
		if time.Now().After(deadline) {
			e.t.Fatalf("run did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (e *env) in(t *testing.T) *env {
	c := *e
	c.t = t
	return &c
}

type finishedRun struct{ project, runID, okJob, badJob, badAttempt string }

func (e *env) createFinishedRun() finishedRun {
	e.t.Helper()
	requireUnixSockets(e.t)
	r := finishedRun{project: "p1"}
	r.okJob = addedJobID(e.t, e.mustRotari("add", "-p", r.project, "--job-name", "ok", "--", "sh", "-c", "echo hello"))
	r.badJob = addedJobID(e.t, e.mustRotari("add", "-p", r.project, "--job-name", "bad", "--", "sh", "-c", "exit 3"))
	if result := e.rotari("run", "-p", r.project, "--quiet"); result.code == 0 {
		e.t.Fatalf("run with a failing job should fail: %s", result)
	}
	var shown struct {
		RunID   string `json:"run_id"`
		Summary struct {
			Results []struct {
				ID, AttemptID string `json:"id"`
			} `json:"results"`
		} `json:"summary"`
	}
	if err := json.Unmarshal([]byte(e.mustRotari("show", "-p", r.project, "--json").stdout), &shown); err != nil {
		e.t.Fatal(err)
	}
	r.runID = shown.RunID
	for _, result := range shown.Summary.Results {
		if result.ID == r.badJob {
			r.badAttempt = result.AttemptID
		}
	}
	return r
}

func killStrays(t *testing.T, root string) {
	entries, _ := os.ReadDir("/proc")
	for _, x := range entries {
		pid, err := strconv.Atoi(x.Name())
		if err != nil {
			continue
		}
		cmd, err := os.ReadFile(filepath.Join("/proc", x.Name(), "cmdline"))
		if err != nil || !bytes.Contains(cmd, []byte(root+string(filepath.Separator))) {
			continue
		}
		pgid, err := syscall.Getpgid(pid)
		if err == nil && pgid != syscall.Getpgrp() {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}
}
func waitUntil(t *testing.T, d time.Duration, done func() (bool, string)) {
	deadline := time.Now().Add(d)
	for {
		if ok, msg := done(); ok {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("after %s: %s", d, msg)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
