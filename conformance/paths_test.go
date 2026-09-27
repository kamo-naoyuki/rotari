package conformance

import (
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

// Contracts RES-9 and RES-10: project names, run IDs, and job IDs are single
// path elements.
// Empty values, ".", "..", and values containing "/" or "\" are rejected
// before filesystem access, by the CLI and by the Web API alike.
// See docs/contracts/01-resolution-and-config.md and the path invariants in
// AGENTS.md.
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
		"x/escape", `x\escape`, "../escape", "..", ".",
		"./" + valid, "../" + parent + "/" + valid,
	}
}

func TestCLIRejectsUnsafePathElements(t *testing.T) {
	covers(t, "RES-9", "RES-10")
	e := newEnv(t)
	run := e.createFinishedRun()

	// Commands run in order; delete removes the run, so it comes last.
	cases := []struct {
		name   string
		args   func(value string) []string
		valid  string
		parent string
	}{
		{"add --project-name", func(v string) []string { return []string{"add", "-p", v, "--", "true"} }, run.project, "projects"},
		{"show --project-name", func(v string) []string { return []string{"show", "-p", v} }, run.project, "projects"},
		{"check PROJECT", func(v string) []string { return []string{"check", v} }, run.project, "projects"},
		{"jobs PROJECT", func(v string) []string { return []string{"jobs", v} }, run.project, "projects"},
		{"show --run-id", func(v string) []string { return []string{"show", "-p", run.project, "--run-id", v} }, run.runID, "runs"},
		{"show --job-id", func(v string) []string {
			return []string{"show", "-p", run.project, "--run-id", run.runID, "--job-id", v}
		}, run.badJob, run.runID},
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

// webRoute is a Web API route that names a project, run, or job.
type webRoute struct {
	method string
	path   string
	fields []string
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
	run := e.createFinishedRun()
	base := e.startWeb()

	valid := map[string]string{"project_name": run.project, "run_id": run.runID, "job_id": run.badJob}
	parents := map[string]string{"project_name": "projects", "run_id": "runs", "job_id": run.runID}

	// Routes run in order; clear-run deletes the run, so it comes last.
	// Job control (cancel, suspend, resume), change, remove, and the config
	// routes need an active run, a queued job, or a config file for a valid
	// request to succeed, so they are not covered yet.
	routes := []webRoute{
		{"GET", "/api/log", []string{"project_name", "run_id", "job_id"}, nil},
		{"GET", "/api/report", []string{"project_name", "run_id", "job_id"}, nil},
		{"POST", "/api/copy", []string{"project_name", "run_id"}, map[string]any{"selection": "failed"}},
		{"POST", "/api/clear-run", []string{"project_name", "run_id"}, nil},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			e := e.in(t)
			values := map[string]string{}
			for _, field := range route.fields {
				values[field] = valid[field]
			}
			for _, field := range route.fields {
				for _, value := range unsafeValues(valid[field], parents[field]) {
					unsafe := copyValues(values)
					unsafe[field] = value
					if got := route.send(e, base, unsafe); got.status < 400 || got.status >= 500 {
						t.Errorf("%s=%q: status %d, want a 4xx rejection; body: %s", field, value, got.status, got.body)
					}
				}
			}
			assertNoEscape(t, e.root)
			if got := route.send(e, base, values); got.status != 200 {
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
