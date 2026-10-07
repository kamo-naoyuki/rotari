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
	if !strings.Contains(runs, "alpha") || !strings.Contains(runs, "beta") || strings.Contains(runs, "environment-only") {
		t.Fatalf("runs used implicit location defaults:\n%s", runs)
	}
	allRuns := e.MustRotari("runs", "--all-basedirs").Stdout
	if !strings.Contains(allRuns, "environment-only") || !strings.Contains(allRuns, "alpha") {
		t.Fatalf("runs --all-basedirs omitted registered state directories:\n%s", allRuns)
	}
	workspaceShow := e.MustRotari("projects", "--basedir", workspaceBase).Stdout
	if !strings.Contains(workspaceShow, "workspace-only") || strings.Contains(workspaceShow, "environment-only") || strings.Contains(workspaceShow, "alpha") {
		t.Fatalf("explicit projects basedir did not narrow the project list:\n%s", workspaceShow)
	}
	positionalWorkspaceShow := e.Without("ROTARI_BASEDIR").MustRotari("show", "workspace-only").Stdout
	if !strings.Contains(positionalWorkspaceShow, "Project: workspace-only") {
		t.Fatalf("positional project selector ignored workspace basedir default:\n%s", positionalWorkspaceShow)
	}

	jobs := e.MustRotari("jobs", "--since", "24h").Stdout
	if !strings.Contains(jobs, "alpha") || !strings.Contains(jobs, "beta") || strings.Contains(jobs, "workspace-only") || strings.Contains(jobs, "environment-only") {
		t.Fatalf("jobs used implicit location defaults:\n%s", jobs)
	}
	allJobs := e.MustRotari("jobs", "--all-basedirs", "--since", "24h").Stdout
	if !strings.Contains(allJobs, "workspace-only") || !strings.Contains(allJobs, "environment-only") {
		t.Fatalf("jobs --all-basedirs omitted registered projects:\n%s", allJobs)
	}
	conflictingJobsScope := e.Rotari("jobs", "--all-basedirs", "--basedir", environmentBase)
	if conflictingJobsScope.Code == 0 || !strings.Contains(conflictingJobsScope.Stderr, "cannot be combined") {
		t.Fatalf("jobs --all-basedirs accepted conflicting --basedir: %+v", conflictingJobsScope)
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
