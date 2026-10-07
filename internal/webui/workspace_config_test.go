package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/model"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

func TestWebWorkspaceConfigSourcesAndSelectedSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home, _ := config.HomeDir()
	cwd, base := t.TempDir(), t.TempDir()
	project := filepath.Join(base, "projects", "demo")
	for _, dir := range []string{home, project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sources := []struct{ scope, path, content string }{{"global", filepath.Join(home, "config.toml"), "quiet = true\n"}, {"workspace", filepath.Join(cwd, config.WorkspaceFile), "basedir = 'state'\n"}, {"basedir", filepath.Join(base, "config.toml"), "project-name = 'demo'\n"}, {"project", filepath.Join(project, "config.toml"), "[run]\nretry = 2\n"}}
	for _, source := range sources {
		if err := os.WriteFile(source.path, []byte(source.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	options := testOptions(base, true)
	options.WorkspaceDir = cwd
	handler := Handler(options)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/config?project_name=demo", nil))
	var payload struct {
		Configs []webprojection.ConfigFile `json:"configs"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Configs) != 4 {
		t.Fatalf("sources = %s", response.Body.String())
	}
	for i, file := range payload.Configs {
		if file.Scope != sources[i].scope || file.Path != sources[i].path || file.Content != sources[i].content {
			t.Fatalf("file = %#v", file)
		}
	}
	save := func(scope, path, content string) int {
		data, _ := json.Marshal(webSaveConfigRequest{QueueName: "demo", Scope: scope, Path: path, Content: content})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/save-config", strings.NewReader(string(data))))
		return response.Code
	}
	if code := save("workspace", sources[1].path, "basedir = 'new-state'\nquiet = false\n"); code != http.StatusOK {
		t.Fatalf("selected save = %d", code)
	}
	for _, i := range []int{0, 2, 3} {
		data, _ := os.ReadFile(sources[i].path)
		if string(data) != sources[i].content {
			t.Fatalf("changed unrelated %s", sources[i].path)
		}
	}
	for _, test := range []struct{ scope, path, content string }{{"project", sources[3].path, "[run]\nproject-name = 'bad'\n"}, {"basedir", sources[2].path, "[show]\nbasedir = 'bad'\n"}, {"global", filepath.Join(t.TempDir(), "outside.toml"), "quiet = false\n"}, {"project", sources[1].path, "quiet = false\n"}, {"", "", "quiet = false\n"}} {
		if code := save(test.scope, test.path, test.content); code == http.StatusOK {
			t.Fatalf("accepted %#v", test)
		}
	}
	// A saved merged run has more sources than snapshots, and sources need
	// not still exist. Both live and static projections read the copy only.
	runDir := filepath.Join(project, "runs", "run-1")
	if err := os.MkdirAll(filepath.Join(runDir, "configs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := options.Store.WriteJSON(filepath.Join(runDir, "context.json"), model.RunContext{ConfigPaths: []string{"deleted-a", "deleted-b"}, ConfigSnapshotFiles: []string{"config.toml"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "configs", "config.toml"), []byte("quiet = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/config?project_name=demo&run_id=run-1", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "quiet = true") {
		t.Fatalf("run = %d %s", response.Code, response.Body.String())
	}
}

func TestWebConfigSourceChooser(t *testing.T) {
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
const errors = [];
const console = new VirtualConsole();
console.on('jsdomError', error => errors.push(String(error)));
const dom = new JSDOM(fs.readFileSync(process.argv[1], 'utf8'), {
	runScripts: 'dangerously', url: 'http://localhost/', virtualConsole: console,
	beforeParse(window) {
		window.fetch = async () => ({ok:true,json:async()=>({projects:[]}),text:async()=>JSON.stringify({configs:[]})});
		window.setInterval = () => 1;
		window.alert = () => {};
	}
});
setTimeout(async () => {
	try {
		const files = [{scope:'global',path:'/global/config.toml',content:'quiet = true\n'}, {scope:'workspace',path:'/work/.rotari.toml',content:'basedir = "state"\n'}];
		let saved;
		dom.window.fetch = async (url, options) => {
			if (url.startsWith('/api/config?')) return {ok:true,text:async()=>JSON.stringify({configs:files})};
			if (url === '/api/save-config') { saved = JSON.parse(options.body); return {ok:true,text:async()=>JSON.stringify({path:saved.path})}; }
			return {ok:true,json:async()=>({projects:[]}),text:async()=>'{"projects":[]}'};
		};
		await dom.window.showConfig();
		const chooser = dom.window.document.getElementById('config-source-select');
		if (!chooser || chooser.options.length !== 2 || !chooser.options[1].textContent.includes('workspace: /work/.rotari.toml')) throw Error('missing sources');
		chooser.value = '1'; chooser.dispatchEvent(new dom.window.Event('change'));
		const editor = dom.window.document.getElementById('config-editor');
		const text = editor.querySelector('textarea');
		if (text.value !== files[1].content || text.value.includes('quiet')) throw Error('synthesized or wrong file');
		text.value = 'basedir = "new-state"\n'; text.dispatchEvent(new dom.window.Event('input'));
		await editor.querySelector('button').onclick();
		if (!saved || saved.scope !== 'workspace' || saved.path !== '/work/.rotari.toml' || saved.content !== text.value) throw Error('wrong save target');
		files.splice(0, 1);
		await dom.window.showConfig();
		if (dom.window.document.getElementById('config-source-select')) throw Error('single source needs no chooser');
		if (errors.length) throw Error(errors.join('\n'));
	} catch(error) { process.stderr.write(String(error)); process.exitCode = 1; }
	dom.window.close();
}, 50);
`
	if output, err := exec.Command("node", "-e", script, htmlPath).CombinedOutput(); err != nil {
		t.Fatalf("chooser: %v\n%s", err, output)
	}
}
