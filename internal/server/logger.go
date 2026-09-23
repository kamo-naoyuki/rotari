package server

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type Logger struct {
	Path     string
	FileMode os.FileMode
	MaxBytes int64
	mu       sync.Mutex
}

func (logger *Logger) Writef(format string, args ...any) {
	if logger == nil {
		return
	}
	line := time.Now().UTC().Format(time.RFC3339) + " " + fmt.Sprintf(format, args...) + "\n"
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
