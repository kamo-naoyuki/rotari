package webui

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestActiveSidebarProjectCanBeCollapsed(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	start := strings.Index(webAppCoreJS, "function activeSidebarProjectsHTML(")
	end := strings.Index(webAppCoreJS, "function sidebarBasedirHTML(")
	if start < 0 || end <= start {
		t.Fatal("active sidebar project renderer was not found")
	}
	script := `
const expandedSidebarProjects = {};
function esc(value) { return String(value); }
function basedirURL(id, path) { return "/_basedir/" + id + path; }
function sidebarRunLinksHTML() { return '<a class="sidebar-run">run-1</a>'; }
` + webAppCoreJS[start:end] + `
const entry = {id: "basedir-1"};
const project = {project_name: "demo", runs: [{run_id: "run-1"}]};
const first = activeSidebarProjectsHTML(entry, [project], "demo", "run-1");
if (!first.includes('class="sidebar-project expanded"')) process.exit(1);
if (!first.includes('aria-expanded="true"')) process.exit(2);
expandedSidebarProjects["basedir-1/demo"] = false;
const collapsed = activeSidebarProjectsHTML(entry, [project], "demo", "run-1");
if (!collapsed.includes('class="sidebar-project"')) process.exit(3);
if (!collapsed.includes('aria-expanded="false"')) process.exit(4);
if (!collapsed.includes('class="sidebar-runs" hidden></div>')) process.exit(5);
`
	path := t.TempDir() + "/sidebar-collapse.js"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", path).CombinedOutput(); err != nil {
		t.Fatalf("active project sidebar collapse failed: %v\n%s", err, output)
	}
}

func TestWebHeaderBrandLinksToHome(t *testing.T) {
	html := testSite().webHTML()
	if !webContains(html, `<a class="header-home" href="/"><img class="brand-icon"`) {
		t.Fatal("header brand logo is not a home link")
	}
	if !webContains(html, `rotari Web</a>`) {
		t.Fatal("header brand link does not include the page title")
	}
}

func TestJobsSidebarLinksToNotificationConfig(t *testing.T) {
	html := jobsHTMLWithSession("/_basedir/base-1/", nil, nil, "24h", true, true, "session")
	for _, want := range []string{
		`href="/_basedir/base-1/?rotari-action=notification-config">Notification settings</a>`,
		`href="/_basedir/base-1/?rotari-action=generate-notification-config">Generate notification settings</a>`,
		`id="notify-toggle"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("Job activity sidebar is missing notification action %q", want)
		}
	}
	controls := strings.Index(html, `class="sidebar-config-controls"`)
	registered := strings.Index(html, "Registered basedirs")
	if controls < 0 || registered <= controls {
		t.Fatal("Job activity notification controls are not above Registered basedirs")
	}
}

func TestOpenRequestedConfigActionDispatchesAndCleansURL(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	start := strings.Index(webAppCoreJS, "function openRequestedConfigAction() {")
	end := strings.Index(webAppCoreJS[start:], "function renderOverview(")
	if start < 0 || end < 0 {
		t.Fatal("notification config URL action handler was not found")
	}
	script := `
const window = { location: { href: "https://example.test/?rotari-action=notification-config&keep=1#top" } };
let cleanedURL = "";
const history = { replaceState(_state, _title, url) { cleanedURL = url; } };
let opened = "";
async function showNotificationConfig() { opened = "notification-config"; }
async function showGenerateNotificationConfig() { opened = "generate-notification-config"; }
function alert(message) { throw new Error(message); }
` + webAppCoreJS[start:start+end] + `
openRequestedConfigAction();
setTimeout(() => {
  if (opened !== "notification-config") process.exit(1);
  if (cleanedURL !== "/?keep=1#top") process.exit(2);
}, 0);
`
	path := t.TempDir() + "/config-action.js"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("node", path).CombinedOutput(); err != nil {
		t.Fatalf("notification config URL action failed: %v\n%s", err, output)
	}
}
