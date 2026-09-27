package resolution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMain(m *testing.M) { os.Exit(support.Run(m)) }

func covers(t *testing.T, _ ...string) { t.Helper() }

func projectCreated(dir, project string) bool {
	_, err := os.Stat(filepath.Join(dir, "projects", project, "queue.json"))
	return err == nil
}

func TestBaseDirResolutionOrder(t *testing.T) {
	covers(t, "RES-1")
	type layout struct{ flag, env, local, xdg, home string }
	dirs := func(e *support.Env) layout {
		return layout{filepath.Join(e.Root, "flag"), e.Base, filepath.Join(e.Root, ".rotari-state"), filepath.Join(e.Root, "state", "rotari"), filepath.Join(e.Root, "home", ".local", "state", "rotari")}
	}
	cases := []struct {
		name  string
		setup func(*support.Env) (*support.Env, []string)
		want  func(layout) string
	}{
		{"--basedir over ROTARI_BASEDIR", func(e *support.Env) (*support.Env, []string) { return e, []string{"-b", dirs(e).flag} }, func(l layout) string { return l.flag }},
		{"ROTARI_BASEDIR over ./.rotari-state", func(e *support.Env) (*support.Env, []string) {
			if err := os.MkdirAll(dirs(e).local, 0o755); err != nil {
				t.Fatal(err)
			}
			return e, nil
		}, func(l layout) string { return l.env }},
		{"./.rotari-state over XDG_STATE_HOME", func(e *support.Env) (*support.Env, []string) {
			if err := os.MkdirAll(dirs(e).local, 0o755); err != nil {
				t.Fatal(err)
			}
			return e.Without("ROTARI_BASEDIR"), nil
		}, func(l layout) string { return l.local }},
		{"XDG_STATE_HOME over the home default", func(e *support.Env) (*support.Env, []string) { return e.Without("ROTARI_BASEDIR"), nil }, func(l layout) string { return l.xdg }},
		{"the home default last", func(e *support.Env) (*support.Env, []string) {
			return e.Without("ROTARI_BASEDIR").Without("XDG_STATE_HOME"), nil
		}, func(l layout) string { return l.home }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := support.NewEnv(t)
			l := dirs(e)
			resolved, flags := c.setup(e)
			resolved.MustRotari(append(append([]string{"add"}, flags...), "-p", "where", "--", "true")...)
			want := c.want(l)
			for _, dir := range []string{l.flag, l.env, l.local, l.xdg, l.home} {
				if got := projectCreated(dir, "where"); got != (dir == want) {
					t.Errorf("project in %s: %t, want it only in %s", dir, got, want)
				}
			}
		})
	}
}

func TestProjectResolutionOrder(t *testing.T) {
	covers(t, "RES-2")
	e := support.NewEnv(t)
	e.MustRotari("add", "--", "true")
	if !projectCreated(e.Base, "default") {
		t.Fatal(`add without a project did not create "default"`)
	}
	var shown struct {
		ProjectName string `json:"project_name"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "--json").Stdout), &shown); err != nil || shown.ProjectName != "default" {
		t.Fatalf("show --json chose project %q, want the only project: %v", shown.ProjectName, err)
	}
	fromEnv := e.WithVar("ROTARI_PROJECT_NAME", "fromenv")
	fromEnv.MustRotari("add", "--", "true")
	fromEnv.MustRotari("add", "-p", "fromflag", "--", "true")
	if !projectCreated(e.Base, "fromenv") || !projectCreated(e.Base, "fromflag") {
		t.Fatal("ROTARI_PROJECT_NAME or --project-name did not name the project")
	}
	if r := e.Rotari("check"); r.Code == 0 || !strings.Contains(r.Stderr+r.Stdout, "fromenv") {
		t.Errorf("check with several projects should fail and list them: %s", r)
	}
}
