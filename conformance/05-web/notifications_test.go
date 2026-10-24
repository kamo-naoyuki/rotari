package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rotari-dev/rotari/conformance/support"
)

func TestWebNotificationInboxWithoutDesktopPermission(t *testing.T) {
	covers(t, "WEB-21")
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	e := support.NewEnv(t)
	base := e.StartWeb()
	script := `
const {JSDOM, VirtualConsole} = require('jsdom');
const fs = require('fs');
const errors = [];
const console = new VirtualConsole();
console.on('jsdomError', error => errors.push(String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  url: 'http://example.test/', runScripts: 'dangerously', virtualConsole: console,
  beforeParse(w) {
    w.Notification = undefined;
    w.isSecureContext = false;
    w.setInterval = () => 1;
    w.fetch = async () => { throw Error('offline'); };
  },
});
setTimeout(async () => {
  try {
    const w = dom.window, d = w.document;
    if (typeof w.checkRunNotifications === 'function') {
      const state = (running) => ({projects: [{project_name: 'demo', runs: [
        {run_id: 'run-1', running, exit_code: running ? undefined : 0, jobs: []}
      ]}]});
      await w.checkRunNotifications(state(true), state(false), '');
      await w.checkRunNotifications(state(false), state(false), '');
    } else {
      w.notifyJobsActivity({state: 'failed', project: 'demo', job: 'job-1', runURL: '/project/demo/run/run-1'});
    }
    const button = d.querySelector('.notification-inbox > button');
    if (!button || button.textContent !== 'Inbox (1)') throw Error('missing unread event');
    button.click();
    const panel = d.getElementById('notification-inbox-panel');
    if (panel.hidden || button.textContent !== 'Inbox (0)' || !panel.querySelector('a'))
      throw Error('inbox not readable');
    if (errors.length) throw Error(errors.join('\n'));
    w.close();
  } catch (error) { process.stderr.write(String(error)); process.exit(1); }
}, 50);
`
	for _, route := range []string{"/", "/jobs/"} {
		t.Run(route, func(t *testing.T) {
			response := e.HTTPGet(base + route)
			if response.Status != 200 {
				t.Fatalf("page status = %d: %s", response.Status, response.Body)
			}
			path := filepath.Join(t.TempDir(), "index.html")
			if err := os.WriteFile(path, []byte(response.Body), 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command("node", "-e", script, path).CombinedOutput(); err != nil {
				t.Fatalf("inbox: %v\n%s", err, output)
			}
		})
	}
}
