package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Browser notifications report what happened while the page watched: a run
// that finished, or a job that ended, since the previous poll. A run or job
// that only appears because the page loaded it, such as an older run opened
// from its project, finished before and must not notify. A run that both
// started and finished between two polls is newer than every run the page
// knew, and does notify.
func TestBrowserNotificationsReportOnlyWhatHappenedSinceThePreviousPoll(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const shown = [];
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
	runScripts: 'dangerously',
	url: 'https://example.test/',
	virtualConsole,
	beforeParse(window) {
		window.Notification = class {
			constructor(title, options) { shown.push(title + ' | ' + options.body); }
		};
		window.Notification.permission = 'granted';
		window.Notification.requestPermission = async () => 'granted';
		window.localStorage.setItem('rotari-notifications-enabled', 'true');
		window.setInterval = () => 1;
		// Notification settings fall back to the defaults: job failures, run
		// failures, and run successes notify; job successes do not.
		window.fetch = async () => { throw new Error('offline'); };
	},
});
const job = (id, status, final) => ({ id, name: id, execution_status: status, final });
const run = (id, running, exitCode, jobs) => ({ run_id: id, running, exit_code: exitCode, jobs });
const state = (...runs) => ({ projects: [{ project_name: 'p', runs }] });
const older = run('20261010-090000-aaaaaaaa', false, 1);
const latest = run('20261010-100000-bbbbbbbb', false, 0);
const cases = [
	{
		name: 'a watched run finishes with a failed job',
		before: state(run('20261010-110000-cccccccc', true, undefined, [job('j1', 'running', false)])),
		after: state(run('20261010-110000-cccccccc', false, 1, [job('j1', 'failed', true)])),
		want: ['rotari: run failed (1 job failed)'],
	},
	{
		name: 'a job of a watched run fails',
		before: state(run('20261010-110000-cccccccc', true, undefined, [job('j1', 'running', false), job('j2', 'running', false)])),
		after: state(run('20261010-110000-cccccccc', true, undefined, [job('j1', 'failed', true), job('j2', 'running', false)])),
		want: ['rotari: 1 job failed'],
	},
	{
		name: 'a run starts and finishes between two polls',
		before: state(latest, older),
		after: state(run('20261010-120000-dddddddd', false, 0, [job('j1', 'success', true)]), latest),
		want: ['rotari: run succeeded'],
	},
	{
		name: 'an old run is opened and its jobs load',
		before: state(latest, older),
		after: state(latest, { ...older, jobs: [job('j1', 'failed', true), job('j2', 'failed', true)] }),
		want: [],
	},
	{
		name: 'a project page loads an older run',
		before: state(latest),
		after: state(latest, { ...older, jobs: [job('j1', 'failed', true)] }),
		want: [],
	},
];
setTimeout(async () => {
	const failures = [];
	for (const test of cases) {
		shown.length = 0;
		try {
			await dom.window.checkRunNotifications(test.before, test.after, '');
		} catch (error) {
			failures.push(test.name + ': threw ' + (error.stack || error));
			continue;
		}
		const titles = shown.map(text => text.split(' | ')[0]);
		if (JSON.stringify(titles) !== JSON.stringify(test.want))
			failures.push(test.name + ': notified ' + JSON.stringify(titles) + ', want ' + JSON.stringify(test.want));
	}
	if (errors.length) failures.push(...errors);
	if (failures.length) {
		console.error(failures.join('\n'));
		process.exit(1);
	}
}, 50);
`
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(htmlPath, []byte(testSite().webHTML()), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("browser notifications: %v\n%s", err, output)
	}
}
