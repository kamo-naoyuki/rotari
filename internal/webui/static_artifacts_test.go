package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// TestStaticExportWithArtifactContents exports the artifact fixture with
// --static-artifact-contents and opens each kind of preview in the exported
// page, with no server.
func TestStaticExportWithArtifactContents(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, false)
	options.StaticArtifactContents = true
	copiedFiles := -1
	options.StaticArtifactsCopied = func(files int, _ int64) { copiedFiles = files }
	output := filepath.Join(t.TempDir(), "static")
	if err := GenerateStatic(output, options); err != nil {
		t.Fatal(err)
	}
	// Under the working directory: plot.png, pic.svg, data.csv, run.log,
	// weights.npy, model.pt, clip.wav, clip.mp4. Not: the directory itself,
	// secret.txt (outside), link.csv (symlink out), gone.csv, rel.csv.
	if copiedFiles != 8 {
		t.Fatalf("copied %d files, want 8", copiedFiles)
	}
	entries := "{"
	for name, index := range f.entries {
		if len(entries) > 1 {
			entries += ","
		}
		entries += strconv.Quote(name) + ":" + strconv.Itoa(index)
	}
	entries += "}"
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const output = process.argv[1];
const runID = process.argv[2];
const entries = JSON.parse(process.argv[3]);
const fail = (code, detail) => { console.error(code, detail || ''); process.exit(1); };
const html = fs.readFileSync(output + '/index.html', 'utf8');
const dom = new JSDOM(html, {
  // A nested route of a site served under a path, as GitHub Pages serves it.
  runScripts: 'dangerously', url: 'http://export.local/site/project/default/run/' + runID + '/', virtualConsole: new VirtualConsole(),
  beforeParse(window) { window.setInterval = () => 1; window.Response = Response; },
});
const window = dom.window;
const view = () => window.document.getElementById('artifact-view');
const preview = () => window.document.querySelector('.artifact-preview');
setTimeout(async () => {
  try {
    await window.showArtifacts('default', runID, 'job-1', '');
    const buttons = view().querySelectorAll('.artifact-table .artifact-open').length;
    if (buttons !== 9) fail('open buttons', buttons);

    let scrolls = 0;
    preview().scrollIntoView = options => {
      if (options.block !== 'start' || options.inline !== 'nearest') fail('preview scroll alignment', options);
      if (preview().textContent.includes('Loading...')) fail('preview scrolled before rendering');
      scrolls++;
    };
    const openArtifact = window.openArtifact;
    window.openArtifact = async (...args) => {
      const before = scrolls;
      await openArtifact(...args);
      if (scrolls !== before + 1) fail('preview not scrolled into view', args);
    };

    await window.openArtifact(entries['plot.png'], '', 'file');
    const src = preview().querySelector('img').getAttribute('src');
    if (!/^\/site\/artifact-files\/\d+\.png$/.test(src)) fail('image src', src);
    if (fs.readFileSync(output + src.slice('/site'.length), 'latin1') !== '\x89PNG-bytes') fail('copied image');
    if (!preview().querySelector('.artifact-download').getAttribute('href').startsWith('/site/artifact-files/')) fail('download link');

    await window.openArtifact(entries['clip.wav'], '', 'file');
    if (!/^\/site\/artifact-files\/\d+\.wav$/.test(preview().querySelector('audio').getAttribute('src'))) fail('audio');

    await window.openArtifact(entries['run.log'], '', 'file');
    const text = preview().querySelector('.artifact-text').textContent;
    if (!text.startsWith('line 00000\n') || !text.endsWith('line 19999\n') || preview().textContent.includes('Load earlier')) fail('log', text.length);

    await window.openArtifact(entries['data.csv'], '', 'file');
    if ([...preview().querySelectorAll('tbody td')].map(cell => cell.textContent).join() !== '1,2') fail('table');

    await window.openArtifact(entries['weights.npy'], '', 'file');
    if (preview().querySelector('.artifact-values').textContent !== '[1, 2, 3]') fail('array');

    await window.openArtifact(entries['model.pt'], '', 'file');
    if (!preview().textContent.includes('No preview for this file type. 9 B')) fail('no preview', preview().textContent);

    await window.openArtifact(entries['many'], '', 'directory');
    if (preview().querySelectorAll('tbody tr').length !== 200) fail('directory');
    await window.loadArtifactDirectory(entries['many'], '', 200);
    if (!preview().textContent.includes('Not included in this static export')) fail('later page', preview().textContent);
    await window.openArtifact(entries['many'], 'f001.txt', 'file');
    if (!preview().textContent.includes('Not included in this static export')) fail('child', preview().textContent);
    process.exit(0);
  } catch (error) { fail('error', error.stack); }
}, 300);
`
	if out, err := exec.Command("node", "-e", script, output, f.runID, entries).CombinedOutput(); err != nil {
		t.Fatalf("static previews: %v\n%s", err, out)
	}
}

func TestStaticArtifactContentsRules(t *testing.T) {
	f := newArtifactFixture(t)
	options := testOptions(f.baseDir, false)
	collector := newStaticArtifactCollector(site{Options: options}, f.baseDir)
	listing, err := webArtifacts(options.Store, f.baseDir, "default", f.runID, "job-1", "")
	if err != nil {
		t.Fatal(err)
	}
	previewable := collector.collect("default", f.runID, "job-1", "", webArtifactListing{ArtifactListing: listing})
	for name, want := range map[string]bool{"plot.png": true, "many": true, "secret.txt": false, "link.csv": false, "gone.csv": false, "rel.csv": false} {
		if previewable[f.entries[name]] != want {
			t.Errorf("%s previewable = %v, want %v", name, previewable[f.entries[name]], want)
		}
	}

	large := filepath.Join(f.work, "large.bin")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(staticArtifactFileLimit + 1); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if collector.collectEntry("large", servedArtifact{root: f.work, relative: "large.bin"}) {
		t.Fatal("a file over the per-file limit was copied")
	}
	// A file already copied is reused; a new one past the total is not copied.
	write(t, filepath.Join(f.work, "fresh.csv"), "a,b\n")
	collector.bytes = staticArtifactTotalLimit - 1
	if !collector.collectEntry("reused", servedArtifact{root: f.work, relative: "data.csv"}) {
		t.Fatal("a file already copied was not reused")
	}
	if collector.collectEntry("budget", servedArtifact{root: f.work, relative: "fresh.csv"}) {
		t.Fatal("a file past the total limit was copied")
	}
}
