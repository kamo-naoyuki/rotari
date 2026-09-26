package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/webui"
)

func TestCLIDocsPageUsesCommandMetadata(t *testing.T) {
	page := webGet(t, t.TempDir(), "", "/docs/")
	for _, want := range []string{`<h1><img class="brand-icon"`, "rotari CLI", "rotari reset", "rotari completion", "--project-name", "Generated from the command metadata"} {
		if !strings.Contains(page, want) {
			t.Fatalf("docs page does not contain %q", want)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	recorder := httptest.NewRecorder()
	webui.Handler(webOptions(t.TempDir(), "", false, true)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "rotari CLI") {
		t.Fatalf("docs response = status %d, body %q", recorder.Code, recorder.Body.String())
	}
}

func TestEnvironmentPageUsesDefinitions(t *testing.T) {
	t.Setenv(envRunID, "web-run")
	page := webGet(t, t.TempDir(), "", "/environment/")
	for _, want := range []string{`<h1><img class="brand-icon"`, "rotari environment variables", envRunID, envBaseDir, "State directory"} {
		if !strings.Contains(page, want) {
			t.Fatalf("environment page does not contain %q", want)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/environment/", nil)
	recorder := httptest.NewRecorder()
	webui.Handler(webOptions(t.TempDir(), "", false, true)).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "rotari environment variables") {
		t.Fatalf("environment response = status %d, body %q", recorder.Code, recorder.Body.String())
	}
}

func TestWebAllowControlDefaultsToTrue(t *testing.T) {
	fs := newFlagSet("web")
	allowControl := cliBool(fs, "allow-control", true)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if !*allowControl {
		t.Fatalf("allow-control default = %v, want true", *allowControl)
	}
}

func TestNewFlagSetWritesUsageToStderr(t *testing.T) {
	fs := newFlagSet("web")
	if fs.Name() != "web" {
		t.Fatalf("flag set name = %q, want %q", fs.Name(), "web")
	}
	if err := fs.Parse([]string{"--unknown-flag"}); err == nil {
		t.Fatal("Parse with an unknown flag did not return an error")
	}
}

func TestInterruptSignalDeliversSIGTERM(t *testing.T) {
	signals := interruptSignal()
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-signals:
	case <-time.After(time.Second):
		t.Fatal("interruptSignal channel did not receive SIGTERM")
	}
}

func TestCmdWebGeneratesStaticSiteWithoutStartingServer(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "job"}, "", nil, nil, "job", nil); err != nil {
		t.Fatal(err)
	}
	staticDir := t.TempDir()

	if code := cmdWeb([]string{"--basedir", baseDir, "--static-dir", staticDir}); code != 0 {
		t.Fatalf("cmdWeb exit code = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(staticDir, "index.html")); err != nil {
		t.Fatalf("static site was not generated: %v", err)
	}
}

func TestCmdWebRejectsInvalidPort(t *testing.T) {
	baseDir := t.TempDir()
	for _, port := range []string{"-1", "70000"} {
		if code := cmdWeb([]string{"--basedir", baseDir, "--port", port}); code != 1 {
			t.Fatalf("cmdWeb exit code = %d, want 1 for invalid port %s", code, port)
		}
	}
}

func TestCmdWebRejectsPositionalArguments(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdWeb([]string{"--basedir", baseDir, "extra"}); code != 1 {
		t.Fatalf("cmdWeb exit code = %d, want 1 for unexpected positional argument", code)
	}
}

func TestWebRunViewDrawsMatrixGrid(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--job-name", "train", "--matrix", "LR=a,b", "--matrix", "SEED=1,2", "--env", "SECRET=hidden", "--", "/bin/sh", "-c", `[ "$LR$SEED" != b2 ]`},
		{"--job-name", "plain", "--", "true"},
	} {
		if code := cmdAdd(append([]string{"--basedir", baseDir, "--project-name", "default", "--quiet"}, args...)); code != 0 {
			t.Fatalf("cmdAdd(%v) exit = %d", args, code)
		}
	}
	executeMixedRun(paths, "run-1", "", 4, 1, 0, "", nil, "", nil, "", true, nil, nil)
	if err := writeJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	data := []byte(webGet(t, baseDir, "default", "/api/state"))
	if strings.Contains(string(data), `"base_environment"`) {
		t.Fatal("web state exposes the matrix base environment")
	}
	statePath := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(htmlPath, []byte(webGet(t, baseDir, "default", "/")), 0o600); err != nil {
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
const dom = new JSDOM(html, {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/project/default/run/run-1',
  virtualConsole,
  beforeParse(window) {
    window.fetch = async () => ({ok: true, json: async () => state, text: async () => 'log text'});
    window.setInterval = () => 1;
  },
});
setTimeout(() => {
  const document = dom.window.document;
  if (errors.length) { console.error(errors.join('\n')); process.exit(1); }
  const panels = document.querySelectorAll('.matrix-panel');
  if (panels.length !== 1 || !panels[0].textContent.includes('Matrix: train') || !panels[0].textContent.includes('3/4 success, 1 failed')) { console.error(document.getElementById('app').innerHTML); process.exit(2); }
  const panel = panels[0];
  const controls = document.querySelector('.web-copy-controls');
  if (!controls || !(panel.compareDocumentPosition(controls) & dom.window.Node.DOCUMENT_POSITION_FOLLOWING)) { console.error('panel is not above the table controls'); process.exit(3); }
  const content = panel.querySelector('.matrix-content');
  if (!content.hidden) { console.error('panel is not collapsed by default'); process.exit(4); }
  panel.querySelector('.matrix-toggle').click();
  if (content.hidden) { console.error('toggle did not expand'); process.exit(5); }
  const classes = () => Array.from(panel.querySelectorAll('td.matrix-cell')).map(cell => cell.className.replace('matrix-cell ', ''));
  const rowHeaders = () => Array.from(panel.querySelectorAll('tbody th, tr > th:first-child')).map(th => th.textContent);
  if (JSON.stringify(classes()) !== JSON.stringify(['matrix-success', 'matrix-success', 'matrix-success', 'matrix-failed'])) { console.error(classes()); process.exit(6); }
  if (!rowHeaders().includes('LR=b')) { console.error(rowHeaders()); process.exit(7); }
  const rows = panel.querySelector('select[data-axis="rows"]');
  rows.value = 'SEED';
  rows.dispatchEvent(new dom.window.Event('change'));
  if (!rowHeaders().includes('SEED=2') || panel.querySelector('select[data-axis="columns"]').value !== 'LR') { console.error('axis swap failed', rowHeaders()); process.exit(8); }
  const cells = Array.from(panel.querySelectorAll('td.matrix-cell'));
  const failedCell = cells.find(cell => cell.classList.contains('matrix-failed'));
  const failedID = JSON.parse(failedCell.dataset.jobs)[0].id;
  failedCell.click();
  const box = document.getElementById('matrix-actions');
  if (!box || !box.textContent.includes('failed')) { console.error('action box missing'); process.exit(9); }
  const nameCopy = box.querySelector('.matrix-actions-heading button.identity-copy');
  if (!nameCopy || !nameCopy.dataset.copyValue.startsWith('train-')) { console.error('job name copy button missing'); process.exit(10); }
  const row = Array.from(document.querySelectorAll('tr[data-job-id]')).find(r => r.dataset.jobId === failedID);
  const tableHeaders = Array.from(row.closest('table').querySelectorAll('thead th')).map(th => th.textContent.trim());
  const rowButtons = row.children[tableHeaders.indexOf('Actions')].querySelectorAll('button').length;
  const boxButtons = Array.from(box.querySelectorAll('.matrix-actions-buttons button'));
  if (rowButtons === 0 || boxButtons.length !== rowButtons + 1) { console.error('buttons', rowButtons, boxButtons.map(b => b.textContent)); process.exit(11); }
  boxButtons[boxButtons.length - 1].click();
  if (document.getElementById('matrix-actions') || !row.classList.contains('matrix-focus')) { console.error('show in table failed'); process.exit(12); }
  failedCell.click();
  document.dispatchEvent(new dom.window.KeyboardEvent('keydown', {key: 'Escape'}));
  if (document.getElementById('matrix-actions')) { console.error('escape did not close'); process.exit(13); }
  if (errors.length) { console.error(errors.join('\n')); process.exit(14); }
  process.exit(0);
}, 100);
`
	if output, err := exec.Command("node", "-e", script, htmlPath, statePath).CombinedOutput(); err != nil {
		t.Fatalf("web matrix grid check failed: %v\n%s", err, output)
	}
}

func TestWebRunViewClampsLongCells(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	long := "echo " + strings.Repeat("very-long-argument ", 8)
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "default", "--quiet", "--job-name", "long", "--", "/bin/sh", "-c", long}); code != 0 {
		t.Fatalf("cmdAdd exit = %d", code)
	}
	executeMixedRun(paths, "run-1", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil)
	if err := writeJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	data := []byte(webGet(t, baseDir, "default", "/api/state"))
	statePath := filepath.Join(t.TempDir(), "state.json")
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(htmlPath, []byte(webGet(t, baseDir, "default", "/")), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const state = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/project/default/run/run-1',
  virtualConsole,
  beforeParse(window) {
    window.fetch = async () => ({ok: true, json: async () => state, text: async () => ''});
    window.setInterval = () => 1;
  },
});
setTimeout(() => {
  const document = dom.window.document;
  const commandCell = () => {
    const table = document.querySelector('#app table.runs');
    const index = [...table.querySelectorAll('thead th')].findIndex(th => th.dataset.sort === 'command');
    return table.querySelector('tbody tr').children[index];
  };
  let cell = commandCell();
  const clamp = cell.querySelector('.cell-clamp');
  if (!clamp || !clamp.classList.contains('collapsed') || clamp.getAttribute('aria-expanded') !== 'false' || cell.querySelector('.cell-toggle')) { console.error(cell.innerHTML); process.exit(2); }
  if (!cell.querySelector(':scope > button.identity-copy')) { console.error('copy button is not outside the clamp'); process.exit(3); }
  clamp.click();
  if (clamp.classList.contains('collapsed') || clamp.getAttribute('aria-expanded') !== 'true') { console.error('clicking the text did not expand'); process.exit(4); }
  clamp.dispatchEvent(new dom.window.KeyboardEvent('keydown', {key: 'Enter', bubbles: true}));
  if (!clamp.classList.contains('collapsed')) { console.error('Enter did not collapse'); process.exit(7); }
  clamp.click();
  dom.window.render();
  cell = commandCell();
  if (cell.querySelector('.cell-clamp').classList.contains('collapsed')) { console.error('expanded state was lost on re-render'); process.exit(5); }
  if (errors.length) { console.error(errors.join('\n')); process.exit(6); }
  process.exit(0);
}, 100);
`
	if output, err := exec.Command("node", "-e", script, htmlPath, statePath).CombinedOutput(); err != nil {
		t.Fatalf("long cell clamp check failed: %v\n%s", err, output)
	}
}

// webGet returns the Web UI's response to a GET of path, failing t unless it
// is 200 OK.
func webGet(t *testing.T, baseDir, projectFilter, path string) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	webui.Handler(webOptions(baseDir, projectFilter, false, true)).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body %q", path, recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}
