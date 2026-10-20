package launcher

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// tcpListenState is TCP_LISTEN in /proc/net/tcp's st column.
const tcpListenState = "0A"

// portListeners reads listening sockets from /proc/net/tcp and tcp6 and
// finds their owners among this user's processes. A socket owned by another
// user, whose /proc/PID/fd cannot be read, keeps pgid 0.
func portListeners(port int) ([]portListener, error) {
	inodes := map[string]net.IP{}
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		if err := readListenTable(table, port, inodes); err != nil {
			if os.IsNotExist(err) && table == "/proc/net/tcp6" {
				continue
			}
			return nil, err
		}
	}
	if len(inodes) == 0 {
		return nil, nil
	}
	owners, err := socketOwnerGroups(inodes)
	if err != nil {
		return nil, err
	}
	listeners := make([]portListener, 0, len(inodes))
	for inode, ip := range inodes {
		listeners = append(listeners, portListener{ip: ip, pgid: owners[inode]})
	}
	return listeners, nil
}

// readListenTable adds the inode and address of each listening socket on
// port that accepts loopback connections.
func readListenTable(path string, port int, inodes map[string]net.IP) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Scan() // header
	for scanner.Scan() {
		// sl local_address rem_address st tx:rx tr:when retrnsmt uid timeout inode
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 || fields[3] != tcpListenState {
			continue
		}
		hexIP, hexPort, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		localPort, err := strconv.ParseUint(hexPort, 16, 16)
		if err != nil || int(localPort) != port {
			continue
		}
		ip, err := parseProcIP(hexIP)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if acceptsLoopback(ip) && fields[9] != "0" {
			inodes[fields[9]] = ip
		}
	}
	return scanner.Err()
}

// parseProcIP decodes /proc/net/tcp's address: 32-bit words in host (little
// endian on supported platforms) byte order.
func parseProcIP(text string) (net.IP, error) {
	raw, err := hex.DecodeString(text)
	if err != nil || (len(raw) != net.IPv4len && len(raw) != net.IPv6len) {
		return nil, fmt.Errorf("invalid address %q", text)
	}
	ip := make(net.IP, len(raw))
	for word := 0; word < len(raw); word += 4 {
		for i := range 4 {
			ip[word+i] = raw[word+3-i]
		}
	}
	return ip, nil
}

// socketOwnerGroups maps each socket inode to the process group of a
// process that holds it.
func socketOwnerGroups(inodes map[string]net.IP) (map[string]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	owners := map[string]int{}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fds, err := os.ReadDir(filepath.Join("/proc", entry.Name(), "fd"))
		if err != nil {
			// Another user's process, or one that has exited.
			continue
		}
		pgid := 0
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join("/proc", entry.Name(), "fd", fd.Name()))
			if err != nil {
				continue
			}
			inode, ok := strings.CutPrefix(link, "socket:[")
			if !ok {
				continue
			}
			inode = strings.TrimSuffix(inode, "]")
			if _, wanted := inodes[inode]; !wanted {
				continue
			}
			if pgid == 0 {
				if pgid, err = processGroup(pid); err != nil {
					break
				}
			}
			// Every holder must be in the group: a socket shared with an
			// outside process is not owned by the launch.
			if previous, seen := owners[inode]; seen && previous != pgid {
				owners[inode] = -1
			} else {
				owners[inode] = pgid
			}
		}
	}
	return owners, nil
}

// processGroup reads the process group from /proc/PID/stat. The command
// name can contain spaces and parentheses, so fields are counted from its
// closing parenthesis.
func processGroup(pid int) (int, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return 0, fmt.Errorf("invalid /proc/%d/stat", pid)
	}
	// After the name: state ppid pgrp ...
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 3 {
		return 0, fmt.Errorf("invalid /proc/%d/stat", pid)
	}
	return strconv.Atoi(fields[2])
}
