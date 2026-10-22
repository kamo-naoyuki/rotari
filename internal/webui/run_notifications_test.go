package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNotificationInboxWithoutDesktopAPI(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const errors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
  runScripts: 'dangerously', url: 'http://example.test/', virtualConsole,
  beforeParse(window) {
    window.Notification = undefined;
    window.isSecureContext = false;
    window.localStorage.setItem('rotari-notifications-enabled', 'false');
    window.setInterval = () => 1;
    window.fetch = async () => { throw new Error('offline'); };
  },
});
setTimeout(() => {
  try {
    const w = dom.window, d = w.document;
    const emit = () => {
      if (typeof w.notifyRunEvent === 'function')
        w.notifyRunEvent({projectName: '<project>', runID: 'run', basedirID: 'other', run: {exit_code: 1}},
          [], [], true, {fields: ['project', 'link']});
      else w.notifyJobsActivity({state: 'failed', project: '<project>', job: 'job', runURL: '/other/run'});
    };
    emit();
    const button = d.querySelector('.notification-inbox > button');
    const panel = d.getElementById('notification-inbox-panel');
    if (!button || button.textContent !== 'Inbox (1)' || !panel.hidden) throw Error('missing unread badge');
    if (!panel.textContent.includes('<project>') || panel.querySelector('project')) throw Error('unsafe content');
    const link = panel.querySelector('a');
    if (!link || (typeof w.notifyRunEvent === 'function' && !link.href.includes('/_basedir/other/project/')))
      throw Error('missing exact basedir link');
    button.click();
    if (panel.hidden || button.textContent !== 'Inbox (0)') throw Error('not marked read');
    emit();
    if (button.textContent !== 'Inbox (0)') throw Error('open inbox became unread');
    d.dispatchEvent(new w.KeyboardEvent('keydown', {key: 'Escape'}));
    if (!panel.hidden || button.getAttribute('aria-expanded') !== 'false') throw Error('Escape failed');
    for (let i = 0; i < 60; i++) emit();
    if (panel.querySelectorAll('article').length !== 50 || button.textContent !== 'Inbox (50)')
      throw Error('inbox not bounded');
    button.click();
    panel.querySelector('button').click();
    if (panel.querySelector('article') || button.textContent !== 'Inbox (0)') throw Error('clear failed');
    if (errors.length) throw Error(errors.join('\n'));
    dom.window.close();
  } catch (error) { console.error(error); process.exit(1); }
}, 50);
`
	for name, page := range map[string]string{
		"application":  testSite().webHTML(),
		"job activity": jobsHTML("/", nil, nil, "", true, true),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.html")
			if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command("node", "-e", script, path).CombinedOutput(); err != nil {
				t.Fatalf("notification inbox: %v\n%s", err, output)
			}
		})
	}
}

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
		dom.window.document.querySelector('#notification-inbox-panel > button').click();
		try {
			await dom.window.checkRunNotifications(test.before, test.after, '');
		} catch (error) {
			failures.push(test.name + ': threw ' + (error.stack || error));
			continue;
		}
		const titles = shown.map(text => text.split(' | ')[0]);
		if (JSON.stringify(titles) !== JSON.stringify(test.want))
			failures.push(test.name + ': notified ' + JSON.stringify(titles) + ', want ' + JSON.stringify(test.want));
		const inboxTitles = [...dom.window.document.querySelectorAll('.notification-inbox-list article > a, .notification-inbox-list article > strong')].map(node => node.textContent);
		if (JSON.stringify(inboxTitles) !== JSON.stringify(test.want))
			failures.push(test.name + ': inbox notified ' + JSON.stringify(inboxTitles));
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

// Browsers allow notifications only in a secure context, and an insecure page
// may not expose Notification at all. The toggle on the application and Job
// activity pages then stays visible but disabled, and says why, instead of
// disappearing or being disabled silently.
func TestNotificationToggleExplainsWhyItIsUnavailable(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const html = fs.readFileSync(process.argv[1], 'utf8');
const cases = [
	{ name: 'secure, permission not asked', secure: true, permission: 'default', hidden: false, disabled: false, title: '' },
	{ name: 'secure, permission denied', secure: true, permission: 'denied', hidden: false, disabled: true, title: 'blocked' },
	{ name: 'secure, no Notification API', secure: true, permission: null, hidden: true },
	{ name: 'insecure, permission denied', secure: false, permission: 'denied', hidden: false, disabled: true, title: 'localhost' },
	{ name: 'insecure, no Notification API', secure: false, permission: null, hidden: false, disabled: true, title: 'localhost' },
];
const failures = [];
let pending = cases.length;
for (const test of cases) {
	const errors = [];
	const virtualConsole = new VirtualConsole();
	virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
	const dom = new JSDOM(html, {
		runScripts: 'dangerously',
		url: 'http://example.test/',
		virtualConsole,
		beforeParse(window) {
			window.isSecureContext = test.secure;
			if (test.permission !== null)
				window.Notification = { permission: test.permission, requestPermission: async () => test.permission };
			window.setInterval = () => 1;
			window.fetch = async () => { throw new Error('offline'); };
		},
	});
	setTimeout(() => {
		const button = dom.window.document.getElementById('notify-toggle');
		const got = !button ? 'no button'
			: button.hidden ? 'hidden'
			: 'disabled=' + button.disabled + ' title=' + JSON.stringify(button.title);
		const ok = button && button.hidden === test.hidden &&
			(test.hidden || (button.disabled === test.disabled && (test.title === '' ? button.title === '' : button.title.includes(test.title))));
		if (!ok) failures.push(test.name + ': got ' + got);
		failures.push(...errors.map(error => test.name + ': ' + error));
		if (--pending === 0 && failures.length) {
			console.error(failures.join('\n'));
			process.exit(1);
		}
	}, 50);
}
`
	pages := map[string]string{
		"application":  testSite().webHTML(),
		"job activity": jobsHTML("/", nil, nil, "", true, true),
	}
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			htmlPath := filepath.Join(t.TempDir(), "index.html")
			if err := os.WriteFile(htmlPath, []byte(page), 0o600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
				t.Fatalf("notification toggle: %v\n%s", err, output)
			}
		})
	}
}
