package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// maxSocketPathLength is the longest socket path that fits in sun_path on
// every supported platform (104 bytes on macOS and BSD, 108 on Linux, both
// including the terminating NUL).
const maxSocketPathLength = 103

// shortSocketRoot holds per-user socket directories for base directories
// whose own socket path would be too long. It is a fixed path rather than
// $TMPDIR so every client of a base directory computes the same socket.
var shortSocketRoot = "/tmp"

// SocketPath returns the server socket for baseDir: <baseDir>/server.sock when
// it fits, otherwise a short per-user path derived from the resolved baseDir.
func SocketPath(baseDir string) string {
	path := filepath.Join(baseDir, "server.sock")
	if len(path) <= maxSocketPathLength {
		return path
	}
	key := baseDir
	if absolute, err := filepath.Abs(baseDir); err == nil {
		key = absolute
		if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
			key = resolved
		}
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(shortSocketDir(), hex.EncodeToString(sum[:8])+".sock")
}

func shortSocketDir() string {
	return filepath.Join(shortSocketRoot, fmt.Sprintf("rotari-%d", os.Getuid()))
}

// prepareSocketDir creates the short socket directory when socketPath lives
// there and refuses one that another user could control.
func prepareSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	if dir != shortSocketDir() {
		return nil
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create server socket directory %s: %w", dir, err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("failed to inspect server socket directory %s: %w", dir, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(stat.Uid) != os.Getuid() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("server socket directory %s must be a directory owned by the current user with mode 0700", dir)
	}
	return nil
}
