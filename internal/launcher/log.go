package launcher

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// DefaultLogSegmentBytes is the size of each of a launch's two log segments.
const DefaultLogSegmentBytes = 8 << 20

// LogChunk is a page of a launch's output.
type LogChunk struct {
	// Offset is where Data starts in the command's whole output.
	Offset int64  `json:"offset"`
	Data   []byte `json:"data"`
	// Next is the offset to request for the following page.
	Next int64 `json:"next"`
	// Dropped is true when output before Offset that the caller asked for
	// was discarded to bound the log's size.
	Dropped bool `json:"dropped"`
	// Total is how many bytes the command has written so far.
	Total int64 `json:"total"`
}

// rotatingLog keeps the most recent output of a launch in two files of at
// most segment bytes each: when "current" is full it becomes "previous",
// replacing the older one.
type rotatingLog struct {
	mu      sync.Mutex
	dir     string
	segment int64
	current *os.File
	// currentStart and previousStart are the output offsets where the files
	// begin; total is the offset of the next byte.
	currentStart, previousStart, total int64
	hasPrevious                        bool
	closed                             bool
}

func newRotatingLog(dir string, segment int64) (*rotatingLog, error) {
	current, err := os.OpenFile(filepath.Join(dir, "current.log"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	return &rotatingLog{dir: dir, segment: segment, current: current}, nil
}

func (log *rotatingLog) Write(data []byte) (int, error) {
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return 0, os.ErrClosed
	}
	written := 0
	for len(data) > 0 {
		room := log.segment - (log.total - log.currentStart)
		if room <= 0 {
			if err := log.rotate(); err != nil {
				return written, err
			}
			continue
		}
		part := data[:min(int64(len(data)), room)]
		n, err := log.current.Write(part)
		written += n
		log.total += int64(n)
		if err != nil {
			return written, err
		}
		data = data[n:]
	}
	return written, nil
}

func (log *rotatingLog) rotate() error {
	if err := log.current.Close(); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(log.dir, "current.log"), filepath.Join(log.dir, "previous.log")); err != nil {
		return err
	}
	current, err := os.OpenFile(filepath.Join(log.dir, "current.log"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	log.current = current
	log.previousStart, log.currentStart, log.hasPrevious = log.currentStart, log.total, true
	return nil
}

// Read returns up to limit bytes of output starting at offset. An offset
// before the oldest kept byte starts at that byte and sets Dropped.
func (log *rotatingLog) Read(offset int64, limit int) (LogChunk, error) {
	log.mu.Lock()
	defer log.mu.Unlock()
	if offset < 0 || limit <= 0 {
		return LogChunk{}, fmt.Errorf("invalid log range: offset %d, limit %d", offset, limit)
	}
	oldest := log.currentStart
	if log.hasPrevious {
		oldest = log.previousStart
	}
	chunk := LogChunk{Offset: min(max(offset, oldest), log.total), Total: log.total}
	chunk.Dropped = offset < oldest
	data := make([]byte, 0, min(int64(limit), log.total-chunk.Offset))
	position := chunk.Offset
	for len(data) < cap(data) {
		name, start := "current.log", log.currentStart
		if position < log.currentStart {
			name, start = "previous.log", log.previousStart
		}
		end := log.total
		if name == "previous.log" {
			end = log.currentStart
		}
		part, err := readRange(filepath.Join(log.dir, name), position-start, min(int64(cap(data)-len(data)), end-position))
		if err != nil {
			return LogChunk{}, err
		}
		data = append(data, part...)
		position += int64(len(part))
		if len(part) == 0 {
			break
		}
	}
	chunk.Data, chunk.Next = data, position
	return chunk, nil
}

func readRange(path string, offset, length int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data := make([]byte, length)
	n, err := file.ReadAt(data, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return data[:n], nil
}

// Close stops further writes; reads keep working until the files are
// removed.
func (log *rotatingLog) Close() error {
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return nil
	}
	log.closed = true
	return log.current.Close()
}
