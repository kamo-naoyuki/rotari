package launcher

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTemplateParsesToTensorBoard(t *testing.T) {
	t.Parallel()
	commands, err := ParseCommands(Template())
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 {
		t.Fatalf("template has %d commands, want 1", len(commands))
	}
	got := commands[0]
	if got.Name != "TensorBoard" || got.Mode != ModeService || !slices.Equal(got.Targets, []TargetKind{TargetDirectory}) ||
		got.CWD != CWDTarget || got.ReadyPath != DefaultReadyPath || got.ReadyTimeout != DefaultReadyTimeout || got.StopGrace != DefaultStopGrace {
		t.Fatalf("template command = %+v", got)
	}
	for _, variable := range []string{EnvArtifact, EnvPort, EnvBasePath} {
		if !strings.Contains(got.Command, `"$`+variable+`"`) {
			t.Errorf("template command does not quote $%s: %s", variable, got.Command)
		}
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
		"relative ready path": {"[[launcher]]\nname = \"A\"\ncommand = \"true\"\ntargets = [\"file\"]\nmode = \"service\"\nready_path = \"health\"\n", "must start with /"},
		"bad duration":        {"[[launcher]]\n" + valid + "stop_grace = \"soon\"\n", `stop_grace "soon"`},
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

	registry, err := LoadRegistry(path)
	if err != nil || !registry.Missing || len(registry.Commands) != 0 || registry.Path != path {
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
