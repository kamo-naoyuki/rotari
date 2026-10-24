package interfaces

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
)

type startedBy struct {
	Interface string `json:"interface"`
	Actor     string `json:"actor"`
}

// latestStartedBy is how rotari runs --json says project's newest run was
// started.
func latestStartedBy(t *testing.T, e *support.Env, project string) *startedBy {
	t.Helper()
	var runs []struct {
		StartedBy *startedBy `json:"started_by"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("runs", "-p", project, "--json").Stdout), &runs); err != nil || len(runs) == 0 {
		t.Fatalf("runs --json: %v %+v", err, runs)
	}
	return runs[0].StartedBy
}

// A run records the interface that started it, always, and an actor label
// only when the starter gave one: --actor, else ROTARI_ACTOR, for the CLI,
// and the MCP server's ROTARI_ACTOR for runs it starts. An invalid label is
// rejected before the run starts.
func TestRunRecordsHowItWasStarted(t *testing.T) {
	covers(t, "RUN-17")
	e := support.NewEnv(t)
	for _, test := range []struct {
		name, env string
		args      []string
		want      startedBy
	}{
		{"unlabelled", "", nil, startedBy{Interface: "cli"}},
		{"environment", "claude-code", nil, startedBy{Interface: "cli", Actor: "claude-code"}},
		{"option over environment", "claude-code", []string{"--actor", "codex"}, startedBy{Interface: "cli", Actor: "codex"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			e.Setenv("ROTARI_ACTOR", test.env)
			project := strings.ReplaceAll(test.name, " ", "-")
			e.MustRotari("add", "-p", project, "--", "true")
			e.MustRotari(append([]string{"run", "-p", project, "--quiet"}, test.args...)...)
			if got := latestStartedBy(t, e, project); got == nil || *got != test.want {
				t.Fatalf("started_by = %+v, want %+v", got, test.want)
			}
		})
	}

	e.Setenv("ROTARI_ACTOR", "")
	e.MustRotari("add", "-p", "invalid", "--", "true")
	if result := e.Rotari("run", "-p", "invalid", "--actor", "line\nbreak"); result.Code == 0 || !strings.Contains(result.Stderr, "control character") {
		t.Fatalf("invalid actor: exit %d, stderr %q", result.Code, result.Stderr)
	}

	e.Setenv("ROTARI_ACTOR", "claude-code")
	e.MustRotari("add", "-p", "agent", "--", "true")
	session := startMCP(t, e)
	target := map[string]any{"basedir_ref": baseDirRef(t, session, "agent"), "project": "agent"}
	var preview struct {
		Revision string `json:"revision"`
	}
	if message := session.call("rotari_preview_run", target, &preview); message != "" || preview.Revision == "" {
		t.Fatalf("preview: %q %+v", message, preview)
	}
	target["if_revision"] = preview.Revision
	var started struct {
		RunID string `json:"run_id"`
	}
	if message := session.call("rotari_start_run", target, &started); message != "" || started.RunID == "" {
		t.Fatalf("start: %q %+v", message, started)
	}
	summary := waitWithMCP(t, session, started.RunID, false)
	if summary.State != "finished" {
		t.Fatalf("MCP run: %+v", summary)
	}
	if got := latestStartedBy(t, e, "agent"); got == nil || *got != (startedBy{Interface: "mcp", Actor: "claude-code"}) {
		t.Fatalf("MCP started_by = %+v, want mcp and claude-code", got)
	}
	var described struct {
		Summary struct {
			Run struct {
				StartedBy *startedBy `json:"started_by"`
			} `json:"run"`
		} `json:"summary"`
	}
	if message := session.call("rotari_run_summary", map[string]any{"run_id": started.RunID}, &described); message != "" || described.Summary.Run.StartedBy == nil || described.Summary.Run.StartedBy.Actor != "claude-code" {
		t.Fatalf("MCP run summary: %q %+v", message, described)
	}
	for _, args := range [][]string{{"show", "-p", "agent", "-r", started.RunID}, {"lineage", "-p", "agent", started.RunID}} {
		if output := e.MustRotari(args...).Stdout; !strings.Contains(output, "Started by: mcp · claude-code") {
			t.Errorf("%s does not say how the run was started:\n%s", strings.Join(args, " "), output)
		}
	}
}
