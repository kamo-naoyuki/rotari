package interfaces

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// mcpSession talks JSON-RPC to `rotari mcp` over its stdin and stdout.
type mcpSession struct {
	t      *testing.T
	stdin  io.WriteCloser
	stdout *bufio.Reader
	nextID int
}

func startMCP(t *testing.T, e *support.Env) *mcpSession {
	t.Helper()
	command := e.Command("mcp")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = command.Wait()
	})
	session := &mcpSession{t: t, stdin: stdin, stdout: bufio.NewReader(stdout)}
	session.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "conformance", "version": "1"}})
	session.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return session
}

func (session *mcpSession) send(message map[string]any) {
	session.t.Helper()
	data, err := json.Marshal(message)
	if err != nil {
		session.t.Fatal(err)
	}
	if _, err := session.stdin.Write(append(data, '\n')); err != nil {
		session.t.Fatal(err)
	}
}

func (session *mcpSession) request(method string, params map[string]any) json.RawMessage {
	session.t.Helper()
	session.nextID++
	session.send(map[string]any{"jsonrpc": "2.0", "id": session.nextID, "method": method, "params": params})
	for {
		line, err := session.stdout.ReadBytes('\n')
		if err != nil {
			session.t.Fatalf("rotari mcp: %v", err)
		}
		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(line, &reply); err != nil {
			session.t.Fatalf("rotari mcp reply %q: %v", line, err)
		}
		if reply.ID == session.nextID {
			if reply.Error != nil {
				session.t.Fatalf("rotari mcp %s: %s", method, reply.Error)
			}
			return reply.Result
		}
	}
}

// call calls tool and decodes its structured result into output, returning
// the error text of a tool error.
func (session *mcpSession) call(tool string, arguments map[string]any, output any) string {
	session.t.Helper()
	var result struct {
		IsError           bool            `json:"isError"`
		StructuredContent json.RawMessage `json:"structuredContent"`
		Content           []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(session.request("tools/call", map[string]any{"name": tool, "arguments": arguments}), &result); err != nil {
		session.t.Fatal(err)
	}
	if result.IsError {
		var texts []string
		for _, content := range result.Content {
			texts = append(texts, content.Text)
		}
		return strings.Join(texts, "\n")
	}
	if output != nil {
		if err := json.Unmarshal(result.StructuredContent, output); err != nil {
			session.t.Fatalf("%s result %s: %v", tool, result.StructuredContent, err)
		}
	}
	return ""
}

func baseDirRef(t *testing.T, session *mcpSession, project string) string {
	t.Helper()
	var listed struct {
		Projects []struct {
			BaseDirRef string `json:"basedir_ref"`
			Project    string `json:"project"`
		} `json:"projects"`
	}
	session.call("rotari_list_projects", map[string]any{}, &listed)
	for _, item := range listed.Projects {
		if item.Project == project {
			return item.BaseDirRef
		}
	}
	t.Fatalf("rotari_list_projects does not list %s: %+v", project, listed)
	return ""
}

type mcpRunSummary struct {
	State   string `json:"state"`
	Reason  string `json:"reason"`
	Summary struct {
		Counts struct {
			Jobs   int `json:"jobs"`
			Failed int `json:"failed"`
		} `json:"counts"`
	} `json:"summary"`
}

// waitWithMCP follows a run with rotari_run_summary right after it starts,
// and then with rotari_wait_run, as an agent without the CLI does.
func waitWithMCP(t *testing.T, session *mcpSession, runID string, untilFailure bool) mcpRunSummary {
	t.Helper()
	var summary mcpRunSummary
	if message := session.call("rotari_run_summary", map[string]any{"run_id": runID}, &summary); message != "" {
		t.Fatalf("rotari_run_summary of run %s: %s", runID, message)
	}
	arguments := map[string]any{"run_id": runID, "timeout_seconds": 30, "until_failure": untilFailure}
	if message := session.call("rotari_wait_run", arguments, &summary); message != "" || summary.Reason == "timeout" {
		t.Fatalf("rotari_wait_run of run %s: %q %+v", runID, message, summary)
	}
	return summary
}

func TestMCPWritesApplyOnlyAtThePreviewedRevision(t *testing.T) {
	covers(t, "MCP-1", "MCP-3")
	e := support.NewEnv(t)
	run := e.CreateFinishedRun()
	session := startMCP(t, e)
	ref := baseDirRef(t, session, run.Project)
	before := projectSnapshot(t, e)

	// A retry preview lists the failed job, changes nothing, and gives the
	// revision `check` reports.
	target := map[string]any{"basedir_ref": ref, "project": run.Project, "retry": true}
	var preview struct {
		Execute []struct {
			ID string `json:"id"`
		} `json:"execute"`
		Revision string `json:"revision"`
	}
	if message := session.call("rotari_preview_run", target, &preview); message != "" {
		t.Fatal(message)
	}
	if len(preview.Execute) != 1 || preview.Execute[0].ID != run.BadJob || preview.Revision != checkRevision(t, e) {
		t.Fatalf("retry preview = %+v, want only %s at the checked revision", preview, run.BadJob)
	}
	if projectSnapshot(t, e) != before {
		t.Fatal("rotari_preview_run changed the project")
	}

	stale := map[string]any{"basedir_ref": ref, "project": run.Project, "retry": true, "if_revision": "0000000000000000"}
	if message := session.call("rotari_start_run", stale, nil); !strings.Contains(message, "project changed since the planned revision") {
		t.Fatalf("start at a stale revision: %q", message)
	}
	if projectSnapshot(t, e) != before {
		t.Fatal("a refused rotari_start_run changed the project")
	}

	var started struct {
		RunID string `json:"run_id"`
	}
	apply := map[string]any{"basedir_ref": ref, "project": run.Project, "retry": true, "if_revision": preview.Revision}
	if message := session.call("rotari_start_run", apply, &started); message != "" || started.RunID == "" {
		t.Fatalf("start at the previewed revision: %q %+v", message, started)
	}
	if summary := waitWithMCP(t, session, started.RunID, false); summary.State != "finished" || summary.Reason != "settled" || summary.Summary.Counts.Jobs != 2 {
		t.Fatalf("summary of the started run: %+v", summary)
	}

	// An import preview gives a revision that the import then needs.
	manifest := `{"version":1,"jobs":[{"name":"imported","command":["true"]}]}`
	importInput := map[string]any{"basedir_ref": ref, "project": "fresh", "manifest": manifest, "format": "json"}
	var plan struct {
		Revision string `json:"revision"`
	}
	if message := session.call("rotari_preview_import", importInput, &plan); message != "" || plan.Revision == "" {
		t.Fatalf("import preview: %q %+v", message, plan)
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "fresh")); !os.IsNotExist(err) {
		t.Fatal("rotari_preview_import created the project")
	}
	importInput["if_revision"] = plan.Revision
	if message := session.call("rotari_import", importInput, &plan); message != "" {
		t.Fatalf("import at the previewed revision: %q", message)
	}
	if output := e.MustRotari("show", "-p", "fresh", "--queue").Stdout; !strings.Contains(output, "imported") {
		t.Fatalf("the imported queue does not hold the job:\n%s", output)
	}
	if message := session.call("rotari_import", importInput, nil); !strings.Contains(message, "project changed since the planned revision") {
		t.Fatalf("a second import at the old revision: %q", message)
	}
}

func TestMCPExportIsARedactedViewThatImportRefuses(t *testing.T) {
	covers(t, "MCP-2")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "secret", "--env", "API_TOKEN=s3cret", "--working-directory", e.Root, "--", "true")
	e.MustRotari("run", "-p", "secret", "--quiet")
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "secret", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	session := startMCP(t, e)
	var exported struct {
		BaseDirRef string `json:"basedir_ref"`
		Manifest   string `json:"manifest"`
	}
	if message := session.call("rotari_export_run", map[string]any{"run_id": shown.RunID}, &exported); message != "" {
		t.Fatal(message)
	}
	if strings.Contains(exported.Manifest, "s3cret") || strings.Contains(exported.Manifest, e.Root) || !strings.Contains(exported.Manifest, "API_TOKEN") {
		t.Fatalf("rotari_export_run is not a redacted view:\n%s", exported.Manifest)
	}
	full := e.MustRotari("export", shown.RunID).Stdout
	if !strings.Contains(full, "s3cret") {
		t.Fatalf("rotari export lost the environment value:\n%s", full)
	}
	importInput := map[string]any{"basedir_ref": exported.BaseDirRef, "project": "secret", "manifest": exported.Manifest, "overwrite": true}
	if message := session.call("rotari_preview_import", importInput, nil); !strings.Contains(message, "redacted placeholders") {
		t.Fatalf("import of the redacted view: %q", message)
	}
}

// TestMCPWaitReturnsOnTheFirstFinalFailure starts a run whose first job
// fails while the second still runs, and checks that rotari_wait_run with
// until_failure returns while the run goes on, as wait --until-failure does.
func TestMCPWaitReturnsOnTheFirstFinalFailure(t *testing.T) {
	covers(t, "MCP-3")
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "p1", "--job-name", "bad", "--", "sh", "-c", "exit 3")
	e.MustRotari("add", "-p", "p1", "--job-name", "slow", "--", "sleep", "30")
	session := startMCP(t, e)
	target := map[string]any{"basedir_ref": baseDirRef(t, session, "p1"), "project": "p1"}
	var preview struct {
		Revision string `json:"revision"`
	}
	if message := session.call("rotari_preview_run", target, &preview); message != "" {
		t.Fatal(message)
	}
	target["if_revision"] = preview.Revision
	var started struct {
		RunID string `json:"run_id"`
	}
	if message := session.call("rotari_start_run", target, &started); message != "" {
		t.Fatal(message)
	}
	t.Cleanup(func() {
		e.Rotari("cancel", "-p", "p1")
		e.Rotari("wait", "-p", "p1", "-r", started.RunID, "--timeout", "30s")
	})
	summary := waitWithMCP(t, session, started.RunID, true)
	if summary.Reason != "failure" || summary.State != "running" || summary.Summary.Counts.Failed != 1 {
		t.Fatalf("wait until failure = %+v, want the failure while the run goes on", summary)
	}
}

// TestMCPJobControlActsOnlyOnThePreviewedRunningRun previews job control on
// a running run, suspends, resumes, and cancels one job, cancels the rest of
// the run, and checks that a cancel naming the run fails once it has ended.
func TestMCPJobControlActsOnlyOnThePreviewedRunningRun(t *testing.T) {
	covers(t, "MCP-4")
	e := support.NewEnv(t)
	run := e.StartRun("live", 2, true)
	t.Cleanup(func() { e.Rotari("cancel", "-p", run.Project, "--wait") })
	session := startMCP(t, e)
	first, second := run.Jobs[0], run.Jobs[1]

	var preview struct {
		RunID  string   `json:"run_id"`
		JobIDs []string `json:"job_ids"`
	}
	if message := session.call("rotari_preview_job_control", map[string]any{"run_id": run.RunID, "operation": "cancel"}, &preview); message != "" {
		t.Fatal(message)
	}
	want := append([]string(nil), run.Jobs...)
	sort.Strings(want)
	if preview.RunID != run.RunID || strings.Join(preview.JobIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("cancel preview = %+v, want %v", preview, want)
	}
	if message := session.call("rotari_preview_job_control", map[string]any{"run_id": run.RunID, "operation": "stop"}, nil); !strings.Contains(message, "cancel, suspend, or resume") {
		t.Fatalf("unknown operation: %q", message)
	}

	one := map[string]any{"run_id": run.RunID, "job_ids": []string{first}}
	for _, operation := range []string{"rotari_suspend", "rotari_resume", "rotari_cancel"} {
		var output struct {
			Message string `json:"message"`
		}
		if message := session.call(operation, one, &output); message != "" || !strings.Contains(output.Message, "Jobs: 1") {
			t.Fatalf("%s of %s: %q %+v", operation, first, message, output)
		}
	}
	support.WaitUntil(t, 30*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, first) == 0, "the cancelled job is still running"
	})
	if support.JobProcesses(t, e.Root, second) == 0 {
		t.Fatalf("job %s stopped too; only %s was cancelled", second, first)
	}

	if message := session.call("rotari_cancel", map[string]any{"run_id": run.RunID}, nil); message != "" {
		t.Fatal(message)
	}
	if summary := waitWithMCP(t, session, run.RunID, false); summary.State != "finished" {
		t.Fatalf("the cancelled run: %+v", summary)
	}
	if message := session.call("rotari_cancel", map[string]any{"run_id": run.RunID}, nil); !strings.Contains(message, "is not running") {
		t.Fatalf("cancel of an ended run: %q", message)
	}
}

// TestMCPResetOnlyClearsTheQueue interrupts a run, then previews and applies
// a reset through MCP: it clears the next queue but leaves the interrupted run
// and its history unchanged, and the removed recovery argument names unlock.
func TestMCPResetOnlyClearsTheQueue(t *testing.T) {
	covers(t, "MCP-5")
	e := support.NewEnv(t)
	run := e.StartRun("live", 1, false)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, "") == 1, "the job did not start"
	})
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	session := startMCP(t, e)
	target := map[string]any{"basedir_ref": baseDirRef(t, session, "live"), "project": "live"}

	var preview struct {
		Cleared  int    `json:"cleared"`
		Revision string `json:"revision"`
	}
	e.MustRotari("add", "-p", "live", "--", "true")
	if message := session.call("rotari_preview_reset", target, &preview); message != "" || preview.Cleared != 1 || preview.Revision == "" {
		t.Fatalf("reset preview: %q %+v", message, preview)
	}
	if state := e.CheckState("live"); state != "interrupted" {
		t.Fatalf("after the preview: state %q", state)
	}

	apply := map[string]any{"basedir_ref": target["basedir_ref"], "project": "live", "if_revision": "0000000000000000"}
	if message := session.call("rotari_reset", apply, nil); !strings.Contains(message, "project changed since the planned revision") {
		t.Fatalf("reset at a stale revision: %q", message)
	}
	if state := e.CheckState("live"); state != "interrupted" {
		t.Fatalf("after stale reset: state %q", state)
	}
	apply["if_revision"] = preview.Revision
	apply["recover_interrupted"] = true
	if message := session.call("rotari_reset", apply, nil); !strings.Contains(message, "unlock") {
		t.Fatalf("removed recovery argument: %q", message)
	}
	delete(apply, "recover_interrupted")
	if message := session.call("rotari_reset", apply, nil); message != "" {
		t.Fatal(message)
	}
	if state := e.CheckState("live"); state != "interrupted" {
		t.Fatalf("after the reset: state %q", state)
	}
	if r := e.Rotari("check", "live"); !strings.Contains(r.Stdout, "queued=0") {
		t.Fatalf("after the reset queue was not empty: %s", r)
	}
	if _, err := os.Stat(filepath.Join(e.Base, "projects", "live", "runs", run.RunID)); err != nil {
		t.Errorf("the reset removed run history: %v", err)
	}
}

// TestMCPUnlockRecoversAnInterruptedRun interrupts a run, then previews and
// applies an unlock through MCP: the preview names the run and changes
// nothing, an unlock at a stale revision is refused, and the unlock recovers
// the run and keeps the queue.
func TestMCPUnlockRecoversAnInterruptedRun(t *testing.T) {
	covers(t, "MCP-7", "SAFE-7")
	e := support.NewEnv(t)
	run := e.StartRun("live", 1, false)
	support.WaitUntil(t, 15*time.Second, func() (bool, string) {
		return support.JobProcesses(t, e.Root, "") == 1, "the job did not start"
	})
	support.KillStrays(t, e.Root)
	support.WaitForInterrupted(t, e, "live")
	e.MustRotari("add", "-p", "live", "--", "true")
	session := startMCP(t, e)
	target := map[string]any{"basedir_ref": baseDirRef(t, session, "live"), "project": "live"}

	var preview struct {
		InterruptedRunID string `json:"interrupted_run_id"`
		JobsMayBeRunning bool   `json:"jobs_may_be_running"`
		Revision         string `json:"revision"`
	}
	if message := session.call("rotari_preview_unlock", target, &preview); message != "" || preview.InterruptedRunID != run.RunID || !preview.JobsMayBeRunning || preview.Revision == "" {
		t.Fatalf("unlock preview: %q %+v", message, preview)
	}
	if state := e.CheckState("live"); state != "interrupted" {
		t.Fatalf("after the preview: state %q", state)
	}

	apply := map[string]any{"basedir_ref": target["basedir_ref"], "project": "live", "if_revision": "0000000000000000"}
	if message := session.call("rotari_unlock", apply, nil); !strings.Contains(message, "project changed since the planned revision") {
		t.Fatalf("unlock at a stale revision: %q", message)
	}
	if state := e.CheckState("live"); state != "interrupted" {
		t.Fatalf("after the refused unlock: state %q", state)
	}
	apply["if_revision"] = preview.Revision
	if message := session.call("rotari_unlock", apply, nil); message != "" {
		t.Fatal(message)
	}
	if r := e.Rotari("check", "live"); !strings.Contains(r.Stdout, "state=ready") || !strings.Contains(r.Stdout, "queued=1") {
		t.Fatalf("after the unlock: %s", r)
	}
}
