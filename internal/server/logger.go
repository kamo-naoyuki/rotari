package server

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LockPath is the lease file of the supervisor of dir, a project directory.
func LockPath(dir string) string { return filepath.Join(dir, "server.lock") }

// PIDPath records the PID of the supervisor of dir while it runs.
func PIDPath(dir string) string { return filepath.Join(dir, "server.pid") }

type Logger struct {
	Path     string
	FileMode os.FileMode
	MaxBytes int64
	// Prefix, if set, starts every event, so the supervisors of several
	// projects can share one log.
	Prefix string
	mu     sync.Mutex
}

func (logger *Logger) Writef(format string, args ...any) {
	if logger == nil {
		return
	}
	line := time.Now().UTC().Format(time.RFC3339) + " " + logger.Prefix + fmt.Sprintf(format, args...) + "\n"
	if logger.MaxBytes > 0 && int64(len(line)) > logger.MaxBytes {
		line = line[:logger.MaxBytes]
	}
	logger.mu.Lock()
	defer logger.mu.Unlock()
	file, err := os.OpenFile(logger.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, logger.FileMode)
	if err != nil {
		return
	}
	defer file.Close()
	if logger.MaxBytes > 0 {
		if info, err := file.Stat(); err == nil && (info.Size() >= logger.MaxBytes || info.Size()+int64(len(line)) > logger.MaxBytes) {
			if err := file.Truncate(0); err != nil {
				return
			}
		}
	}
	_, _ = file.WriteString(line)
}
