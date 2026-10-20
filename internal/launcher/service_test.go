package launcher

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// serviceCommand runs the test binary as an HTTP service, with extra shell
// variable assignments before it.
func serviceCommand(name, assignments string) Command {
	command := testCommand(name, assignments+" "+testRole+`=http exec "$ROTARI_TEST_LAUNCHER_BIN"`)
	command.Mode = ModeService
	command.ReadyPath = "/"
	command.ReadyTimeout = 20 * time.Second
	return command
}

func newServiceManager(t *testing.T) *Manager {
	t.Helper()
	manager := newTestManager(t, ManagerConfig{ServicePath: func(id string) string { return "/_launch/" + id }})
	manager.config.Env = append(manager.config.Env, "ROTARI_TEST_LAUNCHER_BIN="+os.Args[0])
	return manager
}

func waitState(t *testing.T, manager *Manager, id string, done func(Launch) bool) Launch {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		launch, err := manager.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if done(launch) {
			return launch
		}
		time.Sleep(20 * time.Millisecond)
	}
	launch, _ := manager.Get(id)
	t.Fatalf("launch %s did not reach the expected state: %+v", id, launch)
	return Launch{}
}

func notStarting(launch Launch) bool { return launch.State != StateStarting }

func TestServiceBecomesReadyAndExposesItsUpstream(t *testing.T) {
	t.Parallel()
	manager := newServiceManager(t)
	launch, err := manager.Start(StartRequest{Command: serviceCommand("web", ""), Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil || launch.State != StateStarting || launch.BasePath != "/_launch/"+launch.ID || launch.Port == 0 {
		t.Fatalf("Start() = %+v, %v", launch, err)
	}
	if _, err := manager.ServiceUpstream(launch.ID); !errors.Is(err, ErrNotServing) {
		ready, _ := manager.Get(launch.ID)
		if ready.State == StateStarting {
			t.Fatalf("ServiceUpstream() while starting = %v, want ErrNotServing", err)
		}
	}
	ready := waitState(t, manager, launch.ID, notStarting)
	if ready.State != StateRunning {
		t.Fatalf("service = %+v, want running", ready)
	}
	upstream, err := manager.ServiceUpstream(launch.ID)
	port, pgid := upstream.Port, upstream.PGID
	if err != nil || port != launch.Port || pgid <= 1 || upstream.Headers != nil {
		t.Fatalf("ServiceUpstream() = %+v, %v", upstream, err)
	}
	if err := VerifyListener(port, pgid); err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Transport: &http.Transport{Proxy: nil}}).Get(fmt.Sprintf("http://127.0.0.1:%d/_launch/%s/x", port, launch.ID))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != "served /_launch/"+launch.ID+"/x" {
		t.Fatalf("service answered %q", body)
	}
	// A repeated start reuses it; Stop ends it and its upstream.
	if again, _ := manager.Start(StartRequest{Command: serviceCommand("web", ""), Target: launch.Target, Kind: TargetDirectory}); again.ID != launch.ID || !again.Reused {
		t.Fatalf("second Start() = %+v", again)
	}
	if stopped, err := manager.Stop(launch.ID); err != nil || stopped.State != StateStopped {
		t.Fatalf("Stop() = %+v, %v", stopped, err)
	}
	if _, err := manager.ServiceUpstream(launch.ID); !errors.Is(err, ErrNotServing) {
		t.Fatalf("ServiceUpstream() after Stop = %v", err)
	}
}

func TestServiceFailures(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		command Command
		want    string
	}{
		"never ready": {func() Command {
			c := serviceCommand("err", "ROTARI_TEST_LAUNCHER_STATUS=500")
			c.ReadyTimeout = time.Second
			return c
		}(), "was not ready at /_launch/"},
		"wildcard bind": {serviceCommand("any", "ROTARI_TEST_LAUNCHER_HOST=0.0.0.0"), "bind it to 127.0.0.1"},
		"exits without listening": {func() Command {
			c := serviceCommand("gone", "")
			c.Command = "exit 3"
			return c
		}(), "exited without listening"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			manager := newServiceManager(t)
			launch, err := manager.Start(StartRequest{Command: test.command, Target: t.TempDir(), Kind: TargetDirectory})
			if err != nil {
				t.Fatal(err)
			}
			ended := waitState(t, manager, launch.ID, func(launch Launch) bool { return !launch.State.live() })
			if ended.State != StateFailed || !strings.Contains(ended.Error, test.want) {
				t.Fatalf("service ended as %+v, want failed with %q", ended, test.want)
			}
			if _, err := manager.ServiceUpstream(launch.ID); !errors.Is(err, ErrNotServing) {
				t.Fatalf("ServiceUpstream() of a failed service = %v", err)
			}
		})
	}
}

// Feasibility gate, end to end: a process outside the launch that answers
// on the service's port while the service is still alive never makes the
// launch ready.
func TestServiceRejectsForeignListenerOnItsPort(t *testing.T) {
	t.Parallel()
	manager := newServiceManager(t)
	portFile := filepath.Join(t.TempDir(), "port")
	manager.config.Env = append(manager.config.Env, "PORT_FILE="+portFile)
	command := serviceCommand("victim", "")
	command.Command = `echo "$ROTARI_LAUNCHER_PORT" > "$PORT_FILE"; sleep 300`
	launch, err := manager.Start(StartRequest{Command: command, Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil {
		t.Fatal(err)
	}
	port := readNumberFile(t, portFile)
	foreign, err := net.Listen("tcp", net.JoinHostPort(LoopbackHost, strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	go http.Serve(foreign, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(writer, "foreign")
	}))
	ended := waitState(t, manager, launch.ID, func(launch Launch) bool { return !launch.State.live() })
	if ended.State != StateFailed || !strings.Contains(ended.Error, "another process listens on") {
		t.Fatalf("service ended as %+v, want the foreign listener reported", ended)
	}
}

// A service that loses its port to another process exits without
// listening; it is restarted once on a new port.
func TestServiceRetriesOnceOnANewPort(t *testing.T) {
	t.Parallel()
	manager := newServiceManager(t)
	marker := filepath.Join(t.TempDir(), "first")
	manager.config.Env = append(manager.config.Env, "MARKER="+marker)
	command := serviceCommand("flaky", "")
	command.Command = `if [ ! -e "$MARKER" ]; then : > "$MARKER"; echo "port $ROTARI_LAUNCHER_PORT taken"; exit 1; fi; ` + command.Command
	launch, err := manager.Start(StartRequest{Command: command, Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil {
		t.Fatal(err)
	}
	ready := waitState(t, manager, launch.ID, notStarting)
	if ready.State != StateRunning || ready.Port == launch.Port {
		t.Fatalf("service after retry = %+v (first port %d)", ready, launch.Port)
	}
	log := readAllLog(t, manager, launch.ID)
	if !strings.Contains(string(log.Data), fmt.Sprintf("port %d taken", launch.Port)) || !strings.Contains(string(log.Data), fmt.Sprintf("restarted it on port %d", ready.Port)) {
		t.Fatalf("log = %q", log.Data)
	}
}

func TestServiceStopWhileStarting(t *testing.T) {
	t.Parallel()
	manager := newServiceManager(t)
	command := serviceCommand("slow", "")
	command.Command = "sleep 300"
	launch, err := manager.Start(StartRequest{Command: command, Target: t.TempDir(), Kind: TargetDirectory})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := manager.Stop(launch.ID)
	if err != nil || stopped.State != StateStopped || stopped.Error != "" {
		t.Fatalf("Stop() while starting = %+v, %v", stopped, err)
	}
}

func TestServiceNeedsAServicePath(t *testing.T) {
	t.Parallel()
	manager := newTestManager(t, ManagerConfig{})
	if _, err := manager.Start(StartRequest{Command: serviceCommand("web", ""), Target: t.TempDir(), Kind: TargetDirectory}); err == nil || !strings.Contains(err.Error(), "cannot serve services") {
		t.Fatalf("Start() without ServicePath = %v", err)
	}
}

// A service that requires the launch token is ready only because the
// probe presents it, and the upstream carries it for the proxy; each
// launch has its own token.
func TestServiceTokenAndProxyHeaders(t *testing.T) {
	t.Parallel()
	manager := newServiceManager(t)
	command := serviceCommand("guarded", "ROTARI_TEST_LAUNCHER_REQUIRE_TOKEN=1")
	command.ProxyHeaders = map[string]string{"Authorization": "token ${" + EnvToken + "}"}
	tokens := map[string]bool{}
	for range 2 {
		launch, err := manager.Start(StartRequest{Command: command, Target: t.TempDir(), Kind: TargetDirectory})
		if err != nil {
			t.Fatal(err)
		}
		if ready := waitState(t, manager, launch.ID, notStarting); ready.State != StateRunning {
			t.Fatalf("service requiring the token = %+v", ready)
		}
		upstream, err := manager.ServiceUpstream(launch.ID)
		if err != nil {
			t.Fatal(err)
		}
		header := upstream.Headers["Authorization"]
		if !strings.HasPrefix(header, "token ") || len(header) < len("token ")+32 || strings.Contains(header, "$") {
			t.Fatalf("resolved header = %q", header)
		}
		tokens[header] = true
		request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s/", upstream.Port, launch.BasePath), nil)
		client := &http.Client{Transport: &http.Transport{Proxy: nil}}
		if response, err := client.Do(request); err != nil || response.StatusCode != http.StatusForbidden {
			t.Fatalf("request without the token = %v, %v; want 403", response, err)
		}
		request.Header.Set("Authorization", header)
		if response, err := client.Do(request); err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("request with the token = %v, %v", response, err)
		}
		if snapshot, _ := manager.Get(launch.ID); snapshot.token != "" || snapshot.proxyHeaders != nil {
			t.Fatal("a launch snapshot carries its token")
		}
	}
	if len(tokens) != 2 {
		t.Fatal("two launches shared a token")
	}
}
