package web

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

const launchToken = "conformance-token"

// launchServiceRole makes this test binary act as a launched HTTP service;
// TestMain dispatches it before building rotari.
const launchServiceRole = "ROTARI_CONFORMANCE_LAUNCH_SERVICE"

// serveLaunchService answers under $ROTARI_LAUNCHER_BASE_PATH with what it
// received, on 127.0.0.1:$ROTARI_LAUNCHER_PORT.
func serveLaunchService() int {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", os.Getenv("ROTARI_LAUNCHER_PORT")))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	base := os.Getenv("ROTARI_LAUNCHER_BASE_PATH")
	_ = http.Serve(listener, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, base+"/") {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprintf(writer, "path=%s query=%s auth=%q token=%q launch-token-sent=%v", request.URL.Path, request.URL.RawQuery,
			request.Header.Get("Authorization"), request.Header.Get("X-Rotari-Token"),
			os.Getenv("ROTARI_LAUNCHER_TOKEN") != "" && request.Header.Get("X-Launch-Token") == os.Getenv("ROTARI_LAUNCHER_TOKEN"))
		if cookie := request.Header.Get("Cookie"); cookie != "" {
			fmt.Fprintf(writer, " cookie=%q", cookie)
		}
	}))
	return 0
}

// launchFixture is a finished job whose working directory holds data/n.txt,
// declared as the artifact "data", plus "gone.txt", which does not exist.
type launchFixture struct {
	e       *support.Env
	runID   string
	jobID   string
	entries map[string]int
}

func newLaunchFixture(t *testing.T) launchFixture {
	t.Helper()
	e := support.NewEnv(t)
	e.Setenv("ROTARI_WEB_AUTH_TOKEN", launchToken)
	jobID := support.AddedJobID(t, e.MustRotari("add", "-p", "launch", "--artifact", "data", "--artifact", "gone.txt",
		"--", "sh", "-c", "mkdir -p data && printf '1\\n2\\n3\\n' > data/n.txt"))
	e.MustRotari("run", "-p", "launch", "--quiet")
	var shown struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal([]byte(e.MustRotari("show", "-p", "launch", "--json").Stdout), &shown); err != nil {
		t.Fatal(err)
	}
	f := launchFixture{e: e, runID: shown.RunID, jobID: jobID, entries: map[string]int{}}
	var listing struct {
		Entries []struct {
			DisplayPath string `json:"display_path"`
		} `json:"entries"`
	}
	base := e.StartWeb()
	if err := json.Unmarshal([]byte(api(e, http.MethodGet, base+"/api/artifacts?"+url.Values{"project_name": {"launch"}, "run_id": {f.runID}, "job_id": {jobID}}.Encode(), "").Body), &listing); err != nil {
		t.Fatal(err)
	}
	for index, entry := range listing.Entries {
		f.entries[entry.DisplayPath] = index
	}
	if _, ok := f.entries["data"]; !ok {
		t.Fatalf("artifact listing lacks data: %+v", listing)
	}
	return f
}

// writeLaunchers writes the global launcher file, private as rotari wants.
func writeLaunchers(t *testing.T, e *support.Env, content string) string {
	t.Helper()
	dir := filepath.Join(e.Root, "config", "rotari")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "launchers.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func launcherEntry(name, command, targets, mode string) string {
	return fmt.Sprintf("[[launcher]]\nname = %q\ncommand = '''%s'''\ntargets = [%s]\nmode = %q\nstop_grace = \"1s\"\n\n", name, command, targets, mode)
}

func (f launchFixture) request(name, entry, child string, extra map[string]any) string {
	body := map[string]any{"project_name": "launch", "run_id": f.runID, "job_id": f.jobID, "entry": f.entries[entry], "child": child, "name": name}
	for key, value := range extra {
		body[key] = value
	}
	data, _ := json.Marshal(body)
	return string(data)
}

func api(e *support.Env, method, address, body string) support.HTTPResult {
	return e.HTTPDo(method, address, map[string]string{"X-Rotari-Token": launchToken}, body)
}

type launchRecord struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state"`
	ExitCode *int   `json:"exit_code"`
	Signal   string `json:"signal"`
	Error    string `json:"error"`
	Reused   bool   `json:"reused"`
	BasePath string `json:"base_path"`
}

func decodeLaunch(t *testing.T, result support.HTTPResult) launchRecord {
	t.Helper()
	var launch launchRecord
	if err := json.Unmarshal([]byte(result.Body), &launch); err != nil || launch.ID == "" {
		t.Fatalf("not a launch: (%d, %s)", result.Status, result.Body)
	}
	return launch
}

func listLaunches(t *testing.T, e *support.Env, base string) []launchRecord {
	t.Helper()
	var listed struct {
		Launches []launchRecord `json:"launches"`
	}
	result := api(e, http.MethodGet, base+"/api/launches", "")
	if err := json.Unmarshal([]byte(result.Body), &listed); err != nil {
		t.Fatalf("/api/launches = (%d, %s)", result.Status, result.Body)
	}
	return listed.Launches
}

func waitLaunch(t *testing.T, e *support.Env, base, id string, done func(launchRecord) bool) launchRecord {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		for _, launch := range listLaunches(t, e, base) {
			if launch.ID == id && done(launch) {
				return launch
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("launch %s did not reach the expected state: %+v", id, listLaunches(t, e, base))
	return launchRecord{}
}

func ended(launch launchRecord) bool {
	return launch.State != "starting" && launch.State != "running" && launch.State != "stopping"
}

func launchOutput(t *testing.T, e *support.Env, base, id string) string {
	t.Helper()
	var page struct {
		Text string `json:"text"`
	}
	result := api(e, http.MethodGet, base+"/api/launch-log?id="+id, "")
	if err := json.Unmarshal([]byte(result.Body), &page); err != nil {
		t.Fatalf("/api/launch-log = (%d, %s)", result.Status, result.Body)
	}
	return page.Text
}

var listeningPattern = regexp.MustCompile(`rotari web listening at (http://\S+)`)

// startWebProcess starts rotari web and returns its address, process, and
// what it printed on stderr before listening.
func startWebProcess(t *testing.T, e *support.Env, args ...string) (string, *exec.Cmd, string) {
	t.Helper()
	cmd := e.Command(append([]string{"web", "--port", "0"}, args...)...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	errors := make(chan string, 1)
	go func() {
		var text strings.Builder
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			text.WriteString(scanner.Text() + "\n")
			if strings.HasPrefix(scanner.Text(), "Launching is enabled") {
				break
			}
		}
		errors <- text.String()
	}()
	found := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if match := listeningPattern.FindStringSubmatch(scanner.Text()); match != nil {
				found <- match[1]
				return
			}
		}
	}()
	select {
	case address := <-found:
		return address, cmd, <-errors
	case <-time.After(20 * time.Second):
		t.Fatal("rotari web did not report its address")
		return "", nil, ""
	}
}

func TestWebLaunchRequiresOptInAndToken(t *testing.T) {
	covers(t, "WEB-13")
	f := newLaunchFixture(t)
	e := f.e
	writeLaunchers(t, e, launcherEntry("Cat", `cat "$ROTARI_LAUNCHER_ARTIFACT"`, `"file"`, "command"))

	// Off by default: the server explains how to turn it on, and refuses.
	base := e.StartWeb()
	var availability struct {
		Available bool   `json:"available"`
		Reason    string `json:"reason"`
		Hint      string `json:"hint"`
	}
	if err := json.Unmarshal([]byte(api(e, http.MethodGet, base+"/api/launchers", "").Body), &availability); err != nil {
		t.Fatal(err)
	}
	if availability.Available || !strings.Contains(availability.Hint, "rotari web --allow-exec") {
		t.Fatalf("availability without --allow-exec = %+v", availability)
	}
	rejected := api(e, http.MethodPost, base+"/api/launch", f.request("Cat", "data", "n.txt", nil))
	if rejected.Status != http.StatusForbidden || !strings.Contains(rejected.Body, "rotari web --allow-exec") {
		t.Fatalf("launch without --allow-exec = (%d, %s)", rejected.Status, rejected.Body)
	}

	for args, want := range map[string]string{
		"--allow-exec --allow-control=false": "--allow-exec requires job control",
		"--allowed-host gpu01":               "--allowed-host only applies with --allow-exec",
	} {
		result := e.Rotari(append([]string{"web"}, strings.Fields(args)...)...)
		if result.Code != 1 || !strings.Contains(result.Stderr, want) {
			t.Fatalf("web %s = %s", args, result)
		}
	}

	// With it and no configured token, one is generated, printed, and required.
	base, _, printed := startWebProcess(t, e.Without("ROTARI_WEB_AUTH_TOKEN"), "--allow-exec")
	match := regexp.MustCompile(`(?m)^  ([0-9a-f]{32,})$`).FindStringSubmatch(printed)
	if match == nil || !strings.Contains(printed, `user name "rotari"`) {
		t.Fatalf("no generated token in:\n%s", printed)
	}
	if got := e.HTTPGet(base + "/api/launchers"); got.Status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request = %d, want 401", got.Status)
	}
	got := e.HTTPDo(http.MethodGet, base+"/api/launchers", map[string]string{"Authorization": "Bearer " + match[1]}, "")
	if got.Status != http.StatusOK || !strings.Contains(got.Body, `"available":true`) {
		t.Fatalf("authenticated request = (%d, %s)", got.Status, got.Body)
	}

	// With a configured token, remind users of the fixed username without
	// printing the token itself.
	base, _, printed = startWebProcess(t, e, "--allow-exec")
	if !strings.Contains(printed, `user name "rotari"`) ||
		!strings.Contains(printed, "configured token as the password") ||
		strings.Contains(printed, launchToken) {
		t.Fatalf("unexpected configured-token notice:\n%s", printed)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	values := url.Values{"username": {"rotari"}, "password": {launchToken}, "next": {"/"}}
	request, err := http.NewRequest(http.MethodPost, base+"/_auth/login", strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", base)
	response, err := browser.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login form = %d, want 200 after redirect", response.StatusCode)
	}
	if cookies := jar.Cookies(&url.URL{Scheme: "http", Host: strings.TrimPrefix(base, "http://")}); len(cookies) != 1 || cookies[0].Name != "rotari_web_session" {
		t.Fatalf("login session cookies = %v", cookies)
	}
}

func TestWebLaunchGuardsRequests(t *testing.T) {
	covers(t, "WEB-14")
	e := support.NewEnv(t)
	e.Setenv("ROTARI_WEB_AUTH_TOKEN", launchToken)
	base, _, _ := startWebProcess(t, e, "--allow-exec", "--allowed-host", "gpu01.example")
	stop := `{"id":"0000000000000000"}`
	for name, test := range map[string]struct {
		method  string
		headers map[string]string
		want    int
	}{
		"rebinding host":        {http.MethodGet, map[string]string{"Host": "attacker.example:8787", "X-Rotari-Token": launchToken}, http.StatusMisdirectedRequest},
		"localhost other port":  {http.MethodGet, map[string]string{"Host": "localhost:1", "X-Rotari-Token": launchToken}, http.StatusOK},
		"allowed host":          {http.MethodGet, map[string]string{"Host": "GPU01.example:8787", "X-Rotari-Token": launchToken}, http.StatusOK},
		"cross-origin post":     {http.MethodPost, map[string]string{"Origin": "http://attacker.example", "X-Rotari-Token": launchToken}, http.StatusForbidden},
		"same-origin post":      {http.MethodPost, map[string]string{"Origin": base, "Authorization": "Bearer " + launchToken}, http.StatusNotFound},
		"basic auth, no origin": {http.MethodPost, map[string]string{"Authorization": "Basic " + basicAuth()}, http.StatusForbidden},
		"token, no origin":      {http.MethodPost, map[string]string{"X-Rotari-Token": launchToken}, http.StatusNotFound},
		"bearer, no origin":     {http.MethodPost, map[string]string{"Authorization": "Bearer " + launchToken}, http.StatusNotFound},
		// A WebSocket handshake is a GET that opens a channel, so it needs
		// the same Origin as a mutation.
		"cross-site websocket": {http.MethodGet, map[string]string{"Upgrade": "websocket", "Connection": "Upgrade", "Origin": "http://attacker.example", "X-Rotari-Token": launchToken}, http.StatusForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			target, body := base+"/api/launchers", ""
			if test.method == http.MethodPost {
				target, body = base+"/api/launch-stop", stop
			}
			// 404 means the request passed the guard and reached the API,
			// which does not know the launch.
			if got := e.HTTPDo(test.method, target, test.headers, body); got.Status != test.want {
				t.Fatalf("%s = (%d, %s), want %d", name, got.Status, got.Body, test.want)
			}
		})
	}
}

func basicAuth() string {
	return "cm90YXJpOmNvbmZvcm1hbmNlLXRva2Vu" // rotari:conformance-token
}

func TestWebLaunchRunsOnlyRegisteredCommands(t *testing.T) {
	covers(t, "WEB-15")
	f := newLaunchFixture(t)
	e := f.e
	path := writeLaunchers(t, e,
		launcherEntry("List", `ls "$ROTARI_LAUNCHER_ARTIFACT"; echo "cwd=$ROTARI_LAUNCHER_CWD token=${ROTARI_WEB_AUTH_TOKEN-unset}"; cat; echo end`, `"directory"`, "command")+
			launcherEntry("Cat", `cat "$ROTARI_LAUNCHER_ARTIFACT"`, `"file"`, "command")+
			launcherEntry("Missing", `rotari-conformance-missing-tool "$ROTARI_LAUNCHER_ARTIFACT"`, `"file"`, "command"))
	// A launcher file anywhere else is never read.
	if err := os.WriteFile(filepath.Join(e.Base, "launchers.toml"), []byte(launcherEntry("Basedir", "true", `"file"`, "command")), 0o600); err != nil {
		t.Fatal(err)
	}
	if listed := e.MustRotari("config", "--list").Stdout; !strings.Contains(listed, path) {
		t.Fatalf("config --list lacks %s:\n%s", path, listed)
	}
	base, _, _ := startWebProcess(t, e, "--allow-exec")
	var availability struct {
		Commands []struct {
			Name        string `json:"name"`
			Builtin     bool   `json:"builtin"`
			Unavailable string `json:"unavailable"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(api(e, http.MethodGet, base+"/api/launchers", "").Body), &availability); err != nil {
		t.Fatal(err)
	}
	// TensorBoard is built in; the file's commands follow.
	names := []string{}
	for _, command := range availability.Commands {
		names = append(names, command.Name)
	}
	builtins := "TensorBoard,Jupyter Lab,MLflow,MLflow (SQLite),Aim,Streamlit"
	if strings.Join(names, ",") != builtins+",List,Cat,Missing" || !availability.Commands[0].Builtin {
		t.Fatalf("registered commands = %v", names)
	}
	last := len(availability.Commands) - 1
	if availability.Commands[last].Unavailable != "rotari-conformance-missing-tool was not found on PATH" || availability.Commands[last-1].Unavailable != "" {
		t.Fatalf("availability of Cat and Missing = %+v", availability.Commands[last-1:])
	}

	list := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch", f.request("List", "data", "", nil)))
	waitLaunch(t, e, base, list.ID, ended)
	data := filepath.Join(e.Root, "data")
	if got := launchOutput(t, e, base, list.ID); got != "n.txt\ncwd="+data+" token=unset\nend\n" {
		t.Fatalf("List printed %q", got)
	}
	cat := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch", f.request("Cat", "data", "n.txt", nil)))
	if done := waitLaunch(t, e, base, cat.ID, ended); done.ExitCode == nil || *done.ExitCode != 0 || launchOutput(t, e, base, cat.ID) != "1\n2\n3\n" {
		t.Fatalf("Cat = %+v, output %q", done, launchOutput(t, e, base, cat.ID))
	}

	// A run's Launch names the directory holding the basedir, and no job.
	basedirParent := func(extra map[string]any) string {
		body := map[string]any{"project_name": "launch", "run_id": f.runID, "entry": "basedir_parent", "name": "List"}
		for key, value := range extra {
			body[key] = value
		}
		encoded, _ := json.Marshal(body)
		return string(encoded)
	}
	parent := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch", basedirParent(nil)))
	waitLaunch(t, e, base, parent.ID, ended)
	if got := launchOutput(t, e, base, parent.ID); !strings.Contains(got, "cwd="+filepath.Dir(e.Base)+" token=unset\n") {
		t.Fatalf("List on the basedir's parent printed %q", got)
	}

	before := len(listLaunches(t, e, base))
	for name, body := range map[string]string{
		"parent with job":  basedirParent(map[string]any{"job_id": f.jobID}),
		"parent of no run": basedirParent(map[string]any{"run_id": "20990101-000000-00000000"}),
		"command text":     f.request("Cat", "data", "n.txt", map[string]any{"command": "touch pwned"}),
		"working dir":      f.request("Cat", "data", "n.txt", map[string]any{"cwd": "/"}),
		"unknown command":  f.request("Basedir", "data", "n.txt", nil),
		"missing program":  f.request("Missing", "data", "n.txt", nil),
		"wrong kind":       f.request("Cat", "data", "", nil),
		"absent target":    f.request("Cat", "gone.txt", "", nil),
		"escaping child":   f.request("Cat", "data", "../data/n.txt", nil),
	} {
		if got := api(e, http.MethodPost, base+"/api/launch", body); got.Status != http.StatusBadRequest {
			t.Errorf("%s: (%d, %s), want 400", name, got.Status, got.Body)
		}
	}
	if after := len(listLaunches(t, e, base)); after != before {
		t.Fatalf("rejected requests started launches: %d -> %d", before, after)
	}
	if _, err := os.Stat(filepath.Join(data, "pwned")); err == nil {
		t.Fatal("request text was executed")
	}

	// An untrusted launcher file stops the server from starting.
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	if result := e.Rotari("web", "--allow-exec", "--port", "0"); result.Code != 1 || !strings.Contains(result.Stderr, "writable by group or others") {
		t.Fatalf("web with an untrusted launcher file = %s", result)
	}

	// config --launchers writes a private template there.
	fresh := support.NewEnv(t)
	fresh.MustRotari("config", "--launchers")
	info, err := os.Stat(filepath.Join(fresh.Root, "config", "rotari", "launchers.toml"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("template = %v, %v; want mode 0600", info, err)
	}
}

func TestWebLaunchLifecycle(t *testing.T) {
	covers(t, "WEB-16")
	f := newLaunchFixture(t)
	e := f.e
	writeLaunchers(t, e, launcherEntry("Hold", `sleep 300 & echo $! >> "$ROTARI_LAUNCHER_CWD/hold.pids"; wait`, `"directory"`, "command"))
	base, web, _ := startWebProcess(t, e, "--allow-exec")
	first := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch", f.request("Hold", "data", "", nil)))
	again := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch", f.request("Hold", "data", "", nil)))
	if again.ID != first.ID || !again.Reused {
		t.Fatalf("repeated start = %+v, want reuse of %s", again, first.ID)
	}
	second := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch", f.request("Hold", "data", "", map[string]any{"new_instance": true})))
	if second.ID == first.ID || second.Reused {
		t.Fatalf("another instance = %+v", second)
	}
	pidsFile := filepath.Join(e.Root, "data", "hold.pids")
	pids := waitPIDs(t, pidsFile, 2)

	stopped := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch-stop", `{"id":"`+first.ID+`"}`))
	if stopped.State != "stopped" || stopped.Signal != "terminated" {
		t.Fatalf("stop = %+v", stopped)
	}
	// One of the two sleeps ended with its group; the other is still live.
	waitUntil(t, "one sleep to end", func() bool { return processGone(pids[0]) != processGone(pids[1]) })
	if listed := listLaunches(t, e, base); len(listed) != 2 {
		t.Fatalf("launches = %+v", listed)
	}

	// Killing the Web server, which runs none of its shutdown, ends the rest.
	if err := web.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "every sleep to end after SIGKILL of rotari web", func() bool { return processGone(pids[0]) && processGone(pids[1]) })
}

func waitPIDs(t *testing.T, path string, count int) []int {
	t.Helper()
	var pids []int
	waitUntil(t, "the launched commands to start", func() bool {
		data, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		pids = nil
		for _, field := range strings.Fields(string(data)) {
			if pid, err := strconv.Atoi(field); err == nil {
				pids = append(pids, pid)
			}
		}
		return len(pids) == count && strings.HasSuffix(string(data), "\n")
	})
	return pids
}

func processGone(pid int) bool {
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}

func waitUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestWebLaunchServesServices(t *testing.T) {
	covers(t, "WEB-17")
	f := newLaunchFixture(t)
	e := f.e
	e.Setenv("ROTARI_CONFORMANCE_BIN", os.Args[0])
	portFile := filepath.Join(e.Root, "port")
	writeLaunchers(t, e,
		launcherEntry("Viewer", launchServiceRole+`=1 exec "$ROTARI_CONFORMANCE_BIN"`, `"directory"`, "service")+
			"proxy_headers = { X-Launch-Token = \"$ROTARI_LAUNCHER_TOKEN\" }\n\n"+
			launcherEntry("Squatted", `echo "$ROTARI_LAUNCHER_PORT" > `+portFile+`; sleep 300`, `"file"`, "service"))
	base, _, _ := startWebProcess(t, e, "--allow-exec")

	viewer := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch", f.request("Viewer", "data", "", nil)))
	if viewer.State != "starting" || viewer.BasePath != "/_launch/"+viewer.ID {
		t.Fatalf("service start = %+v", viewer)
	}
	ready := waitLaunch(t, e, base, viewer.ID, func(launch launchRecord) bool { return launch.State != "starting" })
	if ready.State != "running" {
		t.Fatalf("service = %+v", ready)
	}

	t.Run("OpenLinksAcrossBasedirs", func(t *testing.T) {
		if _, err := exec.LookPath("node"); err != nil {
			t.Skip("node is not installed")
		}
		command := exec.Command("node", "-e", `
	const { JSDOM, VirtualConsole } = require('jsdom');
	const [base, token, id, expected] = process.argv.slice(1);
	const get = (path, init = {}) => fetch(new URL(String(path), base), {
	  ...init, headers: { ...init.headers, 'X-Rotari-Token': token },
	});
	(async () => {
	  const { basedirs } = await (await get('/api/basedirs')).json();
	  for (const prefix of ['', '/_basedir/' + basedirs[0].id]) {
	    const page = prefix + '/project/launch';
	    const html = await (await get(page)).text();
	    const dom = new JSDOM(html, {
	      runScripts: 'dangerously', url: base + page, virtualConsole: new VirtualConsole(),
	      beforeParse(window) {
	        window.fetch = get;
	        window.setInterval = () => 1;
	      },
	    });
	    const window = dom.window;
	    try {
	      await new Promise(resolve => setTimeout(resolve, 300));
	      const panel = window.document.createElement('div');
	      panel.id = 'artifact-launch-panel';
	      window.document.body.append(panel);
	      await window.followServiceStart(id, 'Viewer');
	      const check = async (selector) => {
	        // Periodic page refreshes must not remount server-wide services.
	        window.applyBasedirLinks();
	        window.applyBasedirLinks();
	        const link = window.document.querySelector(selector);
	        if (!link || link.getAttribute('href') !== expected + '/')
	          throw new Error(page + ': wrong Open link: ' + link?.outerHTML);
	        const response = await get(link.href);
	        const body = await response.text();
	        if (response.status !== 200 || !body.startsWith('path=' + expected + '/ query='))
	          throw new Error(page + ': Open did not reach service: ' + response.status + ' ' + body.slice(0, 200));
	      };
	      await check('#artifact-launch-panel .launch-open');
	      await window.showLaunches(id);
	      await check('#launch-view .launch-open');
	    } finally {
	      window.close();
	    }
	  }
	})().catch(error => { console.error(error.stack); process.exit(1); });
	`, base, launchToken, viewer.ID, viewer.BasePath)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("Open links browser check failed: %v\n%s", err, output)
		}
	})
	proxied := api(e, http.MethodGet, base+viewer.BasePath+"/data/runs?run=a%20b", "")
	// The browser's credentials are dropped; the launch's own token,
	// configured in proxy_headers, is presented instead.
	want := `path=` + viewer.BasePath + `/data/runs query=run=a%20b auth="" token="" launch-token-sent=true`
	if proxied.Status != http.StatusOK || proxied.Body != want {
		t.Fatalf("proxied = (%d, %s), want %s", proxied.Status, proxied.Body, want)
	}
	// A real browser signs in with the Web form, then Jupyter supplies its
	// own Authorization header on API requests. Neither credential leaks.
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	address := base + viewer.BasePath + "/data/runs?run=a%20b"
	loginValues := url.Values{
		"username": {"rotari"},
		"password": {launchToken},
		"next":     {viewer.BasePath + "/data/runs?run=a%20b"},
	}
	request, err := http.NewRequest(http.MethodPost, base+"/_auth/login", strings.NewReader(loginValues.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", base)
	response, err := browser.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("launch navigation = %d", response.StatusCode)
	}
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	hasWebSession := false
	for _, cookie := range jar.Cookies(parsed) {
		hasWebSession = hasWebSession || cookie.Name == "rotari_web_session"
	}
	if !hasWebSession {
		t.Fatalf("login cookies = %v, want Web session", jar.Cookies(parsed))
	}
	jar.SetCookies(parsed, []*http.Cookie{{Name: "application", Value: "kept", Path: viewer.BasePath + "/"}})
	for _, authorization := range []string{"token jupyter-secret", "Bearer application-secret", ""} {
		request, err := http.NewRequest(http.MethodGet, address, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", authorization)
		response, err := browser.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK || string(body) != want+` cookie="application=kept"` {
			t.Fatalf("service Authorization %q = (%d, %s)", authorization, response.StatusCode, body)
		}
	}
	if unknown := api(e, http.MethodGet, base+"/_launch/0000000000000000/", ""); unknown.Status != http.StatusNotFound {
		t.Fatalf("unknown launch = %d", unknown.Status)
	}
	api(e, http.MethodPost, base+"/api/launch-stop", `{"id":"`+viewer.ID+`"}`)
	if after := api(e, http.MethodGet, base+viewer.BasePath+"/", ""); after.Status != http.StatusServiceUnavailable {
		t.Fatalf("proxy after stop = %d", after.Status)
	}

	// A process outside the launch on its port never makes it ready.
	squatted := decodeLaunch(t, api(e, http.MethodPost, base+"/api/launch", f.request("Squatted", "data", "n.txt", nil)))
	var port int
	waitUntil(t, "the service's port", func() bool {
		data, err := os.ReadFile(portFile)
		port, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil && port > 0
	})
	foreign, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	go http.Serve(foreign, http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { fmt.Fprint(writer, "foreign") }))
	failed := waitLaunch(t, e, base, squatted.ID, ended)
	if failed.State != "failed" || !strings.Contains(failed.Error, "another process listens on") {
		t.Fatalf("squatted service = %+v", failed)
	}
	if got := api(e, http.MethodGet, base+squatted.BasePath+"/", ""); got.Status != http.StatusServiceUnavailable || strings.Contains(got.Body, "foreign") {
		t.Fatalf("proxy of the failed service = (%d, %s)", got.Status, got.Body)
	}
}

func TestWebShowsAndGeneratesLauncherConfig(t *testing.T) {
	covers(t, "WEB-18")
	e := support.NewEnv(t)
	path := filepath.Join(e.Root, "config", "rotari", "launchers.toml")
	base := e.StartWeb()
	type view struct {
		Path     string `json:"path"`
		Exists   bool   `json:"exists"`
		Content  string `json:"content"`
		Error    string `json:"error"`
		Enabled  bool   `json:"enabled"`
		Commands []struct {
			Name    string `json:"name"`
			Builtin bool   `json:"builtin"`
		} `json:"commands"`
	}
	read := func() view {
		t.Helper()
		var got view
		if err := json.Unmarshal([]byte(e.HTTPGet(base+"/api/launcher-config").Body), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	// Launching is off; the file is still shown, with the built-in command.
	if got := read(); got.Path != path || got.Exists || got.Enabled || len(got.Commands) != 6 || got.Commands[0].Name != "TensorBoard" || !got.Commands[0].Builtin {
		t.Fatalf("view without a file = %+v", got)
	}
	if generated := e.HTTPPostJSON(base+"/api/generate-launcher-config", map[string]any{}); generated.Status != http.StatusOK {
		t.Fatalf("generate = (%d, %s)", generated.Status, generated.Body)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("generated file = %v, %v; want mode 0600", info, err)
	}
	cli := support.NewEnv(t)
	cli.MustRotari("config", "--launchers")
	template, err := os.ReadFile(filepath.Join(cli.Root, "config", "rotari", "launchers.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := read(); !got.Exists || got.Content != string(template) || got.Error != "" {
		t.Fatalf("view of the generated file = %+v, want the config --launchers template", got)
	}
	if err := os.WriteFile(path, []byte("# mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if again := e.HTTPPostJSON(base+"/api/generate-launcher-config", map[string]any{}); again.Status != http.StatusConflict {
		t.Fatalf("generate over a file = (%d, %s)", again.Status, again.Body)
	}
	if data, _ := os.ReadFile(path); string(data) != "# mine\n" {
		t.Fatalf("generate replaced the file: %q", data)
	}
	readOnly := support.NewEnv(t)
	readOnlyBase := readOnly.StartWeb("--allow-control=false")
	if refused := readOnly.HTTPPostJSON(readOnlyBase+"/api/generate-launcher-config", map[string]any{}); refused.Status != http.StatusForbidden {
		t.Fatalf("generate on a read-only server = (%d, %s)", refused.Status, refused.Body)
	}
}
