package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func writeConfig(t *testing.T, directory, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPathParsesByExtension(t *testing.T) {
	directory := t.TempDir()
	values, err := LoadPath(writeConfig(t, directory, "config.toml", "executor = \"slurm\"\n"))
	if err != nil || stringValue(values, "executor") != "slurm" {
		t.Fatalf("LoadPath = %#v, %v", values, err)
	}
	if _, err := LoadPath(filepath.Join(directory, "missing.yaml")); err == nil {
		t.Fatal("LoadPath accepted a missing file")
	}
	if _, err := LoadPath(writeConfig(t, directory, "config.ini", "executor=slurm\n")); err == nil || !strings.Contains(err.Error(), "unsupported config format") {
		t.Fatalf("LoadPath(.ini) error = %v", err)
	}
	if _, err := LoadPath(writeConfig(t, directory, "bad.json", "{")); err == nil {
		t.Fatal("LoadPath accepted malformed JSON")
	}
}

func TestPathsForRunUsesFirstScopeWithConfig(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	baseDir := t.TempDir()
	projectDir := filepath.Join(baseDir, "projects", "demo")

	if path := EffectivePath(baseDir, "demo"); path != "" {
		t.Fatalf("EffectivePath without config = %q", path)
	}
	if path := GlobalPath(); path != "" {
		t.Fatalf("GlobalPath without config = %q", path)
	}
	global := writeConfig(t, filepath.Join(configHome, "rotari"), "config.yaml", "executor: local\n")
	if path := GlobalPath(); path != global {
		t.Fatalf("GlobalPath = %q, want %q", path, global)
	}
	if path := EffectivePath(baseDir, "demo"); path != global {
		t.Fatalf("EffectivePath with global config = %q, want %q", path, global)
	}
	base := writeConfig(t, baseDir, "config.toml", "executor = \"slurm\"\n")
	if path := EffectivePath(baseDir, "demo"); path != base {
		t.Fatalf("EffectivePath with basedir config = %q, want %q", path, base)
	}
	project := writeConfig(t, projectDir, "config.json", `{"executor":"pbs"}`)
	if path := EffectivePath(baseDir, "demo"); path != project {
		t.Fatalf("EffectivePath with project config = %q, want %q", path, project)
	}
	if path := EffectivePath(baseDir, ""); path != base {
		t.Fatalf("EffectivePath without a project = %q, want %q", path, base)
	}
}

func TestListPathsShowsEveryScope(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	baseDir := t.TempDir()
	global := writeConfig(t, filepath.Join(configHome, "rotari"), "config.yaml", "executor: local\n")
	globalNotifications := writeConfig(t, filepath.Join(configHome, "rotari"), "notifications.toml", "")
	base := writeConfig(t, baseDir, "config.toml", "")
	demo := writeConfig(t, filepath.Join(baseDir, "projects", "demo"), "config.json", "{}")
	otherNotifications := writeConfig(t, filepath.Join(baseDir, "projects", "other"), "notifications.toml", "")
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(baseDir, "notifications.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(baseDir, "projects"), "stray-file", "")
	additional := []string{"notifications.toml", "", "../config.yaml"}

	common, projects := ListPaths(baseDir, "", additional...)
	if want := []string{global, globalNotifications, base}; !reflect.DeepEqual(common, want) {
		t.Fatalf("common = %v, want %v", common, want)
	}
	if want := map[string][]string{"demo": {demo}, "other": {otherNotifications}}; !reflect.DeepEqual(projects, want) {
		t.Fatalf("projects = %v, want %v", projects, want)
	}

	_, projects = ListPaths(baseDir, "demo", additional...)
	if want := map[string][]string{"demo": {demo}}; !reflect.DeepEqual(projects, want) {
		t.Fatalf("projects for demo = %v, want %v", projects, want)
	}
	_, projects = ListPaths(baseDir, "empty", additional...)
	if len(projects) != 0 {
		t.Fatalf("projects for a project without config = %v", projects)
	}
	common, projects = ListPaths(baseDir, "../outside", additional...)
	if len(common) != 3 || len(projects) != 0 {
		t.Fatalf("ListPaths(unsafe project) = %v, %v", common, projects)
	}
}
