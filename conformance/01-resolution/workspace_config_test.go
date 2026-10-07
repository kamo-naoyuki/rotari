package resolution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func writeWorkspaceFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceInitAndCWDOnlyDiscovery(t *testing.T) {
	covers(t, "RES-23", "RES-24")
	e := support.NewEnv(t).Without("ROTARI_BASEDIR")
	e.MustRotari("init", "workspace-state", "demo")
	for _, path := range []string{filepath.Join(e.Root, "workspace-state"), e.Master} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("init created %s: %v", path, err)
		}
	}
	workspace := filepath.Join(e.Root, ".rotari.toml")
	before, _ := os.ReadFile(workspace)
	if result := e.Rotari("init", "replacement"); result.Code == 0 || !strings.Contains(result.Stderr, ".rotari.toml already exists") || strings.Contains(result.Stderr, ".rotari-init-") {
		t.Fatalf("unexpected overwrite diagnostic: %+v", result)
	}
	after, _ := os.ReadFile(workspace)
	if string(before) != string(after) {
		t.Fatal("init changed existing settings")
	}
	e.MustRotari("add", "--", "true")
	if !projectCreated(filepath.Join(e.Root, "workspace-state"), "demo") {
		t.Fatal("workspace defaults ignored")
	}
	e.WithVar("ROTARI_BASEDIR", e.Base).WithVar("ROTARI_PROJECT_NAME", "environment").MustRotari("add", "--", "true")
	if !projectCreated(e.Base, "environment") {
		t.Fatal("environment precedence ignored")
	}
	e.WithVar("ROTARI_BASEDIR", e.Base).MustRotari("add", "-b", filepath.Join(e.Root, "cli-state"), "-p", "cli", "--", "true")
	if !projectCreated(filepath.Join(e.Root, "cli-state"), "cli") {
		t.Fatal("CLI precedence ignored")
	}
	child := filepath.Join(e.Root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	e.In(child).MustRotari("add", "--", "true")
	if !projectCreated(filepath.Join(e.Root, "state", "rotari"), "default") {
		t.Fatal("child inherited ancestor workspace")
	}
	if result := e.In(child).Rotari("init", e.Base); result.Code == 0 {
		t.Fatal("absolute init basedir accepted")
	}
	if result := e.In(child).Rotari("init", "state", `bad\project`); result.Code == 0 {
		t.Fatal("unsafe init project accepted")
	}
	listing := e.MustRotari("config", "--list").Stdout
	if !strings.Contains(listing, workspace) {
		t.Fatalf("workspace absent from list: %s", listing)
	}
}

func TestWorkspaceInitDefaultTemplate(t *testing.T) {
	covers(t, "RES-24")
	for _, args := range [][]string{nil, {"local-state"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			e := support.NewEnv(t).Without("ROTARI_BASEDIR")
			e.MustRotari(append([]string{"init"}, args...)...)
			data, err := os.ReadFile(filepath.Join(e.Root, ".rotari.toml"))
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			if !strings.Contains(content, "project-name = \"default\"") || !strings.Contains(content, "# retry = \"\"") {
				t.Fatalf("missing default project or commented options: %s", content)
			}
			base := ".rotari-state"
			if len(args) > 0 {
				base = args[0]
			}
			if !strings.Contains(content, "basedir = \""+base+"\"") {
				t.Fatalf("missing basedir %q: %s", base, content)
			}
			e.MustRotari("add", "true")
			if !projectCreated(filepath.Join(e.Root, base), "default") {
				t.Fatal("init defaults did not select the expected project")
			}
		})
	}
}

func TestMergedFileConfigSnapshotAndScopeValidation(t *testing.T) {
	covers(t, "RES-23", "RES-25")
	e := support.NewEnv(t)
	global := filepath.Join(e.Root, "config", "rotari", "config.json")
	workspace := filepath.Join(e.Root, ".rotari.toml")
	base := filepath.Join(e.Base, "config.yaml")
	project := filepath.Join(e.Base, "projects", "demo", "config.toml")
	writeWorkspaceFixture(t, global, `{"quiet":true,"run":{"retry":2},"data":{"global":"kept","array":["a","b"]}}`)
	writeWorkspaceFixture(t, workspace, "project-name = 'workspace'\nquiet = false\n[data]\narray = ['workspace']\n")
	writeWorkspaceFixture(t, base, "project-name: demo\nrun:\n  local-concurrency: 3\ndata:\n  base: kept\n")
	writeWorkspaceFixture(t, project, "[run]\nretry = 1\n[data]\nproject = 'kept'\n")
	e.MustRotari("add", "--", "true")
	e.WithVar("ROTARI_RUN_RETRY", "7").MustRotari("run", "--retry", "0", "--local-concurrency", "9", "--quiet")
	var meta struct {
		LastRunID string `json:"last_run_id"`
	}
	metaData, err := os.ReadFile(filepath.Join(e.Base, "projects", "demo", "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(metaData, &meta); err != nil || meta.LastRunID == "" {
		t.Fatalf("meta: %s, %v", metaData, err)
	}
	runDir := filepath.Join(e.Base, "projects", "demo", "runs", meta.LastRunID)
	snapshotPath := filepath.Join(runDir, "configs", "config.toml")
	snapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"retry = 1", "local-concurrency = 3", "global = \"kept\"", "base = \"kept\"", "project = \"kept\"", "array = [\"workspace\"]", "quiet = false"} {
		if !strings.Contains(string(snapshot), want) {
			t.Fatalf("snapshot missing %s:\n%s", want, snapshot)
		}
	}
	for _, unwanted := range []string{"retry = 7", "retry = 0", "local-concurrency = 9", "async ="} {
		if strings.Contains(string(snapshot), unwanted) {
			t.Fatalf("runtime/default leaked: %s", snapshot)
		}
	}
	var context struct {
		ConfigSources       []struct{ Scope, Path string } `json:"config_sources"`
		ConfigSnapshotFiles []string                       `json:"config_snapshot_files"`
	}
	contextData, _ := os.ReadFile(filepath.Join(runDir, "context.json"))
	if err := json.Unmarshal(contextData, &context); err != nil || len(context.ConfigSources) != 4 || len(context.ConfigSnapshotFiles) != 1 {
		t.Fatalf("context = %s, %v", contextData, err)
	}
	for _, path := range []string{global, base, project} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	writeWorkspaceFixture(t, workspace, "basedir = 'other-state'\nproject-name = 'other-project'\n")
	e.Without("ROTARI_BASEDIR").MustRotari("show", meta.LastRunID, "--json")
	unchanged, _ := os.ReadFile(snapshotPath)
	if string(unchanged) != string(snapshot) {
		t.Fatal("run snapshot changed after source deletion")
	}
	for _, test := range []struct{ path, key, content string }{{base, "basedir", "basedir: forbidden\n"}, {base, "basedir", "run:\n  basedir: forbidden\n"}, {project, "project-name", "[run]\nproject-name = 'forbidden'\n"}, {project, "basedir", "basedir = 'forbidden'\n"}} {
		writeWorkspaceFixture(t, test.path, test.content)
		result := e.Rotari("check", "-p", "demo")
		if result.Code == 0 || !strings.Contains(result.Stderr, test.path) || !strings.Contains(result.Stderr, test.key) {
			t.Fatalf("scope validation: %s", result)
		}
		if err := os.Remove(test.path); err != nil {
			t.Fatal(err)
		}
	}
	selected := filepath.Join(e.Root, "selected.toml")
	writeWorkspaceFixture(t, selected, "project-name = 'explicit'\n")
	writeWorkspaceFixture(t, base, "[malformed")
	e.MustRotari("add", "--config", selected, "--", "true")
	if !projectCreated(e.Base, "explicit") {
		t.Fatal("--config did not replace automatic layers")
	}
}

func TestWebWorkspaceSourceSelectionAndRunSnapshot(t *testing.T) {
	covers(t, "RES-23", "RES-25")
	e := support.NewEnv(t)
	workspace := filepath.Join(e.Root, ".rotari.toml")
	base := filepath.Join(e.Base, "config.toml")
	project := filepath.Join(e.Base, "projects", "demo", "config.toml")
	writeWorkspaceFixture(t, workspace, "project-name = 'demo'\n")
	writeWorkspaceFixture(t, base, "quiet = false\n")
	writeWorkspaceFixture(t, project, "[run]\nretry = 1\n")
	e.MustRotari("add", "--", "true")
	e.MustRotari("run", "--quiet", "--async")
	e.MustRotari("wait", "-p", "demo")
	var meta struct {
		LastRunID string `json:"last_run_id"`
	}
	data, _ := os.ReadFile(filepath.Join(e.Base, "projects", "demo", "meta.json"))
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	address := e.StartWeb("--allow-control")
	response := e.HTTPGet(address + "/api/config?project_name=demo")
	var payload struct {
		Configs []struct{ Scope, Path, Content string } `json:"configs"`
	}
	if response.Status != 200 {
		t.Fatalf("config: %#v", response)
	}
	if err := json.Unmarshal([]byte(response.Body), &payload); err != nil || len(payload.Configs) != 3 {
		t.Fatalf("sources: %#v, %v", response, err)
	}
	response = e.HTTPPostJSON(address+"/api/save-config", map[string]string{"project_name": "demo", "scope": "workspace", "path": workspace, "content": "project-name = 'demo'\nquiet = true\n"})
	if response.Status != 200 {
		t.Fatalf("selected save: %#v", response)
	}
	data, _ = os.ReadFile(base)
	if string(data) != "quiet = false\n" {
		t.Fatal("selected workspace save changed basedir")
	}
	for _, request := range []map[string]string{{"project_name": "demo", "scope": "project", "path": project, "content": "[run]\nproject-name = 'wrong'\n"}, {"project_name": "demo", "scope": "global", "path": filepath.Join(e.Root, "outside.toml"), "content": "quiet = true\n"}} {
		if response := e.HTTPPostJSON(address+"/api/save-config", request); response.Status == 200 {
			t.Fatalf("invalid save: %#v", response)
		}
	}
	response = e.HTTPGet(address + "/api/config?project_name=demo&run_id=" + meta.LastRunID)
	if response.Status != 200 || !strings.Contains(response.Body, "config.toml") || !strings.Contains(response.Body, "quiet = false") {
		t.Fatalf("saved snapshot: %#v", response)
	}
}
