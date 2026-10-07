package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func writeListsRun(t *testing.T, baseDir, projectName, runID string) state.ProjectPaths {
	t.Helper()
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), model.RunSummary{
		RunID: runID, RunName: "saved run", Status: "finished", ExitCode: 0,
		StartedAt: "2026-10-08T10:00:00Z", FinishedAt: "2026-10-08T10:00:01Z",
	}); err != nil {
		t.Fatal(err)
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

func TestListsRunsReadsOnlySelectedProject(t *testing.T) {
	baseDir := t.TempDir()
	selected := writeListsRun(t, baseDir, "selected", "run-selected")
	broken := writeListsRun(t, baseDir, "aaa-broken", "run-broken")
	for _, file := range []string{selected.QueueFile, broken.QueueFile, broken.MetaFile,
		filepath.Join(broken.RunsDir, "run-broken", "summary.json")} {
		if err := writeTestFile(file, []byte("{broken")); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := collectRunRows([]string{baseDir}, "selected")
	if err != nil || len(rows) != 1 || rows[0].RunID != "run-selected" {
		t.Fatalf("rows = %#v, err = %v", rows, err)
	}
}

func TestListsRunsDoesNotReadQueues(t *testing.T) {
	baseDir := t.TempDir()
	paths := writeListsRun(t, baseDir, "selected", "run-selected")
	if err := writeTestFile(paths.QueueFile, []byte("{broken")); err != nil {
		t.Fatal(err)
	}
	rows, err := collectRunRows([]string{baseDir}, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %#v, err = %v", rows, err)
	}
}

func TestListsRunsSummaryErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{"malformed", "{broken", ""},
		{"newer-version", `{"state_version":999999,"run_id":"run-bad"}`, "upgrade"},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths := writeListsRun(t, baseDir, "selected", "run-bad")
			if err := writeTestFile(filepath.Join(paths.RunsDir, "run-bad", "summary.json"), []byte(test.data)); err != nil {
				t.Fatal(err)
			}
			for _, active := range []bool{false, true} {
				if active {
					if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-bad"}); err != nil {
						t.Fatal(err)
					}
				}
				_, err := collectRunRows([]string{baseDir}, "selected")
				if err == nil || !strings.Contains(err.Error(), "run-bad") || !strings.Contains(err.Error(), "selected") || !strings.Contains(err.Error(), test.want) {
					t.Fatalf("active = %v, err = %v", active, err)
				}
			}
		})
	}
}

func TestListsRunsPhaseOverridesSummary(t *testing.T) {
	for _, test := range []struct {
		name string
		pid  int
		want string
	}{
		{"active", os.Getpid(), "running"},
		{"interrupted", -1, "interrupted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths := writeListsRun(t, baseDir, "selected", "run-current")
			writeListsRun(t, baseDir, "selected", "run-old")
			host, err := os.Hostname()
			if err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-current"}); err != nil {
				t.Fatal(err)
			}
			if err := writeJSON(paths.LockFile, model.LockInfo{PID: test.pid, RunID: "run-current", Host: host}); err != nil {
				t.Fatal(err)
			}
			for _, missing := range []bool{false, true} {
				if missing {
					if err := os.Remove(filepath.Join(paths.RunsDir, "run-current", "summary.json")); err != nil {
						t.Fatal(err)
					}
				}
				rows, err := collectRunRows([]string{baseDir}, "selected")
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range rows {
					want := "finished"
					if row.RunID == "run-current" {
						want = test.want
						if !missing && (row.Name != "saved run" || row.ExitCode != "0") {
							t.Errorf("summary details lost: %#v", row)
						}
					}
					if row.Status != want {
						t.Errorf("missing summary = %v, row = %#v, want status %q", missing, row, want)
					}
				}
			}
		})
	}
}

func TestListsRunsEmptyAndMissingScopes(t *testing.T) {
	baseDir := t.TempDir()
	for _, filter := range []string{"", "selected"} {
		if rows, err := collectRunRows([]string{baseDir}, filter); err != nil || len(rows) != 0 {
			t.Fatalf("no projects: rows = %#v, err = %v", rows, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "selected"), 0700); err != nil {
		t.Fatal(err)
	}
	if rows, err := collectRunRows([]string{baseDir}, "selected"); err != nil || len(rows) != 0 {
		t.Fatalf("no runs: rows = %#v, err = %v", rows, err)
	}
	if _, err := collectRunRows([]string{baseDir}, "missing"); err == nil || !strings.Contains(err.Error(), `project "missing" not found`) {
		t.Fatalf("missing named project: %v", err)
	}
	otherBase := t.TempDir()
	paths := writeListsRun(t, otherBase, "elsewhere", "run-incomplete")
	if err := os.Remove(filepath.Join(paths.RunsDir, "run-incomplete", "summary.json")); err != nil {
		t.Fatal(err)
	}
	rows, err := collectRunRows([]string{baseDir, otherBase}, "elsewhere")
	if err != nil || len(rows) != 1 || rows[0].Status != "incomplete" {
		t.Fatalf("named project in another basedir: rows = %#v, err = %v", rows, err)
	}
}

func TestListsRunsOrderTiesByRunID(t *testing.T) {
	baseDir := t.TempDir()
	for _, runID := range []string{"run-z", "run-a", "run-m"} {
		writeListsRun(t, baseDir, "selected", runID)
	}
	// Repeating a scope prevents ReadDir's alphabetical order from masking
	// a missing RunID tie-breaker in the final sort.
	rows, err := collectRunRows([]string{baseDir, baseDir}, "")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []string{"run-a", "run-a", "run-m", "run-m", "run-z", "run-z"} {
		if rows[i].RunID != want {
			t.Fatalf("rows = %#v", rows)
		}
	}
}

func TestListsRunsHintWorksWithoutRegistry(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := filepath.Join(t.TempDir(), "state with spaces")
	projectName, runID := "project with spaces", "20261008-100000-00000001"
	paths := writeListsRun(t, baseDir, projectName, runID)
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
			switch arg {
			case "RUN_ID":
				args[index] = runID
			case "BASEDIR":
				args[index] = baseDir
			case "PROJECT":
				args[index] = projectName
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
	t.Fatal("run list omitted its show hint")
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
				captureShowStdout(t, &output, func() int { printRunRows(rows); return 0 })
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
				if !strings.Contains(text, "rotari show --basedir BASEDIR --project-name PROJECT --run-id RUN_ID") {
					t.Errorf("hint needs an explicit location without a registry:\n%s", text)
				}
			})
		}
	}
}
