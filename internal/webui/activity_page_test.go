package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The Activity page opens on the latest day with runs when today has none,
// and says so; it groups that day's lanes by basedir, one per project, gives
// the busiest projects their own colours, links each bar to its run page,
// lists where each project ran, and reports a missing basedir.
func TestActivityPage(t *testing.T) {
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
// Runs two days ago, local time, so today has none.
const day = new Date(); day.setHours(0, 0, 0, 0); day.setDate(day.getDate() - 2);
const at = hours => new Date(day.getTime() + hours * 3600000).toISOString();
const run = (basedir, project, id, start, end, lifecycle, extra = {}) => ({
  basedir_id: basedir, project_name: project, run_id: id, lifecycle, exit_code: lifecycle === 'failed' ? 1 : 0,
  started_at: at(start), ends_at: at(end), finished_at: at(end), note_count: 0, ...extra,
});
const activity = {
  now: new Date().toISOString(),
  basedirs: [
    { id: 'A', path: '/work/asr/.rotari-state', parent: 'asr', name: '.rotari-state', current: true },
    { id: 'B', path: '/work/tts/.rotari-state', parent: 'tts', name: '.rotari-state' },
    { id: 'C', path: '/gone/.rotari-state', parent: 'gone', name: '.rotari-state', missing: true },
  ],
  runs: [
    run('A', 'train', 'r1', 9, 11, 'finished', { run_name: 'baseline', cwd: '/src/asr', sources: ['git 0123456789ab (clean) in /src/asr'] }),
    run('A', 'train', 'r2', 13, 14, 'failed', { run_name: 'lr-3e-4', cwd: '/src/asr-wt/agent', started_by: { interface: 'mcp', actor: 'claude-code' } }),
    run('A', 'eval', 'r3', 15, 16, 'finished'),
    run('B', 'default', 'r4', 10, 12, 'finished', { first_note: 'beam 8', note_count: 1 }),
  ],
};
const state = { base_dir: '/work/asr/.rotari-state', projects: [] };
const requests = [];
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/activity/',
  virtualConsole,
  beforeParse(window) {
    window.setInterval = () => 1;
    window.fetch = async (url) => {
      if (url === '/api/state') return { ok: true, json: async () => state };
      if (String(url).startsWith('/api/activity?')) { requests.push(String(url)); return { ok: true, json: async () => activity }; }
      throw new Error('unexpected fetch: ' + url);
    };
  },
});
setTimeout(() => {
  try {
    const document = dom.window.document;
    const page = document.querySelector('#app .activity-page');
    assert(page, 'the Activity page is not drawn');
    assert(requests.length >= 1 && /from=.*&to=/.test(requests[0]), requests.join());
    assert(document.querySelector('.sidebar-activity-link').classList.contains('active'));
    // Today has no runs: the latest day with runs is selected, and said so.
    assert(page.querySelector('.activity-day .activity-notice').textContent.includes('the latest day with runs'));
    const selected = page.querySelector('.activity-cell.selected');
    assert.equal(Number(selected.dataset.day), day.getTime());
    assert(selected.querySelector('.activity-count').textContent.startsWith('4'));
    assert(selected.querySelector('.activity-mark-bad').textContent === '✕1');
    // Lanes: basedir A (newest end first: eval, train), then B.
    const headers = [...page.querySelectorAll('.activity-basedir')].map(h => h.textContent);
    assert(headers[0].includes('asr') && headers[0].includes('2 projects · 3 runs'), headers.join(' | '));
    assert(headers[1].includes('tts') && headers[1].includes('1 project · 1 run'));
    assert.deepEqual([...page.querySelectorAll('.activity-lane-label')].map(l => l.textContent), ['eval', 'train', 'default']);
    const bars = [...page.querySelectorAll('.activity-bar')];
    assert.equal(bars.length, 4);
    const failed = bars.find(bar => bar.dataset.run === 'A/train/r2');
    assert(failed.classList.contains('tone-bad'));
    assert.equal(failed.getAttribute('href'), '/_basedir/A/project/train/run/r2');
    assert.equal(bars.find(bar => bar.dataset.run === 'B/default/r4').getAttribute('href'), '/_basedir/B/project/default/run/r4');
    // The busiest project gets the first colour.
    const chip = name => [...page.querySelectorAll('.activity-lane-label')].find(l => l.textContent === name).querySelector('.activity-chip').style.background;
    assert.equal(chip('train'), 'var(--c1)');
    // Where you worked: train's last run, both working directories.
    const train = [...page.querySelectorAll('.activity-project-table tbody tr')].find(row => row.textContent.includes('train'));
    assert(train.textContent.includes('lr-3e-4') && train.textContent.includes('/src/asr') && train.textContent.includes('/src/asr-wt/agent'));
    // The missing basedir is reported.
    assert(page.querySelector('.activity-problems').textContent.includes('no longer exists'));
    // Hovering a bar shows its note.
    dom.window.document.querySelector('[data-run="B/default/r4"]').dispatchEvent(new dom.window.MouseEvent('pointerover', { bubbles: true }));
    assert(page.querySelector('.activity-card').textContent.includes('beam 8'));
    // The actor filter narrows runs without repainting projects; an agent's
    // run is marked.
    assert(failed.querySelector('.activity-actor-mark'), 'the agent run is not marked');
    const select = page.querySelector('.activity-actor-select');
    assert.deepEqual([...select.options].map(o => o.textContent), ['all runs', 'claude-code', 'unlabelled']);
    select.value = 'claude-code';
    select.dispatchEvent(new dom.window.Event('change'));
    assert.deepEqual([...page.querySelectorAll('.activity-bar')].map(bar => bar.textContent), ['◆lr-3e-4']);
    assert.equal(chip('train'), 'var(--c1)', 'filtering repainted a project');
    select.value = '-';
    select.dispatchEvent(new dom.window.Event('change'));
    assert.equal(page.querySelectorAll('.activity-bar').length, 3);
    if (errors.length) throw new Error(errors.join('\\n'));
  } catch (error) {
    console.error(error.stack || String(error));
    process.exit(1);
  }
  process.exit(0);
}, 300);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("activity page check failed: %v\n%s", err, output)
	}
}
