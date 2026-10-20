package launcher

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBuiltinCommands(t *testing.T) {
	t.Parallel()
	want := map[string]struct {
		executable string
		targets    []TargetKind
		patterns   []string
		readyPath  string
	}{
		"TensorBoard":     {"tensorboard", []TargetKind{TargetDirectory}, nil, DefaultReadyPath},
		"Jupyter Lab":     {"jupyter", []TargetKind{TargetDirectory}, nil, "/api"},
		"MLflow":          {"mlflow", []TargetKind{TargetDirectory}, []string{"mlruns"}, "/health"},
		"MLflow (SQLite)": {"mlflow", []TargetKind{TargetFile}, []string{"mlflow.db"}, "/health"},
		"Aim":             {"aim", []TargetKind{TargetDirectory}, nil, DefaultReadyPath},
		"Streamlit":       {"streamlit", []TargetKind{TargetFile}, []string{"*.py"}, "/_stcore/health"},
	}
	builtins := BuiltinCommands()
	if len(builtins) != len(want) {
		t.Fatalf("built-in commands = %+v", builtins)
	}
	for _, got := range builtins {
		expected, ok := want[got.Name]
		if !ok {
			t.Errorf("unexpected built-in %q", got.Name)
			continue
		}
		if !got.Builtin || got.Mode != ModeService || got.CWD != CWDTarget || got.ReadyTimeout != DefaultReadyTimeout ||
			got.StopGrace != DefaultStopGrace || !slices.Equal(got.Targets, expected.targets) ||
			!slices.Equal(got.Patterns, expected.patterns) || got.ReadyPath != expected.readyPath {
			t.Errorf("built-in %s = %+v", got.Name, got)
		}
		if got.Executable() != expected.executable {
			t.Errorf("built-in %s executable = %q, want %q", got.Name, got.Executable(), expected.executable)
		}
		// Every built-in serves only on loopback, on its port, under its
		// base path, and quotes what it is given.
		for _, variable := range []string{EnvArtifact, EnvPort, EnvBasePath} {
			if !strings.Contains(got.Command, `"$`+variable+`"`) && !strings.Contains(got.Command, `$`+variable+`"`) {
				t.Errorf("built-in %s does not quote $%s: %s", got.Name, variable, got.Command)
			}
		}
		if !strings.Contains(got.Command, "127.0.0.1") {
			t.Errorf("built-in %s does not bind 127.0.0.1: %s", got.Name, got.Command)
		}
	}
	// Jupyter runs code, so it requires the launch token, which only the
	// proxy presents.
	for _, got := range builtins {
		if got.Name == "Jupyter Lab" {
			if got.ProxyHeaders["Authorization"] != "token $"+EnvToken || !strings.Contains(got.Command, `"$`+EnvToken+`"`) {
				t.Errorf("Jupyter Lab does not require the launch token: %+v", got)
			}
		}
	}
}

// The template registers nothing by itself, and its commented examples
// parse once uncommented.
func TestTemplateExamples(t *testing.T) {
	t.Parallel()
	commands, err := ParseCommands(Template())
	if err != nil || len(commands) != 0 {
		t.Fatalf("template = %+v, %v; want no commands", commands, err)
	}
	var examples strings.Builder
	for _, line := range strings.Split(string(Template()), "\n") {
		if example, ok := strings.CutPrefix(line, "# "); ok && (strings.HasPrefix(example, "[[launcher]]") || strings.Contains(example, " = ")) {
			examples.WriteString(example + "\n")
		}
	}
	commands, err = ParseCommands([]byte(examples.String()))
	if err != nil || len(commands) != 2 || commands[0].Name != "TensorBoard" || commands[1].Name != "Line count" {
		t.Fatalf("template examples = %+v, %v\n%s", commands, err, examples.String())
	}
}

func TestParseCommandsReadsEverySetting(t *testing.T) {
	t.Parallel()
	commands, err := ParseCommands([]byte(`
[[launcher]]
name = "Plot"
command = 'python plot.py "$ROTARI_LAUNCHER_ARTIFACT"'
targets = ["file", "directory"]
mode = "command"
cwd = "/srv/tools"
stop_grace = "2s"

[[launcher]]
name = "Viewer"
command = "viewer"
targets = ["directory"]
mode = "service"
cwd = "parent"
ready_path = "/health"
ready_timeout = "90s"
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Command{
		{Name: "Plot", Command: `python plot.py "$ROTARI_LAUNCHER_ARTIFACT"`, Targets: []TargetKind{TargetFile, TargetDirectory},
			Mode: ModeCommand, CWD: "/srv/tools", StopGrace: 2 * time.Second},
		{Name: "Viewer", Command: "viewer", Targets: []TargetKind{TargetDirectory}, Mode: ModeService, CWD: CWDParent,
			ReadyPath: "/health", ReadyTimeout: 90 * time.Second, StopGrace: DefaultStopGrace},
	}
	if len(commands) != len(want) {
		t.Fatalf("parsed %d commands, want %d", len(commands), len(want))
	}
	for i := range want {
		got := commands[i]
		if got.Name != want[i].Name || got.Command != want[i].Command || !slices.Equal(got.Targets, want[i].Targets) ||
			got.Mode != want[i].Mode || got.CWD != want[i].CWD || got.ReadyPath != want[i].ReadyPath ||
			got.ReadyTimeout != want[i].ReadyTimeout || got.StopGrace != want[i].StopGrace {
			t.Errorf("command %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestParseCommandsRejectsInvalidSettings(t *testing.T) {
	t.Parallel()
	const valid = "name = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"command\"\n"
	for name, test := range map[string]struct{ file, want string }{
		"unknown key":         {"[[launcher]]\n" + valid + "colour = \"red\"\n", `unknown setting "launcher.colour"`},
		"unknown table":       {"[launchers]\nx = 1\n", "unknown setting"},
		"duplicate name":      {"[[launcher]]\n" + valid + "[[launcher]]\n" + valid, `"A" is defined more than once`},
		"missing name":        {"[[launcher]]\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"command\"\n", "launcher 1: name is required"},
		"padded name":         {"[[launcher]]\nname = \" A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"command\"\n", "surrounding spaces"},
		"empty command":       {"[[launcher]]\nname = \"A\"\ncommand = \"  \"\ntargets = [\"file\"]\nmode = \"command\"\n", "command is required"},
		"missing targets":     {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\nmode = \"command\"\n", "targets is required"},
		"unknown target":      {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"dir\"]\nmode = \"command\"\n", `unknown target "dir"`},
		"duplicate target":    {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\", \"file\"]\nmode = \"command\"\n", "listed more than once"},
		"missing mode":        {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\n", "mode is required"},
		"unknown mode":        {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"daemon\"\n", `unknown mode "daemon"`},
		"relative cwd":        {"[[launcher]]\n" + valid + "cwd = \"tools\"\n", `cwd "tools"`},
		"unclean cwd":         {"[[launcher]]\n" + valid + "cwd = \"/srv/../tools\"\n", "clean absolute"},
		"ready in command":    {"[[launcher]]\n" + valid + "ready_path = \"/\"\n", "apply only to mode"},
		"timeout in command":  {"[[launcher]]\n" + valid + "ready_timeout = \"5s\"\n", "apply only to mode"},
		"spaced ready path":   {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"service\"\nready_path = \"/a b\"\n", "no spaces"},
		"relative ready path": {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"service\"\nready_path = \"health\"\n", "must start with /"},
		"bad duration":        {"[[launcher]]\n" + valid + "stop_grace = \"soon\"\n", `stop_grace "soon"`},
		"path pattern":        {"[[launcher]]\n" + valid + "patterns = [\"a/*.py\"]\n", "file name pattern"},
		"bad pattern":         {"[[launcher]]\n" + valid + "patterns = [\"[\"]\n", "file name pattern"},
		"headers in command":  {"[[launcher]]\n" + valid + "proxy_headers = { X-A = \"1\" }\n", "apply only to mode"},
		"reserved header":     {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"service\"\nproxy_headers = { Host = \"x\" }\n", "cannot set Host"},
		"bad header name":     {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"service\"\nproxy_headers = { \"A B\" = \"x\" }\n", "not a valid HTTP header name"},
		"multiline header":    {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"service\"\nproxy_headers = { X-A = \"a\\nb\" }\n", "must be one line"},
		"zero duration":       {"[[launcher]]\n" + valid + "stop_grace = \"0s\"\n", "positive duration"},
		"wrong type":          {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = \"file\"\nmode = \"command\"\n", "targets"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseCommands([]byte(test.file))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParseCommands() = %v, want an error containing %q", err, test.want)
			}
		})
	}
}

func TestLoadRegistry(t *testing.T) {
	t.Parallel()
	dir := trustedDir(t)
	path := filepath.Join(dir, FileName)

	// Without a file, only the built-in commands are registered.
	registry, err := LoadRegistry(path)
	if err != nil || !registry.Missing || len(registry.Commands) != len(BuiltinCommands()) || !registry.Commands[0].Builtin || registry.Path != path {
		t.Fatalf("missing file: LoadRegistry() = %+v, %v", registry, err)
	}

	if err := os.WriteFile(path, Template(), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err = LoadRegistry(path)
	if err != nil || registry.Missing {
		t.Fatalf("LoadRegistry() = %+v, %v", registry, err)
	}
	if _, ok := registry.Lookup("TensorBoard"); !ok {
		t.Fatal("Lookup(TensorBoard) found nothing")
	}
	if _, ok := registry.Lookup("tensorboard"); ok {
		t.Fatal("Lookup is case-insensitive")
	}

	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(path); err == nil || !strings.Contains(err.Error(), "writable by group or others") {
		t.Fatalf("group-writable file: LoadRegistry() = %v", err)
	}

	if err := os.WriteFile(path, []byte("[[launcher]]\nname = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("invalid file: LoadRegistry() = %v, want an error naming the file", err)
	}
}

func TestWorkingDirectory(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		cwd    string
		target string
		kind   TargetKind
		want   string
	}{
		{CWDTarget, "/r/logs", TargetDirectory, "/r/logs"},
		{CWDTarget, "/r/logs/a.txt", TargetFile, "/r/logs"},
		{CWDParent, "/r/logs", TargetDirectory, "/r"},
		{CWDParent, "/r/logs/a.txt", TargetFile, "/r/logs"},
		{"/srv/tools", "/r/logs", TargetDirectory, "/srv/tools"},
	} {
		got := Command{CWD: test.cwd}.WorkingDirectory(test.target, test.kind)
		if got != test.want {
			t.Errorf("cwd %q for %s %s = %q, want %q", test.cwd, test.kind, test.target, got, test.want)
		}
	}
}

// An entry with a built-in command's name replaces it; other entries come
// after the built-ins.
func TestLoadRegistryReplacesBuiltins(t *testing.T) {
	t.Parallel()
	dir := trustedDir(t)
	path := filepath.Join(dir, FileName)
	write := func(content string) Registry {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		registry, err := LoadRegistry(path)
		if err != nil {
			t.Fatal(err)
		}
		return registry
	}
	plot := "[[launcher]]\nname = \"Plot\"\ncommand = \"plot\"\ntargets = [\"file\"]\nmode = \"command\"\n"
	names := func(registry Registry) string {
		var names []string
		for _, command := range registry.Commands {
			names = append(names, command.Name)
		}
		return strings.Join(names, ",")
	}
	registry := write(plot)
	if got := names(registry); got != "TensorBoard,Jupyter Lab,MLflow,MLflow (SQLite),Aim,Streamlit,Plot" {
		t.Fatalf("file without built-in names = %s", got)
	}
	registry = write(plot + "[[launcher]]\nname = \"TensorBoard\"\ncommand = \"conda run -n tb tensorboard\"\ntargets = [\"directory\"]\nmode = \"service\"\n")
	replaced := registry.Commands[len(registry.Commands)-1]
	if got := names(registry); got != "Jupyter Lab,MLflow,MLflow (SQLite),Aim,Streamlit,Plot,TensorBoard" ||
		replaced.Builtin || replaced.Command != "conda run -n tb tensorboard" {
		t.Fatalf("file replacing TensorBoard = %s, %+v", got, replaced)
	}
}

func TestCommandPatternsAndHeaders(t *testing.T) {
	t.Parallel()
	commands, err := ParseCommands([]byte(`
[[launcher]]
name = "App"
command = "app"
targets = ["file"]
mode = "service"
patterns = ["*.py", "app*"]
proxy_headers = { Authorization = "token $ROTARI_LAUNCHER_TOKEN", X-Static = "1" }
`))
	if err != nil {
		t.Fatal(err)
	}
	command := commands[0]
	for name, want := range map[string]bool{"run.py": true, "app.txt": true, "run.txt": false, "": false} {
		if got := command.Matches(name); got != want {
			t.Errorf("Matches(%q) = %v, want %v", name, got, want)
		}
	}
	if (Command{}).Matches("anything") != true {
		t.Error("a command without patterns does not match every name")
	}
	if command.ProxyHeaders["Authorization"] != "token $ROTARI_LAUNCHER_TOKEN" || command.ProxyHeaders["X-Static"] != "1" {
		t.Fatalf("proxy headers = %v", command.ProxyHeaders)
	}
}
