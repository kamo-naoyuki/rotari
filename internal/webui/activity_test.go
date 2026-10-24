package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rotari-dev/rotari/internal/model"
	stateinternal "github.com/rotari-dev/rotari/internal/state"
)

func writeActivityRun(t *testing.T, baseDir, project, runID, name string, started, finished time.Time) stateinternal.ProjectPaths {
	t.Helper()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, project)
	if err != nil {
		t.Fatal(err)
	}
	summary := model.RunSummary{RunID: runID, RunName: name, Status: "finished", StartedAt: started.Format(time.RFC3339Nano)}
	if !finished.IsZero() {
		summary.FinishedAt = finished.Format(time.RFC3339Nano)
	}
	if err := stateinternal.WriteJSON(filepath.Join(paths.RunsDir, runID, "summary.json"), summary); err != nil {
		t.Fatal(err)
	}
	return paths
}

func getActivity(t *testing.T, options Options, from, to string) (*httptest.ResponseRecorder, activityResponse) {
	t.Helper()
	query := url.Values{}
	if from != "" {
		query.Set("from", from)
	}
	if to != "" {
		query.Set("to", to)
	}
	recorder := httptest.NewRecorder()
	Handler(options).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/activity?"+query.Encode(), nil))
	var response activityResponse
	if recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("activity response is not JSON: %v\n%s", err, recorder.Body.String())
		}
	}
	return recorder, response
}

// /api/activity lists, across every registered basedir, the runs whose span
// meets the range: a run crossing the range's start, one inside it, and a
// running run started before it, with where each ran; a missing basedir and
// one whose runs cannot be read are reported, not dropped.
func TestActivityListsRunsAcrossBasedirs(t *testing.T) {
	day := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	first, second, broken := t.TempDir(), t.TempDir(), t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")

	writeActivityRun(t, first, "train", "run-before", "old", day.Add(-5*time.Hour), day.Add(-4*time.Hour))
	crossing := writeActivityRun(t, first, "train", "run-crossing", "overnight", day.Add(-time.Hour), day.Add(2*time.Hour))
	if err := stateinternal.WriteJSON(filepath.Join(crossing.RunsDir, "run-crossing", "context.json"), model.RunContext{CWD: "/work/train", Hostname: "node7"}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.AppendRunNote(filepath.Join(crossing.RunsDir, "run-crossing"), model.RunNote{At: "1", Text: "beam 8\nmore"}); err != nil {
		t.Fatal(err)
	}
	writeActivityRun(t, first, "eval", "run-inside", "", day.Add(10*time.Hour), day.Add(11*time.Hour))
	writeActivityRun(t, second, "default", "run-after", "", day.Add(25*time.Hour), day.Add(26*time.Hour))

	// An active run in the second basedir, started before the range.
	paths, err := stateinternal.ResolveProjectPaths(second, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-active"), 0o755); err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-active"}); err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-active", RunName: "sweep", Host: host, StartedAt: day.Add(-2 * time.Hour).Format(time.RFC3339Nano)}); err != nil {
		t.Fatal(err)
	}

	brokenPaths := writeActivityRun(t, broken, "bad", "run-bad", "", day, day.Add(time.Hour))
	if err := os.WriteFile(filepath.Join(brokenPaths.RunsDir, "run-bad", "summary.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	options := testOptions(first, false)
	options.BaseDirs = []string{second, broken, missing}
	recorder, response := getActivity(t, options, day.Format(time.RFC3339), day.Add(24*time.Hour).Format(time.RFC3339))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body.String())
	}

	ids := map[string]string{}
	for _, entry := range response.BaseDirs {
		ids[entry.ID] = entry.Path
		switch entry.Path {
		case missing:
			if !entry.Missing {
				t.Errorf("missing basedir not marked: %+v", entry)
			}
		case broken:
			if entry.Error == "" || !strings.Contains(entry.Error, "run-bad") {
				t.Errorf("unreadable basedir not reported: %+v", entry)
			}
		default:
			if entry.Missing || entry.Error != "" {
				t.Errorf("readable basedir reported a problem: %+v", entry)
			}
		}
	}
	if len(response.BaseDirs) != 4 {
		t.Fatalf("basedirs = %+v, want all four registered", response.BaseDirs)
	}
	var got []string
	byID := map[string]activityRun{}
	for _, run := range response.Runs {
		got = append(got, ids[run.BaseDirID]+"|"+run.Project+"|"+run.RunID)
		byID[run.RunID] = run
	}
	sort.Strings(got)
	want := []string{first + "|eval|run-inside", first + "|train|run-crossing", second + "|default|run-active"}
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("runs = %v, want %v", got, want)
	}
	crossed := byID["run-crossing"]
	if crossed.RunName != "overnight" || crossed.CWD != "/work/train" || crossed.Host != "node7" || crossed.NoteCount != 1 || crossed.FirstNote != "beam 8" || crossed.ExitCode == nil {
		t.Errorf("crossing run = %+v", crossed)
	}
	if active := byID["run-active"]; active.Lifecycle != "running" || active.RunName != "sweep" || active.ExitCode != nil {
		t.Errorf("active run = %+v", active)
	}
}

// A missing, malformed, reversed, or over-long range is rejected, not
// clamped.
func TestActivityRejectsBadRanges(t *testing.T) {
	options := testOptions(t.TempDir(), false)
	day := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	at := func(d time.Duration) string { return day.Add(d).Format(time.RFC3339) }
	for _, test := range []struct {
		name, from, to, want string
	}{
		{"no from", "", at(time.Hour), "from is required"},
		{"no to", at(0), "", "to is required"},
		{"malformed", "yesterday", at(time.Hour), "invalid from"},
		{"reversed", at(time.Hour), at(0), "to must be after from"},
		{"empty", at(0), at(0), "to must be after from"},
		{"too long", at(0), at(46 * 24 * time.Hour), "at most 45 days"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder, _ := getActivity(t, options, test.from, test.to)
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), test.want) {
				t.Fatalf("status = %d, body %q; want 400 with %q", recorder.Code, recorder.Body.String(), test.want)
			}
		})
	}
	if recorder, _ := getActivity(t, options, at(0), at(45*24*time.Hour)); recorder.Code != http.StatusOK {
		t.Fatalf("a 45-day range: status = %d, body %s", recorder.Code, recorder.Body.String())
	}
}

// A static export carries an Activity page and every run of its basedir,
// which the page sorts into days itself.
func TestStaticExportIncludesActivity(t *testing.T) {
	baseDir := t.TempDir()
	day := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	writeActivityRun(t, baseDir, "train", "run-old", "", day.AddDate(0, -3, 0), day.AddDate(0, -3, 0).Add(time.Hour))
	writeActivityRun(t, baseDir, "train", "run-new", "", day, day.Add(time.Hour))
	outputDir := filepath.Join(t.TempDir(), "site")
	if err := siteFor(baseDir).generateStaticWeb(outputDir); err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(outputDir, "activity", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"window.__ROTARI_STATIC_ACTIVITY__", `"run_id":"run-old"`, `"run_id":"run-new"`} {
		if !strings.Contains(string(page), want) {
			t.Errorf("static Activity page lacks %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(outputDir, "activity", "web_styles.css")); err != nil {
		t.Fatalf("static Activity stylesheet missing: %v", err)
	}
}
