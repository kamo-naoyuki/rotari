package interfaces

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// TestProjectListHintsWork runs the commands the project list suggests, with
// its placeholders filled in, for a project outside the default state
// directory. With ROTARI_BASEDIR set, `show` lists only that directory, so
// the other one is listed with -b; without it, `show` lists every registered
// basedir.
func TestProjectListHintsWork(t *testing.T) {
	covers(t, "CLI-5")
	for _, test := range []struct {
		name     string
		withEnv  bool
		listArgs func(other string) []string
	}{
		{"ROTARI_BASEDIR", true, func(other string) []string { return []string{"show", "-b", other} }},
		{"XDG_STATE_HOME", false, func(string) []string { return []string{"show"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			if !test.withEnv {
				e = e.Without("ROTARI_BASEDIR")
			}
			other := filepath.Join(e.Root, "other")
			e.MustRotari("add", "-b", other, "-p", "exp", "--", "false")
			if result := e.Rotari("run", "-b", other, "-p", "exp", "--quiet"); result.Code == 0 {
				t.Fatalf("run with a failing job should exit 1: %s", result)
			}
			var shown struct {
				RunID string `json:"run_id"`
			}
			if err := json.Unmarshal([]byte(e.MustRotari("show", "-b", other, "-p", "exp", "--json").Stdout), &shown); err != nil || shown.RunID == "" {
				t.Fatalf("show --json did not name the run: %v", err)
			}

			listing := e.MustRotari(test.listArgs(other)...).Stdout
			row := ""
			for _, line := range strings.Split(listing, "\n") {
				if strings.HasPrefix(line, other+" ") {
					row = line
				}
			}
			if !strings.Contains(row, shown.RunID) || !strings.HasSuffix(strings.TrimSpace(row), "failed 1/1") {
				t.Fatalf("project list row for %s = %q, want its last run and result:\n%s", other, row, listing)
			}
			fill := strings.NewReplacer("BASEDIR", other, "PROJECT", "exp", "RUN_ID", shown.RunID)
			hints := 0
			for _, heading := range []string{"To show runs in a project:", "To summarize a run's failures by cause:"} {
				_, after, found := strings.Cut(listing, heading)
				if !found {
					t.Fatalf("project list has no %q hint:\n%s", heading, listing)
				}
				line := strings.Split(after, "\n")[1]
				command, ok := strings.CutPrefix(line, "  rotari ")
				if !ok {
					t.Fatalf("hint under %q is %q", heading, line)
				}
				hints++
				if result := e.Rotari(strings.Fields(fill.Replace(command))...); result.Code != 0 {
					t.Errorf("hinted command failed: %s", result)
				}
			}
			if hints != 2 {
				t.Fatalf("found %d hints, want 2", hints)
			}
		})
	}
}

// TestMissingProjectNamesWhereItIs asks the default state directory for a
// project that only another registered state directory has, as an agent does
// before it knows the basedir, and checks that the error names that
// directory and how to select it; jobs, which lists state directories, points
// to --all-basedirs.
func TestMissingProjectNamesWhereItIs(t *testing.T) {
	covers(t, "CLI-11")
	e := support.NewEnv(t)
	elsewhere := filepath.Join(e.Root, "elsewhere")
	e.MustRotari("add", "-b", elsewhere, "-p", "exp", "--", "true")
	for _, args := range [][]string{{"show", "-p", "exp"}, {"lineage", "-p", "exp"}} {
		result := e.Rotari(args...)
		if result.Code == 0 || !strings.Contains(result.Stderr, elsewhere) || !strings.Contains(result.Stderr, "--basedir") {
			t.Errorf("%v = %s, want an error naming %s and --basedir", args, result, elsewhere)
		}
	}
	if result := e.Rotari("jobs", "exp"); result.Code == 0 || !strings.Contains(result.Stderr, "1 registered state directory has it") || !strings.Contains(result.Stderr, "--all-basedirs") {
		t.Errorf("jobs exp = %s, want a pointer to --all-basedirs", result)
	}
	// A state directory with runs is named with its last run's result, which
	// tells same-named projects apart.
	e.MustRotari("run", "-b", elsewhere, "-p", "exp", "--quiet")
	if result := e.Rotari("show", "-p", "exp"); !strings.Contains(result.Stderr, "last run ") || !strings.Contains(result.Stderr, "finished, 0 of 1 failed") {
		t.Errorf("show -p exp = %s, want the last run's result beside %s", result, elsewhere)
	}
	if result := e.Rotari("show", "-p", "nowhere"); strings.Contains(result.Stderr, elsewhere) {
		t.Errorf("a project no state directory has points to %s: %s", elsewhere, result)
	}
}
