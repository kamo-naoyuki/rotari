package webui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/launcher"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
)

// launchWrapperEnv makes the test binary act as `rotari __launch`.
const launchWrapperEnv = "ROTARI_TEST_WEBUI_LAUNCH_WRAPPER"

// launchServiceEnv makes the test binary act as a launched HTTP service.
const launchServiceEnv = "ROTARI_TEST_WEBUI_SERVICE"

func TestMain(m *testing.M) {
	// A service is started by the wrapper, so it inherits launchWrapperEnv
	// too; check its own role first.
	if os.Getenv(launchServiceEnv) == "1" {
		os.Exit(serveTestService())
	}
	if os.Getenv(launchWrapperEnv) == "1" {
		os.Exit(launcher.RunWrapper(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// serveTestService answers under ROTARI_LAUNCHER_BASE_PATH with what it
// received, so tests can see what the proxy forwarded.
func serveTestService() int {
	listener, err := net.Listen("tcp", net.JoinHostPort(launcher.LoopbackHost, os.Getenv(launcher.EnvPort)))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	base := os.Getenv(launcher.EnvBasePath)
	_ = http.Serve(listener, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, base+"/") {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprintf(writer, "path=%s query=%s auth=%q token=%q host=%s",
			request.URL.Path, request.URL.RawQuery, request.Header.Get("Authorization"), request.Header.Get("X-Rotari-Token"), request.Host)
	}))
	return 0
}

// serviceLaunchCommand runs the test binary as an HTTP service.
func serviceLaunchCommand(name string) launcher.Command {
	return launcher.Command{Name: name, Command: launchServiceEnv + `=1 exec "$ROTARI_TEST_WEBUI_BIN"`,
		Targets: []launcher.TargetKind{launcher.TargetFile, launcher.TargetDirectory}, Mode: launcher.ModeService,
		CWD: launcher.CWDTarget, ReadyPath: "/", ReadyTimeout: 30 * time.Second, StopGrace: time.Second}
}

// launchOptions enables launching with commands, run by the test binary as
// the wrapper.
func launchOptions(t *testing.T, baseDir string, commands ...launcher.Command) Options {
	t.Helper()
	manager, err := launcher.NewManager(launcher.ManagerConfig{
		Wrapper:     []string{os.Args[0]},
		Env:         append(os.Environ(), launchWrapperEnv+"=1", "ROTARI_TEST_WEBUI_BIN="+os.Args[0]),
		ServicePath: LaunchProxyPath,
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
		var index any = f.entries[entry]
		if entry == artifactWorkingDirectoryEntry {
			index = entry
		}
		value, _ := json.Marshal(map[string]any{"project_name": "default", "run_id": f.runID, "job_id": "job-1",
			"entry": index, "child": child, "name": name})
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

	// The working directory, and a file in it, launch as a listed
	// directory and its child do.
	workingDirectory := decodeBody[launcher.Launch](t, start(request("Count", artifactWorkingDirectoryEntry, "")))
	waitLaunch(workingDirectory.ID)
	if workingDirectory.Target != f.work || workingDirectory.Kind != launcher.TargetDirectory {
		t.Fatalf("working directory launch = %+v", workingDirectory)
	}
	inWorkingDirectory := decodeBody[launcher.Launch](t, start(request("Cat", artifactWorkingDirectoryEntry, "data.csv")))
	waitLaunch(inWorkingDirectory.ID)
	if inWorkingDirectory.Target != filepath.Join(f.work, "data.csv") || logText(inWorkingDirectory.ID) != "a,b\n1,2\n" {
		t.Fatalf("launch of a file in the working directory = %+v, output %q", inWorkingDirectory, logText(inWorkingDirectory.ID))
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
		"wrong kind":        {request("Cat", "many", ""), http.StatusBadRequest, "does not accept a directory"},
		"other entry name":  {`{"name":"Cat","entry":"WD"}`, http.StatusBadRequest, `entry must be an index of the listing, "wd", or "basedir_parent"`},
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

	// A preview shows what a start would run and starts nothing.
	before := len(decodeBody[struct{ Launches []launcher.Launch }](t, serveJSON(t, handler, http.MethodGet, "/api/launches", "")).Launches)
	preview := serveJSON(t, handler, http.MethodGet, "/api/launch-preview?project_name=default&run_id="+f.runID+"&job_id=job-1&entry="+fmt.Sprint(f.entries["many"])+"&child=f007.txt&name=Cat", "")
	shown := decodeBody[webLaunchPreview](t, preview)
	if preview.Code != http.StatusOK || shown.Target != filepath.Join(f.work, "many", "f007.txt") || shown.Kind != launcher.TargetFile ||
		shown.CWD != filepath.Join(f.work, "many") || shown.Command != `cat "$ROTARI_LAUNCHER_ARTIFACT"` || shown.Shell != launcher.Shell ||
		shown.User == "" || shown.Labels["job"] != "job-1" {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
	if after := len(decodeBody[struct{ Launches []launcher.Launch }](t, serveJSON(t, handler, http.MethodGet, "/api/launches", "")).Launches); after != before {
		t.Fatal("a preview started a launch")
	}
	if bad := serveJSON(t, handler, http.MethodGet, "/api/launch-preview?project_name=default&run_id="+f.runID+"&job_id=job-1&entry="+fmt.Sprint(f.entries["many"])+"&name=Cat", ""); bad.Code != http.StatusBadRequest {
		t.Fatalf("preview of a directory for a file command = %d", bad.Code)
	}

	if got := serveJSON(t, handler, http.MethodGet, "/api/launch-log?id=unknown", ""); got.Code != http.StatusNotFound {
		t.Fatalf("log of an unknown launch = %d", got.Code)
	}
	if got := serveJSON(t, handler, http.MethodGet, "/api/launch", ""); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/launch = %d; page loads must never execute", got.Code)
	}
}

// A run's Launch targets the directory holding the basedir, resolved by the
// server from the run, under the roots its jobs' entries are served from.
func TestLaunchOnBasedirParent(t *testing.T) {
	f := newArtifactFixture(t)
	parent := filepath.Dir(f.baseDir)
	options := launchOptions(t, f.baseDir, launchCommand("Count", `pwd -P`, launcher.TargetDirectory))
	handler := Handler(options)
	request := func(fields map[string]any) string {
		body := map[string]any{"project_name": "default", "run_id": f.runID, "entry": launchBasedirParentEntry, "name": "Count"}
		for key, value := range fields {
			body[key] = value
		}
		value, _ := json.Marshal(body)
		return string(value)
	}
	started := serveJSON(t, handler, http.MethodPost, "/api/launch", request(nil))
	launch := decodeBody[launcher.Launch](t, started)
	if started.Code != http.StatusOK || launch.Target != parent || launch.Kind != launcher.TargetDirectory ||
		launch.Labels["run"] != f.runID || launch.Labels["project"] != "default" {
		t.Fatalf("launch on the basedir's parent = %d %s", started.Code, started.Body.String())
	}
	if _, ok := launch.Labels["job"]; ok {
		t.Fatalf("a run's launch is labeled with a job: %v", launch.Labels)
	}
	preview := serveJSON(t, handler, http.MethodGet, "/api/launch-preview?project_name=default&run_id="+f.runID+"&job_id=&entry="+launchBasedirParentEntry+"&name=Count", "")
	if shown := decodeBody[webLaunchPreview](t, preview); preview.Code != http.StatusOK || shown.Target != parent || shown.CWD != parent {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}

	for name, test := range map[string]struct {
		body string
		want string
	}{
		"with a job":    {request(map[string]any{"job_id": "job-1"}), "give project_name and run_id only"},
		"with a child":  {request(map[string]any{"child": "base"}), "give project_name and run_id only"},
		"unknown run":   {request(map[string]any{"run_id": "20990101-000000-00000000"}), "run 20990101-000000-00000000 of project default"},
		"invalid run":   {request(map[string]any{"run_id": "../x"}), "invalid path element"},
		"other project": {request(map[string]any{"project_name": "nope"}), "nope"},
	} {
		t.Run(name, func(t *testing.T) {
			rejected := serveJSON(t, handler, http.MethodPost, "/api/launch", test.body)
			if body := decodeBody[webLaunchError](t, rejected); rejected.Code != http.StatusBadRequest || !strings.Contains(body.Error, test.want) {
				t.Fatalf("POST /api/launch %s = %d %s; want 400 containing %q", test.body, rejected.Code, rejected.Body.String(), test.want)
			}
		})
	}

	// When another user could write the run's records, the directory is
	// launched on only under --artifact-root.
	paths, err := stateinternal.ResolveProjectPaths(f.baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(paths.RunsDir, 0o775); err != nil {
		t.Fatal(err)
	}
	refused := serveJSON(t, handler, http.MethodPost, "/api/launch", request(nil))
	if body := decodeBody[webLaunchError](t, refused); refused.Code != http.StatusBadRequest || !strings.Contains(body.Error, parent+" is not served") ||
		!strings.Contains(body.Error, "writable by group or others") {
		t.Fatalf("launch for an untrusted run = %d %s", refused.Code, refused.Body.String())
	}
	options.ArtifactRoots = []string{parent}
	allowed := serveJSON(t, Handler(options), http.MethodPost, "/api/launch", request(map[string]any{"new_instance": true}))
	if got := decodeBody[launcher.Launch](t, allowed); allowed.Code != http.StatusOK || got.Target != parent {
		t.Fatalf("launch under --artifact-root = %d %s", allowed.Code, allowed.Body.String())
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
		"cross-site websocket":   {http.MethodGet, "localhost:8765", map[string]string{"Upgrade": "websocket", "Origin": "http://attacker.example"}, http.StatusForbidden},
		"same-origin websocket":  {http.MethodGet, "localhost:8765", map[string]string{"Upgrade": "websocket", "Origin": "http://localhost:8765"}, http.StatusNoContent},
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

func TestLaunchLogFromEnd(t *testing.T) {
	f := newArtifactFixture(t)
	options := launchOptions(t, f.baseDir, launchCommand("Seq", `seq 1000`, launcher.TargetFile))
	handler := Handler(options)
	started := decodeBody[launcher.Launch](t, serveJSON(t, handler, http.MethodPost, "/api/launch",
		`{"project_name":"default","run_id":"`+f.runID+`","job_id":"job-1","entry":`+fmt.Sprint(f.entries["data.csv"])+`,"name":"Seq"}`))
	deadline := time.Now().Add(10 * time.Second)
	for {
		launch, err := options.Launches.Manager.Get(started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if launch.State != launcher.StateRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("launch did not end")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// seq 1 1000 writes 3893 bytes; the last 9 are "999\n1000\n".
	tail := decodeBody[webLaunchLog](t, serveJSON(t, handler, http.MethodGet, "/api/launch-log?id="+started.ID+"&from=end&limit=9", ""))
	if tail.Text != "999\n1000\n" || tail.Offset != 3884 || tail.Next != 3893 || tail.Total != 3893 {
		t.Fatalf("tail = %+v", tail)
	}
	head := decodeBody[webLaunchLog](t, serveJSON(t, handler, http.MethodGet, "/api/launch-log?id="+started.ID+"&limit=4", ""))
	if head.Text != "1\n2\n" || head.Offset != 0 {
		t.Fatalf("head = %+v", head)
	}
	if bad := serveJSON(t, handler, http.MethodGet, "/api/launch-log?id="+started.ID+"&from=end&offset=3", ""); bad.Code != http.StatusBadRequest {
		t.Fatalf("from=end with offset = %d", bad.Code)
	}
}

func TestLaunchProxyServesReadyService(t *testing.T) {
	f := newArtifactFixture(t)
	options := launchOptions(t, f.baseDir, serviceLaunchCommand("Viewer"))
	handler := Handler(options)
	started := decodeBody[launcher.Launch](t, serveJSON(t, handler, http.MethodPost, "/api/launch",
		`{"project_name":"default","run_id":"`+f.runID+`","job_id":"job-1","entry":`+fmt.Sprint(f.entries["many"])+`,"name":"Viewer"}`))
	if started.State != launcher.StateStarting || started.BasePath != "/_launch/"+started.ID {
		t.Fatalf("start = %+v", started)
	}
	proxied := func(target string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, target, nil)
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	// Not ready yet, or never: no content from the port.
	deadline := time.Now().Add(30 * time.Second)
	for {
		launch, err := options.Launches.Manager.Get(started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if launch.State == launcher.StateRunning {
			break
		}
		if got := proxied(started.BasePath+"/", nil); got.Code != http.StatusServiceUnavailable {
			t.Fatalf("proxy while %s = %d %q", launch.State, got.Code, got.Body.String())
		}
		if time.Now().After(deadline) {
			t.Fatalf("service did not become ready: %+v", launch)
		}
		time.Sleep(20 * time.Millisecond)
	}

	got := proxied(started.BasePath+"/data/runs?run=a%20b&x=1", map[string]string{"Authorization": "Basic cm90YXJpOnNlY3JldA==", "X-Rotari-Token": "secret"})
	want := fmt.Sprintf(`path=/_launch/%s/data/runs query=run=a%%20b&x=1 auth="" token="" host=127.0.0.1:%d`, started.ID, started.Port)
	if got.Code != http.StatusOK || got.Body.String() != want {
		t.Fatalf("proxied = %d %q\nwant %q", got.Code, got.Body.String(), want)
	}
	if redirect := proxied(started.BasePath+"?a=1", nil); redirect.Code != http.StatusTemporaryRedirect || redirect.Header().Get("Location") != started.BasePath+"/?a=1" {
		t.Fatalf("prefix without slash = %d %q", redirect.Code, redirect.Header().Get("Location"))
	}
	if unknown := proxied("/_launch/0000000000000000/", nil); unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown launch = %d", unknown.Code)
	}
	// Under a basedir mount the proxy path does not exist: it belongs to
	// the server.
	if mounted := proxied("/_basedir/x"+started.BasePath+"/", nil); mounted.Code == http.StatusOK {
		t.Fatal("the proxy answered under a basedir mount")
	}

	if _, err := options.Launches.Manager.Stop(started.ID); err != nil {
		t.Fatal(err)
	}
	if after := proxied(started.BasePath+"/", nil); after.Code != http.StatusServiceUnavailable {
		t.Fatalf("proxy after stop = %d %q", after.Code, after.Body.String())
	}
	disabled := Handler(testOptions(f.baseDir, true))
	recorder := httptest.NewRecorder()
	disabled.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, started.BasePath+"/", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("proxy without --allow-exec = %d", recorder.Code)
	}
}

// Each new upstream connection is checked: a transport for a process group
// that does not own the port refuses to use it.
func TestLaunchTransportChecksPortOwner(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go http.Serve(listener, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { fmt.Fprint(writer, "foreign") }))
	port := listener.Addr().(*net.TCPAddr).Port
	// This test process's own group is not a launch's.
	client := &http.Client{Transport: newLaunchTransport(port, 1)}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err == nil {
		response.Body.Close()
		t.Fatal("the transport used a port owned by another process group")
	}
	if !strings.Contains(err.Error(), "another process listens on") {
		t.Fatalf("error = %v", err)
	}
}

// TestLaunchProxyTensorBoard runs the built-in TensorBoard command through
// the proxy, with the command from ROTARI_TEST_LAUNCHER_TENSORBOARD in
// place of "tensorboard" (for example "conda run -n tb tensorboard"). It is
// skipped without it; CI does not install TensorBoard.
func TestLaunchProxyTensorBoard(t *testing.T) {
	tensorBoard := os.Getenv("ROTARI_TEST_LAUNCHER_TENSORBOARD")
	if tensorBoard == "" {
		t.Skip("ROTARI_TEST_LAUNCHER_TENSORBOARD is not set")
	}
	command := launcher.BuiltinCommands()[0]
	command.Command = strings.Replace(command.Command, "tensorboard ", tensorBoard+" ", 1)
	f := newArtifactFixture(t)
	options := launchOptions(t, f.baseDir, command)
	handler := Handler(options)
	started := decodeBody[launcher.Launch](t, serveJSON(t, handler, http.MethodPost, "/api/launch",
		`{"project_name":"default","run_id":"`+f.runID+`","job_id":"job-1","entry":`+fmt.Sprint(f.entries["many"])+`,"name":"TensorBoard"}`))
	deadline := time.Now().Add(90 * time.Second)
	for {
		launch, err := options.Launches.Manager.Get(started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if launch.State == launcher.StateRunning {
			break
		}
		if launch.State != launcher.StateStarting || time.Now().After(deadline) {
			chunk, _ := options.Launches.Manager.ReadLog(started.ID, 0, 1<<20)
			t.Fatalf("TensorBoard = %+v\n%s", launch, chunk.Data)
		}
		time.Sleep(200 * time.Millisecond)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	for path, want := range map[string]string{
		started.BasePath + "/":          "<",
		started.BasePath + "/data/runs": "[]",
	} {
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(body)), want) {
			t.Fatalf("GET %s = %d %.200q", path, response.StatusCode, body)
		}
	}
}

// A command whose program is missing is listed with the reason, and
// neither previews nor starts.
func TestLaunchUnavailableCommand(t *testing.T) {
	f := newArtifactFixture(t)
	options := launchOptions(t, f.baseDir,
		launchCommand("Missing", `rotari-surely-missing-tool "$ROTARI_LAUNCHER_ARTIFACT"`, launcher.TargetFile),
		launchCommand("Cat", `cat "$ROTARI_LAUNCHER_ARTIFACT"`, launcher.TargetFile))
	handler := Handler(options)
	availability := decodeBody[launchAvailability](t, serveJSON(t, handler, http.MethodGet, "/api/launchers", ""))
	if len(availability.Commands) != 2 || availability.Commands[0].Unavailable != "rotari-surely-missing-tool was not found on PATH" ||
		availability.Commands[1].Unavailable != "" {
		t.Fatalf("availability = %+v", availability.Commands)
	}
	body := `{"project_name":"default","run_id":"` + f.runID + `","job_id":"job-1","entry":` + fmt.Sprint(f.entries["data.csv"]) + `,"name":"Missing"}`
	started := serveJSON(t, handler, http.MethodPost, "/api/launch", body)
	if got := decodeBody[webLaunchError](t, started); started.Code != http.StatusBadRequest ||
		!strings.Contains(got.Error, `cannot launch "Missing": rotari-surely-missing-tool was not found on PATH`) {
		t.Fatalf("start = %d %s", started.Code, started.Body.String())
	}
	preview := serveJSON(t, handler, http.MethodGet, "/api/launch-preview?project_name=default&run_id="+f.runID+"&job_id=job-1&entry="+fmt.Sprint(f.entries["data.csv"])+"&name=Missing", "")
	if preview.Code != http.StatusBadRequest {
		t.Fatalf("preview = %d %s", preview.Code, preview.Body.String())
	}
	if launches := options.Launches.Manager.List(); len(launches) != 0 {
		t.Fatalf("an unavailable command started: %+v", launches)
	}
}

func TestLauncherConfigViewAndGenerate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "rotari", launcher.FileName)
	options := testOptions(t.TempDir(), true)
	options.LauncherFile = path
	handler := Handler(options)
	view := func() webLauncherConfig {
		t.Helper()
		return decodeBody[webLauncherConfig](t, serveJSON(t, handler, http.MethodGet, "/api/launcher-config", ""))
	}

	// No file: only the built-in TensorBoard would be registered.
	got := view()
	builtins := len(launcher.BuiltinCommands())
	if got.Path != path || got.Exists || got.Enabled || got.Error != "" || len(got.Commands) != builtins || !got.Commands[0].Builtin {
		t.Fatalf("missing file view = %+v", got)
	}

	generated := serveJSON(t, handler, http.MethodPost, "/api/generate-launcher-config", "")
	if generated.Code != http.StatusOK || !strings.Contains(generated.Body.String(), path) {
		t.Fatalf("generate = %d %s", generated.Code, generated.Body.String())
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("generated file = %v, %v; want mode 0600", info, err)
	}
	got = view()
	if !got.Exists || got.Content != string(launcher.Template()) || got.Error != "" || len(got.Commands) != builtins {
		t.Fatalf("generated file view = %+v", got)
	}
	if again := serveJSON(t, handler, http.MethodPost, "/api/generate-launcher-config", ""); again.Code != http.StatusConflict {
		t.Fatalf("second generate = %d %s; want 409 and the file kept", again.Code, again.Body.String())
	}

	// A file rotari web --allow-exec would refuse is shown with the reason.
	if err := os.WriteFile(path, []byte("[[launcher]]\nname = \"Plot\"\ncommand = \"plot\"\ntargets = [\"file\"]\nmode = \"command\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got = view()
	if len(got.Commands) != builtins+1 || got.Commands[builtins].Name != "Plot" || got.Commands[builtins].Unavailable != "plot was not found on PATH" {
		t.Fatalf("file view = %+v", got)
	}
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	if got = view(); !strings.Contains(got.Error, "writable by group or others") || !strings.Contains(got.Content, `name = "Plot"`) {
		t.Fatalf("untrusted file view = %+v", got)
	}

	readOnly := testOptions(t.TempDir(), false)
	readOnly.LauncherFile = filepath.Join(t.TempDir(), launcher.FileName)
	if refused := serveJSON(t, Handler(readOnly), http.MethodPost, "/api/generate-launcher-config", ""); refused.Code != http.StatusForbidden {
		t.Fatalf("generate on a read-only server = %d", refused.Code)
	}
	if _, err := os.Stat(readOnly.LauncherFile); err == nil {
		t.Fatal("a read-only server wrote the launcher file")
	}
}

// TestBuiltinServicesThroughProxy runs every built-in service command, as
// is, through the proxy. It needs the tools on PATH and runs only when
// ROTARI_TEST_LAUNCHER_TOOLS is set; CI installs none of them.
func TestBuiltinServicesThroughProxy(t *testing.T) {
	if os.Getenv("ROTARI_TEST_LAUNCHER_TOOLS") == "" {
		t.Skip("ROTARI_TEST_LAUNCHER_TOOLS is not set")
	}
	options := launchOptions(t, t.TempDir(), launcher.BuiltinCommands()...)
	server := httptest.NewServer(Handler(options))
	defer server.Close()
	client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 30 * time.Second}
	get := func(path string, headers map[string]string) (int, string) {
		request, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			return 0, err.Error()
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response, err := client.Do(request)
		if err != nil {
			return 0, err.Error()
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(body)
	}
	// handshake opens a WebSocket through the proxy, as a page of the
	// server's own origin would, and returns the response status line.
	handshake := func(path string) string {
		target, err := url.Parse(path)
		if err != nil {
			return err.Error()
		}
		conn, err := net.DialTimeout("tcp", target.Host, 10*time.Second)
		if err != nil {
			return err.Error()
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nOrigin: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n",
			target.RequestURI(), target.Host, server.URL)
		line, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			return err.Error()
		}
		return strings.TrimSpace(line)
	}
	commands := map[string]launcher.Command{}
	for _, command := range launcher.BuiltinCommands() {
		commands[command.Name] = command
	}
	work := t.TempDir()
	write(t, filepath.Join(work, "app.py"), "import streamlit as st\nst.write('hello from rotari')\n")
	for _, dir := range []string{"tb", "nb", "mlruns", "aimrepo"} {
		if err := os.MkdirAll(filepath.Join(work, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(work, "nb", "note.txt"), "x")
	write(t, filepath.Join(work, "mlflow.db"), "")
	if output, err := exec.Command("aim", "init", "--repo", filepath.Join(work, "aimrepo")).CombinedOutput(); err != nil {
		t.Fatalf("aim init: %v\n%s", err, output)
	}
	for _, test := range []struct {
		name   string
		target string
		kind   launcher.TargetKind
		check  func(t *testing.T, base string, launch launcher.Launch)
	}{
		{"TensorBoard", "tb", launcher.TargetDirectory, func(t *testing.T, base string, _ launcher.Launch) {
			if code, body := get(base+"/data/runs", nil); code != http.StatusOK || strings.TrimSpace(body) != "[]" {
				t.Errorf("TensorBoard runs = %d %.200q", code, body)
			}
		}},
		{"Jupyter Lab", "nb", launcher.TargetDirectory, func(t *testing.T, base string, launch launcher.Launch) {
			if code, body := get(base+"/api/contents", nil); code != http.StatusOK || !strings.Contains(body, `"note.txt"`) {
				t.Errorf("Jupyter contents through the proxy = %d %.300q", code, body)
			}
			if code, body := get(base+"/lab", nil); code != http.StatusOK || !strings.Contains(body, "<") {
				t.Errorf("Jupyter Lab page = %d %.200q", code, body)
			}
			// A kernel starts, and its channels open as a WebSocket.
			request, _ := http.NewRequest(http.MethodPost, base+"/api/kernels", strings.NewReader("{}"))
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			var kernel struct {
				ID string `json:"id"`
			}
			_ = json.NewDecoder(response.Body).Decode(&kernel)
			response.Body.Close()
			if response.StatusCode != http.StatusCreated || kernel.ID == "" {
				t.Fatalf("Jupyter kernel start = %d %+v", response.StatusCode, kernel)
			}
			if line := handshake(base + "/api/kernels/" + kernel.ID + "/channels"); !strings.Contains(line, "101") {
				t.Errorf("Jupyter kernel WebSocket = %q, want 101", line)
			}
			// Straight to its port, without the proxy's token, it refuses.
			direct := fmt.Sprintf("http://127.0.0.1:%d%s/api/contents", launch.Port, launch.BasePath)
			if code, _ := get(direct, nil); code != http.StatusForbidden && code != http.StatusUnauthorized {
				t.Errorf("Jupyter without the token = %d, want 403", code)
			}
		}},
		{"MLflow", "mlruns", launcher.TargetDirectory, func(t *testing.T, base string, _ launcher.Launch) {
			if code, body := get(base+"/", nil); code != http.StatusOK || !strings.Contains(strings.ToLower(body), "mlflow") {
				t.Errorf("MLflow page = %d %.200q", code, body)
			}
			// An empty tracking directory has no experiments: "{}".
			if code, body := get(base+"/ajax-api/2.0/mlflow/experiments/search?max_results=10", nil); code != http.StatusOK || !strings.HasPrefix(body, "{") {
				t.Errorf("MLflow API = %d %.300q", code, body)
			}
		}},
		{"MLflow (SQLite)", "mlflow.db", launcher.TargetFile, func(t *testing.T, base string, _ launcher.Launch) {
			if code, body := get(base+"/ajax-api/2.0/mlflow/experiments/search?max_results=10", nil); code != http.StatusOK || !strings.Contains(body, "experiments") {
				t.Errorf("MLflow (SQLite) API = %d %.300q", code, body)
			}
		}},
		{"Aim", "aimrepo", launcher.TargetDirectory, func(t *testing.T, base string, _ launcher.Launch) {
			if code, body := get(base+"/", nil); code != http.StatusOK || !strings.Contains(body, "<") {
				t.Errorf("Aim page = %d %.200q", code, body)
			}
			if code, body := get(base+"/api/projects/", nil); code != http.StatusOK {
				t.Errorf("Aim API = %d %.300q", code, body)
			}
		}},
		{"Streamlit", "app.py", launcher.TargetFile, func(t *testing.T, base string, _ launcher.Launch) {
			if code, body := get(base+"/", nil); code != http.StatusOK || !strings.Contains(body, "<") {
				t.Errorf("Streamlit page = %d %.200q", code, body)
			}
			if code, body := get(base+"/_stcore/health", nil); code != http.StatusOK || strings.TrimSpace(body) != "ok" {
				t.Errorf("Streamlit health = %d %.200q", code, body)
			}
			if line := handshake(base + "/_stcore/stream"); !strings.Contains(line, "101") {
				t.Errorf("Streamlit WebSocket = %q, want 101", line)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := commands[test.name]
			if reason := command.Unavailable(); reason != "" {
				t.Fatalf("%s is unavailable: %s", test.name, reason)
			}
			launch, err := options.Launches.Manager.Start(launcher.StartRequest{Command: command, Target: filepath.Join(work, test.target), Kind: test.kind})
			if err != nil {
				t.Fatal(err)
			}
			defer options.Launches.Manager.Stop(launch.ID)
			deadline := time.Now().Add(90 * time.Second)
			for {
				current, err := options.Launches.Manager.Get(launch.ID)
				if err != nil {
					t.Fatal(err)
				}
				if current.State == launcher.StateRunning {
					launch = current
					break
				}
				if current.State != launcher.StateStarting || time.Now().After(deadline) {
					chunk, _ := options.Launches.Manager.ReadLog(launch.ID, 0, 1<<20)
					t.Fatalf("%s = %+v\n%s", test.name, current, chunk.Data)
				}
				time.Sleep(200 * time.Millisecond)
			}
			test.check(t, server.URL+launch.BasePath, launch)
		})
	}
}

// With a kind and a name, /api/launchers lists only the commands that
// accept that target, by kind and file name pattern, and a start checks
// the pattern too.
func TestLaunchAvailabilityForATarget(t *testing.T) {
	f := newArtifactFixture(t)
	app := launchCommand("App", "true", launcher.TargetFile)
	app.Patterns = []string{"*.py"}
	options := launchOptions(t, f.baseDir, app,
		launchCommand("Cat", "true", launcher.TargetFile), launchCommand("Count", "true", launcher.TargetDirectory))
	handler := Handler(options)
	names := func(query string) (string, map[launcher.TargetKind]string) {
		t.Helper()
		got := decodeBody[launchAvailability](t, serveJSON(t, handler, http.MethodGet, "/api/launchers"+query, ""))
		var list []string
		for _, command := range got.Commands {
			list = append(list, command.Name)
		}
		return strings.Join(list, ","), got.Unmatched
	}
	for query, want := range map[string]string{
		"":                          "App,Cat,Count",
		"?kind=file&name=train.py":  "App,Cat",
		"?kind=file&name=data.csv":  "Cat",
		"?kind=directory&name=many": "Count",
		"?kind=file":                "App,Cat",
	} {
		if got, _ := names(query); got != want {
			t.Errorf("/api/launchers%s = %s, want %s", query, got, want)
		}
	}
	onlyApp := launchOptions(t, f.baseDir, app)
	got := decodeBody[launchAvailability](t, serveJSON(t, Handler(onlyApp), http.MethodGet, "/api/launchers?kind=file&name=data.csv", ""))
	if len(got.Commands) != 0 || got.Unmatched[launcher.TargetFile] != "No registered command accepts the file data.csv. Commands accept: App (file: *.py)." {
		t.Fatalf("unmatched target = %+v", got)
	}
	if bad := serveJSON(t, handler, http.MethodGet, "/api/launchers?kind=socket", ""); bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind = %d", bad.Code)
	}
	body := `{"project_name":"default","run_id":"` + f.runID + `","job_id":"job-1","entry":` + fmt.Sprint(f.entries["data.csv"]) + `,"name":"App"}`
	refused := serveJSON(t, handler, http.MethodPost, "/api/launch", body)
	if got := decodeBody[webLaunchError](t, refused); refused.Code != http.StatusBadRequest || got.Error != `launcher "App" accepts only *.py, not data.csv` {
		t.Fatalf("start of a non-matching file = %d %+v", refused.Code, got)
	}
}

// The proxy presents a service's ProxyHeaders, with its token, in place of
// whatever the browser sent.
func TestLaunchProxyAddsServiceHeaders(t *testing.T) {
	f := newArtifactFixture(t)
	command := serviceLaunchCommand("Viewer")
	command.ProxyHeaders = map[string]string{"Authorization": "token $" + launcher.EnvToken}
	options := launchOptions(t, f.baseDir, command)
	handler := Handler(options)
	started := decodeBody[launcher.Launch](t, serveJSON(t, handler, http.MethodPost, "/api/launch",
		`{"project_name":"default","run_id":"`+f.runID+`","job_id":"job-1","entry":`+fmt.Sprint(f.entries["many"])+`,"name":"Viewer"}`))
	deadline := time.Now().Add(30 * time.Second)
	for {
		launch, _ := options.Launches.Manager.Get(started.ID)
		if launch.State == launcher.StateRunning {
			break
		}
		if launch.State != launcher.StateStarting || time.Now().After(deadline) {
			t.Fatalf("service = %+v", launch)
		}
		time.Sleep(20 * time.Millisecond)
	}
	upstream, err := options.Launches.Manager.ServiceUpstream(started.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, started.BasePath+"/x", nil)
	request.Header.Set("Authorization", "Basic cm90YXJpOng=")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if want := fmt.Sprintf("auth=%q", upstream.Headers["Authorization"]); recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), want) ||
		strings.Contains(recorder.Body.String(), "Basic") {
		t.Fatalf("proxied = %d %s, want %s", recorder.Code, recorder.Body.String(), want)
	}
}

func TestJupyterAuthenticatedProxy(t *testing.T) {
	if os.Getenv("ROTARI_TEST_LAUNCHER_TOOLS") == "" {
		t.Skip("ROTARI_TEST_LAUNCHER_TOOLS is not set")
	}
	f := newArtifactFixture(t)
	options := launchOptions(t, f.baseDir, launcher.BuiltinCommands()...)
	handler := Handler(options)
	started := decodeBody[launcher.Launch](t, serveJSON(t, handler, http.MethodPost, "/api/launch",
		`{"project_name":"default","run_id":"`+f.runID+`","job_id":"job-1","entry":`+fmt.Sprint(f.entries["many"])+`,"name":"Jupyter Lab"}`))
	deadline := time.Now().Add(60 * time.Second)
	for {
		launch, err := options.Launches.Manager.Get(started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if launch.State == launcher.StateRunning {
			break
		}
		if launch.State != launcher.StateStarting || time.Now().After(deadline) {
			t.Fatalf("Jupyter = %+v", launch)
		}
		time.Sleep(20 * time.Millisecond)
	}
	server := httptest.NewServer(WithAuthToken(WithExecGuard(handler, nil), "secret"))
	defer server.Close()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	request, _ := http.NewRequest(http.MethodGet, server.URL+started.BasePath+"/lab", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	page, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("Jupyter navigation = %d, %v", response.StatusCode, err)
	}
	_, config, ok := strings.Cut(string(page), `id="jupyter-config-data"`)
	if !ok {
		t.Fatal("Jupyter configuration missing")
	}
	_, config, _ = strings.Cut(config, ">")
	config, _, _ = strings.Cut(config, "</script>")
	var settings struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(config), &settings); err != nil || settings.Token == "" {
		t.Fatal("Jupyter configuration has no usable token")
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path, body, want := "/api/contents", "", http.StatusOK
		if method == http.MethodPost {
			path, body, want = "/api/kernels", "{}", http.StatusCreated
		}
		request, _ := http.NewRequest(method, server.URL+started.BasePath+path, strings.NewReader(body))
		request.Header.Set("Authorization", "token "+settings.Token)
		request.Header.Set("Origin", server.URL)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != want {
			t.Fatalf("authenticated Jupyter %s = %d, %v", path, response.StatusCode, err)
		}
		if method != http.MethodPost {
			continue
		}
		var kernel struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &kernel); err != nil || kernel.ID == "" {
			t.Fatal("Jupyter kernel response has no ID")
		}
		target, _ := url.Parse(server.URL + started.BasePath + "/api/kernels/" + kernel.ID + "/channels")
		conn, err := net.DialTimeout("tcp", target.Host, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		handshake, _ := http.NewRequest(http.MethodGet, target.String(), nil)
		handshake.Header.Set("Authorization", "token "+settings.Token)
		handshake.Header.Set("Origin", server.URL)
		handshake.Header.Set("Upgrade", "websocket")
		handshake.Header.Set("Connection", "Upgrade")
		handshake.Header.Set("Sec-WebSocket-Version", "13")
		handshake.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		for _, cookie := range jar.Cookies(target) {
			handshake.AddCookie(cookie)
		}
		if err := handshake.Write(conn); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		line, err := bufio.NewReader(conn).ReadString('\n')
		conn.Close()
		if err != nil || !strings.Contains(line, "101") {
			t.Fatalf("authenticated kernel websocket = %q, %v", line, err)
		}
	}
}
