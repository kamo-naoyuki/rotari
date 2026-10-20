package launcher

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// readyPoll is how often a starting service is checked.
const readyPoll = 200 * time.Millisecond

// probeTimeout bounds one connection or HTTP probe.
const probeTimeout = 2 * time.Second

// readiness is how one wait for a service to become ready ended.
type readiness struct {
	ready bool
	// lostPort means the process exited before anything listened on its
	// port, as when another process took the port first.
	lostPort bool
	// failure is why the launch failed; nil with !ready means it ended by
	// itself or was stopped.
	failure error
}

// serve takes a service from start to ready, then watches it like a
// command. Its port is trusted only after VerifyListener: a process outside
// the launch that answers on the port is never reported ready.
func (manager *Manager) serve(launch *Launch, command Command) {
	deadline := time.Now().Add(command.ReadyTimeout)
	retried := false
	for {
		outcome := manager.awaitReady(launch, command, deadline)
		switch {
		case outcome.ready:
			manager.mu.Lock()
			if launch.State == StateStarting {
				launch.State = StateRunning
			}
			manager.mu.Unlock()
			manager.watch(launch, nil)
			return
		case outcome.lostPort && !retried:
			retried = true
			manager.mu.Lock()
			if launch.stopCalled || manager.closed {
				manager.mu.Unlock()
				manager.watch(launch, nil)
				return
			}
			lost := launch.Port
			err := manager.spawn(launch)
			if err == nil {
				fmt.Fprintf(launch.log, "\nrotari: the service exited without listening on port %d; restarted it on port %d\n", lost, launch.Port)
			}
			manager.mu.Unlock()
			if err != nil {
				manager.watch(launch, fmt.Errorf("the service exited without listening on port %d, and restarting it failed: %w", lost, err))
				return
			}
		case outcome.failure != nil:
			manager.mu.Lock()
			process := launch.process
			manager.mu.Unlock()
			_, _ = process.Stop()
			manager.watch(launch, outcome.failure)
			return
		default:
			manager.watch(launch, nil)
			return
		}
	}
}

// awaitReady polls the launch's port until it is ready, its process ends,
// or deadline passes.
func (manager *Manager) awaitReady(launch *Launch, command Command, deadline time.Time) readiness {
	manager.mu.Lock()
	process, port, path := launch.process, launch.Port, launch.BasePath+command.ReadyPath
	manager.mu.Unlock()
	address := net.JoinHostPort(LoopbackHost, strconv.Itoa(port))
	listened := false
	for {
		select {
		case <-process.Done():
			manager.mu.Lock()
			stopped := launch.stopCalled
			manager.mu.Unlock()
			switch {
			case stopped:
				return readiness{}
			case !listened:
				return readiness{lostPort: true, failure: fmt.Errorf("the service exited without listening on %s", address)}
			}
			return readiness{failure: fmt.Errorf("the service exited before it was ready")}
		default:
		}
		if time.Now().After(deadline) {
			return readiness{failure: fmt.Errorf("the service was not ready at %s within %s", path, command.ReadyTimeout)}
		}
		if conn, err := net.DialTimeout("tcp", address, probeTimeout); err == nil {
			conn.Close()
			listened = true
			if err := VerifyListener(port, process.PGID()); err != nil {
				return readiness{failure: err}
			}
			if probeHTTP(address, path) == nil {
				return readiness{ready: true}
			}
		}
		select {
		case <-process.Done():
		case <-time.After(readyPoll):
		}
	}
}

// probeHTTP sends one GET and accepts a 2xx or 3xx status. It speaks just
// enough HTTP/1.1 for that, so this package needs no HTTP client.
func probeHTTP(address, path string) error {
	conn, err := net.DialTimeout("tcp", address, probeTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(probeTimeout)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: rotari-launcher\r\nConnection: close\r\n\r\n", path, address); err != nil {
		return err
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return err
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "HTTP/1.") {
		return fmt.Errorf("not an HTTP response: %q", line)
	}
	status, err := strconv.Atoi(fields[1])
	if err != nil || status < 200 || status >= 400 {
		return fmt.Errorf("status %s", fields[1])
	}
	return nil
}

// ServiceUpstream returns the port and process group of a ready service,
// for a proxy that must check, per connection, that the port still
// belongs to the launch (VerifyListener).
func (manager *Manager) ServiceUpstream(id string) (port, pgid int, err error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	launch, err := manager.find(id)
	if err != nil {
		return 0, 0, err
	}
	if launch.Mode != ModeService || launch.State != StateRunning {
		return 0, 0, fmt.Errorf("%w: %s is %s", ErrNotServing, launch.Name, launch.State)
	}
	return launch.Port, launch.process.PGID(), nil
}
