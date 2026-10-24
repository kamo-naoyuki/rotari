package webui

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rotari-dev/rotari/internal/artifact"
	"github.com/rotari-dev/rotari/internal/launcher"
	stateinternal "github.com/rotari-dev/rotari/internal/state"
)

// runLaunchBrowserScript runs script in jsdom against a server and fails the
// test with its output when it exits non-zero.
func runLaunchBrowserScript(t *testing.T, options Options, f artifactFixture, script string) {
	t.Helper()
	server := httptest.NewServer(Handler(options))
	defer server.Close()
	entries, _ := json.Marshal(f.entries)
	command := exec.Command("node", "-e", `
const { JSDOM, VirtualConsole } = require('jsdom');
const base = process.argv[1];
const runID = process.argv[2];
const entries = JSON.parse(process.argv[3]);
const work = process.argv[4];
const fail = (code, detail) => { console.error(code, detail || ''); process.exit(1); };
const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
(async () => {
  const html = await (await fetch(base + '/')).text();
  const dom = new JSDOM(html, {
    runScripts: 'dangerously', url: base + '/', virtualConsole: new VirtualConsole(),
    beforeParse(window) {
      // Same-origin POSTs carry Origin in a browser; the exec guard is not
      // installed here, so none is needed.
      window.fetch = (input, init) => fetch(new URL(String(input), base), init);
      window.setInterval = () => 1;
    },
  });
  const window = dom.window;
  const document = window.document;
  await wait(300);
  const panelText = () => document.getElementById('artifact-launch-panel')?.textContent || '';
  const choices = () => [...document.querySelectorAll('#artifact-launch-panel .launch-choices button')].map(button => button.textContent);
  // launchers lists a row's, or a preview title line's, viewer and command
  // buttons.
  const launchers = (scope) => [...(scope || document).querySelectorAll('.artifact-launchers a, .artifact-launchers button')].map(button => button.textContent);
  const titleLaunchers = () => launchers(document.querySelector('#artifact-preview .artifact-preview-title'));
`+script+`
})().catch(error => fail('exception', error.stack));
`, server.URL, f.runID, string(entries), f.work)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("browser check failed: %v\n%s", err, output)
	}
}

func TestWebLaunchInBrowser(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	f := newArtifactFixture(t)
	options := launchOptions(t, f.baseDir,
		launchCommand("Cat", `cat "$ROTARI_LAUNCHER_ARTIFACT"`, launcher.TargetFile),
		launchCommand("Count", `ls "$ROTARI_LAUNCHER_ARTIFACT" | wc -l`, launcher.TargetDirectory),
		launchCommand("Sleep", `sleep 300`, launcher.TargetFile, launcher.TargetDirectory),
		serviceLaunchCommand("Viewer"))
	runLaunchBrowserScript(t, options, f, `
  if (document.getElementById('launches-button').hidden) fail('Launches button hidden while launching is available');
  await window.showArtifacts('default', runID, 'job-1', '');
  window.document.getElementById('artifact-preview').scrollIntoView = () => {};

  // A listing row has a button per command that accepts it, and the Table
  // viewer for a table file; a command's button opens its confirmation as
  // a row below, and the same button closes it.
  await wait(300);
  const listingRow = [...document.querySelectorAll('.artifact-table tbody tr')].find(row => row.children[1].textContent === 'data.csv');
  if (JSON.stringify(launchers(listingRow)) !== '["Table viewer","Cat","Sleep","Viewer"]') fail('row buttons', launchers(listingRow));
  const manyRow = [...document.querySelectorAll('.artifact-table tbody tr')].find(row => row.children[1].textContent === 'many');
  if (JSON.stringify(launchers(manyRow)) !== '["Table viewer","Count","Sleep","Viewer"]') fail('directory row buttons', launchers(manyRow));
  const catButton = () => [...listingRow.querySelectorAll('.artifact-launchers button')].find(button => button.textContent === 'Cat');
  catButton().click();
  await wait(300);
  const panelRow = listingRow.nextElementSibling;
  if (!panelRow || !panelRow.classList.contains('artifact-launch-row') || !panelRow.querySelector('#artifact-launch-panel')) fail('row panel', panelRow && panelRow.outerHTML);
  if (panelRow.firstElementChild.colSpan !== listingRow.children.length) fail('row panel span', panelRow.firstElementChild.colSpan);
  if (!panelText().includes('/bin/sh -c cat') || JSON.stringify(choices()) !== '["Launch","Cancel"]') fail('row confirmation', panelText());
  catButton().click();
  await wait(100);
  if (document.querySelector('.artifact-launch-row') || document.getElementById('artifact-launch-panel')) fail('row panel did not close');

  // An opened file has the same buttons in its title line.
  await window.openArtifact(entries['data.csv'], '', 'file');
  await wait(300);
  if (JSON.stringify(titleLaunchers()) !== '["Table viewer","Cat","Sleep","Viewer"]') fail('file buttons', titleLaunchers());

  // The confirmation shows what the server would run, and nothing has run.
  await window.chooseArtifactLaunch(entries['data.csv'], '', 'Cat');
  const details = panelText();
  for (const want of ['Cat', work + '/data.csv (file)', 'Working directory' + work, '/bin/sh -c cat "$ROTARI_LAUNCHER_ARTIFACT"', 'job job-1']) {
    if (!details.includes(want)) fail('confirmation lacks ' + want, details);
  }
  let launches = (await (await fetch(base + '/api/launches')).json()).launches;
  if (launches.length !== 0) fail('preview started a launch', launches);

  await window.startArtifactLaunch(entries['data.csv'], '', 'Cat', false);
  if (!panelText().includes('Started Cat.')) fail('start message', panelText());
  launches = (await (await fetch(base + '/api/launches')).json()).launches;
  if (launches.length !== 1 || launches[0].name !== 'Cat') fail('launch list', launches);
  const catID = launches[0].id;
  // Wait until it has ended; under the race detector the wrapper is slow.
  for (let tries = 0; ; tries++) {
    const cat = (await (await fetch(base + '/api/launches')).json()).launches.find(launch => launch.id === catID);
    if (cat.state !== 'running') break;
    if (tries > 200) fail('Cat did not end', cat);
    await wait(50);
  }

  // The Launches view lists it with its output.
  await window.showLaunches(catID);
  const view = document.getElementById('launch-view');
  if (view.hidden || !document.getElementById('modal-log').hidden || !document.getElementById('artifact-view').hidden) fail('launches view toggling');
  if (!view.textContent.includes('Cat') || !view.textContent.includes('exited (exit 0)')) fail('launches row', view.textContent);
  if (view.querySelector('.launch-log').textContent !== 'a,b\n1,2\n') fail('launch output', view.querySelector('.launch-log').textContent);

  // A directory child offers directory commands; a repeated start reuses
  // the live launch and offers another instance; Stop ends it.
  await window.showArtifacts('default', runID, 'job-1', '');
  window.document.getElementById('artifact-preview').scrollIntoView = () => {};
  await window.openArtifact(entries['many'], '', 'directory');
  await wait(300);
  if (JSON.stringify(titleLaunchers()) !== '["Table viewer","Count","Sleep","Viewer"]') fail('directory buttons', titleLaunchers());
  await window.chooseArtifactLaunch(entries['many'], '', 'Sleep');
  await window.startArtifactLaunch(entries['many'], '', 'Sleep', false);
  await window.startArtifactLaunch(entries['many'], '', 'Sleep', false);
  if (!panelText().includes('already running') || !choices().includes('Start another instance')) fail('reuse', panelText());
  launches = (await (await fetch(base + '/api/launches')).json()).launches;
  const sleep = launches.find(launch => launch.name === 'Sleep');
  await window.showLaunches(sleep.id);
  const stop = [...view.querySelectorAll('button')].find(button => button.textContent === 'Stop');
  if (!stop) fail('no Stop button for a running launch', view.textContent);
  await window.stopLaunch(sleep.id);
  await wait(300);
  await window.renderLaunches();
  if (!view.textContent.includes('stopped (terminated)')) fail('stopped state', view.textContent);

  // A service is followed until it is ready, then opens through the
  // server's proxy in a new tab.
  await window.showArtifacts('default', runID, 'job-1', '');
  window.document.getElementById('artifact-preview').scrollIntoView = () => {};
  await window.openArtifact(entries['many'], '', 'directory');
  await window.chooseArtifactLaunch(entries['many'], '', 'Viewer');
  await window.startArtifactLaunch(entries['many'], '', 'Viewer', false);
  if (!panelText().includes('Viewer is ready.')) fail('service readiness', panelText());
  const open = document.querySelector('#artifact-launch-panel .launch-open');
  if (!open || open.getAttribute('target') !== '_blank' || !/^\/_launch\/[0-9a-f]{16}\/$/.test(open.getAttribute('href'))) fail('Open link', open && open.outerHTML);
  const served = await (await fetch(base + open.getAttribute('href') + 'page')).text();
  if (!served.startsWith('path=' + open.getAttribute('href') + 'page')) fail('proxied page', served);
  await window.showLaunches();
  if (!view.querySelector('.launch-open')) fail('Open in Launches', view.innerHTML);
`)
}

// A job that names no paths, such as one whose tracker writes its default
// store, still lists its working directory, so a viewer can be launched on
// it, from Artifacts or from a run page's Launch.
func TestWebLaunchWorkingDirectoryWithoutCandidatesInBrowser(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	f := newArtifactFixture(t)
	paths, err := stateinternal.ResolveProjectPaths(f.baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	attemptDir := filepath.Join(paths.RunsDir, f.runID, "job-2", "attempts", stateinternal.MakeAttemptID(f.runID, "job-2", 0))
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(attemptDir, stateinternal.ArtifactsFileName), artifact.Record{Version: 7, WorkingDirectory: f.work}); err != nil {
		t.Fatal(err)
	}
	options := launchOptions(t, f.baseDir, launchCommand("Count", `ls "$ROTARI_LAUNCHER_ARTIFACT" | wc -l`, launcher.TargetDirectory))
	runLaunchBrowserScript(t, options, f, `
  await window.showArtifacts('default', runID, 'job-2', '');
  const rows = [...document.querySelectorAll('#artifact-view .artifact-table tbody tr')];
  if (rows.length !== 1 || rows[0].children[1].textContent !== '.') fail('working directory row', document.getElementById('artifact-view').innerHTML);
  await wait(300);
  if (JSON.stringify(launchers(rows[0])) !== '["Table viewer","Count"]') fail('working directory buttons', launchers(rows[0]));

  // A run page's Launch opens Artifacts at the working directory's row of
  // buttons.
  window.HTMLElement.prototype.scrollIntoView = () => {};
  await window.launchOnWorkingDirectory('default', runID, 'job-1', '');
  await wait(300);
  const focused = document.querySelector('#artifact-view tr.artifact-row-focus');
  if (!focused || focused.dataset.entry !== 'wd') fail('Launch focus', document.getElementById('artifact-view').innerHTML);
  if (JSON.stringify(launchers(focused)) !== '["Table viewer","Count"]') fail('Launch buttons', launchers(focused));
`)
	// A working directory missing on the Web server's host is explained.
	if err := stateinternal.WriteJSON(filepath.Join(attemptDir, stateinternal.ArtifactsFileName), artifact.Record{Version: 7, WorkingDirectory: filepath.Join(f.work, "gone")}); err != nil {
		t.Fatal(err)
	}
	runLaunchBrowserScript(t, options, f, `
  await window.launchOnWorkingDirectory('default', runID, 'job-2', '');
  const reason = document.querySelector('#artifact-view .launch-reason')?.textContent || '';
  if (!reason.includes(work + '/gone is missing on the Web server') || document.querySelector('.artifact-row-focus')) fail('missing working directory', document.getElementById('artifact-view').innerHTML);
`)
}

// A run page's toolbar has one Files button, which browses the directory
// holding the basedir and its children, each row with a button per command
// that accepts it.
func TestWebRunFilesInBrowser(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	f := newArtifactFixture(t)
	options := launchOptions(t, f.baseDir,
		launchCommand("Count", `ls "$ROTARI_LAUNCHER_ARTIFACT" | wc -l`, launcher.TargetDirectory),
		launchCommand("Cat", `cat "$ROTARI_LAUNCHER_ARTIFACT"`, launcher.TargetFile))
	runLaunchBrowserScript(t, options, f, `
  const parent = '`+filepath.Dir(f.baseDir)+`';
  const baseName = '`+filepath.Base(f.baseDir)+`';
  window.HTMLElement.prototype.scrollIntoView = () => {};
  const toolbar = new window.DOMParser().parseFromString(window.runFilesButton('default', runID), 'text/html').querySelectorAll('button');
  if ([...toolbar].map(button => button.textContent).join() !== 'Files') fail('toolbar buttons', [...toolbar].map(button => button.outerHTML));
  await window.showRunFiles('default', runID);
  await wait(300);
  const view = document.getElementById('artifact-view');
  if (view.hidden || !view.querySelector('.artifact-preview-title').textContent.includes(parent)) fail('title', view.innerHTML);
  if (view.querySelector('.artifact-up')) fail('Up above the basedir\'s parent', view.innerHTML);
  if (JSON.stringify(titleLaunchers()) !== '["Table viewer","Count"]') fail('title buttons', titleLaunchers());
  const row = (name) => [...view.querySelectorAll('#artifact-preview tbody tr')].find(row => row.children[1].textContent === name);
  if (!row(baseName) || JSON.stringify(launchers(row(baseName))) !== '["Table viewer","Count"]') fail('basedir row', view.innerHTML);

  // A child directory opens with Up, and its rows launch on it.
  row(baseName).querySelector('.artifact-open').click();
  await wait(300);
  if (!view.querySelector('.artifact-preview-title').textContent.includes(parent + '/' + baseName) || !view.querySelector('.artifact-up')) fail('child title', view.innerHTML);
  [...view.querySelectorAll('.artifact-preview-title .artifact-launchers button')].find(button => button.textContent === 'Count').click();
  await wait(300);
  for (const want of ['Count', parent + '/' + baseName + ' (directory)', 'Working directory' + parent + '/' + baseName, 'run ' + runID]) {
    if (!panelText().includes(want)) fail('confirmation lacks ' + want, panelText());
  }
  if (panelText().includes('job ')) fail('confirmation names a job', panelText());
  await window.startArtifactLaunch('basedir_parent', baseName, 'Count', false);
  if (!panelText().includes('Started Count.')) fail('start message', panelText());
  view.querySelector('.artifact-up').click();
  await wait(300);
  if (!row(baseName)) fail('Up', view.innerHTML);
`)
}

func TestWebLaunchExplainsUnavailableInBrowser(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	f := newArtifactFixture(t)
	disabled := testOptions(f.baseDir, true)
	disabled.LauncherFile = filepath.Join("/cfg", "rotari", launcher.FileName)
	runLaunchBrowserScript(t, disabled, f, `
  if (!document.getElementById('launches-button').hidden) fail('Launches button shown while launching is disabled');
  await window.showArtifacts('default', runID, 'job-1', '');
  window.document.getElementById('artifact-preview').scrollIntoView = () => {};
  await window.openArtifact(entries['data.csv'], '', 'file');
  await wait(300);
  // The rows keep the Web UI's own viewers, and the view says why nothing
  // can be launched.
  if (JSON.stringify(titleLaunchers()) !== '["Table viewer"]') fail('viewers while launching is disabled', titleLaunchers());
  const note = document.querySelector('#artifact-view .launch-unavailable-note')?.textContent || '';
  if (document.querySelectorAll('#artifact-view .launch-unavailable-note').length !== 1) fail('one explanation', document.getElementById('artifact-view').innerHTML);
  for (const want of ['Launching is disabled on this server.', 'rotari web --allow-exec', '/cfg/rotari/launchers.toml', 'rotari config --launchers']) {
    if (!note.includes(want)) fail('explanation lacks ' + want, note);
  }
`)
	directoryOnly := launchOptions(t, f.baseDir, launchCommand("TensorBoard", "tb", launcher.TargetDirectory))
	runLaunchBrowserScript(t, directoryOnly, f, `
  await window.showArtifacts('default', runID, 'job-1', '');
  window.document.getElementById('artifact-preview').scrollIntoView = () => {};
  await window.openArtifact(entries['data.csv'], '', 'file');
  await wait(300);
  // A file no command accepts has only its viewers; a directory has the
  // command.
  if (JSON.stringify(titleLaunchers()) !== '["Table viewer"]') fail('file buttons', titleLaunchers());
  await window.openArtifact(entries['many'], '', 'directory');
  await wait(300);
  if (JSON.stringify(titleLaunchers()) !== '["Table viewer","TensorBoard"]') fail('directory buttons', titleLaunchers());
  if (document.querySelector('.launch-unavailable-note')) fail('explanation while launching is available');
`)
	missing := launchOptions(t, f.baseDir,
		launchCommand("Missing", `rotari-surely-missing-tool "$ROTARI_LAUNCHER_ARTIFACT"`, launcher.TargetFile),
		launchCommand("Cat", `cat "$ROTARI_LAUNCHER_ARTIFACT"`, launcher.TargetFile))
	runLaunchBrowserScript(t, missing, f, `
  // A command whose program is missing stays visible but disabled, with
  // the reason.
  await window.showArtifacts('default', runID, 'job-1', '');
  window.document.getElementById('artifact-preview').scrollIntoView = () => {};
  await window.openArtifact(entries['data.csv'], '', 'file');
  await wait(300);
  const buttons = [...document.querySelectorAll('#artifact-preview .artifact-preview-title .artifact-launchers button')];
  const missingButton = buttons.find(button => button.textContent === 'Missing');
  const catButton = buttons.find(button => button.textContent === 'Cat');
  if (!missingButton || !missingButton.disabled || missingButton.title !== 'Missing: rotari-surely-missing-tool was not found on PATH') fail('missing command button', missingButton && missingButton.outerHTML);
  if (!catButton || catButton.disabled) fail('available command button', catButton && catButton.outerHTML);
`)
}

func TestWebLauncherConfigInBrowser(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, true)
	options.LauncherFile = filepath.Join(t.TempDir(), "config", "rotari", launcher.FileName)
	runLaunchBrowserScript(t, options, f, `
  const settings = [...document.querySelectorAll('#settings-actions button')].map(button => button.textContent);
  if (!settings.includes('Launcher config') || !settings.includes('Generate launcher config')) fail('settings buttons', settings);
  window.history.pushState({}, '', '/project/default/run/' + runID);
  window.addConfigButton();
  const runSettings = [...document.querySelectorAll('#settings-actions button')].map(button => button.textContent);
  if (!runSettings.includes('Launcher config') || !runSettings.includes('Generate launcher config')) fail('run page settings buttons', runSettings);
  window.history.pushState({}, '', '/');
  window.addConfigButton();
  await window.showLauncherConfig();
  const view = document.getElementById('launch-view');
  if (view.hidden || !document.getElementById('modal-log').hidden) fail('launcher config view toggling');
  if (view.querySelector('.launch-table thead').textContent.trim() !== 'NameTargetsModeNotesCommand') fail('launcher config table headings', view.querySelector('.launch-table thead').textContent);
  if (!view.textContent.includes('(does not exist)') || !view.textContent.includes('TensorBoard') || !view.textContent.includes('built in') ||
      !view.textContent.includes('start rotari web --allow-exec')) fail('missing file view', view.textContent);
  const generate = view.querySelector('.launch-choices button');
  if (!generate || generate.textContent !== 'Generate launcher config') fail('missing-file action placement', view.innerHTML);
  await window.generateLauncherConfig();
  if (!view.textContent.includes('launcher config generated') || view.textContent.includes('(does not exist)') ||
      !view.querySelector('.launcher-config-editor textarea').value.includes('# TensorBoard is built in')) fail('generated view', view.textContent);
  const editor = view.querySelector('.launcher-config-editor textarea');
  editor.value += '\n[[launcher]]\nname = "Count"\ncommand = "wc -l"\ntargets = ["file"]\nmode = "command"\n';
  editor.dispatchEvent(new window.Event('input'));
  const save = view.querySelector('.launcher-config-editor button');
  if (!save || save.disabled) fail('launcher config save not enabled');
  await save.onclick();
  if (!view.textContent.includes('launcher config saved') ||
      !view.querySelector('.launcher-config-editor textarea').value.includes('name = "Count"')) fail('saved launcher config view', view.textContent);
  await window.generateLauncherConfig();
  if (!view.textContent.includes('launcher file already exists')) fail('second generate', view.textContent);
`)
}
