package executor

import (
	"encoding/json"
	"os"
	"time"

	"github.com/rotari-dev/rotari/internal/state"
)

// SchedulerEvent is one line of a job's scheduler_events.jsonl. Query is
// "queue", "accounting", or "result"; a queue or accounting event is
// written only when its outcome changes.
type SchedulerEvent struct {
	At       string `json:"at"`
	Query    string `json:"query"`
	State    string `json:"state,omitempty"`
	Error    string `json:"error,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

const (
	schedulerEventsSizeLimit  = 256 << 10
	schedulerEventsErrorLimit = 1000
)

type schedulerEventLog struct {
	store state.Store
	path  string
	now   func() time.Time
	last  map[string]string
}

func newSchedulerEventLog(store state.Store, jobDir string, now func() time.Time) *schedulerEventLog {
	path, err := state.ValidatedStateFile(jobDir, state.SchedulerEventsFileName)
	if err != nil {
		path = ""
	}
	return &schedulerEventLog{store: store, path: path, now: now, last: make(map[string]string)}
}

// observe records a queue or accounting outcome that differs from the
// previous outcome of the same query. A query that succeeded without a
// state is recorded as "missing".
func (log *schedulerEventLog) observe(query, value string, err error) {
	event := SchedulerEvent{Query: query, State: value}
	if err != nil {
		event.Error = limitSchedulerEventText(err.Error())
	} else if value == "" {
		event.State = "missing"
	}
	key := event.State + "\x00" + event.Error
	if previous, seen := log.last[query]; seen && previous == key {
		return
	}
	log.last[query] = key
	log.write(event)
}

// result records where the job's result came from: "wrapper",
// "accounting", or "unavailable".
func (log *schedulerEventLog) result(source string, exitCode int, message string) {
	log.write(SchedulerEvent{Query: "result", State: source, ExitCode: &exitCode, Error: limitSchedulerEventText(message)})
}

func (log *schedulerEventLog) write(event SchedulerEvent) {
	if log.path == "" {
		return
	}
	if info, err := os.Stat(log.path); err == nil && info.Size() >= schedulerEventsSizeLimit {
		return
	}
	event.At = log.now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	// codeql[go/path-injection]: path is restricted by ValidatedStateFile to a fixed state file name.
	file, err := os.OpenFile(log.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, log.store.FileMode)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.Write(append(data, '\n'))
}

func limitSchedulerEventText(text string) string {
	if len(text) <= schedulerEventsErrorLimit {
		return text
	}
	return text[:schedulerEventsErrorLimit] + "..."
}
