package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileSupportsAllFormats(t *testing.T) {
	tests := []struct {
		name      string
		extension string
		content   string
	}{
		{name: "yaml", extension: ".yaml", content: "executor: slurm\nlocal-concurrency: 4\n"},
		{name: "toml", extension: ".toml", content: "executor = \"slurm\"\nlocal-concurrency = 4\n"},
		{name: "json", extension: ".json", content: `{"executor":"slurm","local-concurrency":4}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "config"+test.extension), []byte(test.content), 0o644); err != nil {
				t.Fatal(err)
			}
			values, err := LoadFile(directory)
			if err != nil {
				t.Fatal(err)
			}
			if stringValue(values, "executor") != "slurm" || intValue(values, "local-concurrency") != 4 {
				t.Fatalf("config = %#v", values)
			}
		})
	}
}

func TestLoadFileRejectsMultipleFormats(t *testing.T) {
	directory := t.TempDir()
	for _, extension := range []string{".yaml", ".toml"} {
		if err := os.WriteFile(filepath.Join(directory, "config"+extension), []byte("executor = \"local\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := LoadFile(directory)
	if err == nil || !strings.Contains(err.Error(), "multiple config files") {
		t.Fatalf("LoadFile error = %v", err)
	}
}

func TestLoadFileWarnsAndIgnoresInvalidFormat(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte("run: [invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	values, err := LoadFile(directory)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("config = %#v, want empty config", values)
	}
}

func TestPathsForRunRejectsUnsafeProjectName(t *testing.T) {
	baseDir := t.TempDir()
	paths := PathsForRun(baseDir, "../outside")
	for _, path := range paths {
		if strings.Contains(path, "outside") {
			t.Fatalf("PathsForRun returned path outside the project root: %q", path)
		}
	}
}

func stringValue(config map[string]any, name string) string {
	return strings.TrimSpace(strings.Trim(fmt.Sprint(config[name]), `"`))
}

func intValue(config map[string]any, name string) int {
	value := fmt.Sprint(config[name])
	var result int
	_, _ = fmt.Sscanf(value, "%d", &result)
	return result
}
