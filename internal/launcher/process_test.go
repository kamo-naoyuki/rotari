package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// testRole makes the test binary act as one of the processes a launch
// involves; see TestMain.
const testRole = "ROTARI_TEST_LAUNCHER_ROLE"

func TestMain(m *testing.M) {
	switch os.Getenv(testRole) {
	case "wrapper":
		os.Exit(RunWrapper(os.Args[1:]))
	case "web":
		os.Exit(fakeWeb())
	case "http":
		os.Exit(serveHTTP())
	case "listen":
		os.Exit(listenAndWait(os.Getenv("ROTARI_TEST_LAUNCHER_HOST"), os.Getenv("ROTARI_TEST_LAUNCHER_PORT_FILE")))
	}
	os.Exit(m.Run())
}

// fakeWeb stands in for the Web server: it starts one launch from
// ROTARI_TEST_LAUNCHER_COMMAND, prints its process group, and waits to be
// killed without stopping it.
func fakeWeb() int {
	lifetime, err := NewLifetime()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	process, err := Start(lifetime, testSpec(os.Getenv("ROTARI_TEST_LAUNCHER_COMMAND"), os.Stderr))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(process.PGID())
	select {}
}

// listenAndWait listens on host and an unused port, and writes the port to
// portFile once it accepts connections.
func listenAndWait(host, portFile string) int {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	if err := os.WriteFile(portFile, []byte(strconv.Itoa(port)+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	select {}
}

// testSpec runs command under the test binary acting as the wrapper. The
// command sees the role too, so commands that start the test binary again
// set their own role.
func testSpec(command string, output *os.File) Spec {
	return Spec{
		Wrapper: []string{os.Args[0]},
		Command: command,
		Env:     append(os.Environ(), testRole+"=wrapper", "ROTARI_TEST_LAUNCHER_BIN="+os.Args[0]),
		Output:  output,
		Grace:   300 * time.Millisecond,
	}
}

func newTestLifetime(t *testing.T) *Lifetime {
	t.Helper()
	lifetime, err := NewLifetime()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lifetime.Close() })
	return lifetime
}

func startTest(t *testing.T, lifetime *Lifetime, spec Spec) *Process {
	t.Helper()
	process, err := Start(lifetime, spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		select {
		case <-process.Done():
		default:
			_, _ = process.Stop()
		}
	})
	return process
}

func outputFile(t *testing.T) *os.File {
	t.Helper()
	file, err := os.Create(filepath.Join(t.TempDir(), "output"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

// readNumberFile waits for a command to write a number (a PID or a port)
// and a newline to path.
func readNumberFile(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && bytes.HasSuffix(data, []byte("\n")) {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("invalid PID file %s: %q", path, data)
			}
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("command did not write %s", path)
	return 0
}

// waitProcessGone fails unless pid disappears. A killed process whose parent
// already exited is reaped by init, so it does not linger as a zombie.
func waitProcessGone(t *testing.T, pid int, why string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("process %d is still running %s", pid, why)
}

func TestStartReportsExitStatus(t *testing.T) {
	t.Parallel()
	lifetime := newTestLifetime(t)
	for _, test := range []struct {
		command string
		want    Result
	}{
		{"exit 0", Result{ExitCode: 0}},
		{"exit 7", Result{ExitCode: 7}},
		{"kill -USR1 $$", Result{ExitCode: -1, Signal: syscall.SIGUSR1}},
	} {
		t.Run(test.command, func(t *testing.T) {
			process := startTest(t, lifetime, testSpec(test.command, outputFile(t)))
			got, err := process.Wait()
			if err != nil || got != test.want {
				t.Fatalf("Wait() = %+v, %v; want %+v", got, err, test.want)
			}
		})
	}
}

// Values reach the command through its environment, never as shell source.
func TestStartPassesValuesThroughEnvironment(t *testing.T) {
	t.Parallel()
	lifetime := newTestLifetime(t)
	value := "dir with spaces/'quote\" $(touch pwned) `id` \\ ü"
	work := t.TempDir()
	spec := testSpec(`printf '%s' "$ROTARI_LAUNCHER_ARTIFACT"`, outputFile(t))
	spec.Dir = work
	spec.Env = append(spec.Env, "ROTARI_LAUNCHER_ARTIFACT="+value)
	process := startTest(t, lifetime, spec)
	if result, err := process.Wait(); err != nil || result.ExitCode != 0 {
		t.Fatalf("Wait() = %+v, %v", result, err)
	}
	got, err := os.ReadFile(spec.Output.(*os.File).Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != value {
		t.Fatalf("command printed %q, want %q", got, value)
	}
	if _, err := os.Stat(filepath.Join(work, "pwned")); err == nil {
		t.Fatal("a value was executed as shell code")
	}
}

func TestStartRunsInDirWithEmptyStdin(t *testing.T) {
	t.Parallel()
	lifetime := newTestLifetime(t)
	work := t.TempDir()
	// cat would block on an inherited terminal or pipe.
	spec := testSpec(`cat; pwd -P`, outputFile(t))
	spec.Dir = work
	process := startTest(t, lifetime, spec)
	if result, err := process.Wait(); err != nil || result.ExitCode != 0 {
		t.Fatalf("Wait() = %+v, %v", result, err)
	}
	got, _ := os.ReadFile(spec.Output.(*os.File).Name())
	want, _ := filepath.EvalSymlinks(work)
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("command ran in %q, want %q", got, want)
	}
}

func TestStartFailsForMissingDirectory(t *testing.T) {
	t.Parallel()
	lifetime := newTestLifetime(t)
	spec := testSpec("true", outputFile(t))
	spec.Dir = filepath.Join(t.TempDir(), "missing")
	if process, err := Start(lifetime, spec); err == nil {
		_, _ = process.Stop()
		t.Fatal("Start succeeded in a missing directory")
	}
}

func TestStopEndsDescendants(t *testing.T) {
	t.Parallel()
	lifetime := newTestLifetime(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	for _, test := range []struct {
		name, command string
		want          syscall.Signal
	}{
		{"terminate", `sleep 300 & echo $! > "$PID_FILE"; wait`, syscall.SIGTERM},
		// Ignoring SIGTERM is inherited by sleep, so only SIGKILL after the
		// grace period ends the group.
		{"kill after grace", `trap '' TERM; sleep 300 & echo $! > "$PID_FILE"; wait`, syscall.SIGKILL},
	} {
		t.Run(test.name, func(t *testing.T) {
			os.Remove(pidFile)
			spec := testSpec(test.command, outputFile(t))
			spec.Env = append(spec.Env, "PID_FILE="+pidFile)
			process := startTest(t, lifetime, spec)
			child := readNumberFile(t, pidFile)
			result, err := process.Stop()
			if err != nil || result.Signal != test.want {
				t.Fatalf("Stop() = %+v, %v; want signal %v", result, err, test.want)
			}
			waitProcessGone(t, child, "after Stop")
		})
	}
}

// A command that exits and leaves members in its group does not leak them.
func TestCommandExitEndsLeftoverGroupMembers(t *testing.T) {
	t.Parallel()
	lifetime := newTestLifetime(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	spec := testSpec(`sleep 300 & echo $! > "$PID_FILE"`, outputFile(t))
	spec.Env = append(spec.Env, "PID_FILE="+pidFile)
	process := startTest(t, lifetime, spec)
	child := readNumberFile(t, pidFile)
	if result, err := process.Wait(); err != nil || result.ExitCode != 0 {
		t.Fatalf("Wait() = %+v, %v", result, err)
	}
	waitProcessGone(t, child, "after its shell exited")
}

func TestLifetimeCloseStopsCommands(t *testing.T) {
	t.Parallel()
	lifetime, err := NewLifetime()
	if err != nil {
		t.Fatal(err)
	}
	process := startTest(t, lifetime, testSpec("sleep 300", outputFile(t)))
	lifetime.Close()
	select {
	case <-process.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("closing the lifetime did not stop the command")
	}
}

// Feasibility gate: killing the Web process with SIGKILL, which runs none of
// its shutdown code, still stops its launches and their descendants.
func TestKilledOwnerStopsLaunches(t *testing.T) {
	t.Parallel()
	pidFile := filepath.Join(t.TempDir(), "pid")
	web := exec.Command(os.Args[0])
	web.Env = append(os.Environ(), testRole+"=web",
		"ROTARI_TEST_LAUNCHER_COMMAND="+`sleep 300 & echo $! > "$PID_FILE"; wait`, "PID_FILE="+pidFile)
	var stdout bytes.Buffer
	web.Stdout, web.Stderr = &stdout, os.Stderr
	if err := web.Start(); err != nil {
		t.Fatal(err)
	}
	child := readNumberFile(t, pidFile)
	if err := web.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = web.Wait()
	waitProcessGone(t, child, "after its Web process was killed")
	pgid, err := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("fake Web process printed %q", stdout.String())
	}
	if !errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH) {
		t.Fatalf("process group %d survived its Web process", pgid)
	}
}

// serveHTTP stands in for a launched service: it listens on
// ROTARI_TEST_LAUNCHER_HOST (default LoopbackHost) and ROTARI_LAUNCHER_PORT
// and answers under ROTARI_LAUNCHER_BASE_PATH with
// ROTARI_TEST_LAUNCHER_STATUS (default 200) and the path it was asked for.
func serveHTTP() int {
	host := os.Getenv("ROTARI_TEST_LAUNCHER_HOST")
	if host == "" {
		host = LoopbackHost
	}
	status := http.StatusOK
	if value := os.Getenv("ROTARI_TEST_LAUNCHER_STATUS"); value != "" {
		status, _ = strconv.Atoi(value)
	}
	base := os.Getenv(EnvBasePath)
	listener, err := net.Listen("tcp", net.JoinHostPort(host, os.Getenv(EnvPort)))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	// With ROTARI_TEST_LAUNCHER_REQUIRE_TOKEN set, every request must
	// present the launch token as an Authorization header.
	requireToken := os.Getenv("ROTARI_TEST_LAUNCHER_REQUIRE_TOKEN") != ""
	token := os.Getenv(EnvToken)
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, base+"/") {
			http.NotFound(writer, request)
			return
		}
		if requireToken && (token == "" || request.Header.Get("Authorization") != "token "+token) {
			http.Error(writer, "token required", http.StatusForbidden)
			return
		}
		writer.WriteHeader(status)
		fmt.Fprint(writer, "served "+request.URL.Path)
	})
	_ = http.Serve(listener, handler)
	return 0
}
