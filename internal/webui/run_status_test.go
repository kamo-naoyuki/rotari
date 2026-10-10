package webui

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestWebRunDetachedClientLabels(t *testing.T) {
	for _, test := range []struct {
		name, mode, reason, label string
	}{
		{"async", model.RunClientModeAsync, model.RunClientReasonAsync, "detached (async)"},
		{"ctrl-d", model.RunClientModeSync, model.RunClientReasonCtrlD, "detached (Ctrl-D)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const runID = "20261009-120000-12345678"
			paths := writeWebDetachedRun(t, runID, test.mode, test.reason)
			handler := Handler(testOptions(paths.BaseDir, false))
			for _, endpoint := range []string{"/api/state", "/api/project?project_name=demo", "/api/run?project_name=demo&run_id=" + runID} {
				response := serveWebGet(t, handler, endpoint)
				if response.Code != http.StatusOK {
					t.Fatalf("GET %s = %d: %s", endpoint, response.Code, response.Body.String())
				}
				if body := response.Body.String(); !strings.Contains(body, `"client_label":"`+test.label+`"`) {
					t.Fatalf("GET %s = %s; want client_label %q", endpoint, body, test.label)
				}
			}
		})
	}
}

func writeWebDetachedRun(t *testing.T, runID, mode, reason string) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]any{
		paths.QueueFile: model.Queue{},
		paths.MetaFile:  model.Meta{Phase: "running", LastRunID: runID},
		paths.LockFile:  model.LockInfo{PID: os.Getpid(), RunID: runID, Host: host},
		filepath.Join(paths.RunsDir, runID, "commands.json"):               model.Queue{},
		filepath.Join(paths.RunsDir, runID, state.RunClientStatusFileName): model.RunClientStatus{Mode: mode, State: model.RunClientDetached, Reason: reason},
	} {
		if err := state.WriteJSON(path, value); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func TestWebRunStatusRuntime(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(htmlPath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const assert = require('node:assert/strict');
const { JSDOM, VirtualConsole } = require('jsdom');
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
let detail = null;
const requests = [];
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/',
  virtualConsole,
  beforeParse(window) {
    window.setInterval = () => 1;
    window.fetch = async url => {
      if (url === '/api/state') return {ok: true, json: async () => ({base_dir: '/state', projects: []})};
      requests.push(url);
      assert.equal(url, '/api/run?project_name=default&run_id=run-1');
      return {ok: detail !== null, json: async () => detail};
    };
  },
});
setTimeout(async () => {
  try {
    const window = dom.window;
    if (process.argv[2] === 'queue') {
      const runs = [
        {run_id: 'interrupted', status: 'running', lifecycle: 'interrupted', running: false, client_status: {state: 'detached', reason: 'disconnect'}},
        {run_id: 'incomplete', status: 'running', lifecycle: 'incomplete', running: false},
        {run_id: 'active', status: 'running', lifecycle: 'running', running: true, client_status: {state: 'attached'}},
        {run_id: 'finished', status: 'failed', lifecycle: 'finished', finished_at: 'done', exit_code: 7, client_label: 'server <label>', client_status: {state: 'completed', mode: 'sync'}},
        {run_id: 'legacy', status: 'failed', finished_at: 'done', exit_code: 3},
        {run_id: 'async', lifecycle: 'running', client_status: {state: 'detached', mode: 'async', reason: 'async'}},
        {run_id: 'ctrl-d', lifecycle: 'running', client_status: {state: 'detached', mode: 'sync', reason: 'ctrl-d'}},
        {run_id: 'server-async', lifecycle: 'running', client_label: 'detached (async)', client_status: {state: 'attached'}},
      ];
      window.renderQueue({project_name: 'default', queue: {commands: []}, runs});
      const rows = [...window.document.querySelectorAll('#app tbody tr')];
      assert.equal(rows.length, runs.length);
      const states = ['interrupted', 'incomplete', 'running', 'finished', 'failed', 'running', 'running', 'running'];
      const tones = ['warn', 'off', 'run', 'ok', 'bad', 'run', 'run', 'run'];
      rows.forEach((row, index) => {
        const pill = row.children[2].querySelector('span');
        assert.equal(pill.textContent, states[index] + (runs[index].running ? ' ...' : ''));
        assert(pill.classList.contains('status-pill') && pill.classList.contains('tone-' + tones[index]));
        if (index < 2) assert(!pill.classList.contains('tone-run'));
      });
      assert.equal(rows[0].children[3].textContent, 'detached (disconnect)');
      assert.equal(rows[1].children[3].textContent, 'unknown');
      assert.equal(rows[2].children[3].textContent, 'attached');
      assert.equal(rows[3].children[3].textContent, 'server <label>');
      assert.equal(rows[3].children[4].textContent, '7');
      assert.equal(rows[4].children[4].textContent, '3');
      assert.equal(rows[5].children[3].textContent, 'detached (async)');
      assert.equal(rows[6].children[3].textContent, 'detached (Ctrl-D)');
      assert.equal(rows[7].children[3].textContent, 'detached (async)');
      assert(window.document.getElementById('summary').textContent.includes('1 running'));
      window.renderRun({project_name: 'default', queue: {commands: []}, runs: [{...runs[3], jobs: []}]}, 'finished');
      const summary = window.document.getElementById('summary').textContent;
      assert(summary.includes('Status: failed'), 'summary outcome was lost');
      assert(summary.includes('Lifecycle: finished'));
      assert(summary.includes('Client: server <label>'));
      for (const [index, label] of [[5, 'detached (async)'], [6, 'detached (Ctrl-D)'], [7, 'detached (async)']]) {
        window.renderRun({project_name: 'default', queue: {commands: []}, runs: [{...runs[index], jobs: []}]}, runs[index].run_id);
        assert(window.document.getElementById('summary').textContent.includes('Client: ' + label));
      }
    } else {
      const index = {projects: [{project_name: 'default', runs: []}]};
      const active = new Set();
      for (const lifecycle of ['interrupted', 'incomplete']) {
        window.eval('runDetailCache.clear()');
        requests.length = 0;
        for (let version = 1; version <= 3; version++) {
          detail = {run_id: 'run-1', status: 'running', lifecycle, running: false, jobs: [{id: 'job-' + version}]};
          await window.refreshSelectedRun(index, 'default', 'run-1', active);
          assert.equal(requests.length, version, lifecycle + ' stopped polling');
          assert.equal(window.mergeWebIndex(index).projects[0].runs[0].jobs[0].id, 'job-' + version);
        }
        detail = null;
        await window.refreshSelectedRun(index, 'default', 'run-1', active);
        assert.equal(requests.length, 4, 'transient failure was not requested');
        assert.equal(window.mergeWebIndex(index).projects[0].runs[0].jobs[0].id, 'job-3', 'transient failure discarded cached details');
        detail = {run_id: 'run-1', status: 'finished', lifecycle: 'finished', running: false, jobs: [{id: 'final'}]};
        await window.refreshSelectedRun(index, 'default', 'run-1', active);
        assert.equal(requests.length, 5, 'did not recover after transient failure');
        assert.equal(window.mergeWebIndex(index).projects[0].runs[0].jobs[0].id, 'final');
        await window.refreshSelectedRun(index, 'default', 'run-1', active);
        assert.equal(requests.length, 5, 'finalized run was fetched again');
      }
      for (const status of ['finished', 'failed']) {
        window.eval('runDetailCache.clear()');
        requests.length = 0;
        detail = {run_id: 'run-1', status, running: false, jobs: []};
        await window.refreshSelectedRun(index, 'default', 'run-1', active);
        await window.refreshSelectedRun(index, 'default', 'run-1', active);
        assert.equal(requests.length, 1, 'legacy finalized run was fetched again');
      }
    }
    assert.deepEqual(errors, []);
  } catch (error) {
    console.error(error.stack || String(error));
    process.exitCode = 1;
  } finally {
    dom.window.close();
  }
}, 0);
`
	for _, name := range []string{"queue", "cache"} {
		t.Run(name, func(t *testing.T) {
			if output, err := exec.Command("node", "-e", script, htmlPath, name).CombinedOutput(); err != nil {
				t.Fatalf("run status runtime check failed: %v\n%s", err, output)
			}
		})
	}
}
