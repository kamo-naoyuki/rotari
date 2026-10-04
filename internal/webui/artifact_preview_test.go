package webui

import (
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// TestWebArtifactPreviewInBrowser loads the real page in jsdom against the
// real handler and opens each kind of preview the way a user would: the
// listing, an image, a log from its end, a table, a directory and its next
// page, a child whose name has a quote, and a file without a preview.
func TestWebArtifactPreviewInBrowser(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	f := newArtifactFixture(t)
	write(t, filepath.Join(f.work, "many", "it's.txt"), "quoted name")
	server := httptest.NewServer(Handler(testOptions(f.baseDir, false)))
	defer server.Close()
	script := `
const { JSDOM, VirtualConsole } = require('jsdom');
const base = process.argv[1];
const runID = process.argv[2];
const entries = JSON.parse(process.argv[3]);
const fail = (code, detail) => { console.error(code, detail || ''); process.exit(1); };
const wait = (ms) => new Promise(resolve => setTimeout(resolve, ms));
(async () => {
  const html = await (await fetch(base + '/')).text();
  const dom = new JSDOM(html, {
    runScripts: 'dangerously', url: base + '/', virtualConsole: new VirtualConsole(),
    beforeParse(window) {
      window.fetch = (input, init) => fetch(new URL(String(input), base), init);
      window.setInterval = () => 1;
    },
  });
  const window = dom.window;
  const document = window.document;
  await wait(200);
  // A job without a record shows its text in the log box, like a log, so
  // the copy button sits where it does for logs.
  await window.showArtifacts('default', runID, 'job-2', '');
  const log = document.getElementById('modal-log');
  if (log.hidden || log.textContent !== 'Artifacts: (not recorded)' || !document.getElementById('artifact-view').hidden) fail('not recorded', log.textContent);
  await window.showArtifacts('default', runID, 'missing-job', '');
  if (log.hidden || !log.textContent.startsWith('Failed to load artifacts')) fail('failure', log.textContent);
  await window.showArtifacts('default', runID, 'job-1', '');
  const view = document.getElementById('artifact-view');
  if (view.hidden || document.getElementById('modal-log').hidden !== true) fail('view toggling');
  const rows = view.querySelectorAll('.artifact-table tbody tr');
  if (rows.length !== 10) fail('rows', rows.length);
  const buttons = [...view.querySelectorAll('.artifact-table .artifact-open')].map(button => button.textContent);
  if (buttons.some(text => text.includes('secret.txt'))) fail('outside entry is openable');
  if (buttons.length !== 7) fail('buttons', buttons);

  await window.openArtifact(entries['plot.png'], '', 'file');
  const image = view.querySelector('.artifact-image');
  if (!image) fail('image');
  const imageResponse = await fetch(new URL(image.getAttribute('src'), base));
  if (imageResponse.status !== 200 || imageResponse.headers.get('content-type') !== 'image/png') fail('image response', imageResponse.status);
  if (!view.querySelector('.artifact-download').getAttribute('href').includes('download=1')) fail('download link');

  await window.openArtifact(entries['run.log'], '', 'file');
  let text = view.querySelector('.artifact-text').textContent;
  if (!text.endsWith('line 19999\n') || text.startsWith('line 00000')) fail('log tail', text.slice(0, 20));
  const earlier = [...view.querySelectorAll('button')].find(button => button.textContent === 'Load earlier');
  if (!earlier) fail('load earlier');
  await window.moreArtifactText();
  const longer = view.querySelector('.artifact-text').textContent;
  if (longer.length <= text.length || !longer.endsWith(text)) fail('earlier page');

  await window.openArtifact(entries['data.csv'], '', 'file');
  const headers = [...view.querySelectorAll('.artifact-preview thead th')].map(cell => cell.textContent);
  const cells = [...view.querySelectorAll('.artifact-preview tbody td')].map(cell => cell.textContent);
  if (JSON.stringify(headers) !== '["a","b"]' || JSON.stringify(cells) !== '["1","2"]') fail('table', JSON.stringify({ headers, cells }));

  await window.openArtifact(entries['many'], '', 'directory');
  let children = view.querySelectorAll('.artifact-preview tbody tr');
  if (children.length !== 200) fail('directory page', children.length);
  const more = [...view.querySelectorAll('.artifact-preview button')].find(button => button.textContent === 'More');
  more.click();
  await wait(200);
  children = view.querySelectorAll('.artifact-preview tbody tr');
  if (children.length !== 53) fail('second directory page', children.length);
  const quoted = [...view.querySelectorAll('.artifact-preview .artifact-open')].find(button => button.textContent === "it's.txt");
  if (!quoted) fail('quoted child');
  quoted.click();
  await wait(200);
  if (!view.querySelector('.artifact-preview-title').textContent.includes("many/it's.txt")) fail('quoted child title', view.querySelector('.artifact-preview').textContent);

  await window.openArtifact(entries['weights.npy'], '', 'file');
  if (!view.querySelector('.artifact-preview').textContent.includes('No preview for this file type. 8 B')) fail('no preview', view.querySelector('.artifact-preview').textContent);
  process.exit(0);
})().catch(error => fail('error', error.stack));
`
	entries := "{"
	for name, index := range f.entries {
		if len(entries) > 1 {
			entries += ","
		}
		entries += strconv.Quote(name) + ":" + strconv.Itoa(index)
	}
	entries += "}"
	command := exec.Command("node", "-e", script, server.URL, f.runID, entries)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("browser preview: %v\n%s", err, output)
	}
}
