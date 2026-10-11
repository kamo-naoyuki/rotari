package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The Web UI writes dates in English, like the rest of its text, even when
// the browser's language is Japanese: the Activity page's month and day
// titles, and dates and times written in the browser.
func TestWebDatesAreEnglishWhateverTheBrowserLanguage(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(htmlPath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const assert = require('node:assert/strict');
const { JSDOM } = require('jsdom');
assert.equal(new Date(2026, 9, 11).toLocaleDateString(undefined, { month: 'long' }), '10月', 'the test does not run in a Japanese locale');
const day = new Date(); day.setHours(9, 0, 0, 0);
const activity = {
  now: new Date().toISOString(), latest_started_at: day.toISOString(),
  basedirs: [{ id: 'A', path: '/work/.rotari-state', parent: 'work', name: '.rotari-state', current: true }],
  runs: [{ basedir_id: 'A', project_name: 'train', run_id: 'r1', lifecycle: 'finished', exit_code: 0,
    started_at: day.toISOString(), ends_at: new Date(day.getTime() + 3600000).toISOString(), note_count: 0 }],
};
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/activity/',
  beforeParse(window) {
    window.setInterval = () => 1;
    window.fetch = async (url) => String(url).startsWith('/api/activity?')
      ? { ok: true, json: async () => activity }
      : { ok: true, json: async () => ({ base_dir: '/work/.rotari-state', projects: [] }) };
  },
});
setTimeout(() => {
  try {
    const document = dom.window.document;
    const japanese = /[\u3040-\u30ff\u4e00-\u9fff]/;
    const month = document.querySelector('.activity-month-title').textContent;
    assert.match(month, /^[A-Z][a-z]+ \d{4}$/);
    const heading = document.querySelector('.activity-day h2').textContent;
    assert(!japanese.test(heading) && /day/.test(heading), heading);
    assert(!japanese.test(document.querySelector('#app').textContent), 'the Activity page has Japanese text');
    assert.equal(dom.window.formatDateTime(new Date(2026, 9, 11, 5, 2, 3)), '2026-10-11 05:02:03');
    assert.equal(dom.window.formatDate(new Date(2026, 9, 11), { weekday: 'short', month: 'short', day: 'numeric' }), 'Sun, Oct 11');
  } catch (error) {
    console.error(error.stack || String(error));
    process.exit(1);
  }
  process.exit(0);
}, 300);
`
	command := exec.Command("node", "-e", script, htmlPath)
	command.Env = append(os.Environ(), "LANG=ja_JP.UTF-8", "LC_ALL=ja_JP.UTF-8")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("English dates check failed: %v\n%s", err, output)
	}
}
