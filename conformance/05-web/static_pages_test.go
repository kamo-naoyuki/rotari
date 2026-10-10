package web

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// TestStaticExportPagesShowWhatLivePagesShow renders exported pages opened
// from disk, where each path ends in index.html. The projects overview and a
// project's runs must keep the live pages' columns and row actions: an
// overview once lost its Started column to a header relabelled "Actions",
// and project pages lost the latest-run mark and runtime panel, because the
// file name was read as part of the route.
func TestStaticExportPagesShowWhatLivePagesShow(t *testing.T) {
	covers(t, "WEB-9")
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	e := support.NewEnv(t)
	e.MustRotari("add", "-p", "first", "--job-name", "one", "--", "true")
	e.MustRotari("run", "-p", "first", "--quiet")
	e.MustRotari("add", "-p", "first", "--job-name", "two", "--", "true")
	e.MustRotari("run", "-p", "first", "--quiet")
	e.MustRotari("add", "-p", "second", "--job-name", "three", "--", "true")
	e.MustRotari("run", "-p", "second", "--quiet")
	export := filepath.Join(e.Root, "export")
	e.MustRotari("web", "--static-dir", export)

	script := `
const { JSDOM } = require('jsdom');
const path = process.argv[1];
// The page keeps its on-disk path, ending in index.html, as a browser opening
// the file would; an http origin gives it localStorage, which jsdom withholds
// from file: pages. The export answers its own requests with Response
// objects, which jsdom's window lacks.
JSDOM.fromFile(path, {
  runScripts: 'dangerously',
  url: 'http://localhost' + path,
  beforeParse(window) { Object.assign(window, { Response, Request, Headers }); },
}).then((dom) => {
  setTimeout(() => {
    const document = dom.window.document;
    const table = document.querySelector('#app table');
    const text = (node) => node.textContent.replace(/[↑↓↕]/g, '').trim();
    console.log(JSON.stringify({
      headers: table ? [...table.querySelectorAll('thead th')].map(text) : [],
      rowActions: table ? [...table.querySelectorAll('tbody tr')].map((row) =>
        [...row.children[0].querySelectorAll('button')].map(text)) : [],
      latestBadges: document.querySelectorAll('#app .latest-badge').length,
      headings: [...document.querySelectorAll('#app h2')].map(text),
    }));
    process.exit(0);
  }, 1000);
});
`
	type page struct {
		Headers      []string   `json:"headers"`
		RowActions   [][]string `json:"rowActions"`
		LatestBadges int        `json:"latestBadges"`
		Headings     []string   `json:"headings"`
	}
	render := func(relative string) page {
		t.Helper()
		command := exec.Command("node", "-e", script, filepath.Join(export, relative))
		var stderr strings.Builder
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			t.Fatalf("render %s: %v\n%s%s", relative, err, output, stderr.String())
		}
		var got page
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("render %s: %v\n%s", relative, err, output)
		}
		return got
	}

	home := render("index.html")
	if want := []string{"Actions", "Project", "Queued", "Runs", "Running", "Latest run", "Status", "Started"}; !reflect.DeepEqual(home.Headers, want) {
		t.Errorf("exported overview columns = %q, want %q", home.Headers, want)
	}
	if want := [][]string{{"Path"}, {"Path"}}; !reflect.DeepEqual(home.RowActions, want) {
		t.Errorf("exported overview row actions = %q, want %q", home.RowActions, want)
	}

	project := render(filepath.Join("project", "first", "index.html"))
	if want := []string{"Actions", "run-name", "run-id", "Status", "Client", "Exit", "Started", "Finished"}; len(project.Headers) < len(want) || !reflect.DeepEqual(project.Headers[len(project.Headers)-len(want):], want) {
		t.Errorf("exported project run columns = %q, want them to end with %q", project.Headers, want)
	}
	if project.LatestBadges != 1 {
		t.Errorf("exported project page marks %d runs latest, want 1", project.LatestBadges)
	}
	found := false
	for _, heading := range project.Headings {
		found = found || heading == "Project runtime"
	}
	if !found {
		t.Errorf("exported project page lacks the Project runtime panel; headings %q", project.Headings)
	}
}
