package launcher

import (
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// startListener runs the test binary as a listener on host inside a
// launch's process group and returns the launch and its port. The listener
// binds port 0 itself: allocating a port first would race with the other
// tests' connections.
func startListener(t *testing.T, host string) (*Process, int) {
	t.Helper()
	portFile := filepath.Join(t.TempDir(), "port")
	command := testRole + `=listen ROTARI_TEST_LAUNCHER_HOST='` + host + `' ROTARI_TEST_LAUNCHER_PORT_FILE="$PORT_FILE" exec "$ROTARI_TEST_LAUNCHER_BIN"`
	spec := testSpec(command, outputFile(t))
	spec.Env = append(spec.Env, "PORT_FILE="+portFile)
	process := startTest(t, newTestLifetime(t), spec)
	return process, readNumberFile(t, portFile)
}

func waitAccepting(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(LoopbackHost, strconv.Itoa(port)), time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("nothing accepts connections on port %d", port)
}

func allocate(t *testing.T) int {
	t.Helper()
	port, err := AllocatePort()
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestVerifyListenerAcceptsLaunchListener(t *testing.T) {
	t.Parallel()
	process, port := startListener(t, LoopbackHost)
	if err := VerifyListener(port, process.PGID()); err != nil {
		t.Fatal(err)
	}
}

// Feasibility gate: a process outside the launch that bound the allocated
// port first is detected while the launch is still running, so its
// responses are never trusted.
func TestVerifyListenerRejectsForeignListener(t *testing.T) {
	t.Parallel()
	lifetime := newTestLifetime(t)
	foreign, err := net.Listen("tcp", net.JoinHostPort(LoopbackHost, "0"))
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	port := foreign.Addr().(*net.TCPAddr).Port
	process := startTest(t, lifetime, testSpec("sleep 300", outputFile(t)))
	waitAccepting(t, port)
	err = VerifyListener(port, process.PGID())
	if err == nil || !strings.Contains(err.Error(), "another process listens on") {
		t.Fatalf("VerifyListener() = %v, want a foreign listener error", err)
	}
	select {
	case <-process.Done():
		t.Fatal("the launch ended; the check must hold while it is alive")
	default:
	}
}

func TestVerifyListenerRejectsNonLoopbackBind(t *testing.T) {
	t.Parallel()
	process, port := startListener(t, "0.0.0.0")
	err := VerifyListener(port, process.PGID())
	if err == nil || !strings.Contains(err.Error(), "bind it to 127.0.0.1") {
		t.Fatalf("VerifyListener() = %v, want a bind address error", err)
	}
}

func TestVerifyListenerRejectsMissingListener(t *testing.T) {
	t.Parallel()
	lifetime := newTestLifetime(t)
	process := startTest(t, lifetime, testSpec("sleep 300", outputFile(t)))
	// A parallel test can start listening on a port this one just found
	// free; that is a foreign listener, so try another port.
	for range 5 {
		err := VerifyListener(allocate(t), process.PGID())
		if err != nil && strings.Contains(err.Error(), "another process listens on") {
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "no process in the launch listens") {
			t.Fatalf("VerifyListener() = %v, want a missing listener error", err)
		}
		return
	}
	t.Fatal("every allocated port was taken by another listener")
}

func TestParseLsofListeners(t *testing.T) {
	t.Parallel()
	output := "p100\nf5\nn127.0.0.1:8000\np200\nf6\nn*:8000\nf7\nn[::1]:8000\np300\nf8\nn127.0.0.1:9000\np400\n"
	groups := map[int]int{100: 100, 200: 7}
	groupOf := func(pid int) (int, error) {
		if pgid, ok := groups[pid]; ok {
			return pgid, nil
		}
		return 0, errors.New("no such process")
	}
	listeners, err := parseLsofListeners([]byte(output), 8000, groupOf)
	if err != nil {
		t.Fatal(err)
	}
	// [::1] does not accept connections to 127.0.0.1, and port 9000 is
	// another port.
	if len(listeners) != 2 ||
		!listeners[0].ip.Equal(net.ParseIP("127.0.0.1")) || listeners[0].pgid != 100 ||
		!listeners[1].ip.IsUnspecified() || listeners[1].pgid != 7 {
		t.Fatalf("parseLsofListeners() = %+v", listeners)
	}
	if err := checkListeners(listeners, 8000, 100); err == nil || !strings.Contains(err.Error(), "another process") {
		t.Fatalf("checkListeners() = %v, want the wildcard listener of group 7 rejected", err)
	}
	if err := checkListeners(listeners[:1], 8000, 100); err != nil {
		t.Fatal(err)
	}
}
