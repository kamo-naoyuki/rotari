package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The All projects page offers Delete on each project's row, disabled for a
// running project, and Delete posts the project to /api/delete-project.
func TestWebAllProjectsDeleteProject(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(htmlPath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const state = { base_dir: '/state', projects: [
  { project_name: 'idle', runs: [], run_count: 0 },
  { project_name: 'busy', runs: [], run_count: 1, running_run_id: 'run-1' },
] };
const requests = [];
const confirms = [];
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously',
  url: 'http://127.0.0.1/',
  virtualConsole,
  beforeParse(window) {
    window.setInterval = () => 1;
    window.alert = message => errors.push('alert: ' + message);
    window.confirm = message => { confirms.push(message); return true; };
    window.fetch = async (url, options = {}) => {
      if (url === '/api/state') return { ok: true, json: async () => state };
      requests.push({ url, body: options.body && JSON.parse(options.body) });
      return { ok: true, text: async () => '{"message":"deleted project=idle","warnings":[]}', json: async () => ({}) };
    };
  },
});
setTimeout(async () => {
  const assert = (condition, message) => { if (!condition) throw new Error(message); };
  try {
    const document = dom.window.document;
    const button = name => document.querySelector('#app .delete-project-row[data-project="' + name + '"]');
    assert(button('idle') && !button('idle').disabled, 'idle project has no enabled Delete button');
    assert(button('busy') && button('busy').disabled, 'running project Delete button is not disabled');
    await dom.window.deleteProject('idle');
    assert(confirms.length === 1 && confirms[0].includes('idle'), 'Delete did not ask to confirm the project');
    const request = requests.find(request => request.url === '/api/delete-project');
    assert(request && request.body.project_name === 'idle', 'Delete did not post the project');
    // jsdom does not implement the navigation to the reloaded page.
    const unexpected = errors.filter(error => !error.includes('Not implemented: navigation'));
    if (unexpected.length) throw new Error(unexpected.join('\n'));
  } catch (error) {
    console.error(error.stack || String(error));
    process.exit(1);
  }
  process.exit(0);
}, 100);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("All projects delete check failed: %v\n%s", err, output)
	}
}
