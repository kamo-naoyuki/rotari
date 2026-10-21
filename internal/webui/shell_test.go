package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/joblist"
)

// A basedir is labelled by its parent directory and its own name, which tell
// basedirs named .rotari-state apart.
func TestNewWebBaseDirLabelsByParentAndName(t *testing.T) {
	for _, tc := range []struct {
		path, parent, name, html string
	}{
		{"/home/kamo/work/asr/.rotari-state", "asr", ".rotari-state", `<span class="basedir-parent">asr</span><span class="basedir-name">/.rotari-state</span>`},
		{"/state", "", "state", `<span class="basedir-name">state</span>`},
		{"/data/a<b>/s", "a<b>", "s", `<span class="basedir-parent">a&lt;b&gt;</span><span class="basedir-name">/s</span>`},
	} {
		entry := newWebBaseDir("id", tc.path, false)
		if entry.Parent != tc.parent || entry.Name != tc.name {
			t.Errorf("newWebBaseDir(%q) = parent %q name %q, want %q %q", tc.path, entry.Parent, entry.Name, tc.parent, tc.name)
		}
		if got := basedirLabelHTML(entry); got != tc.html {
			t.Errorf("basedirLabelHTML(%q) = %s, want %s", tc.path, got, tc.html)
		}
	}
}

func TestWebShellUsesFullWidth(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	for _, tc := range []struct {
		name, html, css string
	}{
		{"web", testSite().webHTML(), webStylesCSS},
		{"jobs", jobsHTML("/", []string{"demo"}, nil, joblist.DefaultSinceText, true, true), webInfoStylesCSS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			htmlPath := filepath.Join(dir, "index.html")
			cssPath := filepath.Join(dir, "styles.css")
			if err := os.WriteFile(htmlPath, []byte(tc.html), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cssPath, []byte(tc.css+"\n"+webShellCSS), 0o600); err != nil {
				t.Fatal(err)
			}
			script := `
const fs = require('fs');
const { JSDOM } = require('jsdom');
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'));
const style = dom.window.document.createElement('style');
style.textContent = fs.readFileSync(process.argv[2], 'utf8');
dom.window.document.head.append(style);
const main = dom.window.getComputedStyle(dom.window.document.querySelector('main'));
if (main.width !== '100%' || main.maxWidth !== 'none') {
  console.error('page frame must fill the viewport without a width cap:', main.width, main.maxWidth);
  process.exit(1);
}
dom.window.close();
`
			if output, err := exec.Command("node", "-e", script, htmlPath, cssPath).CombinedOutput(); err != nil {
				t.Fatalf("page width: %v\n%s", err, output)
			}
		})
	}
}

// The Settings menu opens from its button and closes on Escape and on a click
// outside it; the drawer opens from its toggle and closes when a sidebar
// link is followed or on Escape. Both page kinds run the same shell script.
func TestWebShellMenuAndDrawer(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	pages := map[string]string{
		"web":  testSite().webHTML(),
		"jobs": jobsHTML("/", []string{"demo"}, nil, joblist.DefaultSinceText, true, true),
	}
	script := `
const { JSDOM } = require('jsdom');
const fs = require('fs');
const html = fs.readFileSync(process.argv[1], 'utf8');
const shell = html.match(/<script>([\s\S]*?)<\/script>/)[1];
const dom = new JSDOM(html.replace(/<script>[\s\S]*?<\/script>/g, ''), { url: 'http://localhost/', runScripts: 'dangerously' });
const window = dom.window, document = window.document;
window.eval(shell);
document.dispatchEvent(new window.Event('DOMContentLoaded'));
const fail = (message) => { console.error(message); process.exit(1); };
const panel = document.getElementById('settings-panel');
const button = document.getElementById('settings-button');
if (!panel.hidden) fail('settings menu starts open');
button.click();
if (panel.hidden || button.getAttribute('aria-expanded') !== 'true') fail('settings button does not open the menu');
document.body.dispatchEvent(new window.MouseEvent('click', { bubbles: true }));
if (!panel.hidden) fail('a click outside does not close the menu');
button.click();
document.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape' }));
if (!panel.hidden) fail('Escape does not close the menu');
const toggle = document.querySelector('.drawer-toggle');
toggle.click();
if (!document.body.classList.contains('drawer-open') || toggle.getAttribute('aria-expanded') !== 'true') fail('drawer toggle does not open the drawer');
const link = document.querySelector('.sidebar a');
link.addEventListener('click', (event) => event.preventDefault());
link.click();
if (document.body.classList.contains('drawer-open')) fail('following a sidebar link leaves the drawer open');
toggle.click();
document.dispatchEvent(new window.KeyboardEvent('keydown', { key: 'Escape' }));
if (document.body.classList.contains('drawer-open')) fail('Escape does not close the drawer');
`
	for name, page := range pages {
		path := filepath.Join(t.TempDir(), name+".html")
		if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command("node", "-e", script, path).CombinedOutput(); err != nil {
			t.Fatalf("%s page shell: %v\n%s", name, err, output)
		}
	}
}
