package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/web"
)

// A run report starts with the run's record: its sources, its notes, and a
// table of its jobs that shows only the environment values telling them
// apart and each job's last log line. Each job's section carries
// that job's notes.
func TestRunReportRecordsSourcesNotesAndJobTable(t *testing.T) {
	runsDir := t.TempDir()
	writeLogs := func(jobID, stdout, stderr string) string {
		dir := filepath.Join(runsDir, "run-1", jobID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, text := range map[string]string{state.StdoutFileName: stdout, state.StderrFileName: stderr} {
			if text == "" {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	long := strings.Repeat("x", reportTableLineRunes+20)
	run := web.Run{
		RunSummary: model.RunSummary{RunID: "run-1", Status: "failed", ExitCode: 1},
		Sources:    []model.SourceRevision{{VCS: "git", Root: "/work/repo", CommitID: "b32d3ad88f5c0000"}},
		Notes: []model.RunNote{
			{At: "2026-10-10T11:20:19Z", Text: "Sweep **LR** with\n- warmup on"},
			{At: "2026-10-10T11:21:00Z", JobID: "a", AttemptID: "att-a-1", Text: "diverged at step 12"},
			{At: "2026-10-10T11:22:00Z", JobID: "a", AttemptID: "att-a-0", Text: "first try ran out of memory"},
			{At: "2026-10-10T11:23:00Z", JobID: "b", AttemptID: "att-b-0", Text: "baseline"},
		},
		Jobs: []web.Job{
			{ID: "a", Name: "train-LR0.1", AttemptID: "att-a-1", ExecutionStatus: "failed", Environment: []string{"LR=0.1", "BS=32", "SEED=7"},
				Result: &model.JobResult{ID: "a", ExitCode: 3}, AttemptDir: writeLogs("a", "config lr=0.1\nstep 1\n", "Traceback\nValueError: a | `b`\n\n")},
			{ID: "b", Name: "train-LR0.01", AttemptID: "att-b-0", ExecutionStatus: "success", Environment: []string{"LR=0.01", "BS=32"},
				Result: &model.JobResult{ID: "b", ExitCode: 0}, AttemptDir: writeLogs("b", "config lr=0.01\n"+long+"\n", "")},
			{ID: "c", Name: "train-LR1", ExecutionStatus: "pending", Environment: []string{"LR=1", "BS=32"}},
		},
	}
	report := formatRunAIReport(state.ProjectPaths{ProjectName: "demo", RunsDir: runsDir}, run, false)

	table := strings.Join([]string{
		"| Job | LR | SEED | Status | Exit | Last log line |",
		"| --- | --- | --- | --- | --- | --- |",
		"| train-LR0.1 | `0.1` | `7` | failed | 3 | `` ValueError: a \\| `b` `` |",
		"| train-LR0.01 | `0.01` | - | success | 0 | `" + strings.Repeat("x", reportTableLineRunes-1) + "…` |",
		"| train-LR1 | `1` | - | pending | - | - |",
	}, "\n")
	for _, want := range []string{
		"- Source: `git b32d3ad88f5c (clean) in /work/repo`\n",
		"\n## Notes\n\n**2026-10-10 ",
		"\n\nSweep **LR** with\n- warmup on\n",
		"\n## Jobs\n\n" + table + "\n",
		"\n### Notes\n\n**2026-10-10 ",
		"\n\ndiverged at step 12\n",
		" (attempt `att-a-0`)**\n\nfirst try ran out of memory\n",
		"\n\nbaseline\n",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	// The run's notes leave out its jobs' notes, and a job's section holds
	// only its own.
	runNotes := report[strings.Index(report, "\n## Notes"):strings.Index(report, "\n## Jobs")]
	if strings.Contains(runNotes, "diverged") || strings.Contains(runNotes, "baseline") {
		t.Errorf("run notes include job notes:\n%s", runNotes)
	}
	jobA := report[strings.Index(report, "\n## Job: train-LR0.1\n"):strings.Index(report, "\n## Job: train-LR0.01\n")]
	if !strings.Contains(jobA, "diverged at step 12") || strings.Contains(jobA, "baseline") || strings.Contains(jobA, "(attempt `att-a-1`)") {
		t.Errorf("job a's section has the wrong notes:\n%s", jobA)
	}
	if strings.Index(report, "\n## Jobs\n") > strings.Index(report, "\n## Job: ") {
		t.Errorf("job table does not come before the job sections:\n%s", report)
	}
}

func TestReportTableCodeQuotesAnyText(t *testing.T) {
	for _, test := range []struct{ text, want string }{
		{"", "-"},
		{"plain", "`plain`"},
		{"a|b", "`a\\|b`"},
		{"x `y` z", "``x `y` z``"},
		{"`edge`", "`` `edge` ``"},
		{"two `` ticks", "```two `` ticks```"},
	} {
		if got := reportTableCode(test.text); got != test.want {
			t.Errorf("reportTableCode(%q) = %q, want %q", test.text, got, test.want)
		}
	}
}

// A retry run carries the results of jobs it did not run. Its report reads a
// carried job's log from the attempt that produced the result, as show and
// the filters do, for the job table and for a failed job's log section.
func TestRunReportReadsCarriedJobLogsFromTheirOrigin(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	write := func(path string, value any) {
		t.Helper()
		if err := state.WriteJSON(path, value); err != nil {
			t.Fatal(err)
		}
	}
	writeLog := func(runID, jobID, text string) {
		t.Helper()
		dir := filepath.Join(paths.RunsDir, runID, jobID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, state.StdoutFileName), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(paths.QueueFile, model.Queue{})
	commands := []model.QueuedCommand{
		{ID: "ok", Name: "train-ok", Command: []string{"python", "train.py"}},
		{ID: "bad", Name: "train-bad", Command: []string{"python", "train.py"}},
		{ID: "fixed", Name: "train-fixed", Command: []string{"python", "train.py"}},
	}
	write(filepath.Join(paths.RunsDir, "run-1", "commands.json"), model.Queue{Commands: commands})
	write(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{RunID: "run-1", Status: "failed", ExitCode: 1, Results: []model.JobResult{
		{ID: "ok", ExitCode: 0}, {ID: "bad", ExitCode: 2, Error: "exit status 2"}, {ID: "fixed", ExitCode: 1, Error: "exit status 1"},
	}})
	writeLog("run-1", "ok", "config ok\nval_acc=0.7810\n")
	writeLog("run-1", "bad", "config bad\nRuntimeError: loss became NaN\n")
	writeLog("run-1", "fixed", "IndexError: list index out of range\n")

	// The retry reran only "fixed" and carried the other two from run-1.
	retried := append([]model.QueuedCommand(nil), commands...)
	retried[0].Origin = &model.JobOrigin{RunID: "run-1", JobID: "ok", Status: model.StatusSuccess}
	retried[1].Origin = &model.JobOrigin{RunID: "run-1", JobID: "bad", Status: model.StatusFailed}
	write(filepath.Join(paths.RunsDir, "run-2", "commands.json"), model.Queue{Commands: retried})
	write(filepath.Join(paths.RunsDir, "run-2", "summary.json"), model.RunSummary{RunID: "run-2", Status: "failed", ExitCode: 1, Results: []model.JobResult{
		{ID: "ok", ExitCode: 0}, {ID: "bad", ExitCode: 2, Error: "exit status 2"}, {ID: "fixed", ExitCode: 0},
	}})
	writeLog("run-2", "fixed", "val_acc=0.8680\n")

	report, err := Build(testStore(), paths, "run-2", "", false, "", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| train-ok | success (carried) | 0 | `val_acc=0.7810` |",
		"| train-bad | failed (carried) | 2 | `RuntimeError: loss became NaN` |",
		"| train-fixed | success | 0 | `val_acc=0.8680` |",
		"config bad\nRuntimeError: loss became NaN",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}
