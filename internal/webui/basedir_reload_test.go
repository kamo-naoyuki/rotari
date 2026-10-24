package webui

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rotari-dev/rotari/internal/model"
	stateinternal "github.com/rotari-dev/rotari/internal/state"
	webprojection "github.com/rotari-dev/rotari/internal/web"
)

// The server rereads the basedir registry on a page load, on /api/basedirs
// (the Refresh button), and for an unknown mount, never on the 2-second polls.
func TestWebRereadsRegisteredBasedirsWithoutRestart(t *testing.T) {
	rootBaseDir, addedBaseDir := t.TempDir(), t.TempDir()
	paths, err := stateinternal.ResolveProjectPaths(addedBaseDir, "added-project")
	if err != nil {
		t.Fatal(err)
	}
	if err := stateinternal.WriteJSON(paths.QueueFile, model.Queue{}); err != nil {
		t.Fatal(err)
	}
	var registered []string
	var failure error
	reloads := 0
	options := testOptions(rootBaseDir, false)
	options.ReloadBaseDirs = func() ([]string, error) {
		reloads++
		return registered, failure
	}
	handler := Handler(options)
	addedID := basedirID(addedBaseDir)

	registered = []string{addedBaseDir}
	if response := serveWebGet(t, handler, "/api/state"); response.Code != http.StatusOK || reloads != 0 {
		t.Fatalf("GET /api/state = %d after %d registry reads, want 200 without reading the registry", response.Code, reloads)
	}
	response := serveWebGet(t, handler, "/api/basedirs")
	var listed struct {
		BaseDirs []webBaseDir `json:"basedirs"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &listed) != nil {
		t.Fatalf("GET /api/basedirs = (%d, %q)", response.Code, response.Body.String())
	}
	if len(listed.BaseDirs) != 2 || listed.BaseDirs[0].ID != addedID && listed.BaseDirs[1].ID != addedID {
		t.Fatalf("GET /api/basedirs = %+v, want the startup and the newly registered basedir", listed.BaseDirs)
	}
	assertWebStateBasedir(t, handler, "/_basedir/"+addedID+"/api/state", addedBaseDir)
	if reloads != 1 {
		t.Fatalf("registry read %d times, want once, by /api/basedirs", reloads)
	}

	registered = nil
	if body := serveWebGet(t, handler, "/").Body.String(); strings.Contains(body, addedID) {
		t.Fatal("page load still lists an unregistered basedir")
	}
	if response := serveWebGet(t, handler, "/_basedir/"+addedID+"/api/state"); response.Code != http.StatusNotFound {
		t.Fatalf("GET unregistered basedir mount = %d, want 404", response.Code)
	}

	registered = []string{addedBaseDir}
	response = serveWebGet(t, handler, "/_basedir/"+addedID+"/")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), addedID) {
		t.Fatalf("page opened at a newly registered basedir = %d, want it served and listed", response.Code)
	}

	failure = errors.New("registry unavailable")
	if response := serveWebGet(t, handler, "/api/basedirs"); response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "registry unavailable") {
		t.Fatalf("GET /api/basedirs with an unreadable registry = (%d, %q), want 500 with the cause", response.Code, response.Body.String())
	}
	assertWebStateBasedir(t, handler, "/_basedir/"+addedID+"/api/state", addedBaseDir)
}

// Refresh rereads the basedir list and the other basedirs' project lists; the
// 2-second poll reads neither.
func TestWebRefreshButtonRereadsBasedirs(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	rootBaseDir, otherBaseDir, addedBaseDir := t.TempDir(), t.TempDir(), t.TempDir()
	registered := []string{otherBaseDir}
	options := testOptions(rootBaseDir, false)
	options.ReloadBaseDirs = func() ([]string, error) { return registered, nil }
	handler := Handler(options)
	page := serveWebGet(t, handler, "/").Body.String()
	registered = []string{otherBaseDir, addedBaseDir}
	basedirs := serveWebGet(t, handler, "/api/basedirs").Body.String()
	pagePath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(pagePath, []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := json.Marshal(webprojection.State{BaseDir: rootBaseDir, Queues: []webprojection.QueueState{{QueueName: "root-project", Runs: []webprojection.Run{}}}})
	if err != nil {
		t.Fatal(err)
	}
	script := `
const fs = require('fs');
const { JSDOM, VirtualConsole } = require('jsdom');
const [, pagePath, state, basedirs, otherID, addedID] = process.argv;
const requests = [];
const errors = [];
let otherProjects = ['old-project'];
const virtualConsole = new VirtualConsole();
virtualConsole.on('jsdomError', error => errors.push(error.stack || String(error)));
const json = value => ({ok: true, json: async () => JSON.parse(value)});
const dom = new JSDOM(fs.readFileSync(pagePath, 'utf8'), {
	runScripts: 'dangerously',
	url: 'http://127.0.0.1/',
	virtualConsole,
	beforeParse(window) {
		window.CanvasRenderingContext2D = function () {};
		window.HTMLCanvasElement.prototype.getContext = () => ({font: "", measureText: text => ({width: text.length * 8})});
		window.fetch = async url => {
			requests.push(String(url));
			if (url === '/api/state') return json(state);
			if (url === '/api/basedirs') return json(basedirs);
			if (url === '/api/projects?basedir_id=' + otherID) return json(JSON.stringify({projects: otherProjects}));
			throw new Error('unexpected fetch: ' + url);
		};
		window.setInterval = () => 1;
	},
});
const settle = () => new Promise(resolve => setTimeout(resolve, 20));
const listed = () => [...dom.window.document.querySelectorAll('#sidebar-basedirs > .basedir-entry')].map(entry => entry.dataset.basedirId);
const projectNames = () => [...dom.window.document.querySelectorAll('#sidebar-basedirs .sidebar-project-link')].map(link => link.textContent.trim());
setTimeout(async () => {
	const assert = (condition, message) => { if (!condition) throw new Error(message); };
	try {
		assert(listed().includes(otherID) && !listed().includes(addedID), 'page did not start with the basedirs registered at load: ' + listed());
		dom.window.document.querySelector('#sidebar-basedirs > [data-basedir-id="' + otherID + '"] .sidebar-toggle').click();
		await settle();
		assert(projectNames().includes('old-project'), 'expanded basedir did not list its projects');
		otherProjects = ['old-project', 'new-project'];
		await dom.window.eval('refresh(false)');
		await settle();
		assert(!requests.includes('/api/basedirs'), 'the 2-second poll reread the basedir list');
		assert(!listed().includes(addedID) && !projectNames().includes('new-project'), 'the 2-second poll changed the basedir sidebar');
		dom.window.document.getElementById('refresh-button').click();
		await settle();
		assert(requests.includes('/api/basedirs'), 'Refresh did not reread the basedir list');
		assert(listed().includes(addedID), 'Refresh did not show the newly registered basedir: ' + listed());
		assert(projectNames().includes('new-project'), 'Refresh did not reread an expanded basedir\'s projects');
		if (errors.length) throw new Error(errors.join('\n'));
	} catch (error) {
		console.error(error.stack || String(error));
		process.exit(1);
	}
}, 50);
`
	if output, err := exec.Command("node", "-e", script, pagePath, string(state), basedirs, basedirID(otherBaseDir), basedirID(addedBaseDir)).CombinedOutput(); err != nil {
		t.Fatalf("Refresh basedir check failed: %v\n%s", err, output)
	}
}
