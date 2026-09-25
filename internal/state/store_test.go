package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestLoadMetaDefaultsMissingAndEmptyFields(t *testing.T) {
	missing, err := LoadMeta(filepath.Join(t.TempDir(), "meta.json"))
	if err != nil || missing.Phase != "collecting" || missing.UpdatedAt == "" {
		t.Fatalf("missing meta = %#v, %v", missing, err)
	}

	path := filepath.Join(t.TempDir(), "meta.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadMeta(path)
	if err != nil || loaded.Phase != "collecting" || loaded.UpdatedAt == "" {
		t.Fatalf("empty meta = %#v, %v", loaded, err)
	}
}

func TestLoadStateReturnsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	if err := os.WriteFile(path, []byte("{invalid}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadQueue(path)
	if !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("LoadQueue error = %v, want ErrInvalidJSON", err)
	}
}

func TestValidateStatePathRejectsTraversalSegments(t *testing.T) {
	for _, path := range []string{
		"",
		".",
		"..",
		"safe/../escape.json",
		"safe\\..\\escape.json",
	} {
		if err := ValidateStatePath(path); err == nil {
			t.Errorf("ValidateStatePath accepted unsafe path %q", path)
		}
	}
	for _, path := range []string{"/tmp/rotari/queue.json", "relative/queue.json", "safe\\queue.json"} {
		if err := ValidateStatePath(path); err != nil {
			t.Errorf("ValidateStatePath rejected valid path %q: %v", path, err)
		}
	}
}

func TestReadJSONRejectsTraversalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "safe") + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escape.json"
	var queue model.Queue
	if err := NewStore(0o700, 0o600).ReadJSON(path, &queue); err == nil {
		t.Fatalf("ReadJSON accepted traversal path %q", path)
	}
	baseDir := filepath.Join(t.TempDir(), "safe")
	basePath := filepath.Join(baseDir, "queue.json")
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(basePath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(0o700, 0o600).ReadJSON(basePath, &queue); err != nil {
		t.Fatalf("ReadJSON rejected a valid absolute state path: %v", err)
	}
}

func TestWriteJSONRejectsTraversalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "safe") + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escape.json"
	if err := WriteJSON(path, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1"}}}); err == nil {
		t.Fatalf("WriteJSON accepted traversal path %q", path)
	}
	basePath := filepath.Join(t.TempDir(), "safe", "value.json")
	if err := WriteJSON(basePath, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1"}}}); err != nil {
		t.Fatalf("WriteJSON rejected a valid absolute state path: %v", err)
	}
}

func TestLoadSamplesRoundTripAndRejectsTraversalPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "load.jsonl")
	want := []model.LoadSample{{At: "2026-09-25T00:00:00Z"}, {At: "2026-09-25T00:00:01Z"}}
	for _, sample := range want {
		if err := AppendLoadSample(path, sample); err != nil {
			t.Fatalf("AppendLoadSample() error = %v", err)
		}
	}
	if got := ReadLoadSamples(path); len(got) != len(want) || got[0].At != want[0].At || got[1].At != want[1].At {
		t.Fatalf("ReadLoadSamples() = %#v, want %#v", got, want)
	}

	unsafePath := filepath.Join(t.TempDir(), "safe") + string(os.PathSeparator) + ".." + string(os.PathSeparator) + "escape.jsonl"
	if err := AppendLoadSample(unsafePath, model.LoadSample{}); err == nil {
		t.Fatalf("AppendLoadSample accepted traversal path %q", unsafePath)
	}
	if got := ReadLoadSamples(unsafePath); got != nil {
		t.Fatalf("ReadLoadSamples accepted traversal path %q: %#v", unsafePath, got)
	}
}

func TestWriteJSONCreatesParentAndReadableDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "value.json")
	if err := WriteJSON(path, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1"}}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") || !strings.Contains(string(data), "job-1") {
		t.Fatalf("written JSON = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("written JSON mode = %o, want 644", got)
	}
}

func TestWriteJSONRespectsConfiguredStateModes(t *testing.T) {
	t.Setenv(privateStateEnv, "")
	path := filepath.Join(t.TempDir(), "shared", "value.json")
	if err := WriteJSON(path, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1"}}}); err != nil {
		t.Fatal(err)
	}
	if got := fileModeForTest(path); got != 0o644 {
		t.Fatalf("shared state mode = %o, want 0644", got)
	}
	if got := dirModeForTest(filepath.Dir(path)); got != 0o755 {
		t.Fatalf("shared state dir mode = %o, want 0755", got)
	}

	t.Setenv(privateStateEnv, "true")
	path = filepath.Join(t.TempDir(), "private", "value.json")
	if err := WriteJSON(path, model.Queue{Commands: []model.QueuedCommand{{ID: "job-2"}}}); err != nil {
		t.Fatal(err)
	}
	if got := fileModeForTest(path); got != 0o600 {
		t.Fatalf("private state mode = %o, want 0600", got)
	}
	if got := dirModeForTest(filepath.Dir(path)); got != 0o700 {
		t.Fatalf("private state dir mode = %o, want 0700", got)
	}
}

func fileModeForTest(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	return info.Mode().Perm()
}

func dirModeForTest(path string) os.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	return info.Mode().Perm()
}

func TestRequireRunStateFileRequiresRegularValidatedFiles(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RequireRunStateFile(runDir, "context.json", "run-1"); err != nil {
		t.Fatalf("RequireRunStateFile() error = %v", err)
	}
	if err := RequireRunStateFile(runDir, "missing.json", "run-1"); err == nil {
		t.Fatal("RequireRunStateFile() accepted an invalid state filename")
	}
}

func TestValidateRunDirectoryChecksRequiredSnapshots(t *testing.T) {
	runsDir := t.TempDir()
	runDir := filepath.Join(runsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateRunDirectory(runsDir, "run-1", false); err != nil {
		t.Fatalf("ValidateRunDirectory() error = %v", err)
	}
	if _, err := ValidateRunDirectory(runsDir, "run-1", true); err == nil {
		t.Fatal("ValidateRunDirectory() accepted interrupted run without commands snapshot")
	}
}

func TestListRunJobDirsIncludesOnlyJobSnapshots(t *testing.T) {
	runDir := t.TempDir()
	for _, name := range []string{"job-2", "job-1", "without-command"} {
		if err := os.MkdirAll(filepath.Join(runDir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"job-1", "job-2"} {
		if err := os.WriteFile(filepath.Join(runDir, name, "command.json"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ListRunJobDirs(runDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(runDir, "job-1"), filepath.Join(runDir, "job-2")}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ListRunJobDirs() = %#v, want %#v", got, want)
	}
}

func TestLoadQueueMissingReturnsEmptyQueue(t *testing.T) {
	queue, err := LoadQueue(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || len(queue.Commands) != 0 {
		t.Fatalf("missing queue = %#v, %v", queue, err)
	}
}

func TestWriteJSONStampsStateVersion(t *testing.T) {
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "queue.json")
	summaryPath := filepath.Join(dir, "summary.json")
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}}}}
	if err := WriteJSON(queuePath, &queue); err != nil {
		t.Fatal(err)
	}
	if queue.StateVersion != 0 {
		t.Fatalf("WriteJSON changed the caller's queue: %#v", queue)
	}
	if err := WriteJSON(summaryPath, model.RunSummary{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{queuePath, summaryPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"state_version": 1`) {
			t.Fatalf("%s does not record the state version:\n%s", path, data)
		}
	}
	loaded, err := LoadQueue(queuePath)
	if err != nil || loaded.StateVersion != model.StateVersion || len(loaded.Commands) != 1 {
		t.Fatalf("LoadQueue = %#v, %v", loaded, err)
	}
}

func TestLoadStateAcceptsLegacyAndRejectsNewerVersions(t *testing.T) {
	dir := t.TempDir()
	legacyQueue := filepath.Join(dir, "legacy-queue.json")
	legacySummary := filepath.Join(dir, "legacy-summary.json")
	if err := os.WriteFile(legacyQueue, []byte(`{"commands":[{"id":"job","command":["true"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacySummary, []byte(`{"run_id":"run-1","status":"finished","results":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if queue, err := LoadQueue(legacyQueue); err != nil || len(queue.Commands) != 1 || queue.StateVersion != 0 {
		t.Fatalf("legacy queue = %#v, %v", queue, err)
	}
	if summary, err := LoadRunSummary(legacySummary); err != nil || summary.RunID != "run-1" {
		t.Fatalf("legacy summary = %#v, %v", summary, err)
	}

	newerQueue := filepath.Join(dir, "newer-queue.json")
	newerSummary := filepath.Join(dir, "newer-summary.json")
	if err := os.WriteFile(newerQueue, []byte(`{"state_version":99,"commands":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newerSummary, []byte(`{"state_version":99,"run_id":"run-2"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadQueue(newerQueue); !errors.Is(err, ErrNewerStateVersion) || !strings.Contains(err.Error(), "upgrade rotari") {
		t.Fatalf("LoadQueue(newer) error = %v, want ErrNewerStateVersion", err)
	}
	if _, err := ReadQueueFile(newerQueue); !errors.Is(err, ErrNewerStateVersion) {
		t.Fatalf("ReadQueueFile(newer) error = %v, want ErrNewerStateVersion", err)
	}
	if _, err := LoadRunSummary(newerSummary); !errors.Is(err, ErrNewerStateVersion) {
		t.Fatalf("LoadRunSummary(newer) error = %v, want ErrNewerStateVersion", err)
	}
	if _, err := ReadQueueFile(filepath.Join(dir, "missing.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadQueueFile(missing) error = %v, want os.ErrNotExist", err)
	}
}
