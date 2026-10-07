package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLayerMerge(t *testing.T) {
	lower := map[string]any{"flag": true, "zero": 9, "text": "old", "array": []any{"a", "b"}, "run": map[string]any{"retry": 2, "executor": "local"}}
	higher := map[string]any{"flag": false, "zero": 0, "text": "", "array": []any{}, "run": map[string]any{"retry": nil, "executor": "slurm"}}
	got := Merge(lower, higher)
	want := map[string]any{"flag": false, "zero": 0, "text": "", "array": []any{}, "run": map[string]any{"retry": 2, "executor": "slurm"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge = %#v", got)
	}
	got["run"].(map[string]any)["retry"] = 3
	if lower["run"].(map[string]any)["retry"] != 2 {
		t.Fatal("merge mutated input")
	}
	data, err := Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	values, err := Parse("config.toml", data)
	if err != nil || values["flag"] != false || values["text"] != "" {
		t.Fatalf("roundtrip = %v, %v", values, err)
	}
}

func TestWorkspaceDiscoveryOnlyCWD(t *testing.T) {
	parent := t.TempDir()
	writeConfig(t, parent, WorkspaceFile, "basedir = 'state'\n")
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadScope("workspace", child)
	if err != nil || len(loaded.Sources) != 0 {
		t.Fatalf("child = %v, %v", loaded, err)
	}
	loaded, err = LoadScope("workspace", parent)
	if err != nil || loaded.Values["basedir"] != "state" {
		t.Fatalf("parent = %v, %v", loaded, err)
	}
}

func TestLayerScopesAndRestrictions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home, _ := HomeDir()
	cwd, base := t.TempDir(), t.TempDir()
	writeConfig(t, home, "config.json", `{"run":{"retry":2,"output":["a","b"]},"quiet":true}`)
	writeConfig(t, cwd, WorkspaceFile, "quiet = false\n[run]\noutput = ['c']\n")
	writeConfig(t, base, "config.yaml", "project-name: demo\nrun:\n  executor: local\n")
	project := filepath.Join(base, "projects", "demo")
	writeConfig(t, project, "config.toml", "[run]\nretry = 0\n")
	loaded, err := Load(cwd, base, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Sources) != 4 || loaded.Values["quiet"] != false {
		t.Fatalf("loaded = %#v", loaded)
	}
	run := loaded.Values["run"].(map[string]any)
	if intValue(run, "retry") != 0 || run["executor"] != "local" || !reflect.DeepEqual(run["output"], []any{"c"}) {
		t.Fatalf("run = %#v", run)
	}
	for _, scope := range []string{"basedir", "project"} {
		for _, content := range []string{"basedir = 'x'", "[run]\nbasedir = 'x'", "project-name = 'x'", "[show]\nproject-name = 'x'"} {
			if scope == "basedir" && strings.Contains(content, "project-name") {
				continue
			}
			values, err := Parse("config.toml", []byte(content))
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(scope, "/selected/config.toml", values); err == nil || !strings.Contains(err.Error(), "/selected/config.toml") {
				t.Fatalf("%s %s: %v", scope, content, err)
			}
		}
	}
	writeConfig(t, project, "config.toml", "[broken")
	if _, err := Load(cwd, base, "demo"); err == nil {
		t.Fatal("malformed layer accepted")
	}
	writeConfig(t, base, "config.json", "{}")
	if _, err := LoadScope("basedir", base); err == nil {
		t.Fatal("duplicate formats accepted")
	}
}
