package resolution

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestLatestRunID(t *testing.T) {
	covers(t, "RES-12")
	e := support.NewEnv(t)
	e.FinishedJobRun("a")
	second, _ := e.FinishedJobRun("a")
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "a", "--run-id", "latest", "--json").Stdout), &shown); err != nil || shown.RunID != second {
		t.Errorf("--run-id latest chose %q, want %q: %v", shown.RunID, second, err)
	}
	for _, args := range [][]string{{"add", "-p", "latest", "--", "true"}, {"add", "-p", "a", "--job-name", "latest", "--", "true"}, {"add", "-p", "a", "--stage", "latest", "--", "true"}, {"run", "-p", "a", "--run-name", "latest"}} {
		if r := e.Rotari(args...); r.Code == 0 || !strings.Contains(r.Stderr, "reserved") {
			t.Errorf("reserved name latest was accepted: %s", r)
		}
	}
}

func TestLatestRunIDSkipsActiveRun(t *testing.T) {
	covers(t, "RES-12")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "a", "--job-name", "settled", "--", "true")
	e.MustRotari("run", "-p", "a", "--quiet")
	settledRunID := showRunID(t, e.MustRotari("show", "-p", "a", "--json").Stdout)
	e.StartRun("a", 1, true)

	if got := showRunID(t, e.MustRotari("show", "-p", "a", "--run-id", "latest", "--json").Stdout); got != settledRunID {
		t.Fatalf("show --run-id latest selected %q, want settled run %q", got, settledRunID)
	}
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "copy defaults to latest", args: nil},
		{name: "copy run-id latest", args: []string{"--run-id", "latest"}},
		{name: "copy job name", args: []string{"--job-name", "settled"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"copy", "-p", "a", "--overwrite", "--quiet"}, test.args...)
			e.MustRotari(args...)
			var queue struct {
				Commands struct {
					Commands []struct {
						Name   string `json:"name"`
						Origin *struct {
							RunID string `json:"run_id"`
						} `json:"origin"`
					} `json:"commands"`
				} `json:"commands"`
			}
			if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "a", "--queue", "--json").Stdout), &queue); err != nil {
				t.Fatal(err)
			}
			if len(queue.Commands.Commands) != 1 {
				t.Fatalf("copied queue has %d commands, want one: %+v", len(queue.Commands.Commands), queue)
			}
			copied := queue.Commands.Commands[0]
			if copied.Name != "settled" || copied.Origin == nil || copied.Origin.RunID != settledRunID {
				t.Fatalf("copied command = %+v, want settled job from run %q", copied, settledRunID)
			}
		})
	}
}

func TestRunIDAloneResolvesLocation(t *testing.T) {
	covers(t, "RES-13")
	e := support.NewEnv(t)
	runID, attemptID := e.FinishedJobRun("a")
	elsewhere := e.Without("ROTARI_BASEDIR")
	for _, id := range []string{runID, attemptID} {
		if r := elsewhere.Rotari("show", id); r.Code != 0 || !strings.Contains(r.Stdout, runID) {
			t.Errorf("show %s outside its base directory: %s", id, r)
		}
	}
	var shown struct {
		BaseDir     string `json:"base_dir"`
		ProjectName string `json:"project_name"`
	}
	if err := json.Unmarshal([]byte(elsewhere.MustRotari("show", "--run-id", runID, "--json").Stdout), &shown); err != nil || shown.BaseDir != e.Base || shown.ProjectName != "a" {
		t.Errorf("show --run-id resolved %q/%q, want %q/a: %v", shown.BaseDir, shown.ProjectName, e.Base, err)
	}
}

func TestExplicitLocationMustMatchRegistry(t *testing.T) {
	covers(t, "RES-14")
	e := support.NewEnv(t)
	runID, _ := e.FinishedJobRun("a")
	e.MustRotari("add", "-p", "b", "--", "true")
	for _, command := range []string{"show", "wait"} {
		for _, args := range [][]string{{command, "-b", filepath.Join(e.Root, "other"), runID}, {command, "-p", "b", runID}} {
			if r := e.Rotari(args...); r.Code == 0 || !strings.Contains(r.Stderr, "registered under") {
				t.Errorf("a location that disagrees with the registry was accepted: %s", r)
			}
		}
	}
	if r := e.Rotari("show", "-p", "a", "--run-id", "20990101-000000-deadbeef"); r.Code == 0 {
		t.Errorf("a missing --run-id fell back instead of failing: %s", r)
	}
}

func TestStateCreatingCommandsDoNotResolveRunIDs(t *testing.T) {
	covers(t, "RES-17")
	e := support.NewEnv(t)
	runID, _ := e.FinishedJobRun("source")
	e.MustRotari("add", "-p", runID, "--", "true")
	check := e.MustRotari("check", runID).Stdout
	if !strings.Contains(check, "project="+runID) {
		t.Fatalf("add treated registered run ID as a run selector: %s", check)
	}
}

func TestWaitResolvesRunIDsIndependently(t *testing.T) {
	covers(t, "RES-19")
	e := support.NewEnv(t)
	first, _ := e.FinishedJobRun("a")
	second, _ := e.FinishedJobRun("b")
	out := e.Without("ROTARI_BASEDIR").MustRotari("wait", first, second, "--json").Stdout
	var runs []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var summary struct {
			RunID string `json:"run_id"`
		}
		if err := json.Unmarshal([]byte(line), &summary); err != nil {
			t.Fatalf("wait --json line %q: %v", line, err)
		}
		runs = append(runs, summary.RunID)
	}
	if strings.Join(runs, ",") != first+","+second {
		t.Errorf("wait --json reported runs %q, want %q and %q", runs, first, second)
	}
}
