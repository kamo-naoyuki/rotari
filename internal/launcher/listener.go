package launcher

import (
	"fmt"
	"net"
)

// LoopbackHost is the only address a launched service may listen on.
const LoopbackHost = "127.0.0.1"

// AllocatePort returns a TCP port on LoopbackHost that was free when this
// function returned. Another process can bind it before the service does;
// VerifyListener detects that.
func AllocatePort() (int, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(LoopbackHost, "0"))
	if err != nil {
		return 0, fmt.Errorf("allocate a loopback port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// VerifyListener returns nil only if a process in process group pgid listens
// on exactly LoopbackHost:port and no process outside the group listens on an
// address that also accepts connections to it (0.0.0.0, ::, or
// ::ffff:127.0.0.1). Call it after a connection to the port succeeds and
// before trusting anything the port returns.
//
// It fails when it cannot inspect listeners on this platform, so callers
// never skip the check by accident.
func VerifyListener(port, pgid int) error {
	listeners, err := portListeners(port)
	if err != nil {
		return fmt.Errorf("cannot verify which process listens on port %d: %w", port, err)
	}
	return checkListeners(listeners, port, pgid)
}

// portListener is one listening socket that can accept connections to
// LoopbackHost:port.
type portListener struct {
	// ip is the address the socket is bound to.
	ip net.IP
	// pgid is the owning process's group, or 0 when the owner is not one of
	// this user's processes.
	pgid int
}

func checkListeners(listeners []portListener, port, pgid int) error {
	exact := false
	for _, listener := range listeners {
		if listener.pgid != pgid {
			return fmt.Errorf("another process listens on %s", net.JoinHostPort(listener.ip.String(), fmt.Sprint(port)))
		}
		// Equal also matches ::ffff:127.0.0.1, which accepts the same
		// IPv4 connections.
		if listener.ip.Equal(net.ParseIP(LoopbackHost)) {
			exact = true
		}
	}
	if exact {
		return nil
	}
	if len(listeners) > 0 {
		return fmt.Errorf("the service listens on %s, not %s; bind it to %s", net.JoinHostPort(listeners[0].ip.String(), fmt.Sprint(port)), net.JoinHostPort(LoopbackHost, fmt.Sprint(port)), LoopbackHost)
	}
	return fmt.Errorf("no process in the launch listens on %s", net.JoinHostPort(LoopbackHost, fmt.Sprint(port)))
}

// acceptsLoopback reports whether a socket bound to ip receives connections
// to LoopbackHost.
func acceptsLoopback(ip net.IP) bool {
	return ip.Equal(net.ParseIP(LoopbackHost)) || ip.IsUnspecified()
}
