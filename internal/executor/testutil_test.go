package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func testStore() state.Store {
	return state.NewStore(0o700, 0o600)
}

func testLogf(string, ...any) {}

func writeExecutable(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func init() {
	// Tests drive scheduler polling with fake queries; spacing them out would
	// only slow the suite down. TestSchedulerQueryGateSpacesQueries covers it.
	schedulerQueryGate = newSchedulerSubmissionGate(0)
}
