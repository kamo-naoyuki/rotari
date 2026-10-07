package resolution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestAggregateCommandsIgnoreImplicitLocationDefaults(t *testing.T) {
	covers(t, "RES-26")
	e := support.NewEnv(t).Without("ROTARI_BASEDIR").Without("ROTARI_PROJECT_NAME")
	defaultBase := filepath.Join(e.Root, "state", "rotari")
	workspaceBase := filepath.Join(e.Root, "workspace-state")
	environmentBase := filepath.Join(e.Root, "environment-state")

	if err := os.WriteFile(filepath.Join(e.Root, ".rotari.toml"), []byte("basedir = \"workspace-state\"\nproject-name = \"workspace-only\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	globalConfigDir := filepath.Join(e.Root, "config", "rotari")
	if err := os.MkdirAll(globalConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalConfigDir, "config.toml"), []byte("basedir = \"global-state\"\nproject-name = \"global-only\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e = e.WithVar("ROTARI_BASEDIR", environmentBase).WithVar("ROTARI_PROJECT_NAME", "environment-only")

	for _, target := range []struct{ baseDir, project string }{
		{defaultBase, "alpha"}, {defaultBase, "beta"},
		{workspaceBase, "workspace-only"}, {environmentBase, "environment-only"},
		{environmentBase, "alpha"}, {environmentBase, "beta"},
	} {
		e.MustRotari("add", "--basedir", target.baseDir, "--project-name", target.project, "echo", target.project)
		e.MustRotari("run", "--basedir", target.baseDir, "--project-name", target.project, "--quiet")
		projectConfig := filepath.Join(target.baseDir, "projects", target.project, "config.toml")
		if err := os.WriteFile(projectConfig, []byte("retry = 0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	show := e.MustRotari("projects").Stdout
	for _, baseDir := range []string{defaultBase, workspaceBase, environmentBase} {
		if !strings.Contains(show, baseDir) {
			t.Errorf("projects omitted registered basedir %q:\n%s", baseDir, show)
		}
	}
	if !strings.Contains(show, "workspace-only") || !strings.Contains(show, "environment-only") {
		t.Fatalf("projects was narrowed by implicit project defaults:\n%s", show)
	}
	runs := e.MustRotari("runs").Stdout
	for _, project := range []string{"alpha", "beta", "workspace-only", "environment-only"} {
		if !strings.Contains(runs, project) {
			t.Errorf("runs omitted project %q from registered basedirs:\n%s", project, runs)
		}
	}
	if !strings.Contains(runs, "BASEDIR") {
		t.Fatalf("runs did not identify its cross-basedir rows:\n%s", runs)
	}
	workspaceProjects := e.MustRotari("projects", "--basedir", workspaceBase).Stdout
	if !strings.Contains(workspaceProjects, "workspace-only") || strings.Contains(workspaceProjects, "environment-only") || strings.Contains(workspaceProjects, "alpha") {
		t.Fatalf("explicit projects basedir did not narrow the project list:\n%s", workspaceProjects)
	}
	workspaceRuns := e.MustRotari("runs", "--basedir", workspaceBase).Stdout
	if !strings.Contains(workspaceRuns, "workspace-only") || strings.Contains(workspaceRuns, "environment-only") {
		t.Fatalf("explicit runs basedir did not narrow the run list:\n%s", workspaceRuns)
	}
	positionalWorkspaceShow := e.Without("ROTARI_BASEDIR").MustRotari("show", "workspace-only").Stdout
	if !strings.Contains(positionalWorkspaceShow, "Project: workspace-only") {
		t.Fatalf("positional project selector ignored workspace basedir default:\n%s", positionalWorkspaceShow)
	}

	jobs := e.MustRotari("jobs").Stdout
	for _, project := range []string{"alpha", "beta", "workspace-only", "environment-only", "BASEDIR"} {
		if !strings.Contains(jobs, project) {
			t.Errorf("jobs omitted %q from the default cross-basedir listing:\n%s", project, jobs)
		}
	}
	explicitJobs := e.MustRotari("jobs", "--basedir", environmentBase).Stdout
	if !strings.Contains(explicitJobs, "environment-only") || strings.Contains(explicitJobs, "workspace-only") {
		t.Fatalf("jobs --basedir did not narrow the list:\n%s", explicitJobs)
	}
	emptyConfig := filepath.Join(e.Root, "empty.toml")
	if err := os.WriteFile(emptyConfig, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ambiguousShow := e.Without("ROTARI_PROJECT_NAME").Rotari("show", "--basedir", environmentBase, "--config", emptyConfig)
	if ambiguousShow.Code == 0 || !strings.Contains(ambiguousShow.Stderr, "rotari projects") {
		t.Fatalf("show without a unique project did not direct the user to projects: %+v", ambiguousShow)
	}

	lineage := e.Rotari("lineage")
	if lineage.Code == 0 || !strings.Contains(lineage.Stderr, "multiple projects found") || !strings.Contains(lineage.Stderr, "alpha") || !strings.Contains(lineage.Stderr, "beta") || !strings.Contains(lineage.Stderr, "environment-only") || strings.Contains(lineage.Stderr, "workspace-only") {
		t.Fatalf("lineage candidates used an implicit target: %+v", lineage)
	}
	selectedLineage := e.MustRotari("lineage", "--project-name", "alpha").Stdout
	if !strings.Contains(selectedLineage, "Project: alpha") {
		t.Fatalf("explicit lineage project was not selected:\n%s", selectedLineage)
	}

	configList := e.MustRotari("config", "--list").Stdout
	if !strings.Contains(configList, filepath.Join(environmentBase, "projects", "alpha", "config.toml")) || !strings.Contains(configList, filepath.Join(environmentBase, "projects", "beta", "config.toml")) || !strings.Contains(configList, filepath.Join(environmentBase, "projects", "environment-only", "config.toml")) {
		t.Fatalf("config --list used implicit location defaults:\n%s", configList)
	}
	filteredConfigList := e.MustRotari("config", "--list", "--project-name", "alpha").Stdout
	if !strings.Contains(filteredConfigList, filepath.Join(environmentBase, "projects", "alpha", "config.toml")) || strings.Contains(filteredConfigList, filepath.Join(environmentBase, "projects", "beta", "config.toml")) || strings.Contains(filteredConfigList, filepath.Join(environmentBase, "projects", "environment-only", "config.toml")) {
		t.Fatalf("explicit config project filter was not applied:\n%s", filteredConfigList)
	}
}
