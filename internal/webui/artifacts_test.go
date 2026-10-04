package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
)

func writeArtifactFixture(t *testing.T, baseDir string) (string, string, string) {
	t.Helper()
	paths, err := stateinternal.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260922-070308-0d83bd39"
	runDir := filepath.Join(paths.RunsDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"true"}}}}
	if err := stateinternal.WriteJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "a.csv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := stateinternal.MakeAttemptID(runID, "job-1", 0)
	retry := stateinternal.MakeAttemptID(runID, "job-1", 1)
	for attemptID, name := range map[string]string{first: "first.csv", retry: "a.csv"} {
		record := artifact.Record{Version: 7, WorkingDirectory: work, Result: artifact.Result{Candidates: []artifact.Candidate{{
			Path: filepath.Join(work, name), Basis: artifact.BasisWorkingDirectory,
			Sources: []artifact.Source{{Kind: artifact.KindArgument, Value: name, Rule: artifact.RuleExtension}},
		}}}}
		attemptDir := filepath.Join(runDir, "job-1", "attempts", attemptID)
		if err := os.MkdirAll(attemptDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := stateinternal.WriteJSON(filepath.Join(attemptDir, stateinternal.ArtifactsFileName), record); err != nil {
			t.Fatal(err)
		}
	}
	return runID, first, work
}

func getArtifacts(t *testing.T, baseDir, query string) (int, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, false)).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/artifacts?"+query, nil))
	return recorder.Code, recorder.Body.String()
}

func TestWebArtifactsAPI(t *testing.T) {
	baseDir := t.TempDir()
	runID, first, work := writeArtifactFixture(t, baseDir)

	for name, test := range map[string]struct {
		query string
		path  string
		kind  string
	}{
		"latest attempt":   {query: "project_name=default&run_id=" + runID + "&job_id=job-1", path: "a.csv", kind: jobstatus.ArtifactFile},
		"selected attempt": {query: "project_name=default&run_id=" + runID + "&job_id=job-1&attempt_id=" + first, path: "first.csv", kind: jobstatus.ArtifactMissing},
	} {
		t.Run(name, func(t *testing.T) {
			code, body := getArtifacts(t, baseDir, test.query)
			var listing jobstatus.ArtifactListing
			if code != http.StatusOK || json.Unmarshal([]byte(body), &listing) != nil {
				t.Fatalf("response = (%d, %s)", code, body)
			}
			if !listing.Recorded || listing.WorkingDirectory != work || len(listing.Entries) != 1 ||
				listing.Entries[0].DisplayPath != test.path || listing.Entries[0].Type != test.kind || listing.Entries[0].Origin != "argument" {
				t.Fatalf("listing = %+v", listing)
			}
		})
	}
	for name, query := range map[string]string{
		"missing job":           "project_name=default&run_id=" + runID,
		"unsafe job":            "project_name=default&run_id=" + runID + "&job_id=..%2Fx",
		"unknown job":           "project_name=default&run_id=" + runID + "&job_id=job-2",
		"unknown run":           "project_name=default&run_id=20260101-000000-aaaaaaaa&job_id=job-1",
		"another job's attempt": "project_name=default&run_id=" + runID + "&job_id=job-1&attempt_id=" + stateinternal.MakeAttemptID(runID, "job-2", 0),
		"malformed attempt":     "project_name=default&run_id=" + runID + "&job_id=job-1&attempt_id=../x",
	} {
		t.Run(name, func(t *testing.T) {
			if code, body := getArtifacts(t, baseDir, query); code == http.StatusOK {
				t.Fatalf("response = (%d, %s), want an error", code, body)
			}
		})
	}
	recorder := httptest.NewRecorder()
	Handler(testOptions(baseDir, false)).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/artifacts?project_name=default&run_id="+runID+"&job_id=job-1", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", recorder.Code)
	}
}

// TestWebArtifactsFormatMatchesCLI checks that the Web UI lays out a
// listing line for line as `show -j JOB --artifacts` prints it (see
// TestWriteArtifactListing in cmd/rotari), and requests the right attempt.
func TestWebArtifactsFormatMatchesCLI(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	script := `
const vm = require('vm');
const code = ` + strconv.Quote(webAppLogsJS) + `;
const output = { textContent: '' };
const requests = [];
const context = {
  console, URLSearchParams, modalOutput: output,
  document: { getElementById: () => ({ dataset: {}, querySelector: () => ({}) }) },
  ensureModalOutput: () => output,
  openOutputModal() {}, isCompactOutput: () => true,
  fetch: async (url) => { requests.push(url); return { ok: true, json: async () => listing }; },
};
const listing = {
  recorded: true, working_directory: '/work',
  entries: [
    { type: 'directory', display_path: 'results', origin: 'a.yaml: out_dir' },
    { type: 'missing', display_path: '/workspace/x.csv', origin: '--in' },
  ],
  diagnostics: [{ source: '/x.yaml', message: 'not inspected: no such file' }, { message: 'limit reached' }],
};
vm.createContext(context);
// The file defines the modal helpers; replace them with stubs after loading.
vm.runInContext(code + '; this.formatArtifactListing = formatArtifactListing; this.showArtifacts = showArtifacts; followTimer = null;' +
  ' openOutputModal = function () {}; ensureModalOutput = function () { return modalOutput; }; isCompactOutput = function () { return true; };', context);
const checks = [
  [context.formatArtifactListing(listing),
   'Artifacts: relative to /work\n  directory  results  (a.yaml: out_dir)\n  missing    /workspace/x.csv  (--in)\nDiscovery notes:\n  /x.yaml: not inspected: no such file\n  limit reached\n'],
  [context.formatArtifactListing({ recorded: false }), 'Artifacts: (not recorded)'],
  [context.formatArtifactListing({ recorded: true }), 'Artifacts: none found\n'],
];
for (const [got, want] of checks) {
  if (got !== want) { console.error(JSON.stringify({ got, want })); process.exit(1); }
}
(async () => {
  await context.showArtifacts('demo', 'run-1', 'job-1', 'att_x');
  if (requests[0] !== '/api/artifacts?project_name=demo&run_id=run-1&job_id=job-1&attempt_id=att_x' || !output.textContent.startsWith('Artifacts: relative to /work')) {
    console.error(JSON.stringify({ requests, output: output.textContent }));
    process.exit(1);
  }
})();
`
	path := filepath.Join(t.TempDir(), "artifacts.js")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", path).CombinedOutput(); err != nil {
		t.Fatalf("artifact formatting: %v\n%s", err, output)
	}
	if !strings.Contains(webAppCoreJS, `onclick="showArtifacts(`) {
		t.Fatal("the job table has no Artifacts button")
	}
}
