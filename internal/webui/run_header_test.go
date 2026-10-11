package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A run page opens with the run's name and status, a facts line, the run's
// own notes, the retry command, and four summary cards.
func TestWebRunPageHeader(t *testing.T) {
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
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/',
  virtualConsole,
  beforeParse(window) {
    window.setInterval = () => 1;
    window.fetch = async () => ({ ok: true, json: async () => ({ base_dir: '/state', projects: [] }) });
  },
});
setTimeout(() => {
  try {
    const window = dom.window, document = window.document;
    const run = {
      run_id: 'run-1', run_name: 'try1', status: 'failed', lifecycle: 'failed', exit_code: 1,
      started_at: '2026-10-11 05:52:22 JST', finished_at: '2026-10-11 05:52:23 JST', cwd: '/work',
      context: { hostname: 'node7', load_samples: [{ at: '2026-10-11T05:52:22Z', one: 1.5, five: 2, fifteen: 2.25 }] },
      notes: [
        { at: '2026-10-11T05:52:00Z', text: 'Raise the **beam** to 8 <script>' },
        { at: '2026-10-11T05:53:00Z', job_id: 'j2', text: 'job note' },
      ],
      lineage_summary: {
        run: { run_id: 'run-1', elapsed: '1.2s' },
        counts: { jobs: 3, succeeded: 2, failed: 1 },
        failures: [{ cause: 'exit 2', count: 1 }],
      },
      jobs: [
        { id: 'j1', command: ['true'], result: { exit_code: 0 } },
        { id: 'j2', command: ['true'], result: { exit_code: 0 } },
        { id: 'j3', command: ['false'], result: { exit_code: 2 } },
      ],
    };
    window.renderRun({ project_name: 'default', queue: { commands: [] }, runs: [run] }, 'run-1');
    const title = document.getElementById('page-title');
    assert.equal(title.firstChild.textContent.trim(), 'try1');
    assert(title.querySelector('.status-pill.tone-bad'), 'the title has no status pill');
    const summary = document.getElementById('summary').textContent;
    for (const want of ['Run ID: run-1', 'Status: failed', 'Lifecycle: failed', 'Host: node7', 'Working directory: /work', 'Started: 2026-10-11 05:52:22 JST']) {
      assert(summary.includes(want), 'facts lack ' + want + ': ' + summary);
    }
    const notes = [...document.querySelectorAll('#app .run-note')];
    assert.equal(notes.length, 1, 'only the run\'s own note belongs at the top');
    assert(notes[0].querySelector('strong')?.textContent === 'beam', 'the note is not rendered as Markdown');
    assert(!notes[0].querySelector('script'), 'note text is not escaped');
    const cards = Object.fromEntries([...document.querySelectorAll('#app .run-card')].map(card => [
      card.querySelector('.run-card-label').textContent, card.querySelector('.run-card-value').textContent]));
    assert.deepEqual(Object.keys(cards), ['Jobs', 'Failure causes', 'Wall time', 'Load at end']);
    assert.equal(cards['Jobs'], '3 2 succeeded · 1 failed · 0 in progress · 0 pending');
    assert.equal(cards['Failure causes'], '1 exit 2 1');
    assert.equal(cards['Wall time'], '1.2s');
    assert.equal(cards['Load at end'], '1.50 / 2.00 / 2.25');
    const guide = document.querySelector('#app .execution-guide');
    assert(guide && guide.dataset.copyText.includes("rotari retry -r 'run-1'"), 'the retry command is missing');
    assert(guide.querySelector('.command-guide-copy'), 'the retry command has no copy button');
    assert(!document.querySelector('#app .toolbar a'), 'the breadcrumbs replace the Back link');
    // A running run counts its jobs in progress and those waiting to start.
    window.renderRun({ project_name: 'default', queue: { commands: [] }, runs: [{
      run_id: 'run-2', status: 'running', lifecycle: 'running', running: true,
      jobs: [
        { id: 'k1', command: ['true'], result: { exit_code: 0 } },
        { id: 'k2', command: ['true'] },
        { id: 'k3', command: ['true'], scheduler_state: 'pending' },
        { id: 'k4', command: ['true'], result: { exit_code: 1, error: 'blocked by failed dependency' } },
      ],
    }] }, 'run-2');
    const jobsCard = [...document.querySelectorAll('#app .run-card')].find(card => card.querySelector('.run-card-label').textContent === 'Jobs');
    assert.equal(jobsCard.querySelector('.run-card-value').textContent, '4 1 succeeded · 0 failed · 1 in progress · 1 pending · 1 blocked');
    if (errors.length) throw new Error(errors.join('\n'));
  } catch (error) {
    console.error(error.stack || String(error));
    process.exit(1);
  }
  process.exit(0);
}, 100);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("run page header check failed: %v\n%s", err, output)
	}
}
