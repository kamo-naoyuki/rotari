package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestProgressJournalIncrementalRead(t *testing.T) {
	runDir := t.TempDir()
	journal, err := NewProgressJournal(runDir)
	if err != nil {
		t.Fatal(err)
	}
	var cursor ProgressCursor
	assertProgressRead(t, &cursor, runDir, nil)
	first := model.ProgressEvent{OK: true, Progress: true, JobID: "job-1", Completed: 1, Total: 3, Succeeded: 1, Message: "line one\nline two"}
	if err := journal.Append(first); err != nil {
		t.Fatal(err)
	}
	assertProgressRead(t, &cursor, runDir, []model.ProgressEvent{first})
	assertProgressRead(t, &cursor, runDir, nil)
	second := model.ProgressEvent{OK: true, Progress: true, JobID: "job-2", Completed: 2, Total: 3, Failed: 1, Message: strings.Repeat("x", 100000)}
	data, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	appendBytes := func(data []byte) {
		t.Helper()
		file, err := os.OpenFile(filepath.Join(runDir, ProgressFileName), os.O_WRONLY|os.O_APPEND, FileMode())
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("append = %v, %v", writeErr, closeErr)
		}
	}
	appendBytes(data[:len(data)/2])
	for range 2 {
		assertProgressRead(t, &cursor, runDir, nil)
	}
	appendBytes(data[len(data)/2:])
	assertProgressRead(t, &cursor, runDir, nil)
	appendBytes([]byte("\n"))
	assertProgressRead(t, &cursor, runDir, []model.ProgressEvent{second})
	var replay ProgressCursor
	assertProgressRead(t, &replay, runDir, []model.ProgressEvent{first, second})
}

func TestProgressCursorSkipsExistingEvents(t *testing.T) {
	runDir := t.TempDir()
	journal, err := NewProgressJournal(runDir)
	if err != nil {
		t.Fatal(err)
	}
	oldEvent := model.ProgressEvent{Progress: true, Completed: 1, Total: 3, Succeeded: 1}
	if err := journal.Append(oldEvent); err != nil {
		t.Fatal(err)
	}
	var cursor ProgressCursor
	if snapshot, ok, err := cursor.SkipExisting(runDir); err != nil || !ok || snapshot.Completed != 1 || snapshot.Total != 3 || snapshot.Succeeded != 1 || snapshot.Failed != 0 {
		t.Fatalf("SkipExisting snapshot = %+v, %t, %v", snapshot, ok, err)
	}
	assertProgressRead(t, &cursor, runDir, nil)
	newEvent := model.ProgressEvent{Progress: true, Message: "happened after attach"}
	if err := journal.Append(newEvent); err != nil {
		t.Fatal(err)
	}
	assertProgressRead(t, &cursor, runDir, []model.ProgressEvent{newEvent})
}

func TestProgressCursorSkipsCompleteLinesButKeepsPartialTail(t *testing.T) {
	runDir := t.TempDir()
	journal, err := NewProgressJournal(runDir)
	if err != nil {
		t.Fatal(err)
	}
	oldEvent := model.ProgressEvent{Progress: true, Message: "old"}
	if err := journal.Append(oldEvent); err != nil {
		t.Fatal(err)
	}
	partialEvent := model.ProgressEvent{Progress: true, Message: "in flight at attach"}
	partial, err := json.Marshal(partialEvent)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(runDir, ProgressFileName), os.O_APPEND|os.O_WRONLY, FileMode())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(partial); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var cursor ProgressCursor
	if snapshot, ok, err := cursor.SkipExisting(runDir); err != nil || ok {
		t.Fatalf("partial event became a snapshot: %+v, %t, %v", snapshot, ok, err)
	}
	assertProgressRead(t, &cursor, runDir, nil)
	file, err = os.OpenFile(filepath.Join(runDir, ProgressFileName), os.O_APPEND|os.O_WRONLY, FileMode())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	assertProgressRead(t, &cursor, runDir, []model.ProgressEvent{partialEvent})
}

func assertProgressRead(t *testing.T, cursor *ProgressCursor, runDir string, want []model.ProgressEvent) {
	t.Helper()
	if got, err := cursor.Read(runDir); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("read: count=%d, want=%d, err=%v", len(got), len(want), err)
	}
}

func TestProgressJournalConcurrentAppend(t *testing.T) {
	runDir := t.TempDir()
	journal, err := NewProgressJournal(runDir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for index := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := journal.Append(model.ProgressEvent{OK: true, Progress: true, JobID: fmt.Sprint(index), Message: strings.Repeat("event", 2000)}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var cursor ProgressCursor
	events, err := cursor.Read(runDir)
	if err != nil || len(events) != 64 {
		t.Fatalf("concurrent events = %d, %v", len(events), err)
	}
	seen := make(map[string]bool)
	for _, event := range events {
		if seen[event.JobID] || event.Message != strings.Repeat("event", 2000) {
			t.Fatalf("corrupt or duplicate event: %+v", event)
		}
		seen[event.JobID] = true
	}
}

func TestProgressJournalPathsAndErrors(t *testing.T) {
	for _, path := range []string{"", "../run", "runs/../run", `runs\..\run`, "runs/./run"} {
		if _, err := NewProgressJournal(path); err == nil {
			t.Fatalf("accepted unsafe path %q", path)
		}
		var cursor ProgressCursor
		if _, err := cursor.Read(path); err == nil {
			t.Fatalf("read accepted unsafe path %q", path)
		}
	}
	runDir := filepath.Join(t.TempDir(), "absent")
	journal, err := NewProgressJournal(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(model.ProgressEvent{Progress: true}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory append = %v", err)
	}
	if _, err := os.Stat(runDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("append created missing run directory: %v", err)
	}
}

func TestProgressJournalModes(t *testing.T) {
	for _, private := range []string{"false", "true"} {
		t.Run(private, func(t *testing.T) {
			t.Setenv("ROTARI_PRIVATE_STATE", private)
			testProgressJournalMode(t)
		})
	}
}

func testProgressJournalMode(t *testing.T) {
	t.Helper()
	runDir := t.TempDir()
	journal, err := NewProgressJournal(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(model.ProgressEvent{Message: "control"}); err == nil {
		t.Fatal("accepted non-progress event")
	}
	path := filepath.Join(runDir, ProgressFileName)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-progress event created journal: %v", err)
	}
	if err := journal.Append(model.ProgressEvent{Progress: true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm()&^FileMode() != 0 {
		t.Fatalf("journal mode exceeds state mode: %v, %v", info, err)
	}
	// Changing the setting must not chmod an existing journal.
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := journal.Append(model.ProgressEvent{Progress: true}); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("existing mode changed: %v, %v", info, err)
	}
}

func TestProgressCursorMalformedLine(t *testing.T) {
	for _, bad := range []string{"not JSON", `{ "message": "control" }`} {
		t.Run(bad, func(t *testing.T) {
			runDir := t.TempDir()
			line := "{\"progress\":true,\"job_id\":\"job\"}\n"
			if err := os.WriteFile(filepath.Join(runDir, ProgressFileName), []byte(line+bad+"\n"+line), 0o600); err != nil {
				t.Fatal(err)
			}
			var cursor ProgressCursor
			if events, err := cursor.Read(runDir); !errors.Is(err, ErrInvalidJSON) || len(events) != 1 {
				t.Fatalf("malformed read = %v, %v", events, err)
			}
			if events, err := cursor.Read(runDir); err != nil || len(events) != 1 {
				t.Fatalf("read after malformed line = %v, %v", events, err)
			}
		})
	}
}
