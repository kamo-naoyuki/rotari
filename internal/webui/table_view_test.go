package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/tableview"
)

// viewerGet requests a Table viewer route for a fixture entry.
func (f artifactFixture) viewerGet(t *testing.T, options Options, route, name string, extra url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return f.get(t, options, "/tableview/"+route, name, extra)
}

func TestTableViewReadsAnArtifact(t *testing.T) {
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, false)

	recorder := f.viewerGet(t, options, "api/info", "data.csv", nil)
	var info tableViewInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &info); err != nil {
		t.Fatalf("info = %d %s", recorder.Code, recorder.Body)
	}
	if info.Kind != "file" || info.Separator != "," || info.Size != int64(len("a,b\n1,2\n")) || !strings.HasPrefix(info.Download, "../api/artifact-file?") {
		t.Fatalf("info = %+v", info)
	}
	// Media paths resolve from the basedir by default, then the job's
	// working directory, then the file's own directory.
	var bases []string
	for _, base := range info.Bases {
		bases = append(bases, base.ID)
	}
	if !reflect.DeepEqual(bases, []string{"basedir", "wd", tableview.FileBase}) {
		t.Fatalf("bases = %q", bases)
	}
	// The download link is the artifact's own download.
	link, err := url.Parse("/tableview/" + info.Download)
	if err != nil {
		t.Fatal(err)
	}
	download := httptest.NewRecorder()
	Handler(options).ServeHTTP(download, httptest.NewRequest(http.MethodGet, link.ResolveReference(&url.URL{}).String(), nil))
	if download.Code != http.StatusOK || download.Body.String() != "a,b\n1,2\n" {
		t.Fatalf("download = %d %q", download.Code, download.Body)
	}

	for header, want := range map[string][][]string{"1": {{"1", "2"}}, "0": {{"a", "b"}, {"1", "2"}}} {
		recorder = f.viewerGet(t, options, "api/rows", "data.csv", url.Values{"sep": {","}, "header": {header}})
		var page tableview.Page
		if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
			t.Fatalf("rows = %d %s", recorder.Code, recorder.Body)
		}
		var rows [][]string
		for _, row := range page.Rows {
			rows = append(rows, row.Cells)
		}
		if !reflect.DeepEqual(rows, want) {
			t.Fatalf("header=%s rows = %q, want %q", header, rows, want)
		}
	}

	page := f.viewerGet(t, options, "", "data.csv", nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `src="table_view.js"`) || !strings.Contains(page.Body.String(), `href="/web_tokens.css"`) {
		t.Fatalf("page = %d %s", page.Code, page.Body)
	}
	for _, asset := range []string{"table_view.js", "table_view.css"} {
		recorder := httptest.NewRecorder()
		Handler(options).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/tableview/"+asset, nil))
		if recorder.Code != http.StatusOK || recorder.Body.Len() == 0 {
			t.Fatalf("%s = %d", asset, recorder.Code)
		}
	}
}

func TestTableViewRefusesWhatContentRequestsRefuse(t *testing.T) {
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, false)
	for _, test := range []struct {
		name   string
		extra  url.Values
		status int
	}{
		{"secret.txt", url.Values{"sep": {","}}, http.StatusForbidden}, // outside every root
		{"link.csv", url.Values{"sep": {","}}, http.StatusForbidden},   // a symlink leaving the root
		{"gone.csv", url.Values{"sep": {","}}, http.StatusNotFound},    // missing
		{"data.csv", url.Values{"sep": {""}}, http.StatusBadRequest},   // no separator
		{"data.csv", url.Values{"sep": {",;"}}, http.StatusBadRequest}, // two characters
		{"data.csv", url.Values{"sep": {`"`}}, http.StatusBadRequest},  // a quote
		{"data.csv", url.Values{"sep": {","}, "header": {"yes"}}, http.StatusBadRequest},
		{"data.csv", url.Values{"sep": {","}, "limit": {"5000"}}, http.StatusBadRequest},
		{"data.csv", url.Values{"sep": {","}, "offset": {"x"}}, http.StatusBadRequest},
		{"many", url.Values{"sep": {","}}, http.StatusBadRequest}, // a directory read as a file
	} {
		if recorder := f.viewerGet(t, options, "api/rows", test.name, test.extra); recorder.Code != test.status {
			t.Errorf("%s %v = %d %s, want %d", test.name, test.extra, recorder.Code, recorder.Body, test.status)
		}
	}
	if recorder := f.viewerGet(t, options, "", "secret.txt", nil); recorder.Code != http.StatusForbidden {
		t.Errorf("page of an unserved entry = %d", recorder.Code)
	}
}

// The Table viewer is offered for the files it reads, on every live
// server: it runs no command, so it needs no --allow-exec.
func TestLaunchersListTheTableViewer(t *testing.T) {
	f := newArtifactFixture(t)
	for name, options := range map[string]Options{
		"read-only":         testOptions(f.baseDir, false),
		"without launch":    testOptions(f.baseDir, true),
		"with --allow-exec": launchOptions(t, f.baseDir, launchCommand("Cat", "cat", "file")),
	} {
		for query, want := range map[string]int{
			"kind=file&name=data.csv": 1, "kind=file&name=wav.scp": 1, "kind=file&name=run.log": 0,
			"kind=directory&name=many": 0, "": 1,
		} {
			recorder := httptest.NewRecorder()
			Handler(options).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/launchers?"+query, nil))
			var availability launchAvailability
			if err := json.Unmarshal(recorder.Body.Bytes(), &availability); err != nil {
				t.Fatal(err)
			}
			if len(availability.Viewers) != want || (want == 1 && (availability.Viewers[0].Name != "Table viewer" || availability.Viewers[0].Path != "/tableview/")) {
				t.Errorf("%s, %s: viewers = %+v, want %d", name, query, availability.Viewers, want)
			}
		}
	}
}

// runTableViewScript runs script in jsdom on a server, with the Table
// viewer page of entry name open.
func runTableViewScript(t *testing.T, options Options, f artifactFixture, name, fragment, script string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	server := httptest.NewServer(Handler(options))
	defer server.Close()
	query := url.Values{"project_name": {"default"}, "run_id": {f.runID}, "job_id": {"job-1"}, "entry": {strconv.Itoa(f.entries[name])}}
	command := exec.Command("node", "-e", `
const { JSDOM, VirtualConsole } = require('jsdom');
const pageURL = process.argv[1];
const fail = (code, detail) => { console.error(code, detail || ''); process.exit(1); };
const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
(async () => {
  const errors = [];
  const virtualConsole = new VirtualConsole();
  virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
  const dom = await JSDOM.fromURL(pageURL, {
    runScripts: 'dangerously', resources: 'usable', virtualConsole,
    // jsdom has no fetch; relative URLs resolve against the page, as in a browser.
    beforeParse(window) {
      window.fetch = (input, init) => fetch(new URL(String(input), window.location.href), init);
    },
  });
  const window = dom.window;
  const document = window.document;
  const until = async (what, check) => {
    for (let tries = 0; tries < 100; tries++) {
      if (check()) return;
      await wait(30);
    }
    fail('timed out waiting for ' + what, document.body.innerHTML.slice(0, 2000) + '\n' + errors.join('\n'));
  };
  const cells = () => [...document.querySelectorAll('#tv-table tbody tr')].map(row => [...row.children].map(cell => cell.textContent));
  const headings = () => [...document.querySelectorAll('#tv-table thead th')].map(cell => cell.textContent);
  const fragment = () => new window.URLSearchParams(window.location.hash.slice(1));
`+script+`
  if (errors.length) fail('page errors', errors.join('\n'));
})().catch(error => fail('exception', error.stack));
`, server.URL+"/tableview/?"+query.Encode()+fragment)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("browser check failed: %v\n%s", err, output)
	}
}

func TestTableViewPagesInBrowser(t *testing.T) {
	f := newArtifactFixture(t)
	var content strings.Builder
	content.WriteString("n|text\n")
	for n := 1; n <= 120; n++ {
		fmt.Fprintf(&content, "%d|row %d\n", n, n)
	}
	write(t, filepath.Join(f.work, "data.csv"), content.String())
	options := testOptions(f.baseDir, false)
	// The fragment chooses the separator and the page size.
	runTableViewScript(t, options, f, "data.csv", "#sep=%7C&limit=50", `
  await until('rows', () => cells().length === 50);
  if (JSON.stringify(headings()) !== '["#","n","text"]') fail('headings', headings());
  if (JSON.stringify(cells()[0]) !== '["1","1","row 1"]') fail('first row', cells()[0]);
  const count = () => document.querySelector('.tv-pager-count').textContent;
  if (!count().includes('Rows 1–50 of 120')) fail('count', count());
  if (!document.getElementById('tv-summary').textContent.includes('120 rows · 2 columns')) fail('summary', document.getElementById('tv-summary').textContent);

  // Last goes to page 3 of 3, and the fragment keeps it.
  [...document.querySelectorAll('.tv-pager-buttons button')].find(button => button.textContent === 'Last »').click();
  await until('last page', () => cells().length === 20);
  if (JSON.stringify(cells()[19]) !== '["120","120","row 120"]') fail('last row', cells()[19]);
  if (fragment().get('page') !== '3') fail('fragment page', window.location.hash);

  // Another separator reads the file differently and starts over at page 1.
  const select = document.querySelector('.tv-toolbar select');
  select.value = ',';
  select.dispatchEvent(new window.Event('change'));
  await until('one column', () => headings().length === 2);
  if (JSON.stringify(headings()) !== '["#","n|text"]' || fragment().get('page') !== '1' || fragment().get('sep') !== ',') fail('comma', headings(), window.location.hash);

  // A separator that is not one character is refused with an explanation.
  select.value = 'other';
  select.dispatchEvent(new window.Event('change'));
  const other = document.querySelector('.tv-separator-other');
  if (other.hidden) fail('other input hidden');
  other.value = '||';
  other.dispatchEvent(new window.Event('change'));
  if (!document.getElementById('tv-error').textContent.includes('one character')) fail('two-character separator', document.getElementById('tv-error').textContent);
  other.value = '|';
  other.dispatchEvent(new window.Event('change'));
  await until('pipe again', () => headings().length === 3);
`)
}

func TestLaunchWithOpensTheTableViewerInBrowser(t *testing.T) {
	f := newArtifactFixture(t)
	runLaunchBrowserScript(t, testOptions(f.baseDir, false), f, `
  await window.showArtifacts('default', runID, 'job-1', '');
  const row = [...document.querySelectorAll('.artifact-table tbody tr')].find(row => row.children[1].textContent === 'data.csv');
  row.querySelector('.artifact-launch-button').click();
  await wait(300);
  const link = document.querySelector('#artifact-launch-panel a.launch-viewer');
  if (!link || link.textContent !== 'Table viewer' || link.target !== '_blank') fail('viewer link', panelText());
  const href = new URL(link.getAttribute('href'), base);
  if (href.pathname !== '/tableview/' || href.searchParams.get('entry') !== String(entries['data.csv']) || href.searchParams.get('job_id') !== 'job-1') fail('viewer href', link.getAttribute('href'));
  // Launching is off on this server: the viewer is offered with the reason
  // commands are not.
  if (!panelText().includes('Launching is disabled')) fail('reason', panelText());
  const page = await fetch(href);
  if (page.status !== 200) fail('viewer page', page.status);
`)
}

func TestTableViewFiltersSortsAndDescribes(t *testing.T) {
	f := newArtifactFixture(t)
	write(t, filepath.Join(f.work, "data.csv"), "id,score\na,3\nb,1\nc,x\nd,2\n")
	options := testOptions(f.baseDir, false)
	filters := `[{"column":1,"op":">=","value":"2"}]`
	recorder := f.viewerGet(t, options, "api/rows", "data.csv", url.Values{"sep": {","}, "filters": {filters}, "sort": {"1"}, "desc": {"1"}})
	var page tableview.Page
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("rows = %d %s", recorder.Code, recorder.Body)
	}
	var got []string
	for _, row := range page.Rows {
		got = append(got, row.Cells[0])
	}
	if !reflect.DeepEqual(got, []string{"a", "d"}) || page.Matched != 2 || page.Total != 4 {
		t.Fatalf("rows = %q matched %d of %d", got, page.Matched, page.Total)
	}
	recorder = f.viewerGet(t, options, "api/describe", "data.csv", url.Values{"sep": {","}, "filters": {filters}})
	var description tableview.Description
	if err := json.Unmarshal(recorder.Body.Bytes(), &description); err != nil {
		t.Fatalf("describe = %d %s", recorder.Code, recorder.Body)
	}
	if description.Matched != 2 || *description.Columns[1].Sum != 5 || description.Columns[0].Unique != 2 {
		t.Fatalf("describe = %s", recorder.Body)
	}
	for _, extra := range []url.Values{
		{"filters": {`{"column":1}`}},
		{"filters": {`[{"column":9,"op":"="}]`}},
		{"filters": {`[{"column":1,"op":">","value":"high"}]`}},
		{"sort": {"x"}},
		{"sort": {"1"}, "desc": {"yes"}},
		{"desc": {"1"}},
	} {
		extra.Set("sep", ",")
		if recorder := f.viewerGet(t, options, "api/rows", "data.csv", extra); recorder.Code != http.StatusBadRequest {
			t.Errorf("%v = %d %s", extra, recorder.Code, recorder.Body)
		}
	}
}

func TestTableViewFiltersInBrowser(t *testing.T) {
	f := newArtifactFixture(t)
	write(t, filepath.Join(f.work, "data.csv"), "id,score,text\na,0.9,meeting room\nb,0.75,notes\nc,,thanks\nd,10,meeting\n")
	runTableViewScript(t, testOptions(f.baseDir, false), f, "data.csv", "", `
  await until('rows', () => cells().length === 4);
  const bar = document.getElementById('tv-filterbar');
  const [column, operator] = bar.querySelectorAll('select');
  const value = bar.querySelector('.tv-filter-value');
  const add = [...bar.querySelectorAll('button')].find(button => button.textContent === '+ Add');
  column.value = '1'; column.dispatchEvent(new window.Event('change'));
  operator.value = '>='; operator.dispatchEvent(new window.Event('change'));
  value.value = '0.8'; value.dispatchEvent(new window.Event('input'));
  add.click();
  await until('filtered', () => cells().length === 2);
  if (JSON.stringify(cells().map(row => row[1])) !== '["a","d"]') fail('filtered rows', cells());
  const chip = () => document.querySelector('.tv-chip');
  if (!chip() || !chip().textContent.includes('score') || !chip().textContent.includes('≥') || !chip().textContent.includes('0.8')) fail('chip', chip() && chip().textContent);
  if (!document.querySelector('.tv-pager-count').textContent.includes('of 2 matching (4 in all)')) fail('count', document.querySelector('.tv-pager-count').textContent);
  if (JSON.parse(fragment().get('filters'))[0].value !== '0.8') fail('fragment filters', window.location.hash);

  // Describe summarizes the matching rows under the column names.
  document.querySelector('.tv-describe-toggle').click();
  await until('describe', () => document.querySelectorAll('tr.tv-describe').length === 7);
  const stat = name => [...document.querySelectorAll('tr.tv-describe')].find(row => row.children[0].textContent === name);
  if (stat('count').children[2].textContent !== '2' || stat('sum').children[2].textContent !== '10.9' || stat('mean').children[1].textContent !== '–') fail('describe', [...document.querySelectorAll('tr.tv-describe')].map(row => row.textContent));
  if (fragment().get('describe') !== '1') fail('fragment describe', window.location.hash);

  // A column heading sorts, ascending and then descending.
  const sortBy = () => [...document.querySelectorAll('#tv-table thead tr:first-child .tv-sort')][1];
  sortBy().click();
  await until('ascending', () => cells()[0] && cells()[0][1] === 'a');
  sortBy().click();
  await until('descending', () => cells()[0] && cells()[0][1] === 'd');
  if (document.querySelectorAll('#tv-table thead th')[2].getAttribute('aria-sort') !== 'descending' || fragment().get('desc') !== '1') fail('sort state', window.location.hash);

  // Removing the condition shows every row again; a bad value is explained.
  chip().querySelector('button').click();
  await until('unfiltered', () => cells().length === 4);
  const bar2 = document.getElementById('tv-filterbar');
  const [, operator2] = bar2.querySelectorAll('select');
  operator2.value = '>'; operator2.dispatchEvent(new window.Event('change'));
  const value2 = bar2.querySelector('.tv-filter-value');
  value2.value = 'high'; value2.dispatchEvent(new window.Event('input'));
  [...bar2.querySelectorAll('button')].find(button => button.textContent === '+ Add').click();
  await until('error', () => document.getElementById('tv-error').textContent.includes('needs a number'));
`)
}

func TestTableViewDescribesAndServesMedia(t *testing.T) {
	f := newArtifactFixture(t)
	wav, err := os.ReadFile(filepath.Join("..", "tableview", "testdata", "media", "pcm16.wav"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.baseDir, "audio", "a.wav"), string(wav))
	write(t, filepath.Join(f.work, "audio", "a.wav"), string(wav[:44]))
	write(t, filepath.Join(f.work, "data.csv"), "id,audio\n1,audio/a.wav\n2,"+filepath.Join(f.outside, "secret.wav")+"\n")
	write(t, filepath.Join(f.outside, "secret.wav"), string(wav))
	options := testOptions(f.baseDir, false)
	media := func(base string) []*tableview.Media {
		t.Helper()
		recorder := f.viewerGet(t, options, "api/rows", "data.csv", url.Values{"sep": {","}, "base": {base}})
		var page tableview.Page
		if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
			t.Fatalf("rows = %d %s", recorder.Code, recorder.Body)
		}
		return []*tableview.Media{page.Rows[0].Media[1], page.Rows[1].Media[1]}
	}
	// By default relative paths resolve from the basedir.
	got := media("")
	if got[0].Properties == nil || got[0].Properties.Duration != 0.5 || got[1].Error != "Not shown: outside the allowed roots" {
		t.Fatalf("basedir media = %+v %+v", got[0], got[1])
	}
	// From the working directory the same path names a file with no samples.
	if got := media("wd"); got[0].Properties == nil || got[0].Properties.Duration != 0 || got[0].Base != "wd" {
		t.Fatalf("working directory media = %+v", got[0])
	}
	if recorder := f.viewerGet(t, options, "api/rows", "data.csv", url.Values{"sep": {","}, "base": {"elsewhere"}}); recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown base = %d", recorder.Code)
	}

	served := f.viewerGet(t, options, "api/media", "data.csv", url.Values{"ref": {"audio/a.wav"}})
	if served.Code != http.StatusOK || served.Body.Len() != len(wav) || served.Header().Get("Content-Type") != "audio/wav" ||
		served.Header().Get("Cache-Control") != "no-store" || !strings.Contains(served.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("media = %d %v", served.Code, served.Header())
	}
	for _, test := range []struct {
		ref, base string
		status    int
	}{
		{filepath.Join(f.outside, "secret.wav"), "", http.StatusForbidden}, // outside every root
		{"../../" + filepath.Base(f.outside) + "/secret.wav", "wd", http.StatusForbidden},
		{"data.csv", "file", http.StatusBadRequest}, // not media
		{"audio/gone.wav", "", http.StatusNotFound}, // missing
		{"audio/a.wav", "elsewhere", http.StatusBadRequest},
	} {
		if recorder := f.viewerGet(t, options, "api/media", "data.csv", url.Values{"ref": {test.ref}, "base": {test.base}}); recorder.Code != test.status {
			t.Errorf("%s from %q = %d %s, want %d", test.ref, test.base, recorder.Code, recorder.Body, test.status)
		}
	}
}

func TestTableViewShowsMediaInBrowser(t *testing.T) {
	f := newArtifactFixture(t)
	wav, err := os.ReadFile(filepath.Join("..", "tableview", "testdata", "media", "pcm16.wav"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(f.work, "audio", "a.wav"), string(wav))
	write(t, filepath.Join(f.work, "data.csv"), "id,audio\n1,audio/a.wav\n2,audio/gone.wav\n")
	runTableViewScript(t, testOptions(f.baseDir, false), f, "data.csv", "", `
  await until('rows', () => cells().length === 2);
  // From the basedir neither file exists.
  if (!cells()[0][2].includes('File not found')) fail('basedir', cells()[0]);
  const base = document.querySelector('select[aria-label="Resolve media paths from"]');
  if ([...base.options].map(option => option.value).join() !== 'basedir,wd,file') fail('bases', base.innerHTML);
  base.value = 'wd';
  base.dispatchEvent(new window.Event('change'));
  await until('working directory', () => cells()[0][2].includes('WAV PCM 16-bit · 0:00.50 · 16 kHz · 1 ch · 256 kbps · 15.7 KiB'));
  const link = document.querySelector('#tv-table tbody a.tv-media-path');
  const href = new URL(link.href);
  if (href.pathname !== '/tableview/api/media' || href.searchParams.get('ref') !== 'audio/a.wav' || href.searchParams.get('base') !== 'wd') fail('media link', link.href);
  if ((await fetch(href)).status !== 200) fail('media fetch');
  if (!cells()[1][2].includes('File not found') || fragment().get('base') !== 'wd') fail('missing file', cells()[1], window.location.hash);
`)
}
