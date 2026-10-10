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
// apart and each job's first and last log lines. Each job's section carries
// that job's notes.
func TestRunReportRecordsSourcesNotesAndJobTable(t *testing.T) {
	writeLogs := func(stdout, stderr string) string {
		dir := t.TempDir()
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
				Result: &model.JobResult{ID: "a", ExitCode: 3}, AttemptDir: writeLogs("config lr=0.1\nstep 1\n", "Traceback\nValueError: a | `b`\n\n")},
			{ID: "b", Name: "train-LR0.01", AttemptID: "att-b-0", ExecutionStatus: "success", Environment: []string{"LR=0.01", "BS=32"},
				Result: &model.JobResult{ID: "b", ExitCode: 0}, AttemptDir: writeLogs("config lr=0.01\n"+long+"\n", "")},
			{ID: "c", Name: "train-LR1", ExecutionStatus: "pending", Environment: []string{"LR=1", "BS=32"}, AttemptDir: t.TempDir()},
		},
	}
	report := formatRunAIReport(state.ProjectPaths{ProjectName: "demo"}, run, false)

	table := strings.Join([]string{
		"| Job | LR | SEED | Status | Exit | First log line | Last log line |",
		"| --- | --- | --- | --- | --- | --- | --- |",
		"| train-LR0.1 | `0.1` | `7` | failed | 3 | `config lr=0.1` | `` ValueError: a \\| `b` `` |",
		"| train-LR0.01 | `0.01` | - | success | 0 | `config lr=0.01` | `" + strings.Repeat("x", reportTableLineRunes-1) + "…` |",
		"| train-LR1 | `1` | - | pending | - | - | - |",
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
