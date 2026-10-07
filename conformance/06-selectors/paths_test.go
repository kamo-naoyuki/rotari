package conformance

import (
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

// Contracts RES-9 and RES-10: project names, run IDs, job IDs, and attempt
// IDs are single path elements. ".", "..", absolute paths, and values
// containing "/" or "\" are rejected before filesystem access, by the CLI and
// by the Web API alike. See contracts/01-resolution-and-config.md and
// the path invariants in AGENTS.md. Empty values are not checked: an empty
// option means "not given" on the CLI, and some Web fields are optional.
//
// Each command or route is checked the same way: it fails for every unsafe
// value, nothing named after an unsafe value appears on disk, and it then
// succeeds for the valid value. The unsafe values include aliases such as
// "./NAME" and "../projects/NAME", which name the valid target once joined
// as a path, so a missing check shows up as a success rather than hiding
// behind a not-found error.

// unsafeValues returns the values that must be rejected where valid names a
// project, run, or job. parent is the directory the value is joined to, for
// the alias that climbs out of it and back in.
func unsafeValues(valid, parent string) []string {
	return []string{
		"x/escape", `x\escape`, "/escape", "../escape", "..", ".",
		"./" + valid, "../" + parent + "/" + valid,
	}
}

func TestCLIRejectsUnsafePathElements(t *testing.T) {
	covers(t, "RES-9", "RES-10")
	e := newEnv(t)
	run := e.createFinishedRun()
	queued := "queued"
	e.mustRotari("add", "-p", queued, "--", "true")
	live := e.startActiveRun("live", 1)

	// Commands run in order: suspend, resume, and cancel act on the live
	// run's job, and delete removes the finished run, so it comes last.
	cases := []struct {
		name   string
		args   func(value string) []string
		valid  string
		parent string
	}{
		{"add --project-name", func(v string) []string { return []string{"add", "-p", v, "--", "true"} }, run.project, "projects"},
		{"show --project-name", func(v string) []string { return []string{"show", "-p", v} }, run.project, "projects"},
		{"check PROJECT", func(v string) []string { return []string{"check", v} }, run.project, "projects"},
		{"jobs PROJECT", func(v string) []string { return []string{"jobs", "--basedir", e.base, v} }, run.project, "projects"},
		{"show --run-id", func(v string) []string { return []string{"show", "-p", run.project, "--run-id", v} }, run.runID, "runs"},
		{"show --job-id", func(v string) []string {
			return []string{"show", "-p", run.project, "--run-id", run.runID, "--job-id", v}
		}, run.badJob, run.runID},
		{"run --project-name", func(v string) []string { return []string{"run", "-p", v, "--quiet"} }, queued, "projects"},
		{"reset PROJECT", func(v string) []string { return []string{"reset", v, "--quiet"} }, queued, "projects"},
		{"suspend JOB_ID", func(v string) []string { return []string{"suspend", "-p", live.project, v} }, live.jobs[0], live.runID},
		{"resume JOB_ID", func(v string) []string { return []string{"resume", "-p", live.project, v} }, live.jobs[0], live.runID},
		{"cancel JOB_ID", func(v string) []string { return []string{"cancel", "-p", live.project, v} }, live.jobs[0], live.runID},
		{"delete RUN_ID", func(v string) []string { return []string{"delete", "-p", run.project, v} }, run.runID, "runs"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := e.in(t)
			for _, value := range unsafeValues(c.valid, c.parent) {
				r := e.rotari(c.args(value)...)
				if r.code == 0 {
					t.Errorf("accepted %q: %s", value, r)
				}
				if strings.Contains(r.stderr, "panic:") {
					t.Errorf("panicked on %q: %s", value, r)
				}
			}
			assertNoEscape(t, e.root)
			e.mustRotari(c.args(c.valid)...)
		})
	}
}

// webRoute is a Web API route that names a project, run, or job. valid
// holds the fields of a request that succeeds; parent names, for each field,
// the directory it is joined to.
type webRoute struct {
	method string
	path   string
	valid  map[string]string
	parent map[string]string
	extra  map[string]any
}

// send requests the route with the given fields, as a query for GET and a
// JSON body with the route's extra fields for POST.
func (route webRoute) send(e *env, base string, values map[string]string) httpResult {
	e.t.Helper()
	if route.method == "GET" {
		query := url.Values{}
		for name, value := range values {
			query.Set(name, value)
		}
		return e.httpGet(base + route.path + "?" + query.Encode())
	}
	body := map[string]any{}
	for name, value := range values {
		body[name] = value
	}
	for name, value := range route.extra {
		body[name] = value
	}
	return e.httpPostJSON(base+route.path, body)
}

func TestWebAPIRejectsUnsafePathElements(t *testing.T) {
	covers(t, "RES-9", "RES-10")
	e := newEnv(t)
	finished := e.createFinishedRun()
	queued := "queued"
	queuedJob := addedJobID(t, e.mustRotari("add", "-p", queued, "--", "true"))
	live := e.startActiveRun("live", 2)
	base := e.startWeb()

	// Each project's fields and where each field is joined.
	finishedRun := map[string]string{"project_name": finished.project, "run_id": finished.runID, "job_id": finished.badJob}
	finishedParents := map[string]string{"project_name": "projects", "run_id": "runs", "job_id": finished.runID, "attempt_id": "attempts"}
	finishedAttempt := copyValues(finishedRun)
	finishedAttempt["attempt_id"] = finished.badAttempt
	liveRun := map[string]string{"project_name": live.project, "run_id": live.runID, "job_id": live.jobs[0]}
	liveParents := map[string]string{"project_name": "projects", "run_id": "runs", "job_id": live.runID}
	queue := map[string]string{"project_name": queued, "job_id": queuedJob}
	queueParents := map[string]string{"project_name": "projects", "job_id": "runs"}
	pick := func(from map[string]string, names ...string) map[string]string {
		picked := map[string]string{}
		for _, name := range names {
			picked[name] = from[name]
		}
		return picked
	}

	// Routes run in order: a route's valid request may change what later
	// routes see. The job control routes act on the live run, cancel-job on
	// its first job and cancel-run on the rest; generate-config creates the
	// config file that save-config then edits; clear-run deletes the
	// finished run, so it comes last.
	routes := []webRoute{
		{"GET", "/api/log", finishedRun, finishedParents, nil},
		{"GET", "/api/log", finishedAttempt, finishedParents, nil},
		{"GET", "/api/report", finishedRun, finishedParents, nil},
		{"GET", "/api/config", pick(finishedRun, "project_name", "run_id"), finishedParents, nil},
		{"GET", "/api/config-targets", pick(finishedRun, "project_name"), finishedParents, nil},
		{"POST", "/api/suspend-job", liveRun, liveParents, nil},
		{"POST", "/api/resume-job", liveRun, liveParents, nil},
		{"POST", "/api/cancel-job", liveRun, liveParents, nil},
		{"POST", "/api/cancel-run", pick(liveRun, "project_name", "run_id"), liveParents, nil},
		{"POST", "/api/change", queue, queueParents, map[string]any{"command": []string{"true", "changed"}}},
		{"POST", "/api/remove", queue, queueParents, nil},
		{"POST", "/api/generate-config", pick(queue, "project_name"), queueParents, map[string]any{"location": "project"}},
		{"POST", "/api/save-config", pick(queue, "project_name"), queueParents, map[string]any{"content": ""}},
		{"POST", "/api/copy", pick(finishedRun, "project_name", "run_id"), finishedParents, map[string]any{"selection": "failed"}},
		{"POST", "/api/clear-run", pick(finishedRun, "project_name", "run_id"), finishedParents, nil},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			e := e.in(t)
			for field, valid := range route.valid {
				for _, value := range unsafeValues(valid, route.parent[field]) {
					unsafe := copyValues(route.valid)
					unsafe[field] = value
					if got := route.send(e, base, unsafe); got.status < 400 || got.status >= 500 {
						t.Errorf("%s=%q: status %d, want a 4xx rejection; body: %s", field, value, got.status, got.body)
					}
				}
			}
			assertNoEscape(t, e.root)
			if got := route.send(e, base, route.valid); got.status != 200 {
				t.Fatalf("valid request: status %d: %s", got.status, got.body)
			}
		})
	}
}

func copyValues(values map[string]string) map[string]string {
	copied := make(map[string]string, len(values))
	for name, value := range values {
		copied[name] = value
	}
	return copied
}

// assertNoEscape fails when anything named after an unsafe value exists
// under root, which would mean a value reached the filesystem.
func assertNoEscape(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(entry.Name(), "escape") {
			t.Errorf("unsafe value reached the filesystem: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
