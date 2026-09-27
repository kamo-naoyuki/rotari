package conformance

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update conformance golden files")

func TestGoldenOutputs(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		goldenOutput(t, "help.txt", commandOutput(t, "--help"))
	})
	t.Run("schema", func(t *testing.T) {
		goldenOutput(t, "schema.json", commandOutput(t, "schema", "--json"))
	})
	t.Run("show-json", func(t *testing.T) {
		requireUnixSockets(t)
		e := newEnv(t)
		e.mustRotari("add", "-p", "golden", "--", "true")
		if result := e.rotari("run", "-p", "golden", "--quiet"); result.code != 0 {
			if strings.Contains(result.stderr, "server did not become ready") {
				t.Skipf("supervisor unavailable: %s", result.stderr)
			}
			t.Fatalf("run failed: %s", result)
		}
		output := e.mustRotari("show", "-p", "golden", "--json").stdout
		goldenOutput(t, "show.json", normalizeJSON(t, output))
	})
}

func commandOutput(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(rotariBin, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rotari %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func goldenOutput(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update)", path, err)
	}
	if !bytes.Equal(want, []byte(got)) {
		t.Errorf("golden %s differs; run with -update to accept the new output", path)
	}
}

func normalizeJSON(t *testing.T, output string) string {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(output), &value); err != nil {
		t.Fatal(err)
	}
	normalizeJSONValue(value)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func normalizeJSONValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch {
			case key == "base_dir", key == "run_id", key == "attempt_id", key == "updated_at", key == "started_at", key == "finished_at", key == "submitted_at":
				typed[key] = "<normalized>"
			default:
				normalizeJSONValue(child)
			}
		}
	case []any:
		for _, child := range typed {
			normalizeJSONValue(child)
		}
	}
}
