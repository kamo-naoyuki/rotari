package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A run's jobs table shows each command under its job name, leaves out
// columns that are the same for every job until the Columns menu turns them
// on, starts with Job ID hidden, offers a copy of each host and path value,
// and ends each row with all its actions as buttons, without a menu, Launch
// among them where launching is available. The toolbar and the run's sections around
// it keep the viewer's choices.
func TestWebRunJobsTable(t *testing.T) {
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
const job = (id, extra) => ({
  id, name: 'decode-' + id, command: ['sh', '-c', 'decode ' + id], executor: 'local', working_directory: '/work/' + id,
  attempt_id: 'att_20261010-205222-3185eff0-' + id + '-0', submitted_at: '2026-10-11 05:52:22 JST',
  finished_at: '2026-10-11 05:55:27 JST', elapsed: '3m 05s', ...extra,
});
const run = {
  run_id: 'run-1', status: 'failed', lifecycle: 'failed', started_at: '2026-10-11 05:52:20 JST',
  jobs: [
    job('a', { result: { exit_code: 0, hosts: ['node1'] } }),
    job('b', { result: { exit_code: 2, hosts: ['node2'], diagnosis_status: 'final', diagnoses: [{ name: 'oom' }] }, elapsed: '5s' }),
    job('c', { result: { exit_code: 0 }, submitted_at: '2026-10-12 00:00:01 JST', note_labels: ['n1'] }),
  ],
};
let launchers = { available: true };
const state = { base_dir: '/state', projects: [{ project_name: 'default', queue: { commands: [] }, runs: [run] }] };
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/project/default/run/run-1',
  virtualConsole,
  beforeParse(window) {
    window.localStorage.setItem('rotari-run-sections', JSON.stringify({ 'job-timeline': false }));
    window.setInterval = () => 1;
    window.fetch = async (url) => {
      if (url === '/api/state') return { ok: true, json: async () => state };
      if (url === '/api/launchers') return { ok: true, json: async () => launchers };
      throw new Error('unexpected fetch: ' + url);
    };
  },
});
setTimeout(async () => {
  try {
    const window = dom.window, document = window.document;
    const table = () => document.querySelector('#app table.run-jobs');
    const headers = () => [...table().querySelectorAll('thead th')];
    const shown = () => headers().filter(th => !th.hidden && th.dataset.sort).map(th => th.dataset.sort);
    const row = id => document.querySelector('tr[data-job-id="' + id + '"]');
    const cell = (id, key) => row(id).children[headers().findIndex(th => th.dataset.sort === key)];

    // Executor is the same for every job; Job ID and Finished start hidden;
    // the rest differ or are always shown.
    assert.deepEqual(shown(), ['name', 'status', 'exit', 'attempt', 'hosts', 'working_directory', 'started', 'duration']);
    assert.equal(cell('a', 'executor').hidden, true, 'a hidden column keeps its cells, hidden');

    // The command is under the job name.
    assert.equal(cell('a', 'name').querySelector('.job-command').textContent, 'sh -c decode a');
    assert(cell('a', 'name').querySelector('.job-command [title="Copy command"]'), 'the command has no copy button');

    // Hosts and the working directory can be copied; a job without hosts
    // has nothing to copy.
    const copy = (id, key) => cell(id, key).querySelector('.identity-copy');
    assert.equal(copy('a', 'hosts').dataset.copyValue, 'node1');
    assert.equal(copy('a', 'working_directory').dataset.copyValue, '/work/a');
    assert.equal(copy('c', 'hosts'), null, 'an empty Hosts cell has a copy button');

    // Attempt IDs are shown by their end, with the full ID kept in the text,
    // title, and copy.
    const attempt = cell('a', 'attempt');
    assert.equal(attempt.querySelector('.id-tail').title, run.jobs[0].attempt_id);
    assert.equal(attempt.querySelector('.id-tail').textContent, run.jobs[0].attempt_id, 'the ID is shortened only by CSS');
    assert.equal(attempt.querySelector('[title="Copy attempt ID"]').dataset.copyValue, run.jobs[0].attempt_id);

    // Times on the run's start date drop the date; Duration sorts by seconds.
    assert.equal(cell('a', 'started').textContent, '05:52:22');
    assert.equal(cell('c', 'started').textContent, '2026-10-12 00:00:01 JST');
    assert.equal(cell('a', 'duration').dataset.sortValue, '185');
    window.sortTable(table(), 'duration', 'job');
    assert.equal(table().querySelector('tbody tr').dataset.jobId, 'b', 'Duration does not sort by seconds');

    // Actions come last, all in the row: Log, Artifacts, Launch where
    // launching is available, Diagnosis only on the failed job, Notes
    // (disabled without notes), Report, and Path.
    const actions = id => row(id).lastElementChild;
    assert(actions('a').classList.contains('actions'));
    const labels = id => [...actions(id).querySelectorAll('.action-buttons > button')].map(b => b.textContent);
    assert.deepEqual(labels('a'), ['Log', 'Artifacts', 'Launch', 'Notes', 'Report', 'Path']);
    assert(actions('a').querySelector('.view-notes').disabled, 'Notes without notes is not disabled');
    assert.deepEqual(labels('b'), ['Log', 'Artifacts', 'Launch', 'Diagnosis', 'Notes', 'Report', 'Path']);
    assert.deepEqual(labels('c'), ['Log', 'Artifacts', 'Launch', 'Notes (1)', 'Report', 'Path']);
    assert(!actions('a').querySelector('details'), 'a row still has a menu');
    launchers = { available: false };
    await window.loadLauncherAvailability();
    window.render();
    assert.deepEqual(labels('a'), ['Log', 'Artifacts', 'Notes', 'Report', 'Path'], 'Launch offered where launching is unavailable');
    launchers = { available: true };
    await window.loadLauncherAvailability();
    window.render();

    // An open menu survives a re-render and closes on a press outside.
    const menu = () => document.querySelector('.run-jobs-bar .columns-menu');
    menu().open = true;
    window.setPopupMenuOpen(menu().dataset.menuKey, true);
    window.render();
    assert(menu().open, 'the Columns menu closed on re-render');
    document.body.dispatchEvent(new window.PointerEvent('pointerdown', { bubbles: true }));
    assert(!menu().open, 'the Columns menu did not close on a press outside');

    // The toolbar: selection buttons above, actions on the checked jobs
    // below, enabled and counted as jobs are checked.
    const button = label => [...document.querySelectorAll('#app .toolbar button')].find(b => b.textContent === label);
    const count = () => document.querySelector('.run-jobs-bar .selection-count').textContent;
    assert(button('Create queue').disabled && button('Unselect all').disabled && count() === '');
    assert(!button('Delete run').disabled && !button('Cancel run'), 'a finished run can be deleted, not cancelled');
    button('Select failed').click();
    assert.equal(count(), '1 selected');
    assert(row('b').querySelector('.job-selection').checked);
    button('Select all').click();
    assert.equal(count(), '3 selected');
    assert(!button('Create queue').disabled && !button('Append to queue').disabled && !button('Report').disabled);
    button('Unselect all').click();
    assert(button('Create queue').disabled && count() === '');

    // A section closed before stays closed (the job timeline otherwise starts
    // open), the others start closed, and a section opened now is remembered.
    const section = kind => document.querySelector('#app section.run-graphic.' + kind);
    const open = kind => !section(kind).classList.contains('collapsed');
    assert(!open('job-timeline') && !open('run-environment'));
    assert(!section('run-statistics'), 'Run statistics repeats the Jobs card');
    section('run-environment').querySelector('.graphic-toggle').click();
    window.render();
    assert(open('run-environment'), 'the opened section closed on re-render');
    assert.deepEqual(JSON.parse(window.localStorage.getItem('rotari-run-sections')), { 'job-timeline': false, 'run-environment': true });

    // The Columns menu turns columns on and off, and remembers the choice.
    const columnsMenu = document.querySelector('.run-jobs-bar .columns-menu');
    const choice = key => columnsMenu.querySelector('input[data-column="' + key + '"]');
    assert(choice('executor').closest('label').textContent.includes('same for all'));
    choice('finished').checked = true;
    choice('finished').dispatchEvent(new window.Event('change'));
    choice('working_directory').checked = false;
    choice('working_directory').dispatchEvent(new window.Event('change'));
    assert(shown().includes('finished') && !shown().includes('working_directory'), shown().join());
    window.render();
    assert(shown().includes('finished') && !shown().includes('working_directory'), 'the choice was not kept');
    assert.equal(JSON.parse(window.localStorage.getItem('rotari-run-columns')).finished, true);
    if (errors.length) throw new Error(errors.join('\n'));
  } catch (error) {
    console.error(error.stack || String(error));
    process.exit(1);
  }
  process.exit(0);
}, 100);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("run jobs table check failed: %v\n%s", err, output)
	}
}
