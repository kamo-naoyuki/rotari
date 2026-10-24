package webui

import (
	"encoding/json"
	"fmt"
	"maps"
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

	"github.com/rotari-dev/rotari/internal/tableview"
)

// viewerGet requests a Table viewer route for a fixture entry, or for the
// working directory, "wd".
func (f artifactFixture) viewerGet(t *testing.T, options Options, route, name string, extra url.Values) *httptest.ResponseRecorder {
	t.Helper()
	if name == "wd" {
		extra = maps.Clone(extra)
		if extra == nil {
			extra = url.Values{}
		}
		extra.Set("entry", "wd")
	}
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
		{"data.csv", url.Values{"sep": {","}, "expanded": {"sub"}}, http.StatusBadRequest},
	} {
		if recorder := f.viewerGet(t, options, "api/rows", test.name, test.extra); recorder.Code != test.status {
			t.Errorf("%s %v = %d %s, want %d", test.name, test.extra, recorder.Code, recorder.Body, test.status)
		}
	}
	if recorder := f.viewerGet(t, options, "", "secret.txt", nil); recorder.Code != http.StatusForbidden {
		t.Errorf("page of an unserved entry = %d", recorder.Code)
	}
}

// The Table viewer is offered for the files it reads and any directory, on
// every live server: it runs no command, so it needs no --allow-exec.
func TestLaunchersListTheTableViewer(t *testing.T) {
	f := newArtifactFixture(t)
	for name, options := range map[string]Options{
		"read-only":         testOptions(f.baseDir, false),
		"without launch":    testOptions(f.baseDir, true),
		"with --allow-exec": launchOptions(t, f.baseDir, launchCommand("Cat", "cat", "file")),
	} {
		for query, want := range map[string]int{
			"kind=file&name=data.csv": 1, "kind=file&name=wav.scp": 1, "kind=file&name=run.log": 0,
			"kind=directory&name=many": 1, "": 2,
		} {
			recorder := httptest.NewRecorder()
			Handler(options).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/launchers?"+query, nil))
			var availability launchAvailability
			if err := json.Unmarshal(recorder.Body.Bytes(), &availability); err != nil {
				t.Fatal(err)
			}
			if len(availability.Viewers) != want || (want > 0 && (availability.Viewers[0].Name != "Table viewer" || availability.Viewers[0].Path != "/tableview/")) {
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
	entry := name // the working directory, "wd"
	if index, ok := f.entries[name]; ok {
		entry = strconv.Itoa(index)
	}
	query := url.Values{"project_name": {"default"}, "run_id": {f.runID}, "job_id": {"job-1"}, "entry": {entry}}
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
  await wait(300);
  const link = row.querySelector('.artifact-launchers a.launch-viewer');
  if (!link || link.textContent !== 'Table viewer' || link.target !== '_blank') fail('viewer link', row.innerHTML);
  const href = new URL(link.getAttribute('href'), base);
  if (href.pathname !== '/tableview/' || href.searchParams.get('entry') !== String(entries['data.csv']) || href.searchParams.get('job_id') !== 'job-1') fail('viewer href', link.getAttribute('href'));
  // Launching is off on this server: the viewer is offered, and the view
  // says why commands are not.
  const note = document.querySelector('#artifact-view .launch-unavailable-note')?.textContent || '';
  if (!note.includes('Launching is disabled')) fail('reason', note);
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

func TestTableViewListsADirectory(t *testing.T) {
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, false)
	recorder := f.viewerGet(t, options, "api/info", "many", nil)
	var info tableViewInfo
	if err := json.Unmarshal(recorder.Body.Bytes(), &info); err != nil || info.Kind != "directory" || info.Download != "" || len(info.Bases) != 1 || info.Bases[0].ID != tableview.DirectoryBase {
		t.Fatalf("info = %d %s", recorder.Code, recorder.Body)
	}
	recorder = f.viewerGet(t, options, "api/rows", "many", url.Values{"pattern": {"f00*.txt"}})
	var page tableview.Page
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("rows = %d %s", recorder.Code, recorder.Body)
	}
	if page.Total != 11 || page.Rows[0].Cells[0] != "f000.txt" || page.Rows[0].Cells[8] != "1" {
		t.Fatalf("rows = %+v", page)
	}
	// The working directory lists its media with their properties; the
	// symlink out of it is not followed.
	recorder = f.viewerGet(t, options, "api/rows", "wd", url.Values{"pattern": {"**/*.{wav,png}"}})
	page = tableview.Page{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &page); err != nil {
		t.Fatalf("working directory rows = %d %s", recorder.Code, recorder.Body)
	}
	var paths []string
	for _, row := range page.Rows {
		paths = append(paths, row.Cells[0])
	}
	if !reflect.DeepEqual(paths, []string{"clip.wav", "many/", "plot.png"}) || page.Rows[0].Media[0] == nil || page.Rows[0].Media[0].Properties == nil {
		t.Fatalf("working directory = %q %s", paths, recorder.Body)
	}
	if served := f.viewerGet(t, options, "api/media", "wd", url.Values{"ref": {"plot.png"}, "base": {tableview.DirectoryBase}}); served.Code != http.StatusOK || served.Body.String() != "\x89PNG-bytes" {
		t.Fatalf("media = %d %q", served.Code, served.Body)
	}
	for _, extra := range []url.Values{
		{"sep": {","}},     // a file's parameter
		{"view": {"tree"}}, // no such view
		{"view": {"stems"}, "group": {"color"}},
		{"view": {"files"}, "incomplete": {"1"}},
		{"pattern": {"["}},
		{"expanded": {""}},
		{"expanded": {"../outside"}},
		{"expanded": {"zz-sub/missing"}},
		{"expanded": {"escape"}},
		{"expanded": {"f000.txt"}},
		{"base": {"basedir"}}, // a file's base
	} {
		if recorder := f.viewerGet(t, options, "api/rows", "many", extra); recorder.Code != http.StatusBadRequest {
			t.Errorf("%v = %d %s", extra, recorder.Code, recorder.Body)
		}
	}
	if recorder := f.viewerGet(t, options, "api/rows", "data.csv", url.Values{"sep": {","}, "view": {"files"}}); recorder.Code != http.StatusBadRequest {
		t.Errorf("a directory's parameter on a file = %d", recorder.Code)
	}
}

func TestTableViewExpandsDirectoriesInBrowser(t *testing.T) {
	f := newArtifactFixture(t)
	for _, name := range []string{"root.tree", "tree/child.tree", "tree/deep/grand.tree"} {
		write(t, filepath.Join(f.work, filepath.FromSlash(name)), "x")
	}
	if err := os.Mkdir(filepath.Join(f.work, "tree", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	runTableViewScript(t, testOptions(f.baseDir, false), f, "wd", "#pattern=**%2F*.tree&sort=0&desc=1", `
  const button = label => document.querySelector('button[aria-label="' + label + '"]');
  const hasFile = path => cells().some(row => row[1] === path);
  await until('collapsed tree', () => cells().length === 3 && button('Expand tree'));
  if (hasFile('tree/child.tree') || hasFile('tree/deep/grand.tree')) fail('read collapsed files', cells());
  const originalFetch = window.fetch;
  let denyExpansion = true;
  window.fetch = (input, init) => {
    const url = new window.URL(input, window.location.href);
    if (denyExpansion && url.pathname.endsWith('/api/rows') && url.searchParams.has('expanded')) {
      denyExpansion = false;
      return Promise.resolve({ ok: false, status: 403, text: async () => 'Cannot read tree' });
    }
    return originalFetch(input, init);
  };
  button('Expand tree').click();
  await until('failed expansion restored', () => button('Expand tree') && document.getElementById('tv-error').textContent === 'Cannot read tree');
  if (fragment().has('expanded')) fail('failed expansion retained', window.location.hash);
  button('Expand tree').click();
  await until('expanded tree', () => cells().length === 6 && button('Collapse tree'));
  if (!hasFile('tree/child.tree') || hasFile('tree/deep/grand.tree')) fail('direct children only', cells());
  if (fragment().get('sort') !== '0' || fragment().get('desc') !== '1') fail('expansion cleared sort');
  button('Expand tree/deep').click();
  await until('expanded child', () => hasFile('tree/deep/grand.tree'));
  if (fragment().getAll('expanded').join() !== 'tree,tree/deep') fail('expanded fragment', window.location.hash);
  document.querySelector('.tv-describe-toggle').click();
  await until('expanded describe', () => document.querySelector('tr.tv-describe')?.textContent.includes('count7'));
  button('Expand tree/empty').click();
  await until('expanded empty directory', () => button('Collapse tree/empty'));
  if (cells().length !== 7) fail('empty directory rows', cells());
  [...document.querySelectorAll('.tv-segmented button')].find(button => button.textContent === 'Group by stem').click();
  await until('expanded stems', () => headings().includes('stem') && cells().length === 3);
  [...document.querySelectorAll('.tv-segmented button')].find(button => button.textContent === 'Files').click();
  await until('files again', () => button('Collapse tree'));
  button('Collapse tree').click();
  await until('collapsed again', () => cells().length === 3 && button('Expand tree'));
  if (fragment().has('expanded')) fail('descendant expansion retained', window.location.hash);
  button('Expand tree').click();
  await until('reopened parent', () => cells().length === 6 && button('Expand tree/deep'));
  if (hasFile('tree/deep/grand.tree')) fail('child reopened automatically');
`)
	runTableViewScript(t, testOptions(f.baseDir, false), f, "wd", "#pattern=**%2F*.tree&expanded=tree&expanded=tree%2Fdeep", `
  await until('restored expansion', () => cells().length === 7);
  if (!document.querySelector('button[aria-label="Collapse tree/deep"]')) fail('restored child expansion');
`)
	runTableViewScript(t, testOptions(f.baseDir, false), f, "many", "#page=3", `
  const button = label => document.querySelector('button[aria-label="' + label + '"]');
  await until('third page', () => button('Expand zz-sub'));
  button('Expand zz-sub').click();
  await until('expanded on third page', () => button('Collapse zz-sub'));
  if (fragment().get('page') !== '3') fail('expansion changed page', window.location.hash);
`)
}

func TestTableViewGroupsStemsInBrowser(t *testing.T) {
	f := newArtifactFixture(t)
	wav, err := os.ReadFile(filepath.Join("..", "tableview", "testdata", "media", "pcm16.wav"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"eval/ref/u1.wav", "eval/ref/u2.wav", "eval/hyp/u1.wav"} {
		write(t, filepath.Join(f.work, filepath.FromSlash(name)), string(wav))
	}
	runTableViewScript(t, testOptions(f.baseDir, false), f, "wd", "#pattern=eval%2F**%2F*.wav&expanded=eval&expanded=eval%2Fhyp&expanded=eval%2Fref", `
  await until('files', () => document.querySelectorAll('#tv-table a.tv-media-path').length === 3);
  if (JSON.stringify(headings().slice(0, 4)) !== '["#","path","format","duration"]') fail('file headings', headings());
  const wavRow = cells().find(row => row[1].includes('eval/hyp/u1.wav'));
  if (!wavRow || wavRow[3] !== '0:00.50' || !wavRow[1].includes('WAV PCM')) fail('file row', wavRow);
  if (!document.getElementById('tv-summary').textContent.includes('7 entries')) fail('summary', document.getElementById('tv-summary').textContent);

  // Group by stem lines up the first-level directories under eval/.
  [...document.querySelectorAll('.tv-segmented button')].find(button => button.textContent === 'Group by stem').click();
  // A stems view's rows start with Open row.
  await until('stems', () => headings().length === 4);
  if (JSON.stringify(headings()) !== '["#","","stem","eval/"]') fail('stem headings', headings());
  const group = document.querySelector('select[aria-label="Groups from"]');
  group.value = 'parent';
  group.dispatchEvent(new window.Event('change'));
  await until('parents', () => headings().length === 5);
  if (JSON.stringify(headings()) !== '["#","","stem","eval/hyp/","eval/ref/"]') fail('parent headings', headings());
  const u2 = cells().find(row => row[2] === 'u2');
  if (!u2 || u2[1] !== 'Open row' || u2[3] !== '— missing' || !u2[4].includes('eval/ref/u2.wav')) fail('missing cell', u2);
  if (fragment().get('listing') !== 'stems' || fragment().get('group') !== 'parent') fail('fragment', window.location.hash);

  // Only incomplete stems, and Describe of the stems view.
  const incomplete = [...document.querySelectorAll('.tv-toolbar input[type=checkbox]')][0];
  incomplete.checked = true;
  incomplete.dispatchEvent(new window.Event('change'));
  await until('incomplete', () => cells().length === 1);
  document.querySelector('.tv-describe-toggle').click();
  await until('describe', () => document.querySelectorAll('tr.tv-describe').length === 7);
`)
}

func TestTableViewOpensFilesInPanesInBrowser(t *testing.T) {
	f := newArtifactFixture(t)
	wav, err := os.ReadFile(filepath.Join("..", "tableview", "testdata", "media", "pcm16.wav"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"eval/ref/u1.wav", "eval/hyp/u1.wav", "eval/ref/u2.wav"} {
		write(t, filepath.Join(f.work, filepath.FromSlash(name)), string(wav))
	}
	runTableViewScript(t, testOptions(f.baseDir, false), f, "wd", "#pattern=eval%2F**%2F*.wav&expanded=eval&expanded=eval%2Fhyp&expanded=eval%2Fref", `
  await until('files', () => document.querySelectorAll('#tv-table a.tv-media-path').length === 3);
  const panesArea = document.getElementById('tv-panes');
  if (!panesArea.hidden) fail('panes shown before a file is opened');
  const link = path => [...document.querySelectorAll('#tv-table a.tv-media-path')].find(a => a.textContent === path);
  const paneList = () => [...document.querySelectorAll('.tv-pane')];
  const tabNames = pane => [...pane.querySelectorAll('.tv-tab-name')].map(button => button.textContent);
  const active = () => document.querySelector('.tv-pane-active');

  // A click opens the file in a tab of a new pane; again, it does not add
  // a second tab.
  link('eval/hyp/u1.wav').click();
  link('eval/hyp/u1.wav').click();
  if (panesArea.hidden || paneList().length !== 1 || JSON.stringify(tabNames(paneList()[0])) !== '["audiou1.wav"]') fail('first open', panesArea.innerHTML);
  if (!active().textContent.includes('Files open here')) fail('active label');
  const view = active().querySelector('.tv-tab-view');
  if (!view.textContent.includes('eval/hyp/u1.wav') || !view.textContent.includes('WAV PCM 16-bit') || !view.querySelector('audio')) fail('tab view', view.innerHTML);
  if (view.querySelector('.tv-waveform-box').hidden || view.querySelector('.tv-spectrogram-box').hidden) fail('plots hidden');
  // jsdom cannot decode audio; the view says so instead of drawing.
  await until('status', () => view.querySelector('.tv-analysis-status').textContent.includes('Cannot draw'));

  // Waveform and Spectrogram toggle their plots.
  const toggle = name => [...view.querySelectorAll('.tv-audio-controls button')].find(button => button.textContent === name);
  toggle('Spectrogram').click();
  if (!view.querySelector('.tv-spectrogram-box').hidden || toggle('Spectrogram').getAttribute('aria-pressed') !== 'false') fail('spectrogram toggle');

  // Split right copies the current file into a new pane, which becomes
  // active; the next file opens there.
  active().querySelector('button[aria-label="Split right"]').click();
  if (paneList().length !== 2 || active() !== paneList()[1] || JSON.stringify(tabNames(paneList()[1])) !== '["audiou1.wav"]') fail('split', panesArea.innerHTML);
  if (!document.querySelector('.tv-split-row')) fail('row split');
  link('eval/ref/u2.wav').click();
  if (JSON.stringify(tabNames(paneList()[1])) !== '["audiou1.wav","audiou2.wav"]' || JSON.stringify(tabNames(paneList()[0])) !== '["audiou1.wav"]') fail('open in active', tabNames(paneList()[0]), tabNames(paneList()[1]));

  // Clicking in the first pane makes it active; a split down nests.
  paneList()[0].dispatchEvent(new window.Event('pointerdown'));
  if (active() !== paneList()[0]) fail('activate by click');
  active().querySelector('button[aria-label="Split down"]').click();
  if (paneList().length !== 3 || !document.querySelector('.tv-split-row .tv-split-column')) fail('split down', panesArea.innerHTML);

  // Closing a pane makes its neighbour active; closing the last tab of the
  // last pane hides the panes.
  active().querySelector('button[aria-label="Close pane"]').click();
  if (paneList().length !== 2 || !active()) fail('close pane');
  for (const pane of paneList().reverse()) {
    while (pane.isConnected && pane.querySelector('.tv-tab-close')) pane.querySelector('.tv-tab-close').click();
  }
  for (let tries = 0; paneList().length && tries < 5; tries++) paneList()[0].querySelector('.tv-tab-close').click();
  if (!panesArea.hidden || paneList().length) fail('all closed', panesArea.innerHTML);

  // A stems row opens its files side by side.
  [...document.querySelectorAll('.tv-segmented button')].find(button => button.textContent === 'Group by stem').click();
  const group = await (async () => { await until('stems', () => document.querySelector('select[aria-label="Groups from"]')); return document.querySelector('select[aria-label="Groups from"]'); })();
  group.value = 'parent';
  group.dispatchEvent(new window.Event('change'));
  await until('rows', () => cells().some(row => row[2] === 'u1'));
  const u1 = [...document.querySelectorAll('#tv-table tbody tr')].find(row => row.children[2].textContent === 'u1');
  u1.querySelector('.tv-open-row').click();
  if (paneList().length !== 2 || JSON.stringify(paneList().map(pane => pane.querySelector('.tv-tab-view strong').textContent)) !== '["eval/hyp/u1.wav","eval/ref/u1.wav"]') fail('open row', panesArea.innerHTML);
`)
}

// The spectrogram's FFT finds a tone in its bin, and its colour scale runs
// from viridis' first stop to its last; jsdom has no canvas to draw them.
func TestSpectrogramFindsATone(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	command := exec.Command("node", "-e", `
const vm = require('vm');
const context = vm.createContext({ Math, Float32Array, Uint32Array, Uint8Array, Map });
vm.runInContext(process.argv[1] + '; this.magnitudes = magnitudes; this.viridisTable = viridisTable;', context);
const rate = 16000, size = 512, samples = new Float32Array(4096);
for (let index = 0; index < samples.length; index++) samples[index] = Math.sin(2 * Math.PI * 1000 * index / rate);
const out = new Float32Array(size / 2);
context.magnitudes(samples, 1000, size, out);
const peak = out.indexOf(Math.max(...out));
if (peak !== 1000 / rate * size) { console.error('peak bin', peak); process.exit(1); }
// Past the end of the samples the window reads silence.
context.magnitudes(samples, samples.length, size, out);
if (Math.max(...out) !== 0) { console.error('silence', Math.max(...out)); process.exit(1); }
const table = context.viridisTable;
if ([...table.slice(0, 3)].join() !== '68,1,84' || [...table.slice(255 * 3)].join() !== '253,231,37') { console.error('colours', table.slice(0, 3), table.slice(255 * 3)); process.exit(1); }
`, tableViewPanesJS)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("spectrogram check failed: %v\n%s", err, output)
	}
}

func TestTableViewMovesTabsAndLinksPanesInBrowser(t *testing.T) {
	f := newArtifactFixture(t)
	wav, err := os.ReadFile(filepath.Join("..", "tableview", "testdata", "media", "pcm16.wav"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.wav", "b.wav", "c.wav"} {
		write(t, filepath.Join(f.work, "set", name), string(wav))
	}
	runTableViewScript(t, testOptions(f.baseDir, false), f, "wd", "#pattern=set%2F*.wav&expanded=set", `
  await until('files', () => document.querySelectorAll('#tv-table a.tv-media-path').length === 3);
  const link = path => [...document.querySelectorAll('#tv-table a.tv-media-path')].find(a => a.textContent === path);
  const paneList = () => [...document.querySelectorAll('.tv-pane')];
  const tabs = pane => [...pane.querySelectorAll('.tv-tab')].map(tab => tab.title);
  const layout = () => JSON.stringify(paneList().map(tabs));
  for (const path of ['set/a.wav', 'set/b.wav', 'set/c.wav']) link(path).click();
  const first = () => paneList()[0];
  const source = (pane, ref) => ({ pane: pane.dataset.pane, tab: [...pane.querySelectorAll('.tv-tab')].find(tab => tab.title === ref).dataset.tab });

  // Tabs are draggable; a drop in the middle of the strip reorders them.
  if (!first().querySelector('.tv-tab').draggable) fail('not draggable');
  window.dropTab(source(first(), 'set/c.wav'), first().dataset.pane, 'center', 0);
  if (layout() !== '[["set/c.wav","set/a.wav","set/b.wav"]]') fail('reorder', layout());

  // A drop near the right edge splits the pane and moves the tab into the
  // new pane, which becomes active.
  window.dropTab(source(first(), 'set/b.wav'), first().dataset.pane, 'right', 0);
  if (layout() !== '[["set/c.wav","set/a.wav"],["set/b.wav"]]' || !paneList()[1].classList.contains('tv-pane-active')) fail('split right', layout());
  window.dropTab(source(first(), 'set/a.wav'), paneList()[1].dataset.pane, 'top', 0);
  if (layout() !== '[["set/c.wav"],["set/a.wav"],["set/b.wav"]]' || !document.querySelector('.tv-split-row .tv-split-column')) fail('split top', layout(), document.getElementById('tv-pane-root').innerHTML);

  // Moving the last tab out of a pane closes it; dropping a pane's only tab
  // on its own edge changes nothing.
  window.dropTab(source(first(), 'set/c.wav'), paneList()[2].dataset.pane, 'center', 1);
  if (layout() !== '[["set/a.wav"],["set/b.wav","set/c.wav"]]') fail('close emptied', layout());
  window.dropTab(source(first(), 'set/a.wav'), first().dataset.pane, 'left', 0);
  if (layout() !== '[["set/a.wav"],["set/b.wav","set/c.wav"]]') fail('own edge', layout());

  // Link time & zoom is one toggle for every pane.
  const toggle = () => document.querySelector('.tv-link-toggle');
  if (!toggle() || toggle().getAttribute('aria-pressed') !== 'false') fail('link toggle', document.getElementById('tv-pane-toolbar').innerHTML);
  toggle().click();
  if (toggle().getAttribute('aria-pressed') !== 'true') fail('linked');
  toggle().click();
  if (toggle().getAttribute('aria-pressed') !== 'false') fail('unlinked');
`)
}
