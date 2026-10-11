package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
)

func TestFinishedRunWithoutJobTimestampsStillHasTimelineEnd(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	commands := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), commands); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{
		RunID: "run-1", Status: "success",
		StartedAt: "2026-10-01T10:00:00Z", FinishedAt: "2026-10-01T10:00:10Z",
		Results: []model.JobResult{{ID: "job-1", ExitCode: 0}},
	}); err != nil {
		t.Fatal(err)
	}

	state, err := siteFor(baseDir).loadWebState(baseDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Queues) != 1 || len(state.Queues[0].Runs) != 1 {
		t.Fatalf("state projects/runs = %d/%d, want 1/1", len(state.Queues), len(state.Queues[0].Runs))
	}
	points := state.Queues[0].Runs[0].Timeline
	if len(points) != 2 {
		t.Fatalf("timeline points = %#v, want start and run-end points", points)
	}
	if points[0].Pending != 1 || points[0].Finished != 0 {
		t.Fatalf("initial timeline point = %#v, want one pending job", points[0])
	}
	if points[1].Finished != 1 || points[1].Success != 1 || points[1].Pending != 0 {
		t.Fatalf("final timeline point = %#v, want one completed success", points[1])
	}
}

// The job timeline draws one lane per job in job order: an executed job's
// span coloured by its status, a note for a job not started or carried, an
// axis that starts at the run timeline's initial point (dated when the run
// crosses a day) and marks offsets from it, and labels that show the job's
// row.
func TestJobTimelineDrawsOneLanePerJob(t *testing.T) {
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
const run = {
  run_id: 'run-1', status: 'failed', lifecycle: 'failed',
  timeline: [{ at: '2026-10-01T10:00:00Z', pending: 3 }, { at: '2026-10-02T10:00:00Z', success: 1, failed: 1 }],
  lanes: [
    { job_id: 'a', start: '2026-10-01T10:00:00Z', end: '2026-10-01T22:00:00Z' },
    { job_id: 'b', start: '2026-10-01T22:00:00Z', end: '2026-10-02T10:00:00Z' },
    { job_id: 'c' },
    { job_id: 'd', carried: true },
  ],
  jobs: [
    { id: 'a', name: 'train', result: { exit_code: 0 } },
    { id: 'b', name: 'eval', result: { exit_code: 3 } },
    { id: 'c', name: 'report' },
    { id: 'd', name: 'prepare', result: { exit_code: 0 }, origin: { run_id: 'run-0', job_id: 'd' } },
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
    const document = dom.window.document;
    const section = document.querySelector('#app section.job-timeline');
    assert(!section.classList.contains('collapsed'), 'the job timeline is not open');
    const rows = [...section.querySelectorAll('.lane-row:not(.lane-axis-row)')];
    assert.deepEqual(rows.map(row => row.querySelector('.lane-label').textContent), ['train', 'eval', 'report', 'prepare']);
    const bar = index => rows[index].querySelector('.lane-bar');
    assert(bar(0).classList.contains('tone-ok') && bar(1).classList.contains('tone-bad'));
    const at = (index, side) => parseFloat(bar(index).style[side]);
    assert.deepEqual([at(0, 'left'), at(0, 'width'), at(1, 'left'), at(1, 'width')], [0, 50, 50, 50]);
    assert(!bar(2) && rows[2].querySelector('.lane-note'), 'a job not started has a note, not a span');
    assert.equal(rows[3].querySelector('.lane-note').textContent, 'carried from an earlier run');
    const labels = [...section.querySelectorAll('.lane-axis > span')].map(span => span.textContent);
    assert.equal(labels.length, 5);
    assert(labels[0].startsWith('start · ') && /Oct|10/.test(labels[0]), 'a start in a run that crosses a day lacks its date: ' + labels[0]);
    assert.deepEqual(labels.slice(1), ['+6h 00m', '+12h 00m', '+18h 00m', '+24h 00m']);
    assert(section.querySelector('.graphic-note').textContent.startsWith('4 jobs · 24h 00m'));
    assert.deepEqual([...section.querySelectorAll('.lane-legend > span')].map(item => item.textContent), ['success', 'failed']);
    let focused = '';
    dom.window.focusMatrixJob = id => { focused = id; };
    rows[1].querySelector('.lane-label').click();
    assert.equal(focused, 'b');
    if (errors.length) throw new Error(errors.join('\n'));
  } catch (error) {
    console.error(error.stack || String(error));
    process.exit(1);
  }
  process.exit(0);
}, 100);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("job timeline lanes check failed: %v\n%s", err, output)
	}
}
