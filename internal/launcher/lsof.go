package launcher

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"strconv"
)

// parseLsofListeners reads `lsof -F pn` output, used on macOS: a "pPID" line starts each
// process and "nADDRESS:PORT" lines name its sockets.
func parseLsofListeners(output []byte, port int, groupOf func(pid int) (int, error)) ([]portListener, error) {
	var listeners []portListener
	pgid := 0
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, err := strconv.Atoi(line[1:])
			if err != nil {
				return nil, fmt.Errorf("invalid lsof process line %q", line)
			}
			if pgid, err = groupOf(pid); err != nil {
				// The process exited; its sockets are gone with it.
				pgid = 0
			}
		case 'n':
			host, portText, err := net.SplitHostPort(line[1:])
			if err != nil || portText != strconv.Itoa(port) {
				continue
			}
			ip := net.IPv4zero
			if host != "*" {
				if ip = net.ParseIP(host); ip == nil {
					return nil, fmt.Errorf("invalid lsof address line %q", line)
				}
			}
			if acceptsLoopback(ip) {
				listeners = append(listeners, portListener{ip: ip, pgid: pgid})
			}
		}
	}
	return listeners, scanner.Err()
}
