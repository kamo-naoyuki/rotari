package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/runlist"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestRunLifecycleFilters(t *testing.T) {
	runs := make([]runlist.Run, 0, len(runLifecycleFilterValues))
	for _, lifecycle := range runLifecycleFilterValues {
		runs = append(runs, runlist.Run{Lifecycle: lifecycle})
	}
	for _, lifecycle := range runLifecycleFilterValues {
		for _, option := range []string{"--filter-" + lifecycle, "--" + lifecycle} {
			t.Run(option, func(t *testing.T) {
				fs := flag.NewFlagSet("runs", flag.ContinueOnError)
				options := cliRunLifecycleOptions(fs)
				if err := fs.Parse([]string{option}); err != nil {
					t.Fatal(err)
				}
				filtered := filterRunsByLifecycle(runs, options)
				if len(filtered) != 1 || filtered[0].Lifecycle != lifecycle {
					t.Fatalf("filterRunsByLifecycle(%s) = %+v", option, filtered)
				}
			})
		}
	}

	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	options := cliRunLifecycleOptions(fs)
	if err := fs.Parse([]string{"--filter-running", "--finished"}); err != nil {
		t.Fatal(err)
	}
	filtered := filterRunsByLifecycle(runs, options)
	if len(filtered) != 2 || filtered[0].Lifecycle != "running" || filtered[1].Lifecycle != "finished" {
		t.Fatalf("OR filters returned %+v", filtered)
	}
}

func writeListsRun(t *testing.T, baseDir, projectName, runID string) state.ProjectPaths {
	t.Helper()
	return writeListsRunAt(t, baseDir, projectName, runID, time.Date(2026, 10, 8, 10, 0, 1, 0, time.UTC))
}

func writeListsRunAt(t *testing.T, baseDir, projectName, runID string, finishedAt time.Time) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{
		RunID: runID, RunName: "saved run", Status: "finished", ExitCode: 0,
		StartedAt: finishedAt.Add(-time.Second).Format(time.RFC3339Nano), FinishedAt: finishedAt.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestListsAndShowDetachedClientLabels(t *testing.T) {
	for _, test := range []struct {
		name, mode, reason, label string
	}{
		{"async", model.RunClientModeAsync, model.RunClientReasonAsync, "detached (async)"},
		{"ctrl-d", model.RunClientModeSync, model.RunClientReasonCtrlD, "detached (Ctrl-D)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			const runID = "20261009-120000-12345678"
			paths := writeListsDetachedRun(t, runID, test.mode, test.reason)
			rows, err := runlist.Collect([]string{paths.BaseDir}, "demo")
			if err != nil || len(rows) != 1 || rows[0].ClientLabel != test.label {
				t.Fatalf("run rows = %#v, %v; want %q", rows, err, test.label)
			}
			var output bytes.Buffer
			code := captureShowStdout(t, &output, func() int {
				return cmdShow([]string{"--basedir", paths.BaseDir, "--project-name", "demo", "--run-id", runID})
			})
			if code != 0 || !strings.Contains(output.String(), test.label) {
				t.Fatalf("show exit = %d, output = %q; want %q", code, output.String(), test.label)
			}
		})
	}
}

func writeListsDetachedRun(t *testing.T, runID, mode, reason string) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for path, value := range map[string]any{
		paths.MetaFile: model.Meta{Phase: "running", LastRunID: runID},
		paths.LockFile: model.LockInfo{PID: os.Getpid(), RunID: runID, Host: host},
		filepath.Join(paths.RunsDir, runID, state.RunClientStatusFileName): model.RunClientStatus{Mode: mode, State: model.RunClientDetached, Reason: reason},
	} {
		if err := writeJSON(path, value); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

func TestListsRunsRejectsExplicitInvalidProject(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../outside", "a/b", `a\b`, "/absolute"} {
		for _, positional := range []bool{false, true} {
			t.Run(name+"/"+map[bool]string{false: "flag", true: "positional"}[positional], func(t *testing.T) {
				args := []string{"--basedir", t.TempDir()}
				if positional {
					args = append(args, name)
				} else {
					args = append(args, "--project-name", name)
				}
				var output bytes.Buffer
				code, stderr := captureStderr(t, func() int {
					return captureShowStdout(t, &output, func() int { return cmdRuns(args) })
				})
				if code != 1 || !strings.Contains(stderr, "invalid project name") {
					t.Fatalf("exit = %d, stderr = %q, stdout = %q", code, stderr, output.String())
				}
			})
		}
	}
}

func TestListsRunsHintWorksWithRegisteredRun(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := filepath.Join(t.TempDir(), "state with spaces")
	projectName, runID := "project with spaces", "20261008-100000-00000001"
	// Keep the finished run inside the default window regardless of the date.
	paths := writeListsRunAt(t, baseDir, projectName, runID, time.Now().UTC())
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "commands.json"), model.Queue{}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := captureShowStdout(t, &output, func() int {
		return cmdRuns([]string{"--basedir", baseDir, "--project-name", projectName})
	}); code != 0 {
		t.Fatalf("runs exit = %d, stdout = %q", code, output.String())
	}
	const prefix = "  rotari show "
	for _, line := range strings.Split(output.String(), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		args := strings.Fields(strings.TrimPrefix(line, prefix))
		for index, arg := range args {
			if arg == "RUN_ID" {
				args[index] = runID
			}
		}
		output.Reset()
		if code := captureShowStdout(t, &output, func() int { return cmdShow(args) }); code != 0 {
			t.Fatalf("hint exit = %d, stdout = %q", code, output.String())
		}
		if !strings.Contains(output.String(), runID) {
			t.Fatalf("hint did not inspect the listed run: %s", output.String())
		}
		return
	}
	t.Fatalf("run list omitted its show hint: %s", output.String())
}

func TestListsRunsTableAlignmentAndHint(t *testing.T) {
	for _, colored := range []bool{false, true} {
		for _, multipleBases := range []bool{false, true} {
			t.Run(map[bool]string{false: "plain", true: "color"}[colored]+"/"+map[bool]string{false: "single", true: "multiple"}[multipleBases], func(t *testing.T) {
				oldCheck := terminalCheck
				terminalCheck = func(*os.File) bool { return colored }
				t.Cleanup(func() { terminalCheck = oldCheck })
				rows := []runListRow{
					{BaseDir: "/long-state-directory-with-more-than-32-characters", Project: "project-longer-than-twelve", RunID: "run-id-longer-than-twenty-four-characters", Name: strings.Repeat("long-name", 6), Status: "interrupted", ExitCode: "17", StartedAt: "26/10/08_10:00:00", Finished: "26/10/08_10:00:01"},
					{BaseDir: "/short", Project: "p", RunID: "r", Name: "n", Status: "success", ExitCode: "0", StartedAt: "-", Finished: "-"},
				}
				if !multipleBases {
					rows[1].BaseDir = rows[0].BaseDir
				}
				var output bytes.Buffer
				captureShowStdout(t, &output, func() int { printRunRows(rows, multipleBases); return 0 })
				text := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(output.String(), "")
				lines := strings.Split(text, "\n")
				headers := []string{"PROJECT", "RUN ID", "NAME", "STATUS", "EXIT CODE", "STARTED", "FINISHED"}
				for i, row := range rows {
					values := []string{row.Project, row.RunID, row.Name, row.Status, row.ExitCode, row.StartedAt, row.Finished}
					offset := 0
					for col, value := range values {
						index := strings.Index(lines[i+1][offset:], value) + offset
						if index != strings.Index(lines[0], headers[col]) {
							t.Errorf("%s column: row index %d, header index %d:\n%s", headers[col], index, strings.Index(lines[0], headers[col]), text)
						}
						offset = index + len(value)
					}
				}
				if !strings.Contains(text, "rotari show -r RUN_ID") {
					t.Errorf("hint does not show the basic run selector:\n%s", text)
				}
			})
		}
	}
}

func TestListsRunsJSON(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	finishedAt := time.Now().Add(-time.Hour).UTC()
	paths := writeListsRunAt(t, baseDir, "demo", "run-done", finishedAt)
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-done", state.RunClientStatusFileName), model.RunClientStatus{Mode: model.RunClientModeAsync, State: model.RunClientCompleted, Reason: model.RunClientReasonAsync}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-done", "context.json"), model.RunContext{CWD: "/work/repo", Hostname: "node7"}); err != nil {
		t.Fatal(err)
	}
	if err := state.AppendRunNote(filepath.Join(paths.RunsDir, "run-done"), model.RunNote{At: "1", Text: "baseline"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-active"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-active"}); err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-active", RunName: "sweep", Host: host, StartedAt: startedAt}); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := captureShowStdout(t, &output, func() int { return cmdRuns([]string{"--basedir", baseDir, "--json"}) }); code != 0 {
		t.Fatalf("runs --json exit = %d: %s", code, output.String())
	}
	var runs []struct {
		BaseDir      string                `json:"base_dir"`
		Project      string                `json:"project_name"`
		RunID        string                `json:"run_id"`
		RunName      string                `json:"run_name"`
		Lifecycle    string                `json:"lifecycle"`
		ClientStatus model.RunClientStatus `json:"client_status"`
		ClientLabel  string                `json:"client_label"`
		ExitCode     *int                  `json:"exit_code"`
		StartedAt    string                `json:"started_at"`
		FinishedAt   string                `json:"finished_at"`
		CWD          string                `json:"cwd"`
		Host         string                `json:"host"`
		NoteCount    int                   `json:"note_count"`
	}
	if err := json.Unmarshal(output.Bytes(), &runs); err != nil {
		t.Fatalf("runs --json is not JSON: %v\n%s", err, output.String())
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %+v", runs)
	}
	active, done := runs[0], runs[1]
	if active.RunID != "run-active" || active.Lifecycle != "running" || active.RunName != "sweep" || active.ExitCode != nil || active.StartedAt != startedAt || active.FinishedAt != "" || active.BaseDir != baseDir || active.Project != "demo" {
		t.Errorf("active run = %+v", active)
	}
	if done.RunID != "run-done" || done.Lifecycle != "finished" || done.ExitCode == nil || *done.ExitCode != 0 || done.FinishedAt != finishedAt.Format(time.RFC3339Nano) ||
		done.ClientStatus.Mode != model.RunClientModeAsync || done.ClientLabel != "async (completed)" ||
		done.CWD != "/work/repo" || done.Host != "node7" || done.NoteCount != 1 {
		t.Errorf("finished run = %+v", done)
	}

	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return cmdRuns([]string{"--basedir", t.TempDir(), "--json"}) }); code != 0 || strings.TrimSpace(output.String()) != "[]" {
		t.Fatalf("empty runs --json = %d %q, want []", code, output.String())
	}
}
