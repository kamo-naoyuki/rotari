package interfaces

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestMCPWritesApplyOnlyAtThePreviewedRevision(t *testing.T) {
	covers(t, "MCP-1")
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
	e.Rotari("wait", "-p", run.Project, "-r", started.RunID, "--timeout", "30s")
	var summary struct {
		Summary struct {
			Counts struct {
				Jobs int `json:"jobs"`
			} `json:"counts"`
		} `json:"summary"`
	}
	if message := session.call("rotari_run_summary", map[string]any{"run_id": started.RunID}, &summary); message != "" || summary.Summary.Counts.Jobs != 2 {
		t.Fatalf("summary of the started run: %q %+v", message, summary)
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
