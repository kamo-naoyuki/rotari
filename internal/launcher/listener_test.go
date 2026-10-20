package launcher

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// listenCommand runs the test binary as a listener on address inside the
// launch's process group.
func listenCommand(address string) string {
	return testRole + `=listen ROTARI_LAUNCHER_TEST_ADDR='` + address + `' exec "$ROTARI_LAUNCHER_TEST_BIN"`
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
	lifetime := newTestLifetime(t)
	port := allocate(t)
	process := startTest(t, lifetime, testSpec(listenCommand(net.JoinHostPort(LoopbackHost, strconv.Itoa(port))), outputFile(t)))
	waitAccepting(t, port)
	if err := VerifyListener(port, process.PGID()); err != nil {
		t.Fatal(err)
	}
}

// Feasibility gate: a process outside the launch that bound the allocated
// port first is detected while the launch is still running, so its
// responses are never trusted.
func TestVerifyListenerRejectsForeignListener(t *testing.T) {
	lifetime := newTestLifetime(t)
	port := allocate(t)
	foreign, err := net.Listen("tcp", net.JoinHostPort(LoopbackHost, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
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
	lifetime := newTestLifetime(t)
	port := allocate(t)
	process := startTest(t, lifetime, testSpec(listenCommand(net.JoinHostPort("0.0.0.0", strconv.Itoa(port))), outputFile(t)))
	waitAccepting(t, port)
	err := VerifyListener(port, process.PGID())
	if err == nil || !strings.Contains(err.Error(), "bind it to 127.0.0.1") {
		t.Fatalf("VerifyListener() = %v, want a bind address error", err)
	}
}

func TestVerifyListenerRejectsMissingListener(t *testing.T) {
	lifetime := newTestLifetime(t)
	process := startTest(t, lifetime, testSpec("sleep 300", outputFile(t)))
	err := VerifyListener(allocate(t), process.PGID())
	if err == nil || !strings.Contains(err.Error(), "no process in the launch listens") {
		t.Fatalf("VerifyListener() = %v, want a missing listener error", err)
	}
}

func TestParseLsofListeners(t *testing.T) {
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
