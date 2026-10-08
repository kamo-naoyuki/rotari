package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

const ProgressFileName = "progress.jsonl"

// ProgressJournal appends progress events to one existing run directory.
// Share one instance between the run's writers; Append serializes whole lines.
// It holds no open file and does not create directories or rewrite history.
type ProgressJournal struct {
	mu   sync.Mutex
	path string
}

func progressPath(runDir string) (string, error) {
	if err := ValidateStatePath(runDir); err != nil {
		return "", err
	}
	return SafeJoin(runDir, ProgressFileName)
}

func NewProgressJournal(runDir string) (*ProgressJournal, error) {
	path, err := progressPath(runDir)
	if err != nil {
		return nil, err
	}
	return &ProgressJournal{path: path}, nil
}

// Append writes one newline-terminated JSON event using the state file mode.
// Non-progress events are rejected. Errors are returned to the caller, which
// decides whether journaling is best-effort; Append does not fsync each event.
func (journal *ProgressJournal) Append(event model.ProgressEvent) error {
	if !event.Progress {
		return errors.New("not a progress event")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	journal.mu.Lock()
	defer journal.mu.Unlock()
	file, err := os.OpenFile(journal.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, FileMode())
	if err != nil {
		return err
	}
	n, err := file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return closeErr
}

// ProgressCursor is an incremental reader for one run's append-only journal.
// Its zero value starts at the beginning. Use a separate cursor per run and
// consumer; it is not safe for concurrent reads.
type ProgressCursor struct {
	offset int64
}

// SkipExisting advances the cursor past complete events already in the
// journal and returns their latest progress-count snapshot, if one exists. If
// the final line is still being appended, the cursor stays at its start so
// Read can consume it once the line is complete.
func (cursor *ProgressCursor) SkipExisting(runDir string) (model.ProgressEvent, bool, error) {
	path, err := progressPath(runDir)
	if err != nil {
		return model.ProgressEvent{}, false, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		cursor.offset = 0
		return model.ProgressEvent{}, false, nil
	}
	if err != nil {
		return model.ProgressEvent{}, false, err
	}
	defer file.Close()

	var snapshot model.ProgressEvent
	hasSnapshot := false
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadBytes('\n')
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return model.ProgressEvent{}, false, readErr
		}
		cursor.offset += int64(len(line))
		var event model.ProgressEvent
		if err := json.Unmarshal(line, &event); err != nil || !event.Progress {
			continue
		}
		if strings.HasPrefix(event.Message, "=== Run started ===") || event.Total > 0 {
			snapshot = event
			hasSnapshot = true
		}
	}
	return snapshot, hasSnapshot, nil
}

// Read returns newly completed lines. An absent journal (including old runs)
// is an empty successful read. A trailing line without a newline is left at
// the cursor for the next read, even if it already contains valid JSON.
// A malformed complete line is consumed and returns ErrInvalidJSON along with
// any preceding events; a later Read can continue after it.
func (cursor *ProgressCursor) Read(runDir string) ([]model.ProgressEvent, error) {
	path, err := progressPath(runDir)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err := file.Seek(cursor.offset, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	var events []model.ProgressEvent
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return events, nil
		}
		if err != nil {
			return events, err
		}
		cursor.offset += int64(len(line))
		var event model.ProgressEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return events, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
		}
		if !event.Progress {
			return events, fmt.Errorf("%w: not a progress event", ErrInvalidJSON)
		}
		events = append(events, event)
	}
}
