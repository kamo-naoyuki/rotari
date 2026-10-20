package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/launcher"
)

// launchWrapperEnv makes the test binary act as `rotari __launch`.
const launchWrapperEnv = "ROTARI_TEST_WEBUI_LAUNCH_WRAPPER"

func TestMain(m *testing.M) {
	if os.Getenv(launchWrapperEnv) == "1" {
		os.Exit(launcher.RunWrapper(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// launchOptions enables launching with commands, run by the test binary as
// the wrapper.
func launchOptions(t *testing.T, baseDir string, commands ...launcher.Command) Options {
	t.Helper()
	manager, err := launcher.NewManager(launcher.ManagerConfig{
		Wrapper: []string{os.Args[0]},
		Env:     append(os.Environ(), launchWrapperEnv+"=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { manager.Close() })
	options := testOptions(baseDir, true)
	options.Launches = &LaunchOptions{Manager: manager, Registry: launcher.Registry{Path: "/cfg/rotari/launchers.toml", Commands: commands}}
	options.LauncherFile = "/cfg/rotari/launchers.toml"
	return options
}

func launchCommand(name, command string, targets ...launcher.TargetKind) launcher.Command {
	return launcher.Command{Name: name, Command: command, Targets: targets, Mode: launcher.ModeCommand,
		CWD: launcher.CWDTarget, StopGrace: time.Second}
}

func serveJSON(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if method == http.MethodPost {
		request.Header.Set("Origin", "http://example.com")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return value
}

func TestLaunchAvailabilityExplainsHowToEnable(t *testing.T) {
	baseDir := t.TempDir()
	readOnly := testOptions(baseDir, false)
	readOnly.LauncherFile = "/cfg/rotari/launchers.toml"
	disabled := testOptions(baseDir, true)
	disabled.LauncherFile = "/cfg/rotari/launchers.toml"
	empty := launchOptions(t, baseDir)
	dirOnly := launchOptions(t, baseDir, launchCommand("TensorBoard", "tb", launcher.TargetDirectory))
	for name, test := range map[string]struct {
		options Options
		want    launchAvailability
	}{
		"read-only": {readOnly, launchAvailability{Reason: "read-only", Hint: "--allow-control=false"}},
		"disabled":  {disabled, launchAvailability{Reason: "disabled", Hint: "rotari web --allow-exec"}},
		"no commands": {empty, launchAvailability{Reason: "No commands are registered",
			Hint: "/cfg/rotari/launchers.toml (`rotari config --launchers`"}},
	} {
		t.Run(name, func(t *testing.T) {
			got := decodeBody[launchAvailability](t, serveJSON(t, Handler(test.options), http.MethodGet, "/api/launchers", ""))
			if got.Available || !strings.Contains(got.Reason, test.want.Reason) || !strings.Contains(got.Hint, test.want.Hint) ||
				!strings.Contains(got.Hint, "/cfg/rotari/launchers.toml") || len(got.Commands) != 0 {
				t.Fatalf("availability = %+v, want reason %q and hint %q", got, test.want.Reason, test.want.Hint)
			}
			// The start API gives the same explanation.
			rejected := serveJSON(t, Handler(test.options), http.MethodPost, "/api/launch", `{"name":"TensorBoard","entry":0}`)
			if test.options.Launches == nil {
				body := decodeBody[webLaunchError](t, rejected)
				if rejected.Code != http.StatusForbidden || body.Error != got.Reason || body.Hint != got.Hint {
					t.Fatalf("POST /api/launch = %d %+v, want the availability explanation", rejected.Code, body)
				}
			}
		})
	}

	got := decodeBody[launchAvailability](t, serveJSON(t, Handler(dirOnly), http.MethodGet, "/api/launchers", ""))
	if !got.Available || len(got.Commands) != 1 || got.Commands[0].Command != "tb" || got.Shell != launcher.Shell || got.Host == "" || got.User == "" {
		t.Fatalf("availability = %+v", got)
	}
	if hint := got.Unmatched[launcher.TargetFile]; !strings.Contains(hint, "No registered command accepts a file") || !strings.Contains(hint, "TensorBoard (directory)") {
		t.Fatalf("unmatched file explanation = %q", hint)
	}
	if _, ok := got.Unmatched[launcher.TargetDirectory]; ok {
		t.Fatal("directories are accepted but reported unmatched")
	}
}

func TestLaunchRunsRegisteredCommandOnArtifact(t *testing.T) {
	f := newArtifactFixture(t)
	options := launchOptions(t, f.baseDir,
		launchCommand("Count", `ls "$ROTARI_LAUNCHER_ARTIFACT" | wc -l; pwd -P`, launcher.TargetDirectory),
		launchCommand("Cat", `cat "$ROTARI_LAUNCHER_ARTIFACT"`, launcher.TargetFile),
		launchCommand("Sleep", `sleep 300`, launcher.TargetFile, launcher.TargetDirectory))
	handler := Handler(options)
	start := func(body string) *httptest.ResponseRecorder {
		return serveJSON(t, handler, http.MethodPost, "/api/launch", body)
	}
	request := func(name, entry, child string) string {
		value, _ := json.Marshal(map[string]any{"project_name": "default", "run_id": f.runID, "job_id": "job-1",
			"entry": f.entries[entry], "child": child, "name": name})
		return string(value)
	}
	waitLaunch := func(id string) launcher.Launch {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			for _, launch := range decodeBody[struct{ Launches []launcher.Launch }](t, serveJSON(t, handler, http.MethodGet, "/api/launches", "")).Launches {
				if launch.ID == id && launch.State != launcher.StateRunning {
					return launch
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("launch %s did not end", id)
		return launcher.Launch{}
	}
	logText := func(id string) string {
		return decodeBody[webLaunchLog](t, serveJSON(t, handler, http.MethodGet, "/api/launch-log?id="+id, "")).Text
	}

	directory := start(request("Count", "many", ""))
	launch := decodeBody[launcher.Launch](t, directory)
	if directory.Code != http.StatusOK || launch.Target != filepath.Join(f.work, "many") || launch.Kind != launcher.TargetDirectory ||
		launch.Labels["run"] != f.runID || launch.Labels["job"] != "job-1" {
		t.Fatalf("directory launch = %d %s", directory.Code, directory.Body.String())
	}
	waitLaunch(launch.ID)
	realMany, _ := filepath.EvalSymlinks(filepath.Join(f.work, "many"))
	// 250 files, zz-sub, and the escape symlink.
	if got := logText(launch.ID); got != "252\n"+realMany+"\n" {
		t.Fatalf("directory launch output = %q", got)
	}

	// A file inside a listed directory, named the way previews name it.
	file := decodeBody[launcher.Launch](t, start(request("Cat", "many", "f007.txt")))
	waitLaunch(file.ID)
	if file.Target != filepath.Join(f.work, "many", "f007.txt") || file.CWD != filepath.Join(f.work, "many") || logText(file.ID) != "x" {
		t.Fatalf("file launch = %+v, output %q", file, logText(file.ID))
	}

	// A live launch is reused; Stop ends it.
	first := decodeBody[launcher.Launch](t, start(request("Sleep", "data.csv", "")))
	again := decodeBody[launcher.Launch](t, start(request("Sleep", "data.csv", "")))
	if again.ID != first.ID || !again.Reused {
		t.Fatalf("second start = %+v, want reuse of %s", again, first.ID)
	}
	stopped := serveJSON(t, handler, http.MethodPost, "/api/launch-stop", `{"id":"`+first.ID+`"}`)
	if got := decodeBody[launcher.Launch](t, stopped); stopped.Code != http.StatusOK || got.State != launcher.StateStopped {
		t.Fatalf("stop = %d %s", stopped.Code, stopped.Body.String())
	}

	for name, test := range map[string]struct {
		body   string
		status int
		want   string
	}{
		"command text":      {`{"name":"Cat","entry":2,"command":"rm -rf ~"}`, http.StatusBadRequest, `unknown field "command"`},
		"cwd":               {`{"name":"Cat","entry":2,"cwd":"/"}`, http.StatusBadRequest, "come only from the launcher file"},
		"unknown command":   {request("Nope", "data.csv", ""), http.StatusBadRequest, `no command named "Nope"`},
		"wrong kind":        {request("Cat", "many", ""), http.StatusUnprocessableEntity, "does not accept a directory"},
		"missing entry":     {`{"name":"Cat","project_name":"default","run_id":"` + f.runID + `","job_id":"job-1"}`, http.StatusBadRequest, "entry must name"},
		"missing on host":   {request("Cat", "gone.csv", ""), http.StatusBadRequest, "does not exist on this host"},
		"outside roots":     {request("Cat", "secret.txt", ""), http.StatusBadRequest, "outside the allowed roots"},
		"unresolved":        {request("Cat", "rel.csv", ""), http.StatusBadRequest, "outside the allowed roots"},
		"symlink escape":    {request("Cat", "link.csv", ""), http.StatusBadRequest, "cannot open"},
		"child escape":      {request("Cat", "many", "../data.csv"), http.StatusBadRequest, "child must be a relative path"},
		"child via symlink": {request("Cat", "many", "escape/secret.txt"), http.StatusBadRequest, "cannot open"},
	} {
		t.Run(name, func(t *testing.T) {
			before := len(decodeBody[struct{ Launches []launcher.Launch }](t, serveJSON(t, handler, http.MethodGet, "/api/launches", "")).Launches)
			rejected := start(test.body)
			if body := decodeBody[webLaunchError](t, rejected); rejected.Code != test.status || !strings.Contains(body.Error, test.want) {
				t.Fatalf("POST /api/launch %s = %d %s; want %d containing %q", test.body, rejected.Code, rejected.Body.String(), test.status, test.want)
			}
			after := len(decodeBody[struct{ Launches []launcher.Launch }](t, serveJSON(t, handler, http.MethodGet, "/api/launches", "")).Launches)
			if after != before {
				t.Fatal("a rejected request started or recorded a launch")
			}
		})
	}

	if got := serveJSON(t, handler, http.MethodGet, "/api/launch-log?id=unknown", ""); got.Code != http.StatusNotFound {
		t.Fatalf("log of an unknown launch = %d", got.Code)
	}
	if got := serveJSON(t, handler, http.MethodGet, "/api/launch", ""); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/launch = %d; page loads must never execute", got.Code)
	}
}

func TestExecGuard(t *testing.T) {
	next := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	guard := WithExecGuard(next, []string{"gpu01.example", "10.0.0.5"})
	for name, test := range map[string]struct {
		method, host string
		headers      map[string]string
		want         int
	}{
		"loopback any port":      {http.MethodGet, "localhost:9000", nil, http.StatusNoContent},
		"ipv6 loopback":          {http.MethodGet, "[::1]:8765", nil, http.StatusNoContent},
		"allowed name":           {http.MethodGet, "GPU01.example:8765", nil, http.StatusNoContent},
		"allowed address":        {http.MethodGet, "10.0.0.5:8765", nil, http.StatusNoContent},
		"rebinding name":         {http.MethodGet, "attacker.example:8765", nil, http.StatusMisdirectedRequest},
		"same-origin post":       {http.MethodPost, "localhost:8765", map[string]string{"Origin": "http://localhost:8765"}, http.StatusNoContent},
		"cross-origin post":      {http.MethodPost, "localhost:8765", map[string]string{"Origin": "http://attacker.example"}, http.StatusForbidden},
		"other port origin":      {http.MethodPost, "localhost:8765", map[string]string{"Origin": "http://localhost:9999"}, http.StatusForbidden},
		"null origin":            {http.MethodPost, "localhost:8765", map[string]string{"Origin": "null"}, http.StatusForbidden},
		"no origin, basic auth":  {http.MethodPost, "localhost:8765", map[string]string{"Authorization": "Basic cm90YXJpOng="}, http.StatusForbidden},
		"no origin, token":       {http.MethodPost, "localhost:8765", map[string]string{"X-Rotari-Token": "t"}, http.StatusNoContent},
		"no origin, bearer":      {http.MethodPost, "localhost:8765", map[string]string{"Authorization": "Bearer t"}, http.StatusNoContent},
		"get needs no origin":    {http.MethodGet, "localhost:8765", nil, http.StatusNoContent},
		"cross-origin rebinding": {http.MethodPost, "attacker.example", map[string]string{"Origin": "http://attacker.example"}, http.StatusMisdirectedRequest},
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/api/launch", nil)
			request.Host = test.host
			for key, value := range test.headers {
				request.Header.Set(key, value)
			}
			recorder := httptest.NewRecorder()
			guard.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("%s %s = %d %q, want %d", test.method, test.host, recorder.Code, recorder.Body.String(), test.want)
			}
		})
	}
}

func TestStaticExportExplainsLaunching(t *testing.T) {
	bootstrap := composeStaticBootstrap("{}", "{}", "{}", "{}", "{}", "{}", "{}", "{}", "[]")
	if strings.Contains(bootstrap, "__ROTARI_STATIC_LAUNCHERS_DATA__") || !strings.Contains(bootstrap, "Launching is unavailable in a static export") {
		t.Fatalf("static bootstrap does not answer /api/launchers:\n%s", bootstrap)
	}
}
