package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The project page's queue table hides columns whose values are the same
// for every queued job, as the run's jobs table does, and Save still sends
// the values of the hidden editors.
func TestWebQueueTable(t *testing.T) {
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
const job = (id, extra) => ({ id, name: 'job-' + id, command: ['echo', id], executor: 'local', working_directory: '/work', ...extra });
const project = {
  project_name: 'demo', _loaded: true, runs: [],
  queue: { commands: [
    job('a', { depends_on: ['b'], origin: { run_id: 'run-1', job_id: 'a', status: 'failed' } }),
    job('b', { origin: { run_id: 'run-1', job_id: 'b', status: 'success' } }),
  ] },
};
const state = { base_dir: '/state', projects: [project] };
const requests = [];
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/project/demo',
  virtualConsole,
  beforeParse(window) {
    window.setInterval = () => 1;
    window.alert = message => errors.push('alert: ' + message);
    window.fetch = async (url, options = {}) => {
      if (url === '/api/state') return { ok: true, json: async () => state };
      requests.push({ url, body: JSON.parse(options.body) });
      return { ok: true, text: async () => '{}', json: async () => state };
    };
  },
});
setTimeout(async () => {
  try {
    const window = dom.window, document = window.document;
    const table = () => document.querySelector('#app table.web-queue-jobs');
    const shown = () => [...table().querySelectorAll('thead th')].filter(th => !th.hidden && th.dataset.sort).map(th => th.dataset.sort);
    assert.deepEqual(shown(), ['name', 'depends', 'command', 'source_run', 'source_status']);
    assert.deepEqual([...document.querySelectorAll('#app section h2')].map(h => h.textContent), ['Queue', 'Runs', 'Project runtime']);

    // Save sends the hidden editors' values too.
    const row = table().querySelector('tr[data-job-id="a"]');
    const name = row.querySelector('.job-name-input');
    name.value = 'renamed';
    name.dispatchEvent(new window.Event('input'));
    assert(!row.querySelector('.save-job').disabled, 'Save stays disabled after an edit');
    await window.saveQueueJob('demo', 'a', row);
    const body = requests.find(request => request.url === '/api/change').body;
    assert.equal(body.set_job_name, 'renamed');
    assert.equal(body.executor, 'local');
    assert.equal(body.working_directory, '/work');
    assert.deepEqual(body.depends_on, ['b']);

    // The Columns menu shows a hidden column, remembered per browser.
    const choice = document.querySelector('.web-queue-commands .columns-menu input[data-column="working_directory"]');
    assert(choice.closest('label').textContent.includes('same for all'));
    choice.checked = true;
    choice.dispatchEvent(new window.Event('change'));
    assert(shown().includes('working_directory'));
    assert.equal(JSON.parse(window.localStorage.getItem('rotari-queue-columns')).working_directory, true);
    if (errors.length) throw new Error(errors.join('\n'));
  } catch (error) {
    console.error(error.stack || String(error));
    process.exit(1);
  }
  process.exit(0);
}, 100);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("queue table check failed: %v\n%s", err, output)
	}
}
