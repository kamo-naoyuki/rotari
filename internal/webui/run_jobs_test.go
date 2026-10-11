package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A run's jobs table shows each command under its job name, leaves out
// columns that are the same for every job until the Columns menu turns them
// on, and ends each row with its actions: the log, the diagnosis of a failed
// job, and a menu with the rest.
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
const state = { base_dir: '/state', projects: [{ project_name: 'default', queue: { commands: [] }, runs: [run] }] };
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/project/default/run/run-1',
  virtualConsole,
  beforeParse(window) {
    window.setInterval = () => 1;
    window.fetch = async (url) => {
      if (url === '/api/state') return { ok: true, json: async () => state };
      throw new Error('unexpected fetch: ' + url);
    };
  },
});
setTimeout(() => {
  try {
    const window = dom.window, document = window.document;
    const table = () => document.querySelector('#app table.run-jobs');
    const headers = () => [...table().querySelectorAll('thead th')];
    const shown = () => headers().filter(th => !th.hidden && th.dataset.sort).map(th => th.dataset.sort);
    const row = id => document.querySelector('tr[data-job-id="' + id + '"]');
    const cell = (id, key) => row(id).children[headers().findIndex(th => th.dataset.sort === key)];

    // Executor is the same for every job; Finished starts hidden; the rest
    // differ or are always shown.
    assert.deepEqual(shown(), ['name', 'status', 'exit', 'attempt', 'id', 'hosts', 'working_directory', 'started', 'duration']);
    assert.equal(cell('a', 'executor').hidden, true, 'a hidden column keeps its cells, hidden');

    // The command is under the job name.
    assert.equal(cell('a', 'name').querySelector('.job-command').textContent, 'sh -c decode a');
    assert(cell('a', 'name').querySelector('.job-command [title="Copy command"]'), 'the command has no copy button');

    // Attempt IDs are shortened, with the full ID in the title and copy.
    const attempt = cell('a', 'attempt');
    assert.equal(attempt.querySelector('.mono').title, run.jobs[0].attempt_id);
    assert(attempt.querySelector('.mono').textContent.startsWith('…'));
    assert.equal(attempt.querySelector('[title="Copy attempt ID"]').dataset.copyValue, run.jobs[0].attempt_id);

    // Times on the run's start date drop the date; Duration sorts by seconds.
    assert.equal(cell('a', 'started').textContent, '05:52:22');
    assert.equal(cell('c', 'started').textContent, '2026-10-12 00:00:01 JST');
    assert.equal(cell('a', 'duration').dataset.sortValue, '185');
    window.sortTable(table(), 'duration', 'job');
    assert.equal(table().querySelector('tbody tr').dataset.jobId, 'b', 'Duration does not sort by seconds');

    // Actions come last: Log; Diagnosis only on the failed job; Notes when
    // the job has some; Report, Path, and Artifacts in the menu.
    const actions = id => row(id).lastElementChild;
    assert(actions('a').classList.contains('actions'));
    const labels = id => [...actions(id).querySelectorAll('.action-buttons > button')].map(b => b.textContent);
    assert.deepEqual(labels('a'), ['Log']);
    assert.deepEqual(labels('b'), ['Log', 'Diagnosis']);
    assert.deepEqual(labels('c'), ['Log', 'Notes (1)']);
    const menu = id => actions(id).querySelector('details.row-menu');
    assert.deepEqual([...menu('a').querySelectorAll('button')].map(b => b.textContent), ['Report', 'Path', 'Artifacts', 'Notes']);

    // An open menu survives a re-render and closes on a press outside.
    menu('a').open = true;
    window.setPopupMenuOpen(menu('a').dataset.menuKey, true);
    window.render();
    assert(menu('a').open, 'the row menu closed on re-render');
    document.body.dispatchEvent(new window.PointerEvent('pointerdown', { bubbles: true }));
    assert(!menu('a').open, 'the row menu did not close on a press outside');

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
