package webui

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

func testStore() stateinternal.Store {
	return stateinternal.NewStore(0o755, 0o644)
}

// envRunID is the variable testOptions documents.
const envRunID = "ROTARI_RUN_ID"

// testOptions serves baseDir with the built-in executors and no CLI
// metadata.
func testOptions(baseDir string, allowControl bool) Options {
	store := testStore()
	executors := executor.NewRegistry(store, func(string, ...any) {})
	return Options{
		BaseDir: baseDir, RootBaseDir: baseDir, AllowControl: allowControl, Notifications: true,
		Store:      store,
		Editor:     queueops.Editor{Store: store, Executors: executors, NewJobID: func() string { return "new-job" }, UnregisterRun: func(string) error { return nil }},
		Controller: jobcontrol.Controller{Store: store, Executors: executors},
		Executors:  executors.Names(),
		Environments: []webprojection.EnvironmentDefinition{
			{Name: envRunID, CLIDefault: true, Job: true, Array: true, Description: "Current run ID; --run-id default."},
		},
		// A stand-in for the CLI's TOML template.
		ConfigTemplate: func() ([]byte, error) { return []byte("[run]\n"), nil },
	}
}

// testRunner is a run lifecycle wired as the CLI wires it for config files.
func testRunner() projectrun.Runner {
	store := testStore()
	return projectrun.Runner{
		Store: store, Executors: executor.NewRegistry(store, nil),
		ConfigPaths: func(paths stateinternal.ProjectPaths) []string {
			return config.PathsForRun(paths.BaseDir, paths.ProjectName)
		},
	}
}

func writeRunContext(paths stateinternal.ProjectPaths, runID, cwd string) error {
	return testRunner().WriteContext(paths, runID, cwd)
}

func finishRunContext(paths stateinternal.ProjectPaths, runID string) error {
	return testRunner().FinishContext(paths, runID)
}

func writeSchedulerStatus(jobDir, state string) {
	executor.WriteSchedulerStatus(testStore(), jobDir, state, time.Now())
}

func siteFor(baseDir string) site {
	return site{Options: testOptions(baseDir, true), notificationSession: newNotificationSession()}
}

func testSite() site {
	return siteFor("")
}

func compactWebHTML(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, value)
	value = strings.ReplaceAll(value, `"`, `'`)
	// Prettier adds trailing commas before closing brackets when it wraps a
	// call/array/object onto multiple lines; strip them so marker literals
	// don't depend on incidental line-wrapping.
	for _, closer := range []string{")", "]", "}"} {
		value = strings.ReplaceAll(value, ","+closer, closer)
	}
	return value
}

func webContains(html, marker string) bool {
	return strings.Contains(compactWebHTML(html), compactWebHTML(marker))
}

func TestWebHTMLJavaScriptSyntax(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	path := filepath.Join(t.TempDir(), "web.js")
	script := testSite().webHTML()
	start := strings.Index(script, "<script>")
	end := strings.LastIndex(script, "</script>")
	if start < 0 || end < start {
		t.Fatal("web HTML does not contain a script")
	}
	if err := os.WriteFile(path, []byte(script[start+len("<script>"):end]), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", "--check", path).CombinedOutput(); err != nil {
		t.Fatalf("web JavaScript syntax check failed: %v\n%s", err, output)
	}
}

func TestWebHTMLRendersState(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const state = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(html, {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/',
  virtualConsole,
  beforeParse(window) {
    window.fetch = async () => ({ok: true, json: async () => state});
    window.setInterval = () => 1;
  },
});
setTimeout(() => {
  const app = dom.window.document.getElementById('app');
  if (errors.length) {
    console.error(errors.join('\n'));
    process.exit(1);
  }
  if (!app || app.textContent.includes('loading...')) process.exit(2);
	const viewConfig = dom.window.document.querySelector('.config-button');
	const generateConfig = dom.window.document.querySelector('.generate-config-button');
	if (!viewConfig || !viewConfig.disabled || !generateConfig) process.exit(3);
	const modal = dom.window.document.getElementById('output-modal');
	modal.dataset.view = 'generate-config';
	modal.querySelector('strong').textContent = 'Generate config';
	dom.window.styleActionColumns();
	if (modal.querySelector('strong').textContent !== 'Generate config') process.exit(4);
	modal.dataset.view = 'config';
	modal.dataset.editing = 'true';
	dom.window.openOutputModal(false);
	if (!dom.window.document.getElementById('copy-modal').hidden) process.exit(5);
	const editor = dom.window.document.getElementById('config-editor');
	const textarea = editor.querySelector('textarea');
	const save = editor.querySelector('button');
	textarea.dataset.initial = 'original';
	textarea.value = 'original';
	dom.window.updateConfigSaveState(editor);
	if (!save.disabled || textarea.classList.contains('dirty')) process.exit(6);
	textarea.value = 'changed';
	dom.window.updateConfigSaveState(editor);
	if (save.disabled || !textarea.classList.contains('dirty')) process.exit(7);
	dom.window.showPath('/work/job');
	if (!editor.hidden || !dom.window.document.getElementById('config-generator').hidden) process.exit(8);
	if (dom.window.document.getElementById('modal-log').hidden) process.exit(9);
	if (modal.querySelector('strong').textContent !== 'Job path') process.exit(10);
}, 50);
`
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(htmlPath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	state, err := siteFor(baseDir).loadWebState(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	state.ConfigPath = ""
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", "-e", script, htmlPath, statePath).CombinedOutput(); err != nil {
		t.Fatalf("web runtime check failed: %v\n%s", err, output)
	}
}

func TestWebHTMLRendersUnreadableRun(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "20260927-000000-00000000")
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "job", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "summary.json"), []byte(`{"state_version": 99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := siteFor(baseDir).loadWebState(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(htmlPath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const state = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
for (const [path, want] of [['/project/default', 'unreadable'], ['/project/default/run/20260927-000000-00000000', 'upgrade rotari']]) {
  const dom = new JSDOM(html, {
    runScripts: 'dangerously',
    url: 'http://127.0.0.1' + path,
    virtualConsole,
    beforeParse(window) {
      window.fetch = async () => ({ok: true, json: async () => state});
      window.setInterval = () => 1;
    },
  });
  setTimeout(() => {
    if (errors.length) {
      console.error(errors.join('\n'));
      process.exit(1);
    }
    const app = dom.window.document.getElementById('app').textContent;
    if (!app.includes(want)) {
      console.error(path + ' does not show ' + want + ': ' + app);
      process.exit(2);
    }
  }, 50);
}
`
	if output, err := exec.Command("node", "-e", script, htmlPath, statePath).CombinedOutput(); err != nil {
		t.Fatalf("web runtime check failed: %v\n%s", err, output)
	}
}

func TestWebShowDiagnosisRendersAnalysisStatus(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(html, {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/',
  virtualConsole,
  beforeParse(window) {
    window.fetch = async () => ({ok: true, json: async () => ({project_name: 'demo', queue: {commands: []}, runs: []})});
    window.setInterval = () => 1;
  },
});
setTimeout(() => {
  const render = analysis => {
    const trigger = dom.window.document.createElement('button');
    trigger.dataset.diagnoses = JSON.stringify(analysis);
    dom.window.showDiagnosis(trigger);
    return dom.window.document.getElementById('output-modal').textContent;
  };
  const checks = [
    [{status: 'matched', diagnoses: [{name: 'Rule', evidence: 'line', suggestion: 'fix'}]}, ['Rule', 'Evidence: line', 'Next: fix'], ['earlier diagnosis rules']],
    [{status: 'no_match', outdated: true, diagnoses: []}, ['No known rule matched.', 'earlier diagnosis rules'], ['Evidence:']],
    [{status: 'unavailable', note: 'read failed', diagnoses: []}, ['Unavailable: read failed'], ['earlier diagnosis rules']],
  ];
  for (const [analysis, wanted, unwanted] of checks) {
    const text = render(analysis);
    for (const want of wanted) if (!text.includes(want)) { console.error(JSON.stringify(analysis), 'missing', want, text); process.exit(2); }
    for (const want of unwanted) if (text.includes(want)) { console.error(JSON.stringify(analysis), 'unexpected', want, text); process.exit(3); }
  }
  if (errors.length) {
    console.error(errors.join('\n'));
    process.exit(1);
  }
}, 50);
`
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(htmlPath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("web diagnosis check failed: %v\n%s", err, output)
	}
}

func TestStaticWebUsesGenerateConfigReadOnlyFlow(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "config.toml"), []byte("name = \"static demo\"\n"), stateinternal.FileMode()); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "web")
	if err := siteFor(baseDir).generateStaticWeb(outputDir); err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(html, {
	runScripts: 'dangerously',
	url: 'https://example.test/',
	virtualConsole,
	beforeParse(window) {
		window.Response = class Response {
			constructor(body, init = {}) {
				this.body = body;
				this.status = init.status || 200;
				this.ok = this.status >= 200 && this.status < 300;
			}
			json() { return Promise.resolve(JSON.parse(this.body)); }
			text() { return Promise.resolve(this.body); }
		};
		window.Notification = {
			permission: 'default',
			requestPermission: async () => 'default',
		};
		window.setInterval = () => 1;
	},
});
setTimeout(() => {
	const readOnlyMessage = 'the web UI is read-only; restart with --allow-control to enable job control';
	if (errors.length) {
		console.error(errors.join('\n'));
		process.exit(1);
	}
	const button = dom.window.document.querySelector('.generate-config-button');
	if (!button) process.exit(2);
	const notify = dom.window.document.getElementById('notify-toggle');
	if (!notify || notify.textContent !== 'Notification off' || notify.disabled) process.exit(8);
	const view = dom.window.document.querySelector('.config-button');
	if (!view || view.disabled) process.exit(3);
	dom.window.confirm = () => true;
	let alertText = '';
	dom.window.alert = text => { alertText = String(text); };
	view.click();
	setTimeout(() => {
		const editor = dom.window.document.getElementById('config-editor');
		const textarea = editor.querySelector('textarea');
		if (editor.hidden || textarea.value !== 'name = "static demo"\n') process.exit(4);
		textarea.value = 'name = "changed"\n';
		textarea.dispatchEvent(new dom.window.Event('input'));
		editor.querySelector('button').click();
		setTimeout(() => {
			if (alertText !== readOnlyMessage + '\n') process.exit(5);
			alertText = '';
			button.click();
			setTimeout(() => {
				const target = dom.window.document.querySelector('.config-target-options button');
				if (!target) process.exit(6);
				target.click();
				setTimeout(() => {
					if (alertText !== readOnlyMessage + '\n') process.exit(7);
					if (errors.length) {
						console.error(errors.join('\n'));
						process.exit(1);
					}
				}, 0);
			}, 0);
		}, 0);
	}, 0);
}, 50);
`
	if output, err := exec.Command("node", "-e", script, filepath.Join(outputDir, "index.html")).CombinedOutput(); err != nil {
		t.Fatalf("static web config flow check failed: %v\n%s", err, output)
	}
}

func TestWebRunAttemptSelectionUpdatesDisplayedJob(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const state = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(html, {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/project/default/run/run-1',
  virtualConsole,
  beforeParse(window) {
    window.fetch = async (url) => {
      if (url === '/api/state') return {ok: true, json: async () => state};
			if (url.startsWith('/api/log?')) return {ok: true, text: async () => 'old-attempt-log'};
      throw new Error('unexpected fetch: ' + url);
    };
    window.setInterval = () => 1;
  },
});
setTimeout(async () => {
  const row = () => dom.window.document.querySelector('tr[data-job-id="job-1"]');
  const assert = (condition, message) => { if (!condition) throw new Error(message); };
  try {
    assert(row(), 'job row was not rendered');
	const headers = [...row().closest('table').querySelectorAll('thead th')];
	const identityIndexes = ['name', 'id', 'attempt'].map(key => headers.findIndex(header => header.dataset.sort === key));
	assert(identityIndexes[0] >= 0 && identityIndexes[1] === identityIndexes[0] + 1 && identityIndexes[2] === identityIndexes[1] + 1, 'job identity columns are not separated');
	assert(row().children.length === headers.length, 'job row does not match separated identity columns');
	assert(row().querySelector('[title="Copy job name"]'), 'job name copy button is missing');
	assert(row().querySelector('[title="Copy job ID"]'), 'job ID copy button is missing');
	assert(row().querySelector('[title="Copy attempt ID"]'), 'attempt ID copy button is missing');
	assert(row().querySelector('[title="Copy command"]'), 'command copy button is missing');
	assert(row().querySelectorAll('.table-copy').length >= 4, 'table copy buttons are not styled as table controls');
    assert(row().textContent.includes('attempt-1'), 'latest attempt is not displayed');
    const statusPill = () => row().querySelector('.status-value');
    assert(statusPill().tagName === 'SPAN' && statusPill().classList.contains('status-failed'), 'failed job status is not rendered as a pill');
    dom.window.setAttemptMenuOpen('default', 'run-1', 'job-1', true);
    dom.window.render();
    assert(row().querySelector('.attempt-menu').open, 'attempt menu did not stay open after render');
	dom.window.document.body.dispatchEvent(new dom.window.PointerEvent('pointerdown', {bubbles: true}));
	assert(row().querySelector('.attempt-menu').open === false, 'attempt menu did not close on outside click');
	dom.window.setAttemptMenuOpen('default', 'run-1', 'job-1', true);
	dom.window.render();
    dom.window.selectJobAttempt('default', 'run-1', 'job-1', 'attempt-0');
    assert(row().textContent.includes('attempt-0'), 'selected attempt is not displayed');
    assert(row().textContent.includes('0'), 'selected attempt result is not displayed');
    assert(row().textContent.includes('old-start'), 'selected attempt timestamp is not displayed');
    assert(statusPill().classList.contains('status-finished'), 'successful job status is not rendered as a pill');
    assert(row().querySelector('.attempt-menu').open === false, 'attempt menu did not close after selection');
	const logButton = row().querySelector('button[onclick*="attempt-0"]');
	assert(logButton, 'log button does not target selected attempt');
	logButton.click();
	await new Promise(resolve => setTimeout(resolve, 0));
	assert(dom.window.document.getElementById('modal-log').textContent === 'old-attempt-log', 'selected attempt log was not loaded');
    if (errors.length) throw new Error(errors.join('\n'));
  } catch (error) {
    console.error(error.stack || String(error));
    process.exit(1);
  }
}, 50);
`
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(htmlPath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	state := webprojection.State{Queues: []webprojection.QueueState{{
		QueueName: "default",
		Runs: []webprojection.Run{{
			RunSummary: model.RunSummary{RunID: "run-1", Status: "finished"},
			Jobs: []webprojection.Job{{
				ID: "job-1", Name: "train", Command: []string{"true"}, AttemptID: "attempt-1",
				Result: &model.JobResult{ID: "job-1", AttemptID: "attempt-1", ExitCode: 1},
				Attempts: []webprojection.Attempt{
					{ID: "attempt-1", Result: &model.JobResult{ID: "job-1", AttemptID: "attempt-1", ExitCode: 1}, SubmittedAt: "latest-start"},
					{ID: "attempt-0", Result: &model.JobResult{ID: "job-1", AttemptID: "attempt-0", ExitCode: 0}, SubmittedAt: "old-start"},
				},
			}},
		}},
	}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", "-e", script, htmlPath, statePath).CombinedOutput(); err != nil {
		t.Fatalf("attempt selection runtime check failed: %v\n%s", err, output)
	}
}

func TestRunBulkControlsOperateOnSelectedJobs(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	pagePath := filepath.Join(t.TempDir(), "index.html")
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(pagePath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	state := webprojection.State{Queues: []webprojection.QueueState{{
		QueueName: "demo",
		Runs: []webprojection.Run{{
			RunSummary: model.RunSummary{RunID: "run-1", Status: "running"},
			Running:    true,
			Jobs: []webprojection.Job{
				{ID: "running-1", Name: "active", SchedulerState: "running"},
				{ID: "suspended-1", Name: "paused", SchedulerState: "suspended"},
				{ID: "pending-1", Name: "waiting", SchedulerState: "pending"},
			},
		}},
	}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const state = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const rootID = process.argv[4];
const requests = [];
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(html, {
	runScripts: 'dangerously',
	url: 'http://127.0.0.1/project/demo/run/run-1',
	virtualConsole,
	beforeParse(window) {
		window.fetch = async (url, options = {}) => {
			if (url === '/api/state') return {ok: true, json: async () => state};
			requests.push({url, body: JSON.parse(options.body)});
			return {ok: true, text: async () => '{"message":"ok"}'};
		};
		window.confirm = () => true;
		window.alert = message => { throw new Error(String(message)); };
		window.setInterval = () => 1;
	},
});
setTimeout(async () => {
	const assert = (condition, message) => { if (!condition) throw new Error(message); };
	const button = selector => dom.window.document.querySelector(selector);
	const select = id => {
		const input = dom.window.document.querySelector('tr[data-job-id="' + id + '"] .job-selection');
		input.checked = true;
		input.dispatchEvent(new dom.window.Event('change', {bubbles: true}));
	};
	try {
		const controls = [...dom.window.document.querySelectorAll('.web-copy-controls button')];
		const labels = controls.map(control => control.textContent.trim());
		const positions = ['Report', 'Cancel selected', 'Suspend selected', 'Delete run'].map(label => labels.indexOf(label));
		assert(positions.every(index => index >= 0) && positions.every((index, i) => i === 0 || positions[i - 1] < index), 'bulk job actions are not between Report and Delete run: ' + labels.join(' | '));
		assert(button('.cancel-selected-jobs').disabled && button('.suspend-resume-selected-jobs').disabled, 'control buttons should start disabled');
		select('suspended-1');
		assert(button('.cancel-selected-jobs').disabled, 'Cancel should stay disabled without a selected running job');
		assert(!button('.suspend-resume-selected-jobs').disabled && button('.suspend-resume-selected-jobs').textContent === 'Resume selected', 'toggle should switch to Resume for a selected suspended job');
		await dom.window.controlSelectedRunJobs('suspend-resume');
		assert(requests[0].url === '/api/resume-job' && requests[0].body.job_ids.join() === 'suspended-1', 'Resume did not target the checked suspended job');
		select('running-1');
		assert(!button('.cancel-selected-jobs').disabled && !button('.suspend-resume-selected-jobs').disabled && button('.suspend-resume-selected-jobs').textContent === 'Suspend selected', 'toggle should switch to Suspend when any checked job is running');
		await dom.window.controlSelectedRunJobs('suspend-resume');
		assert(requests[1].url === '/api/suspend-job' && requests[1].body.job_ids.join() === 'running-1', 'Suspend did not target only the checked running job');
		await dom.window.controlSelectedRunJobs('cancel');
		assert(requests[2].url === '/api/cancel-job' && requests[2].body.job_ids.join() === 'running-1,suspended-1', 'Cancel did not target checked unfinished jobs');
		assert(!dom.window.document.querySelector('tr[data-job-id="running-1"] .cancel-job'), 'per-row Cancel action should be removed');
		if (errors.length) throw new Error(errors.join('\n'));
	} catch (error) {
		console.error(error.stack || String(error));
		process.exit(1);
	}
}, 50);
`
	if output, err := exec.Command("node", "-e", script, pagePath, statePath).CombinedOutput(); err != nil {
		t.Fatalf("bulk run controls check failed: %v\n%s", err, output)
	}
}

func TestWebRunGuidanceUsesRunIDOnly(t *testing.T) {
	html := testSite().webHTML()
	if !webContains(html, "rotari run'+basedir+' --project-name '+shellQuote(queueName)") {
		t.Fatal("web queue guidance does not use the project-name option")
	}
	if !webContains(html, "rotari retry -r '+shellQuote(runID)") {
		t.Fatal("web run guidance does not contain a run-id-only retry command")
	}
	if webContains(html, "rotari retry'+basedir+' --queue-name '+shellQuote(queueName)") {
		t.Fatal("web run guidance still contains basedir and project name")
	}
	if !webContains(html, "Cancel run") || !webContains(html, "/api/cancel-run") {
		t.Fatal("web run page does not contain run cancellation controls")
	}
	for _, marker := range []string{"command-guide-copy", "command-guide-copied", "copyCommandGuide", "Copy command", "Copied!"} {
		if !webContains(html, marker) {
			t.Fatalf("web command guidance is missing copy control %q", marker)
		}
	}
	for _, marker := range []string{"select-all-jobs", "job-selection", "copySelectedJobs", "Select failed", "selectFailedJobs", "Select failed + unfinished", "Unselect all", `querySelectorAll(".unselect-all")`, "unselectAll.className = \"unselect-all\"", "unselectAll.disabled = true", ">Create</button>", ">Append</button>", "Cancel selected", "Suspend selected", "Resume selected", "suspend-resume-selected-jobs", "controlSelectedRunJobs", "job_ids:targets.map"} {
		if !webContains(html, marker) {
			t.Fatalf("web run page is missing job queue selection control %q", marker)
		}
	}
	for _, marker := range []string{"selectedRunJobsByRun", "function restoreSelectedRunJobs()", "restoreSelectedRunJobs();"} {
		if !webContains(html, marker) {
			t.Fatalf("web run page does not preserve job selection across refreshes: %q", marker)
		}
	}
}

func TestRunSelectionCheckboxesUseCompactDimensions(t *testing.T) {
	for _, marker := range []string{
		".job-selection,\n#select-all-jobs {",
		"width: 16px;",
		"height: 16px;",
		"min-width: 0;",
		"min-height: 0;",
		"padding: 0;",
	} {
		if !strings.Contains(webStylesCSS, marker) {
			t.Fatalf("run selection checkbox styling is missing %q", marker)
		}
	}
}

func TestRunToolbarButtonsUseConsistentMinimumWidth(t *testing.T) {
	if !strings.Contains(webStylesCSS, ".web-copy-controls > button {\n  min-width: 100px;\n}") {
		t.Fatal("run toolbar buttons do not share a minimum width")
	}
}

func TestWebRunPageCopiesConfigPathsAndRunID(t *testing.T) {
	html := testSite().webHTML()
	for _, marker := range []string{"function setLocation(base, paths)", `copyIconForValue(path, "config path")`, `copyIconForValue(run.run_id, "run ID")`} {
		if !webContains(html, marker) {
			t.Fatalf("web run page is missing identity copy control %q", marker)
		}
	}
	if strings.Count(html, `copyIconForValue(run.run_id, "run ID")`) != 1 {
		t.Fatal("web run page has duplicate run ID copy controls")
	}
	if webContains(html, "run.context.config_paths") {
		t.Fatal("web run page displays source config paths instead of snapshots")
	}
}

func TestWebProjectAndOverviewPagesCopyConfigPaths(t *testing.T) {
	html := testSite().webHTML()
	for _, marker := range []string{
		`setLocation(state.base_dir + " / all projects", state.config_path ? [state.config_path] : [])`,
		`setLocation(state.base_dir + " / " + q.project_name, q.config_path ? [q.config_path] : [])`,
	} {
		if !webContains(html, marker) {
			t.Fatalf("web page is missing config path copy control setup %q", marker)
		}
	}
}

func TestWebHTMLContainsFinalProjectHooks(t *testing.T) {
	html := testSite().webHTML()
	for _, marker := range []string{"function rowCell(row,key)", "function copySelectedJobs(queue,run,append)", "function arrangeRunControls()", "function orderJobActions()"} {
		if !webContains(html, marker) {
			t.Fatalf("web HTML is missing required generated hook %q", marker)
		}
	}
	for _, obsolete := range []string{"queue_name", "/queue/", "state.queues"} {
		if webContains(html, obsolete) {
			t.Fatalf("web HTML contains obsolete project identifier %q", obsolete)
		}
	}
}

func TestWebIndexTemplateUsesProjectVocabulary(t *testing.T) {
	template := webTemplateHTML + strings.Join([]string{webAppCoreJS, webAppActionsJS, webAppLogsJS, webAppTablesJS, webAppChartsJS, webAppBootstrapJS}, "\n")
	for _, obsolete := range []string{"queue_name", "/queue/", "state.queues"} {
		if strings.Contains(template, obsolete) {
			t.Fatalf("project web template contains obsolete identifier %q", obsolete)
		}
	}
	for _, required := range []string{"project_name", "/project/", "state.projects"} {
		if !strings.Contains(template, required) {
			t.Fatalf("project web template is missing %q", required)
		}
	}
	if !strings.Contains(webTemplateHTML, `href="/web_styles.css"`) {
		t.Fatal("web template does not load the stylesheet from the server root")
	}
	if !strings.Contains(webTemplateHTML, `href="/web_sidebar_styles.css"`) {
		t.Fatal("web template does not load the shared sidebar stylesheet")
	}
}

func TestWebSidebarStylesAreSharedWithJobsPage(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/web_sidebar_styles.css", nil)
	response := httptest.NewRecorder()
	Handler(testOptions(t.TempDir(), false)).ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Fatalf("GET /web_sidebar_styles.css = %d (%q), want CSS response", response.Code, response.Header().Get("Content-Type"))
	}
	if response.Body.String() != webSidebarStylesCSS {
		t.Fatal("sidebar stylesheet route does not serve the shared stylesheet")
	}
	for _, selector := range []string{".sidebar {", ".sidebar-brand {", ".sidebar-project-row {"} {
		if strings.Contains(webStylesCSS, selector) || strings.Contains(webInfoStylesCSS, selector) {
			t.Fatalf("page-specific stylesheets duplicate shared sidebar selector %q", selector)
		}
	}
	jobsHTML := jobsHTML("/", []string{"demo"}, nil, joblist.DefaultSinceText, true, true)
	for _, marker := range []string{".sidebar-brand {", ".sidebar-project-row {", ".sidebar-link.active {"} {
		if !strings.Contains(jobsHTML, marker) || !strings.Contains(webSidebarStylesCSS, marker) {
			t.Fatalf("Job activity page is missing shared sidebar style %q", marker)
		}
	}
	if !strings.Contains(jobsHTML, `class="sidebar-project-row"><span class="sidebar-toggle-placeholder"`) {
		t.Fatal("Job activity project links do not use the shared sidebar row layout")
	}
	for _, marker := range []string{".sidebar-section-heading {", ".sidebar-section-note {", ".sidebar-resizer {", "resize: none;", "min-width: 190px;", "max-width: 520px;", "overflow-y: auto;", "overflow-x: hidden;", "text-overflow: ellipsis;"} {
		if !strings.Contains(webSidebarStylesCSS, marker) {
			t.Fatalf("shared sidebar style is missing %q", marker)
		}
	}
}

func TestWebSidebarLazilyListsProjectsInOtherBasedirs(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	rootBaseDir, otherBaseDir := t.TempDir(), t.TempDir()
	options := testOptions(rootBaseDir, false)
	options.BaseDirs = []string{otherBaseDir}
	pagePath := filepath.Join(t.TempDir(), "index.html")
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(pagePath, []byte((site{Options: options, notificationSession: newNotificationSession()}).webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(webprojection.State{BaseDir: rootBaseDir, Queues: []webprojection.QueueState{{QueueName: "root-project", Runs: []webprojection.Run{}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	otherID := basedirID(otherBaseDir)
	rootID := basedirID(rootBaseDir)
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const state = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const rootID = process.argv[4];
const requests = [];
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(html, {
	runScripts: 'dangerously',
	url: 'http://127.0.0.1/project/root-project',
	virtualConsole,
	beforeParse(window) {
		window.CanvasRenderingContext2D = function () {};
		window.HTMLCanvasElement.prototype.getContext = () => ({font: "", measureText: text => ({width: text.length * 8})});
		window.fetch = async url => {
			requests.push(String(url));
			if (url === '/api/state') return {ok: true, json: async () => state};
			if (url === '/api/projects?basedir_id=' + process.argv[3]) return {ok: true, json: async () => ({projects: ['other-project']})};
			throw new Error('unexpected fetch: ' + url);
		};
		window.setInterval = () => 1;
	},
});
setTimeout(async () => {
	const assert = (condition, message) => { if (!condition) throw new Error(message); };
	try {
		const bases = [...dom.window.document.querySelectorAll('#sidebar-basedirs > .basedir-entry')];
		assert(bases.length === 2, 'sidebar does not show both registered basedirs');
		assert(dom.window.document.querySelector('.sidebar-section-heading')?.textContent.trim() === 'Registered basedirs', 'sidebar does not explain what the basedir list contains');
		assert(rootID && dom.window.document.querySelector('.basedir-notification-toggle'), 'basedir notification checkbox is missing');
		assert(dom.window.document.querySelector('.basedir-notification-row')?.textContent.includes('Monitor notifications'), 'basedir notification checkbox is not explained');
		const rootSelector = '#sidebar-basedirs > [data-basedir-id="' + rootID + '"]';
		let root = dom.window.document.querySelector(rootSelector);
		assert(root && !root.querySelector(':scope > .basedir-contents').hidden, 'startup basedir should start expanded');
		assert(root.querySelector('.basedir-notification-toggle')?.checked, 'startup basedir should be monitored by default');
		assert(root.querySelector('.sidebar-project:not(.basedir-entry) .sidebar-project-link.active')?.textContent.trim() === 'root-project', 'selected project is not active in the sidebar');
		root.querySelector(':scope > .basedir-row .sidebar-toggle').click();
		root = dom.window.document.querySelector(rootSelector);
		assert(root.querySelector(':scope > .basedir-contents').hidden, 'expanded basedir cannot be collapsed');
		root.querySelector(':scope > .basedir-row .sidebar-toggle').click();
		root = dom.window.document.querySelector(rootSelector);
		assert(!root.querySelector(':scope > .basedir-contents').hidden, 'collapsed basedir cannot be reopened');
		assert(root.querySelector('.sidebar-project-link.basedir-path').title.startsWith('/'), 'basedir path tooltip is not absolute');
		const jobActivity = root.querySelector('a[href="/jobs/"]');
		const allProjects = [...root.querySelectorAll('.sidebar-project-link')].find(link => link.textContent.trim() === 'All projects');
		assert(jobActivity && allProjects, 'Job activity or All projects is missing from its basedir');
		assert(allProjects.compareDocumentPosition(jobActivity) & dom.window.Node.DOCUMENT_POSITION_FOLLOWING, 'Job activity should appear after the All projects tree');
		assert(allProjects.closest('.all-projects').querySelector('.project-list'), 'project list is not nested under All projects');
		const allProjectsToggle = allProjects.closest('.sidebar-project-row').querySelector('.sidebar-toggle');
		assert(allProjectsToggle && allProjectsToggle.getAttribute('aria-expanded') === 'true', 'All projects should have an expanded toggle');
		allProjectsToggle.click();
		root = dom.window.document.querySelector(rootSelector);
		assert(root.querySelector('.all-projects > .sidebar-projects').hidden, 'All projects list cannot be collapsed');
		root.querySelector('.all-projects .sidebar-toggle').click();
		root = dom.window.document.querySelector(rootSelector);
		assert(!root.querySelector('.all-projects > .sidebar-projects').hidden, 'All projects list cannot be reopened');
		const sidebar = dom.window.document.querySelector('.sidebar');
		sidebar.scrollTop = 87;
		sidebar.dispatchEvent(new dom.window.Event('scroll'));
		assert(dom.window.sessionStorage.getItem('rotari-sidebar-scroll:' + rootID) === '87', 'sidebar scroll position was not saved');
		sidebar.scrollTop = 0;
		dom.window.renderSidebar(state.projects);
		assert(dom.window.document.querySelector('.sidebar').scrollTop === 87, 'sidebar scroll position was not restored after rerender');
		const pathLink = dom.window.document.querySelector('.basedir-path');
		pathLink.dataset.fullPath = '/very/long/base/directory/with/a/unique-ending';
		Object.defineProperty(pathLink, 'clientWidth', {configurable: true, value: 48});
		Object.defineProperty(pathLink, 'scrollWidth', {configurable: true, get() { return this.textContent.length * 8; }});
		dom.window.fitBasedirPaths(dom.window.document.querySelector('.sidebar'));
		assert(pathLink.textContent === pathLink.dataset.fullPath, 'basedir path should keep its full value for CSS ellipsis');
		const other = [...dom.window.document.querySelectorAll('#sidebar-basedirs > .basedir-entry')].find(base => base.dataset.basedirId === process.argv[3]);
		assert(other && other.querySelector(':scope > .sidebar-projects').hidden, 'other basedir should start collapsed');
		other.querySelector('.sidebar-toggle').click();
		await new Promise(resolve => setTimeout(resolve, 0));
		const link = [...dom.window.document.querySelectorAll('#sidebar-basedirs a')].find(anchor => anchor.textContent.trim() === 'other-project');
		assert(link && link.getAttribute('href') === '/_basedir/' + process.argv[3] + '/project/other-project', 'other project link does not target its basedir');
		assert(requests.filter(url => url === '/api/state').length === 1, 'opening another basedir scanned its full Web state');
		assert(requests.includes('/api/projects?basedir_id=' + process.argv[3]), 'opening the basedir did not request its lightweight project list');
		if (errors.length) throw new Error(errors.join('\n'));
	} catch (error) {
		console.error(error.stack || String(error));
		process.exit(1);
	}
}, 50);
`
	if output, err := exec.Command("node", "-e", script, pagePath, statePath, otherID, rootID).CombinedOutput(); err != nil {
		t.Fatalf("basedir sidebar check failed: %v\n%s", err, output)
	}
}

func TestWebAuthTokenAcceptsBearerAndHeaderToken(t *testing.T) {
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := WithAuthToken(next, "secret")

	for _, test := range []struct {
		name       string
		header     string
		value      string
		wantStatus int
	}{
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "wrong", header: "Authorization", value: "Bearer wrong", wantStatus: http.StatusUnauthorized},
		{name: "bearer", header: "Authorization", value: "Bearer secret", wantStatus: http.StatusNoContent},
		{name: "token header", header: "X-Rotari-Token", value: "secret", wantStatus: http.StatusNoContent},
		{name: "basic", header: "Authorization", value: "Basic cm90YXJpOnNlY3JldA==", wantStatus: http.StatusNoContent},
		{name: "basic wrong user", header: "Authorization", value: "Basic b3RoZXI6c2VjcmV0", wantStatus: http.StatusUnauthorized},
		{name: "basic wrong password", header: "Authorization", value: "Basic cm90YXJpOndyb25n", wantStatus: http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			if test.header != "" {
				request.Header.Set(test.header, test.value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if test.wantStatus == http.StatusUnauthorized && recorder.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("missing WWW-Authenticate header")
			}
		})
	}
}

func TestWebHTMLIncludesEmbeddedThemeFavicons(t *testing.T) {
	html := testSite().webHTML()
	for _, want := range []string{
		`media="(prefers-color-scheme: dark)"`,
		`media="(prefers-color-scheme: light)"`,
		`data:image/svg+xml;base64,`,
		`<span class="brand-mark" aria-hidden="true"><img class="brand-icon" alt="" src="data:image/svg+xml;base64,`,
		`<h1><img class="brand-icon"`,
		`rotari Web</h1>`,
	} {
		if !webContains(html, want) {
			t.Fatalf("web HTML does not contain %q", want)
		}
	}
}

func TestWebReadOnlyControlEndpointsRejectMutations(t *testing.T) {
	baseDir := t.TempDir()
	for _, endpoint := range []struct {
		name string
		body string
		path string
	}{
		{name: "cancel-job", path: "/api/cancel-job", body: `{"project_name":"demo","run_id":"run-1","job_id":"job-1"}`},
		{name: "remove", path: "/api/remove", body: `{"project_name":"demo","job_id":"job-1"}`},
		{name: "clear-run", path: "/api/clear-run", body: `{"project_name":"demo","run_id":"run-1"}`},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, endpoint.path, strings.NewReader(endpoint.body))
			recorder := httptest.NewRecorder()
			Handler(testOptions(baseDir, false)).ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("endpoint %s status = %d, want %d; body=%s", endpoint.path, recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "read-only") {
				t.Fatalf("endpoint %s body = %q, want read-only message", endpoint.path, recorder.Body.String())
			}
		})
	}
}

func TestLoadWebConfigFilesRejectsUnsafeInputs(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := testSite().loadWebConfigFiles(baseDir, "", "run-1"); err == nil || !strings.Contains(err.Error(), "project_name is required with run_id") {
		t.Fatalf("testSite().loadWebConfigFiles(baseDir, \"\", \"run-1\") error = %v, want project_name requirement", err)
	}
	if _, err := testSite().loadWebConfigFiles(baseDir, "../outside", ""); err == nil || !strings.Contains(err.Error(), "invalid project_name") {
		t.Fatalf("testSite().loadWebConfigFiles(baseDir, \"../outside\", \"\") error = %v, want invalid project_name", err)
	}
	if _, err := testSite().loadWebConfigFiles(baseDir, "demo", "../outside"); err == nil || !strings.Contains(err.Error(), "invalid run_id") {
		t.Fatalf("testSite().loadWebConfigFiles(baseDir, \"demo\", \"../outside\") error = %v, want invalid run_id", err)
	}
}

func TestWebHTMLIncludesProjectRuntime(t *testing.T) {
	html := testSite().webHTML()
	for _, want := range []string{"let projectRuntimeDetailsOpen=false", "runtimeDetails.open", "projectRuntimeDetailsOpen?' open'", "function addProjectRuntime()", "addProjectRuntime();addRunHostLine()", "Project runtime", "Internal state", "State lock: advisory and intentionally not probed"} {
		if !webContains(html, want) {
			t.Fatalf("web HTML does not contain %q", want)
		}
	}
}

func TestWebHTMLIncludesConfigPaths(t *testing.T) {
	html := testSite().webHTML()
	for _, want := range []string{"function setLocation(base, paths)", "function addConfigButton()", "function showGenerateConfig()", "generate-config-button", "config-editor", "/api/save-config", "/api/config-targets", "config-target-options", "state.config_path", "q.config_path", "run.context.config_snapshot_paths"} {
		if !webContains(html, want) {
			t.Fatalf("web HTML does not contain %q", want)
		}
	}
	if !strings.Contains(webStylesCSS, ".config-editor[hidden],\n.config-generator[hidden]") {
		t.Fatal("web stylesheet does not hide inactive config controls")
	}
}

func TestWebCancelRunRejectsStaleRunID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.LockFile, model.LockInfo{RunID: "run-current", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/cancel-run", strings.NewReader(`{"project_name":"default","run_id":"run-old"}`))
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, true)).ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatalf("status = %d, want stale run rejection", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `run "run-old" is not running; the active run of project "default" is "run-current"`) {
		t.Fatalf("body = %q, want stale run error", recorder.Body.String())
	}
}

// TestWebJobControlRejectsStaleRunID checks that job actions from a page
// showing a finished run do not reach the same job in the active run.
func TestWebJobControlRejectsStaleRunID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.LockFile, model.LockInfo{RunID: "run-current", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/cancel-job", "/api/suspend-job", "/api/resume-job"} {
		t.Run(path, func(t *testing.T) {
			for _, body := range []string{
				`{"project_name":"default","run_id":"run-old","job_id":"job-1"}`,
				`{"project_name":"default","run_id":"run-old","job_ids":["job-1","job-2"]}`,
			} {
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				recorder := httptest.NewRecorder()
				Handler(testOptions(baseDir, true)).ServeHTTP(recorder, request)
				if recorder.Code == http.StatusOK || !strings.Contains(recorder.Body.String(), `run "run-old" is not running; the active run of project "default" is "run-current"`) {
					t.Fatalf("body %s: status = %d, body = %q; want stale run rejection", body, recorder.Code, recorder.Body.String())
				}
			}

			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"project_name":"default","job_id":"job-1"}`))
			recorder := httptest.NewRecorder()
			Handler(testOptions(baseDir, true)).ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK || !strings.Contains(recorder.Body.String(), "run_id") {
				t.Fatalf("status = %d, body = %q; want run_id to be required", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestLoadWebStateIncludesAllQueues(t *testing.T) {
	baseDir := t.TempDir()
	for _, queueName := range []string{"build", "test"} {
		paths, err := stateinternal.ResolveProjectPaths(baseDir, queueName)
		if err != nil {
			t.Fatal(err)
		}
		if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
			t.Fatal(err)
		}
		if err := stateinternal.WriteJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{RunID: "run-1", Status: "finished"}); err != nil {
			t.Fatal(err)
		}
	}

	state, err := siteFor(baseDir).loadWebState(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Queues) != 2 || state.Queues[0].QueueName != "build" || state.Queues[1].QueueName != "test" {
		t.Fatalf("queues = %#v, want build and test", state.Queues)
	}
	t.Setenv(envRunID, "web-run")
	state, err = siteFor(baseDir).loadWebState(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	var foundRunID bool
	for _, definition := range state.Environments {
		if definition.Name == envRunID {
			foundRunID = definition.Set && definition.Value == "" && definition.Job && definition.Array
			break
		}
	}
	if !foundRunID {
		t.Fatalf("web environments missing current run ID: %#v", state.Environments)
	}

}

func TestLoadWebStateIncludesConfigPaths(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if err := os.MkdirAll(filepath.Join(configHome, "rotari"), 0o755); err != nil {
		t.Fatal(err)
	}
	globalPath := filepath.Join(configHome, "rotari", "config.yaml")
	if err := os.WriteFile(globalPath, []byte("run:\n  retry: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte("run:\n  retry: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(paths.ProjectDir, "config.yaml")
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectPath, []byte("run:\n  retry: 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}

	state, err := siteFor(baseDir).loadWebState(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	basePath := filepath.Join(baseDir, "config.yaml")
	if state.ConfigPath != basePath || state.Queues[0].ConfigPath != projectPath {
		t.Fatalf("config paths = base %q, project %q; want %q, %q", state.ConfigPath, state.Queues[0].ConfigPath, basePath, projectPath)
	}
}

func TestWebConfigAPIReadsResolvedFiles(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	globalDir := filepath.Join(configHome, "rotari")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	globalPath := filepath.Join(globalDir, "config.yaml")
	if err := os.WriteFile(globalPath, []byte("global: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.ProjectDir, "config.yaml"), []byte("project: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{ConfigPaths: config.PathsForRun(baseDir, "demo")}); err != nil {
		t.Fatal(err)
	}
	handler := Handler(testOptions(baseDir, false))
	for _, test := range []struct {
		query string
		want  []string
	}{
		{query: "", want: []string{globalPath, "global: true"}},
		{query: "?project_name=demo", want: []string{"config.yaml", "project: true"}},
		{query: "?project_name=demo&run_id=run-1", want: []string{"project: true"}},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/config"+test.query, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("config request %s status = %d, body = %q", test.query, recorder.Code, recorder.Body.String())
		}
		for _, want := range test.want {
			if !strings.Contains(recorder.Body.String(), want) {
				t.Fatalf("config request %s body does not contain %q: %s", test.query, want, recorder.Body.String())
			}
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/api/config?project_name=../outside", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatal("config API accepted a traversal project name")
	}
}

func TestWebConfigAPIReadsRunConfigSnapshots(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	globalDir := filepath.Join(configHome, "rotari")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "config.yaml"), []byte("global: original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte("base: original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(paths.ProjectDir, "config.yaml")
	if err := os.WriteFile(projectPath, []byte("project: original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeRunContext(paths, "run-1", "/work/project"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(globalDir, "config.yaml"), filepath.Join(baseDir, "config.yaml"), projectPath} {
		if err := os.WriteFile(path, []byte("changed: true\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/config?project_name=demo&run_id=run-1", nil)
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, false)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("config snapshot status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{"project: original"} {
		if !strings.Contains(recorder.Body.String(), want) {
			t.Fatalf("config snapshot body does not contain %q: %s", want, recorder.Body.String())
		}
	}
	for _, unwanted := range []string{"global: original", "base: original"} {
		if strings.Contains(recorder.Body.String(), unwanted) {
			t.Fatalf("config snapshot body contains ignored lower-priority config %q: %s", unwanted, recorder.Body.String())
		}
	}
	if strings.Contains(recorder.Body.String(), "changed: true") {
		t.Fatalf("config snapshot body contains updated source config: %s", recorder.Body.String())
	}
}

func TestWebSaveConfigWritesOnlyTheResolvedCurrentConfig(t *testing.T) {
	baseDir := t.TempDir()
	basePath := filepath.Join(baseDir, "config.toml")
	if err := os.WriteFile(basePath, []byte("base = true\n"), stateinternal.FileMode()); err != nil {
		t.Fatal(err)
	}
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, stateinternal.DirectoryMode()); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(paths.ProjectDir, "config.toml")
	if err := os.WriteFile(projectPath, []byte("project = true\n"), stateinternal.FileMode()); err != nil {
		t.Fatal(err)
	}
	handler := Handler(testOptions(baseDir, true))
	request := httptest.NewRequest(http.MethodPost, "/api/save-config", strings.NewReader(`{"project_name":"demo","content":"project = false\n"}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save config status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	projectData, err := os.ReadFile(projectPath)
	if err != nil || string(projectData) != "project = false\n" {
		t.Fatalf("project config = %q, err = %v", projectData, err)
	}
	baseData, err := os.ReadFile(basePath)
	if err != nil || string(baseData) != "base = true\n" {
		t.Fatalf("base config = %q, err = %v", baseData, err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/save-config", strings.NewReader(`{"project_name":"../outside","content":"bad = true\n"}`))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK || !strings.Contains(recorder.Body.String(), "invalid project_name") {
		t.Fatalf("unsafe save status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestWebSaveConfigRejectsReadOnlyMode(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/save-config", strings.NewReader(`{"content":"value = true\n"}`))
	recorder := httptest.NewRecorder()
	Handler(testOptions(t.TempDir(), false)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("read-only save status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestWebSaveConfigRejectsInvalidFormatWithoutWriting(t *testing.T) {
	for _, test := range []struct {
		name      string
		extension string
		original  string
		invalid   string
	}{
		{name: "json", extension: ".json", original: `{"run":{"retry":1}}`, invalid: `{"run":`},
		{name: "toml", extension: ".toml", original: "[run]\nretry = 1\n", invalid: "[run\n"},
		{name: "yaml", extension: ".yaml", original: "run:\n  retry: 1\n", invalid: "run: [invalid\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			path := filepath.Join(baseDir, "config"+test.extension)
			if err := os.WriteFile(path, []byte(test.original), stateinternal.FileMode()); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/save-config", strings.NewReader(`{"content":`+strconv.Quote(test.invalid)+`}`))
			recorder := httptest.NewRecorder()
			Handler(testOptions(baseDir, true)).ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK || !strings.Contains(recorder.Body.String(), "invalid "+test.name+" config") {
				t.Fatalf("save status = %d, body = %q", recorder.Code, recorder.Body.String())
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != test.original {
				t.Fatalf("config after rejected save = %q, err = %v", data, err)
			}
		})
	}
}

func TestWebGenerateConfigCreatesAndOverwritesTOMLAtSelectedLocation(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	baseDir := t.TempDir()
	handler := Handler(testOptions(baseDir, true))

	request := httptest.NewRequest(http.MethodPost, "/api/generate-config", strings.NewReader(`{"location":"basedir"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("generate config status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	path := filepath.Join(baseDir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "[run]") {
		t.Fatalf("generated config = %q, err = %v", data, err)
	}
	if err := os.WriteFile(path, []byte("custom: true\n"), stateinternal.FileMode()); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/generate-config", strings.NewReader(`{"location":"basedir"}`))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	data, err = os.ReadFile(path)
	if recorder.Code != http.StatusOK || err != nil || strings.Contains(string(data), "custom: true") {
		t.Fatalf("overwrite status = %d, config = %q, err = %v", recorder.Code, data, err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/generate-config", strings.NewReader(`{"location":"invalid"}`))
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK || !strings.Contains(recorder.Body.String(), "invalid config location") {
		t.Fatalf("invalid location status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestWebGenerateConfigRejectsReadOnlyAndUnsafeProject(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/generate-config", strings.NewReader(`{"location":"basedir"}`))
	recorder := httptest.NewRecorder()
	Handler(testOptions(t.TempDir(), false)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("read-only generate status = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/generate-config", strings.NewReader(`{"project_name":"../outside","location":"project"}`))
	recorder = httptest.NewRecorder()
	Handler(testOptions(t.TempDir(), true)).ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK || !strings.Contains(recorder.Body.String(), "invalid project_name") {
		t.Fatalf("unsafe project status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
}

func TestWebConfigTargetsListResolvedTOMLLocations(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	targets, err := webConfigTargets(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	want := []webConfigTarget{
		{Location: "global", Path: filepath.Join(configHome, "rotari", "config.toml")},
		{Location: "basedir", Path: filepath.Join(baseDir, "config.toml")},
		{Location: "project", Path: filepath.Join(paths.ProjectDir, "config.toml")},
	}
	if len(targets) != len(want) {
		t.Fatalf("targets = %#v, want %#v", targets, want)
	}
	for index := range want {
		if targets[index] != want[index] {
			t.Fatalf("target %d = %#v, want %#v", index, targets[index], want[index])
		}
	}
}

func TestLoadWebStateIncludesRuntimeRecords(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	lock := model.LockInfo{RunID: "run-active", PID: 1234, Host: "worker-a", StartedAt: "2026-09-16T00:00:00Z"}
	if err := stateinternal.WriteJSON(paths.LockFile, lock); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(serverinternal.PIDPath(paths.ProjectDir), []byte("5678\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := siteFor(baseDir).loadWebState(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	project := state.Queues[0]
	if project.RunningRunID != lock.RunID || project.RunnerPID != lock.PID || project.RunnerHost != lock.Host {
		t.Fatalf("project runtime = %#v, want lock %#v", project, lock)
	}
	if project.RunnerStartedAt != model.FormatDisplayTimestamp(lock.StartedAt) {
		t.Fatalf("runner started at = %q, want formatted lock timestamp", project.RunnerStartedAt)
	}
	if !project.Server.PIDFileExists || project.Server.PID != 5678 {
		t.Fatalf("server runtime = %#v, want the project's PID record", project.Server)
	}
}

func TestGenerateStaticWebWritesProjectPages(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{RunID: "run-1", Status: "finished"}); err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "web")
	if err := siteFor(baseDir).generateStaticWeb(outputDir); err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"function rewriteStaticLinks()",
		"path === root || path.startsWith(root + \"/\")",
		"new MutationObserver(rewriteStaticLinks)",
	} {
		if !strings.Contains(string(index), want) {
			t.Fatalf("static web page does not contain %q", want)
		}
	}
	if !strings.Contains(string(index), "rewriteStaticLinks();") {
		t.Fatal("static web page does not rewrite links before rendering")
	}
	if !strings.Contains(string(index), "data:image/svg+xml;base64,") {
		t.Fatal("static web page does not contain embedded favicon data")
	}
	for _, page := range []string{
		filepath.Join(outputDir, "index.html"),
		filepath.Join(outputDir, "project", "default", "index.html"),
		filepath.Join(outputDir, "project", "default", "run", "run-1", "index.html"),
	} {
		pageData, readErr := os.ReadFile(page)
		if readErr != nil {
			t.Fatalf("static web page is missing: %v", readErr)
		}
		if !strings.Contains(string(pageData), `href="web_styles.css"`) || strings.Contains(string(pageData), `href="/web_styles.css"`) {
			t.Fatalf("static web page %s does not use a relative stylesheet path", page)
		}
		if !strings.Contains(string(pageData), `href="web_sidebar_styles.css"`) || strings.Contains(string(pageData), `href="/web_sidebar_styles.css"`) {
			t.Fatalf("static web page %s does not use the relative shared sidebar stylesheet path", page)
		}
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(page), "web_styles.css")); statErr != nil {
			t.Fatalf("static web stylesheet beside %s is missing: %v", page, statErr)
		}
		if _, statErr := os.Stat(filepath.Join(filepath.Dir(page), "web_sidebar_styles.css")); statErr != nil {
			t.Fatalf("static sidebar stylesheet beside %s is missing: %v", page, statErr)
		}
	}
	if stylesheet, readErr := os.ReadFile(filepath.Join(outputDir, "web_styles.css")); readErr != nil || !strings.Contains(string(stylesheet), "--bg:") {
		t.Fatalf("static web stylesheet is missing or invalid: %v", readErr)
	}
	if stylesheet, readErr := os.ReadFile(filepath.Join(outputDir, "web_sidebar_styles.css")); readErr != nil || !strings.Contains(string(stylesheet), ".sidebar-brand {") {
		t.Fatalf("static shared sidebar stylesheet is missing or invalid: %v", readErr)
	}
	for _, obsolete := range []string{"queue_name", "/queue/", "state.queues", "All queues", "No queues found."} {
		if strings.Contains(string(index), obsolete) {
			t.Fatalf("static web page contains obsolete project identifier %q", obsolete)
		}
	}
	for _, want := range []string{"project_name", "/project/", "state.projects", "All projects", "No projects found.", "__ROTARI_STATIC_REPORTS__", "__ROTARI_STATIC_CONFIG_TARGETS__", "__ROTARI_STATIC_CONFIGS__", "/api/report", "/api/config-targets", "/api/config", "staticReportKey", "staticConfigKey", "the web UI is read-only"} {
		if !strings.Contains(string(index), want) {
			t.Fatalf("static web page does not contain %q", want)
		}
	}
	for _, want := range []string{"request.searchParams.getAll(\"job_ids\")", "selectedReports", "join(\"\\n\\n\")"} {
		if !strings.Contains(string(index), want) {
			t.Fatalf("static web page does not handle selected report jobs with %q", want)
		}
	}
}

func TestWebJobsPageShowsRecentJobs(t *testing.T) {
	baseDir := t.TempDir()
	runID := "20260922-090000-00000001"
	now := time.Now().UTC()
	writeTestJobsRun(t, baseDir, "demo", runID, "job-1", now.Add(-time.Minute), now, 0)

	request := httptest.NewRequest(http.MethodGet, "/jobs/", nil)
	response := httptest.NewRecorder()
	Handler(testOptions(baseDir, false)).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /jobs/ status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	for _, want := range []string{
		"rotari Job activity",
		"Registered basedirs",
		"State directories known to rotari",
		`aria-label="Toggle projects"`,
		`onclick="toggleJobsSidebar(this)"`,
		`class="basedir-notification-toggle"`,
		`Monitor notifications`,
		`data-jobs-url="/jobs/"`,
		`onchange="toggleJobsNotificationBasedir(this)"`,
		`rotari-notification-basedirs`,
		`class="sidebar-project-link basedir-path"`,
		`href="/project/demo"`,
		`class="brand-icon"`,
		`name="since" value="24h"`,
		"job-1",
		`href="/project/demo"`,
		`href="/project/demo/run/` + runID + `"`,
		`href="/jobs/">Job activity</a>`,
		`title="Copy command"`,
		`title="Copy attempt ID"`,
		`data-copy-value="true"`,
		`id="notify-toggle"`,
		`onclick="toggleJobsNotifications()"`,
		`onclick="location.reload()">Refresh</button>`,
		`setInterval(pollJobsActivity, 2000)`,
	} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("GET /jobs/ response does not contain %q: %s", want, response.Body.String())
		}
	}
	body := response.Body.String()
	allProjectsIndex := strings.Index(body, `>All projects</a>`)
	projectListIndex := strings.Index(body, `class="sidebar-projects project-list"`)
	jobActivityIndex := strings.Index(body, `href="/jobs/">Job activity</a>`)
	if allProjectsIndex < 0 || projectListIndex <= allProjectsIndex || jobActivityIndex <= projectListIndex {
		t.Fatalf("Job activity sidebar order is invalid: All projects=%d project list=%d Job activity=%d", allProjectsIndex, projectListIndex, jobActivityIndex)
	}
}

func TestWebSwitchesBetweenRegisteredBasedirs(t *testing.T) {
	rootBaseDir := t.TempDir()
	otherBaseDir := t.TempDir()
	for baseDir, projectName := range map[string]string{rootBaseDir: "root-project", otherBaseDir: "other-project"} {
		paths, err := stateinternal.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			t.Fatal(err)
		}
		if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
			t.Fatal(err)
		}
	}
	options := testOptions(rootBaseDir, false)
	options.BaseDirs = []string{otherBaseDir}
	handler := Handler(options)
	otherID := basedirID(otherBaseDir)
	assertWebStateBasedir(t, handler, "/api/state", rootBaseDir)
	assertWebStateBasedir(t, handler, "/_basedir/"+otherID+"/api/state", otherBaseDir)
	response := serveWebGet(t, handler, "/api/projects?basedir_id="+otherID)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "other-project") {
		t.Fatalf("GET /api/projects for registered basedir = (%d, %q)", response.Code, response.Body.String())
	}
	response = serveWebGet(t, handler, "/_basedir/not-registered/api/state")
	if response.Code != http.StatusNotFound {
		t.Fatalf("GET unregistered basedir status = %d, want 404", response.Code)
	}
	response = serveWebGet(t, handler, "/_basedir/"+otherID+"/jobs/")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `href="/_basedir/`+otherID+`/project/other-project"`) {
		t.Fatalf("GET basedir Job activity page = (%d, %q), want links scoped to selected basedir", response.Code, response.Body.String())
	}
	response = serveWebGet(t, handler, "/_basedir/"+otherID+"/jobs")
	if response.Code != http.StatusMovedPermanently || response.Header().Get("Location") != "/_basedir/"+otherID+"/jobs/" {
		t.Fatalf("GET basedir /jobs redirect = (%d, %q), want selected basedir path", response.Code, response.Header().Get("Location"))
	}
	response = serveWebGet(t, handler, "/")
	for _, marker := range []string{`id="sidebar-basedirs"`, otherID, rootBaseDir, `"current":true`} {
		if !strings.Contains(response.Body.String(), marker) {
			t.Fatalf("Web app HTML is missing basedir navigation data %q", marker)
		}
	}
}

func serveWebGet(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func assertWebStateBasedir(t *testing.T, handler http.Handler, path, basedir string) {
	t.Helper()
	response := serveWebGet(t, handler, path)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body %q", path, response.Code, response.Body.String())
	}
	var state webprojection.State
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.BaseDir != basedir || len(state.Queues) != 1 {
		t.Fatalf("GET %s state = %#v, want basedir %q and one project", path, state, basedir)
	}
}

func TestWebJobsPageFiltersBySince(t *testing.T) {
	baseDir := t.TempDir()
	runID := "20260922-090000-00000001"
	now := time.Now().UTC()
	writeTestJobsRun(t, baseDir, "demo", runID, "old-job", now.Add(-2*time.Hour), now.Add(-time.Hour), 0)

	request := httptest.NewRequest(http.MethodGet, "/jobs/?since=30m", nil)
	response := httptest.NewRecorder()
	Handler(testOptions(baseDir, false)).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /jobs/?since=30m status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "old-job") || !strings.Contains(response.Body.String(), `value="30m"`) {
		t.Fatalf("GET /jobs/?since=30m response = %s", response.Body.String())
	}
}

func TestWebJobsPageRejectsInvalidSince(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/jobs/?since=invalid", nil)
	response := httptest.NewRecorder()
	Handler(testOptions(t.TempDir(), false)).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("GET /jobs/?since=invalid status = %d, want %d: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestJobsHTMLStylesStates(t *testing.T) {
	html := jobsHTML("/", nil, []joblist.Row{{State: "success"}, {State: "failed"}, {State: "running"}}, joblist.DefaultSinceText, true, true)
	for _, want := range []string{
		`class="jobs-state jobs-state-success"`,
		`class="jobs-state jobs-state-failed"`,
		`class="jobs-state jobs-state-running"`,
		`.jobs-state-success`,
		`.jobs-state-failed`,
		`.jobs-state-running`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("jobs HTML does not contain %q", want)
		}
	}
}

func TestJobsHTMLSupportsSortingAndTimezoneTimestamps(t *testing.T) {
	started := time.Date(2026, 9, 29, 1, 2, 3, 0, time.UTC)
	finished := started.Add(time.Minute)
	html := jobsHTML("/", nil, []joblist.Row{{State: "success", Project: "p", RunID: "r", JobName: "j", StartedAt: started, FinishedAt: finished}}, joblist.DefaultSinceText, false, false)
	for _, want := range []string{
		`class="jobs-table"`,
		`data-sort="started"`,
		`data-sort="finished"`,
		`data-sort-value="2026-09-29T01:02:03Z"`,
		`data-sort-value="2026-09-29T01:03:03Z"`,
		"function sortJobsTable(table, key, direction)",
		`header.textContent = header.dataset.label + " ↕"`,
		`header.dataset.label + " ↕"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("jobs HTML does not contain %q", want)
		}
	}
}

func TestGenerateStaticWebIncludesJobsPage(t *testing.T) {
	baseDir := t.TempDir()
	runID := "20260922-090000-00000001"
	now := time.Now().UTC()
	writeTestJobsRun(t, baseDir, "demo", runID, "job-1", now.Add(-time.Minute), now, 0)
	outputDir := filepath.Join(t.TempDir(), "web")
	if err := siteFor(baseDir).generateStaticWeb(outputDir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(outputDir, "jobs", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"rotari Job activity", `aria-label="Toggle projects"`, `href="../project/demo"`, `class="brand-icon"`, "job-1", `href="../project/demo/run/` + runID + `"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("static jobs page does not contain %q: %s", want, string(data))
		}
	}
}

func TestWebSeparatesLogsFromActions(t *testing.T) {
	html := testSite().webHTML()
	for _, want := range []string{"function mergeActionColumns(){}", "function orderJobActions()", "view-log", "show-path", "delete-run", "Job log", "Job log — merged", `logMode === "separate"`, "changeLogStream", `id="log-stream"`, "View log", "Source log", "Logs", "showDiagnosis(this)", "data-diagnoses", "function showDiagnosis(trigger)", "const buttons=[...logCell.querySelectorAll('button')]", "const diagnosisControl=canDiagnose", "disabled title=\"Available after a finalized failed result with saved analysis\"", "function showPath(path)", "textContent='Job path'", "dataset.view!=='path'", "cell.style.display='table-cell'", "buttonGrid.className='action-buttons'", "buttonGrid.style.gridTemplateColumns='repeat(2, max-content)'", "cell.querySelector(\":scope > .action-buttons\")", "button.style.width='auto'", "cell.style.width='max-content'"} {
		if !webContains(html, want) {
			t.Fatalf("web page does not contain %q", want)
		}
	}
}

func TestWebRunGraphicsShowLoadSummary(t *testing.T) {
	html := testSite().webHTML()
	for _, want := range []string{
		"function runLoadSummary(run)",
		"context.load_samples",
		"context.started_load",
		"load \" +",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("run graphics are missing load summary support %q", want)
		}
	}
}

func TestWebProvidesAttemptSelector(t *testing.T) {
	html := testSite().webHTML()
	for _, want := range []string{"selectedAttemptByJob", "openAttemptMenuByJob", "function selectJobAttempt", "function setAttemptMenuOpen", "function closeAttemptMenus", "document.addEventListener(\"pointerdown\"", "attempt-menu", "attempt-options", "Select attempt", "attempt_id="} {
		if !webContains(html, want) {
			t.Fatalf("web page does not contain %q", want)
		}
	}
}

func TestWebProvidesCopyAndAIReports(t *testing.T) {
	html := testSite().webHTML()
	for _, want := range []string{
		`id="copy-modal"`,
		`id="copy-tail"`,
		`class="output-box"`,
		`class="modal-copy-status"`,
		`Copy last 100 lines</button`,
		`class="command-guide-copy modal-copy"`,
		`title="Copy last 100 lines"`,
		`<rect x="4" y="9" width="11" height="11" rx="1"></rect><rect x="9" y="4" width="11" height="11" rx="1"></rect>`,
		`id="report-note"`,
		`Markdown report for pasting into an AI assistant. Nothing is sent to external services automatically.`,
		`fetch('/api/report?'+params)`,
		`function addAIButtons()`,
		`const actions=row.children[actionIndex]`,
		`button.textContent='Report'`,
		`Prepare run report`,
		`Prepare job report`,
		`dataset.view!=='ai'`,
	} {
		if !webContains(html, want) {
			t.Fatalf("web page does not contain %q", want)
		}
	}
	for _, removed := range []string{`Open ChatGPT`, `Open Gemini`, `Open Claude`, `openAI(`, `chatgpt.com`, `gemini.google.com`, `claude.ai`} {
		if strings.Contains(html, removed) {
			t.Fatalf("removed AI button content is still present: %q", removed)
		}
	}
}

func TestWebReportModalOutputScrollsWithinPanel(t *testing.T) {
	for _, want := range []string{
		".output-panel {\n  display: flex;\n  flex-direction: column;",
		".output-box {\n  position: relative;\n  flex: 1;\n  min-height: 0;",
		".output-panel .log {\n  height: 100%;",
	} {
		if !strings.Contains(webStylesCSS, want) {
			t.Fatalf("web stylesheet does not constrain report output with %q", want)
		}
	}
}

func TestWebHostsColumnIsSortable(t *testing.T) {
	if !webContains(testSite().webHTML(), "header.dataset.sort='hosts'") {
		t.Fatal("web page Hosts column is not sortable")
	}
}

func TestWebQueueWorkingDirectoryUsesSeparateEditableColumn(t *testing.T) {
	html := testSite().webHTML()
	for _, want := range []string{
		"header.dataset.sort='working_directory';header.textContent='Working directory'",
		"function rowCell(row,key)",
		"rowCell(row,'command')",
		"rowCell(row,'working_directory')",
		"ensureQueueWorkingDirectoryColumn(commands);addQueueEditors(queue,commands)",
		"<td>'+esc(j.working_directory||'-')+'</td><td class=\"command\">",
		"data-sort=\"working_directory\">Working directory",
		"data-sort=\"command\">Command",
		"function markJobHeaders(){}",
	} {
		if !webContains(html, want) {
			t.Fatalf("web queue table does not contain %q", want)
		}
	}
}

func TestLoadWebJobsIncludesCommandMetadata(t *testing.T) {
	runDir := t.TempDir()
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "job-1", Name: "train", Command: []string{"python", "train.py"},
		Executor: "slurm", ExecutorOptions: []string{"-p", "gpu"}, DependsOn: []string{"prepare"},
	}}}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	jobs, err := webprojection.LoadRunJobs(testStore(), runDir, model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 0}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %#v, want one job", jobs)
	}
	job := jobs[0]
	if job.Name != "train" || job.Executor != "slurm" || len(job.ExecutorOptions) != 2 || len(job.DependsOn) != 1 || job.Result == nil {
		t.Fatalf("job = %#v, want command metadata and result", job)
	}
}

func TestLoadWebJobsIncludesAttemptsNewestFirst(t *testing.T) {
	runID := "20260922-070308-0d83bd39"
	runDir := t.TempDir()
	job := model.QueuedCommand{ID: "job-1", Command: []string{"true"}}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{job}}); err != nil {
		t.Fatal(err)
	}
	for number, exitCode := range []int{1, 0} {
		attemptID := stateinternal.MakeAttemptID(runID, job.ID, number)
		attemptDir := filepath.Join(runDir, job.ID, "attempts", attemptID)
		if err := os.MkdirAll(attemptDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(attemptDir, "status"), []byte(strconv.Itoa(exitCode)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(attemptDir, "finished_at"), []byte("2026-09-22T00:00:0"+strconv.Itoa(number)+"Z\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	jobs, err := webprojection.LoadRunJobs(testStore(), runDir, model.RunSummary{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || len(jobs[0].Attempts) != 2 {
		t.Fatalf("jobs = %#v, want two attempts", jobs)
	}
	latest := stateinternal.MakeAttemptID(runID, job.ID, 1)
	if jobs[0].Attempts[0].ID != latest || jobs[0].Attempts[0].Result == nil || jobs[0].Attempts[0].Result.ExitCode != 0 {
		t.Fatalf("attempts = %#v, want latest successful attempt first", jobs[0].Attempts)
	}
}

func TestFormatWebQueueDisplayTimesFormatsAttemptTimes(t *testing.T) {
	state := webprojection.QueueState{Runs: []webprojection.Run{{Jobs: []webprojection.Job{{Attempts: []webprojection.Attempt{{
		SubmittedAt: "2026-09-22T08:47:59Z",
		FinishedAt:  "2026-09-22T08:48:00Z",
	}}}}}}}

	formatWebQueueDisplayTimes(&state)
	attempt := state.Runs[0].Jobs[0].Attempts[0]
	if attempt.SubmittedAt != model.FormatDisplayTimestamp("2026-09-22T08:47:59Z") || attempt.FinishedAt != model.FormatDisplayTimestamp("2026-09-22T08:48:00Z") {
		t.Fatalf("attempt timestamps = %#v, want display timestamps", attempt)
	}
}

func TestWebLogReadsSelectedAttempt(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260922-070308-0d83bd39"
	attemptID := stateinternal.MakeAttemptID(runID, "job-1", 0)
	attemptDir := filepath.Join(paths.RunsDir, runID, "job-1", "attempts", attemptID)
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stdout"), []byte("selected stdout\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attemptDir, "stderr"), []byte("selected stderr\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/log?project_name=default&run_id="+runID+"&job_id=job-1&attempt_id="+attemptID+"&stream=stdout", nil)
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, false)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "selected stdout\n" {
		t.Fatalf("selected stdout log = (%d, %q)", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/log?project_name=default&run_id="+runID+"&job_id=job-1&attempt_id="+attemptID+"&stream=stderr", nil)
	recorder = httptest.NewRecorder()
	Handler(testOptions(baseDir, false)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "selected stderr\n" {
		t.Fatalf("selected stderr log = (%d, %q)", recorder.Code, recorder.Body.String())
	}
}

func TestWebLogRejectsAttemptForAnotherJobOrRun(t *testing.T) {
	baseDir := t.TempDir()
	runID := "20260922-070308-0d83bd39"
	for _, attemptID := range []string{
		stateinternal.MakeAttemptID(runID, "other-job", 0),
		stateinternal.MakeAttemptID("20260922-070309-0d83bd40", "job-1", 0),
		stateinternal.MakeAttemptID(runID, "job-1", 99),
		"not-an-attempt",
	} {
		t.Run(attemptID, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/log?project_name=default&run_id="+runID+"&job_id=job-1&attempt_id="+attemptID, nil)
			recorder := httptest.NewRecorder()
			Handler(testOptions(baseDir, false)).ServeHTTP(recorder, request)
			if recorder.Code == http.StatusOK {
				t.Fatalf("mismatched attempt %q was accepted", attemptID)
			}
		})
	}
}

func TestLoadWebJobsRejectsUnsafeJobID(t *testing.T) {
	runDir := t.TempDir()
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "../outside", Command: []string{"true"}}}}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}

	if _, err := webprojection.LoadRunJobs(testStore(), runDir, model.RunSummary{}, ""); err == nil {
		t.Fatal("LoadRunJobs accepted an unsafe job ID")
	}
}

func TestLoadWebJobsIncludesSchedulerState(t *testing.T) {
	runDir := t.TempDir()
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"sleep", "10"}, Executor: "slurm"}}}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	writeSchedulerStatus(filepath.Join(runDir, "job-1"), "PENDING")

	jobs, err := webprojection.LoadRunJobs(testStore(), runDir, model.RunSummary{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].SchedulerState != "pending" {
		t.Fatalf("jobs = %#v, want pending scheduler state", jobs)
	}
}

func TestLoadWebJobsIncludesFinishedLocalJobBeforeRunSummary(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run-1")
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"sh", "-c", "exit 0"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "status"), []byte("0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "finished_at"), []byte("2026-09-19T00:00:01Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	jobs, err := webprojection.LoadRunJobs(testStore(), runDir, model.RunSummary{RunID: "run-1", Status: "running"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Result == nil || jobs[0].Result.ExitCode != 0 {
		t.Fatalf("jobs = %#v, want finished local result", jobs)
	}
}

func TestLoadWebJobsProjectsFinishedSchedulerStatus(t *testing.T) {
	runDir := t.TempDir()
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "array-1", Command: []string{"true"}, Executor: "slurm"}}}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "array-1", "status.json"), executor.WrapperStatus{Phase: "running", ExitCode: 0, FinishedAt: "2026-09-18T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	jobs, err := webprojection.LoadRunJobs(testStore(), runDir, model.RunSummary{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Result == nil || jobs[0].Result.ExitCode != 0 || jobs[0].FinishedAt == "" {
		t.Fatalf("jobs = %#v, want finished result from status.json", jobs)
	}
}

func TestLoadWebJobsUsesCarriedOriginTimestamps(t *testing.T) {
	runsDir := t.TempDir()
	sourceRunDir := filepath.Join(runsDir, "run-1")
	currentRunDir := filepath.Join(runsDir, "run-2")
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "job-1", Command: []string{"true"},
		Origin: &model.JobOrigin{RunID: "run-1", JobID: "job-1", Status: "success"},
	}}}
	if err := stateinternal.WriteJSON(filepath.Join(currentRunDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(sourceRunDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "submitted_at"), []byte("2026-09-16T00:00:01Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "finished_at"), []byte("2026-09-16T00:00:02Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	jobs, err := webprojection.LoadRunJobs(testStore(), currentRunDir, model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 0}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if jobs[0].SubmittedAt != "2026-09-16T00:00:01Z" || jobs[0].FinishedAt != "2026-09-16T00:00:02Z" {
		t.Fatalf("job timestamps = %#v, want carried origin timestamps", jobs[0])
	}
}

func TestReadJobTimestampRejectsUnsafePathElements(t *testing.T) {
	runDir := t.TempDir()
	if got := stateinternal.ReadJobTimestamp(runDir, "../outside", "submitted_at"); got != "" {
		t.Fatalf("unsafe job ID timestamp = %q, want empty", got)
	}
	if got := stateinternal.ReadJobTimestamp(runDir, "job-1", "../submitted_at"); got != "" {
		t.Fatalf("unsafe timestamp name = %q, want empty", got)
	}
}

func TestLoadWebStateIncludesRunContextAndTimeline(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}
	if err := stateinternal.WriteJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{CWD: "/work/project", Hostname: "node-a", StartedLoad: &model.LoadAverage{One: 1.25, Five: 1.5, Fifteen: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.AppendLoadSample(loadSamplesPath(paths, "run-1"), model.LoadSample{At: "2026-09-16T00:00:01Z", LoadAverage: model.LoadAverage{One: 1.25, Five: 1.5, Fifteen: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: "run-1", Status: "finished", StartedAt: "2026-09-16T00:00:00Z", FinishedAt: "2026-09-16T00:00:03Z", Results: []model.JobResult{{ID: "job-1", ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "submitted_at"), []byte("2026-09-16T00:00:01Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "finished_at"), []byte("2026-09-16T00:00:02Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	state, err := siteFor(baseDir).loadWebState(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	run := state.Queues[0].Runs[0]
	if run.Context.Hostname != "node-a" || run.Context.StartedLoad == nil || run.CWD != "/work/project" {
		t.Fatalf("context = %#v, cwd = %q, want host/load/cwd", run.Context, run.CWD)
	}
	if run.StartedAt != model.FormatDisplayTimestamp("2026-09-16T00:00:00Z") || run.FinishedAt != model.FormatDisplayTimestamp("2026-09-16T00:00:03Z") {
		t.Fatalf("run timestamps = %#v, want display timestamps", run.RunSummary)
	}
	if run.Jobs[0].SubmittedAt != model.FormatDisplayTimestamp("2026-09-16T00:00:01Z") || run.Jobs[0].FinishedAt != model.FormatDisplayTimestamp("2026-09-16T00:00:02Z") {
		t.Fatalf("job timestamps = %#v, want submitted and finished timestamps", run.Jobs[0])
	}
	if len(run.Timeline) != 3 || run.Timeline[1].Running != 1 || run.Timeline[2].Finished != 1 || run.Timeline[2].Success != 1 {
		t.Fatalf("timeline = %#v, want pending, submitted, and successful finished transitions", run.Timeline)
	}
	if run.Timeline[1].At != "2026-09-16T00:00:01Z" {
		t.Fatalf("timeline timestamp = %q, want RFC3339 timestamp for browser parsing", run.Timeline[1].At)
	}
	if run.Context.LoadSamples[0].At != "2026-09-16T00:00:01Z" {
		t.Fatalf("load sample timestamp = %q, want RFC3339 timestamp for browser parsing", run.Context.LoadSamples[0].At)
	}
}

func TestBuildWebTimelineCountsCarriedResultsAtStart(t *testing.T) {
	summary := model.RunSummary{StartedAt: "2026-09-16T00:00:00Z"}
	jobs := []webprojection.Job{
		{ID: "carried-success", Origin: &model.JobOrigin{RunID: "previous", JobID: "carried-success"}, SubmittedAt: "2026-09-15T00:00:01Z", FinishedAt: "2026-09-15T00:00:02Z", Result: &model.JobResult{ID: "carried-success", ExitCode: 0}},
		{ID: "rerun-failed", SubmittedAt: "2026-09-16T00:00:01Z", FinishedAt: "2026-09-16T00:00:02Z", Result: &model.JobResult{ID: "rerun-failed", ExitCode: 1}},
	}

	inputs := make([]webprojection.JobTimelineInput, 0, len(jobs))
	for _, job := range jobs {
		inputs = append(inputs, webprojection.JobTimelineInput{Finished: job.Result != nil, Carried: job.Origin != nil, SubmittedAt: job.SubmittedAt, FinishedAt: job.FinishedAt, Success: job.Result != nil && job.Result.ExitCode == 0})
	}
	timeline := webprojection.BuildTimeline(summary.StartedAt, inputs)
	if len(timeline) != 3 {
		t.Fatalf("timeline = %#v, want start, submitted, and finished points", timeline)
	}
	if timeline[0].Pending != 1 || timeline[0].Finished != 1 || timeline[0].Success != 1 {
		t.Fatalf("timeline start = %#v, want carried success counted as finished at start", timeline[0])
	}
	if timeline[2].Pending != 0 || timeline[2].Finished != 2 || timeline[2].Success != 1 || timeline[2].Failed != 1 {
		t.Fatalf("timeline end = %#v, want one carried success and one executed failure", timeline[2])
	}
}

func TestWriteRunContext(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte("run:\n  retry: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeRunContext(paths, "run-1", "/work/project"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(paths.RunsDir, "run-1", "context.json"))
	if err != nil {
		t.Fatal(err)
	}
	var context model.RunContext
	if err := json.Unmarshal(data, &context); err != nil {
		t.Fatal(err)
	}
	if context.CWD != "/work/project" {
		t.Fatalf("cwd = %q, want /work/project", context.CWD)
	}
	if len(context.ConfigPaths) == 0 || context.ConfigPaths[len(context.ConfigPaths)-1] != filepath.Join(baseDir, "config.yaml") {
		t.Fatalf("config paths = %#v, want basedir config", context.ConfigPaths)
	}
	if len(context.ConfigSnapshotFiles) != 1 {
		t.Fatalf("config snapshot files = %#v, want one snapshot", context.ConfigSnapshotFiles)
	}
	if len(context.ConfigSnapshotPaths) != 1 || context.ConfigSnapshotPaths[0] != filepath.Join(paths.RunsDir, "run-1", "configs", context.ConfigSnapshotFiles[0]) {
		t.Fatalf("config snapshot paths = %#v, want run-local config path", context.ConfigSnapshotPaths)
	}
	snapshot, err := os.ReadFile(filepath.Join(paths.RunsDir, "run-1", "configs", context.ConfigSnapshotFiles[0]))
	if err != nil || string(snapshot) != "run:\n  retry: 1\n" {
		t.Fatalf("config snapshot = %q, err = %v", snapshot, err)
	}
	samples := stateinternal.ReadLoadSamples(loadSamplesPath(paths, "run-1"))
	if context.StartedLoad != nil && len(samples) != 1 {
		t.Fatalf("load samples = %#v, want initial load sample", samples)
	}
	if err := finishRunContext(paths, "run-1"); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(paths.RunsDir, "run-1", "context.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &context); err != nil {
		t.Fatal(err)
	}
	samples = stateinternal.ReadLoadSamples(loadSamplesPath(paths, "run-1"))
	if context.FinishedLoad != nil && len(samples) != 2 {
		t.Fatalf("load samples = %#v, want initial and final load samples", samples)
	}
}

func TestWebControlEndpointsRejectedWhenControlDisabled(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := stateinternal.ResolveProjectPaths(baseDir, "default"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/copy", "/api/change", "/api/remove", "/api/clear-run", "/api/cancel-job", "/api/cancel-run"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"project_name":"default"}`))
		recorder := httptest.NewRecorder()
		Handler(testOptions(baseDir, false)).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("%s status = %d, want %d (rejected with --allow-control=false)", path, recorder.Code, http.StatusForbidden)
		}
	}
}

func TestWebCopyEndpointCopiesWithoutRunner(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Name: "failed", Command: []string{"false"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/copy", strings.NewReader(`{"project_name":"default","run_id":"run-1","selection":"failed"}`))
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, true)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	queue, err := stateinternal.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].ID != "job-1" {
		t.Fatalf("queue = %#v, want one copied job with the source ID preserved", queue)
	}
	if _, err := os.Stat(serverinternal.LockPath(paths.ProjectDir)); !os.IsNotExist(err) {
		t.Fatalf("web copy started a supervisor: %v", err)
	}
}

func TestWebCopyEndpointQueuesOneJobWithoutRunner(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "existing", Command: []string{"true"}}}}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Name: "failed", Command: []string{"false"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{Results: []model.JobResult{{ID: "job-1", ExitCode: 1}}}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/copy", strings.NewReader(`{"project_name":"default","run_id":"run-1","job_id":"job-1"}`))
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, true)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	queue, err := stateinternal.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 || queue.Commands[1].ID != "job-1" {
		t.Fatalf("queue = %#v, want existing job plus queued retry", queue)
	}
}

func TestWebChangeEndpointUpdatesQueueJob(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"old"}}}}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/change", strings.NewReader(`{"project_name":"default","job_id":"job-1","command":["new","arg"],"executor_options":["-p","gpu"]}`))
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, true)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	queue, err := stateinternal.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	job := queue.Commands[0]
	if len(job.Command) != 2 || job.Command[0] != "new" || len(job.ExecutorOptions) != 2 || job.ExecutorOptions[1] != "gpu" {
		t.Fatalf("job = %#v, want updated command and executor options", job)
	}
}

func TestWebChangeEndpointRejectsUnknownExecutor(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"old"}}}}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/change", strings.NewReader(`{"project_name":"default","job_id":"job-1","command":["old"],"executor":"nosuch"}`))
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, true)).ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK || !strings.Contains(recorder.Body.String(), "unsupported executor") {
		t.Fatalf("status = %d, body = %s, want unsupported executor error", recorder.Code, recorder.Body.String())
	}
	queue, err := stateinternal.LoadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if queue.Commands[0].Executor != "" {
		t.Fatalf("executor = %q, want unchanged", queue.Commands[0].Executor)
	}
}

func TestMethodNotAllowedRejectsNonGetOnAPIState(t *testing.T) {
	baseDir := t.TempDir()
	request := httptest.NewRequest(http.MethodPost, "/api/state", nil)
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, false)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestListenWebFallsBackToNextPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	occupiedPort := occupied.Addr().(*net.TCPAddr).Port

	listener, err := Listen("127.0.0.1", occupiedPort, true)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if listener.Addr().(*net.TCPAddr).Port <= occupiedPort {
		t.Fatalf("listener port = %d, want a port greater than %d", listener.Addr().(*net.TCPAddr).Port, occupiedPort)
	}
}

func TestListenWebDoesNotFallbackForExplicitPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	occupiedPort := occupied.Addr().(*net.TCPAddr).Port

	listener, err := Listen("127.0.0.1", occupiedPort, false)
	if err == nil {
		listener.Close()
		t.Fatal("listenWeb succeeded on an occupied explicit port")
	}
}

func TestJobsPageCopyButtonShowsFeedback(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	htmlPath := filepath.Join(t.TempDir(), "jobs.html")
	page := jobsHTML("/", nil, []joblist.Row{{State: "success", Project: "p", RunID: "r", JobName: "j", Command: "echo hi", FullCommand: "echo hi", AttemptID: "att_1"}}, joblist.DefaultSinceText, false, false)
	if err := os.WriteFile(htmlPath, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const { JSDOM } = require('jsdom');
const copied = [];
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  beforeParse(window) {
    Object.defineProperty(window.navigator, 'clipboard', {value: {writeText: async value => copied.push(value)}});
  },
});
const button = dom.window.document.querySelectorAll('button.jobs-copy')[1];
button.click();
setTimeout(() => {
  if (copied[0] !== 'att_1') { console.error('copied', copied); process.exit(2); }
  if (!button.classList.contains('copied') || button.title !== 'Copied!' || !button.innerHTML.includes('path')) { console.error(button.outerHTML); process.exit(3); }
  process.exit(0);
}, 50);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("jobs page copy check failed: %v\n%s", err, output)
	}
}

func writeTestJobsRun(t *testing.T, baseDir, project, runID, jobID string, started, finished time.Time, exitCode int) {
	t.Helper()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, project)
	if err != nil {
		t.Fatal(err)
	}
	attemptID := stateinternal.MakeAttemptID(runID, jobID, 0)
	runDir := filepath.Join(paths.RunsDir, runID)
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: jobID, Command: []string{"true"}}}}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, jobID, "attempts", attemptID)
	if err := stateinternal.WriteJSON(filepath.Join(jobDir, "command.json"), model.JobSpec{ID: jobID, AttemptID: attemptID, Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: runID, Status: model.RunStatus(exitCode), StartedAt: started.Format(time.RFC3339), FinishedAt: finished.Format(time.RFC3339), Results: []model.JobResult{{ID: jobID, AttemptID: attemptID, ExitCode: exitCode}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeTestTimestamp(filepath.Join(jobDir, "submitted_at"), started); err != nil {
		t.Fatal(err)
	}
	if err := writeTestTimestamp(filepath.Join(jobDir, "finished_at"), finished); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(jobDir, "status"), []byte(formatInt(exitCode)+"\n")); err != nil {
		t.Fatal(err)
	}
}

func writeTestTimestamp(path string, value time.Time) error {
	return writeTestFile(path, []byte(value.Format(time.RFC3339)+"\n"))
}

func writeTestFile(path string, data []byte) error {
	if err := ensureTestDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ensureTestDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func formatInt(value int) string {
	if value == 0 {
		return "0"
	}
	return "1"
}
