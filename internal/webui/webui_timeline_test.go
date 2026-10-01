package webui

import (
	"os/exec"
	"path/filepath"
	"strings"
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

func TestJobTimelineRendersOneBarPerPoint(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	start := strings.LastIndex(webAppChartsJS, "function renderJobTimelineScratch() {")
	if start < 0 {
		t.Fatal("job timeline renderer was not found")
	}
	end := strings.Index(webAppChartsJS[start:], "function syncTimelineBar() {")
	if end <= 0 {
		t.Fatal("job timeline renderer was not found")
	}
	script := `
const { JSDOM } = require('jsdom');
const dom = new JSDOM('<div id="app"><div class="run-environment"></div></div>');
const document = dom.window.document;
function pageParts() { return ['project', 'demo', 'run', 'run-1']; }
const state = { projects: [{ project_name: 'demo', runs: [{
  run_id: 'run-1', jobs: [{}, {}], timeline: [
    {at: '2026-10-01T10:00:00Z', pending: 2},
    {at: '2026-10-01T10:00:01Z', pending: 1, running: 1},
    {at: '2026-10-01T10:00:02Z', success: 1, running: 1},
    {at: '2026-10-02T10:00:00Z', success: 2},
  ]
}]}]};
` + webAppChartsJS[start:start+end] + `
renderJobTimelineScratch();
const bars = document.querySelectorAll('.job-timeline svg rect');
const barPositions = new Set([...bars].map(bar => bar.getAttribute('x')));
if (bars.length !== 6 || barPositions.size !== 4) {
  console.error('rendered bars = ' + bars.length + ' across ' + barPositions.size + ' time points, want 6 segments across 4 points');
  process.exit(1);
}
const timeLabels = [...document.querySelectorAll('.job-timeline svg text')]
  .filter(label => label.getAttribute('y') === '236')
  .map(label => label.textContent);
if (timeLabels.length !== 4 || !timeLabels[0].startsWith('start · ') || timeLabels[0] === timeLabels[3]) {
  console.error('timeline labels = ' + JSON.stringify(timeLabels) + ', want a distinct start label and disambiguated repeated clock times');
  process.exit(3);
}
if (document.querySelector('.job-timeline .meta').textContent !== 'time → / share ↑') process.exit(2);
`
	if output, err := exec.Command("node", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("job timeline render failed: %v\n%s", err, output)
	}
}
