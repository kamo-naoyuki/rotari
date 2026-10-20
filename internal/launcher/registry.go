package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/BurntSushi/toml"
)

// FileName is the launcher file. Only the global config directory is read;
// the caller supplies the path.
const FileName = "launchers.toml"

// maxRegistryBytes bounds the launcher file, which only holds short entries.
const maxRegistryBytes = 1 << 20

// TargetKind is the kind of artifact a command accepts.
type TargetKind string

const (
	TargetFile      TargetKind = "file"
	TargetDirectory TargetKind = "directory"
)

// Mode says whether a command is expected to exit or to serve HTTP.
type Mode string

const (
	ModeCommand Mode = "command"
	ModeService Mode = "service"
)

// Working directory rules besides an absolute directory.
const (
	// CWDTarget is the target directory, or a target file's directory.
	CWDTarget = "target"
	// CWDParent is the directory containing the target.
	CWDParent = "parent"
)

// Defaults for optional settings.
const (
	DefaultReadyPath    = "/"
	DefaultReadyTimeout = 60 * time.Second
	DefaultStopGrace    = 5 * time.Second
)

// Command is one registered command.
type Command struct {
	Name    string
	Command string
	Targets []TargetKind
	Mode    Mode
	// CWD is CWDTarget, CWDParent, or an absolute directory.
	CWD string
	// ReadyPath and ReadyTimeout apply to ModeService only.
	ReadyPath    string
	ReadyTimeout time.Duration
	StopGrace    time.Duration
	// Builtin marks a command rotari provides without a launcher file
	// entry (BuiltinCommands).
	Builtin bool
}

// tensorBoardCommand is the TensorBoard invocation that works behind the
// Web server's proxy; it was checked against TensorBoard 2.21.0.
const tensorBoardCommand = `tensorboard --logdir "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"`

// BuiltinCommands are registered without a launcher file. An entry in the
// file with the same name replaces one, for example to run TensorBoard
// from a particular environment.
func BuiltinCommands() []Command {
	return []Command{{
		Name: "TensorBoard", Command: tensorBoardCommand, Targets: []TargetKind{TargetDirectory},
		Mode: ModeService, CWD: CWDTarget, ReadyPath: DefaultReadyPath, ReadyTimeout: DefaultReadyTimeout,
		StopGrace: DefaultStopGrace, Builtin: true,
	}}
}

// withBuiltins puts the built-in commands the file does not replace first,
// then the file's own.
func withBuiltins(commands []Command) []Command {
	var result []Command
	for _, builtin := range BuiltinCommands() {
		if !slices.ContainsFunc(commands, func(command Command) bool { return command.Name == builtin.Name }) {
			result = append(result, builtin)
		}
	}
	return append(result, commands...)
}

// Accepts reports whether the command can run against a target of kind.
func (command Command) Accepts(kind TargetKind) bool {
	return slices.Contains(command.Targets, kind)
}

// WorkingDirectory applies the command's cwd rule to an absolute target.
func (command Command) WorkingDirectory(target string, kind TargetKind) string {
	switch command.CWD {
	case CWDTarget:
		if kind == TargetDirectory {
			return target
		}
		return filepath.Dir(target)
	case CWDParent:
		return filepath.Dir(target)
	default:
		return command.CWD
	}
}

// Registry is the loaded launcher file.
type Registry struct {
	// Path is the file that was read, or would have been when Missing.
	Path string
	// Missing is true when the file does not exist; no command is
	// registered then.
	Missing  bool
	Commands []Command
}

// Lookup finds a registered command by name.
func (registry Registry) Lookup(name string) (Command, bool) {
	for _, command := range registry.Commands {
		if command.Name == name {
			return command, true
		}
	}
	return Command{}, false
}

// LoadRegistry reads the launcher file at the absolute path after
// OpenTrustedFile's checks, and adds the built-in commands it does not
// replace. A missing file is not an error and registers only the
// built-ins; an untrusted, unreadable, or invalid one is an error.
func LoadRegistry(path string) (Registry, error) {
	registry := Registry{Path: path}
	file, err := OpenTrustedFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		registry.Missing = true
		registry.Commands = withBuiltins(nil)
		return registry, nil
	}
	if err != nil {
		return registry, fmt.Errorf("launcher file %s: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxRegistryBytes+1))
	if err != nil {
		return registry, fmt.Errorf("read launcher file %s: %w", path, err)
	}
	if len(data) > maxRegistryBytes {
		return registry, fmt.Errorf("launcher file %s is larger than %d bytes", path, maxRegistryBytes)
	}
	commands, err := ParseCommands(data)
	if err != nil {
		return registry, fmt.Errorf("launcher file %s: %w", path, err)
	}
	registry.Commands = withBuiltins(commands)
	return registry, nil
}

type rawFile struct {
	Launcher []rawCommand `toml:"launcher"`
}

type rawCommand struct {
	Name         *string  `toml:"name"`
	Command      *string  `toml:"command"`
	Targets      []string `toml:"targets"`
	Mode         *string  `toml:"mode"`
	CWD          *string  `toml:"cwd"`
	ReadyPath    *string  `toml:"ready_path"`
	ReadyTimeout *string  `toml:"ready_timeout"`
	StopGrace    *string  `toml:"stop_grace"`
}

// ParseCommands parses and validates launcher file contents. Every
// problem is an error; nothing is ignored or corrected.
func ParseCommands(data []byte) ([]Command, error) {
	var raw rawFile
	metadata, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw)
	if err != nil {
		return nil, err
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown setting %q", undecoded[0].String())
	}
	commands := make([]Command, 0, len(raw.Launcher))
	for index, entry := range raw.Launcher {
		command, err := entry.command()
		if err != nil {
			label := fmt.Sprintf("launcher %d", index+1)
			if entry.Name != nil && *entry.Name != "" {
				label = fmt.Sprintf("launcher %q", *entry.Name)
			}
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		for _, previous := range commands {
			if previous.Name == command.Name {
				return nil, fmt.Errorf("launcher %q is defined more than once", command.Name)
			}
		}
		commands = append(commands, command)
	}
	return commands, nil
}

func (entry rawCommand) command() (Command, error) {
	if entry.Name == nil || strings.TrimSpace(*entry.Name) == "" {
		return Command{}, errors.New("name is required")
	}
	if *entry.Name != strings.TrimSpace(*entry.Name) || strings.ContainsFunc(*entry.Name, unicode.IsControl) {
		return Command{}, errors.New("name must not have surrounding spaces or control characters")
	}
	if entry.Command == nil || strings.TrimSpace(*entry.Command) == "" {
		return Command{}, errors.New("command is required")
	}
	if strings.ContainsRune(*entry.Command, 0) {
		return Command{}, errors.New("command must not contain a NUL byte")
	}
	command := Command{Name: *entry.Name, Command: *entry.Command, CWD: CWDTarget, StopGrace: DefaultStopGrace}

	if len(entry.Targets) == 0 {
		return Command{}, fmt.Errorf("targets is required: list %q, %q, or both", TargetFile, TargetDirectory)
	}
	for _, target := range entry.Targets {
		kind := TargetKind(target)
		if kind != TargetFile && kind != TargetDirectory {
			return Command{}, fmt.Errorf("unknown target %q; use %q or %q", target, TargetFile, TargetDirectory)
		}
		if command.Accepts(kind) {
			return Command{}, fmt.Errorf("target %q is listed more than once", target)
		}
		command.Targets = append(command.Targets, kind)
	}

	if entry.Mode == nil {
		return Command{}, fmt.Errorf("mode is required: %q or %q", ModeCommand, ModeService)
	}
	command.Mode = Mode(*entry.Mode)
	switch command.Mode {
	case ModeCommand:
		if entry.ReadyPath != nil || entry.ReadyTimeout != nil {
			return Command{}, fmt.Errorf("ready_path and ready_timeout apply only to mode %q", ModeService)
		}
	case ModeService:
		command.ReadyPath, command.ReadyTimeout = DefaultReadyPath, DefaultReadyTimeout
		if entry.ReadyPath != nil {
			if !strings.HasPrefix(*entry.ReadyPath, "/") || strings.ContainsFunc(*entry.ReadyPath, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) {
				return Command{}, fmt.Errorf("ready_path %q must start with / and contain no spaces", *entry.ReadyPath)
			}
			command.ReadyPath = *entry.ReadyPath
		}
		if entry.ReadyTimeout != nil {
			timeout, err := positiveDuration("ready_timeout", *entry.ReadyTimeout)
			if err != nil {
				return Command{}, err
			}
			command.ReadyTimeout = timeout
		}
	default:
		return Command{}, fmt.Errorf("unknown mode %q; use %q or %q", *entry.Mode, ModeCommand, ModeService)
	}

	if entry.CWD != nil {
		cwd := *entry.CWD
		if cwd != CWDTarget && cwd != CWDParent && (!filepath.IsAbs(cwd) || filepath.Clean(cwd) != cwd) {
			return Command{}, fmt.Errorf("cwd %q must be %q, %q, or a clean absolute directory", cwd, CWDTarget, CWDParent)
		}
		command.CWD = cwd
	}
	if entry.StopGrace != nil {
		grace, err := positiveDuration("stop_grace", *entry.StopGrace)
		if err != nil {
			return Command{}, err
		}
		command.StopGrace = grace
	}
	return command, nil
}

func positiveDuration(name, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s %q must be a positive duration such as \"30s\"", name, value)
	}
	return duration, nil
}

// Template is the commented launcher file `rotari config --launchers`
// writes. It registers nothing by itself; its examples are comments.
func Template() []byte {
	return []byte(`# Commands that rotari web --allow-exec can run against artifacts.
# rotari reads this file only from its global config directory, at startup,
# and only when no other user can write it or the directories above it.
#
# Each command is shell source run with /bin/sh -c. It receives:
#   ROTARI_LAUNCHER_ARTIFACT   absolute path of the selected file or directory
#   ROTARI_LAUNCHER_CWD        the working directory chosen by "cwd"
#   ROTARI_LAUNCHER_PORT       service mode: the port to listen on
#   ROTARI_LAUNCHER_BASE_PATH  service mode: the URL path prefix to serve under
# Quote them ("$ROTARI_LAUNCHER_ARTIFACT"); paths can contain any character.
#
# Settings:
#   name           shown in "Launch with"; unique
#   command        shell source
#   targets        ["file"], ["directory"], or both
#   mode           "command" (runs and exits) or "service" (serves HTTP on
#                  127.0.0.1:$ROTARI_LAUNCHER_PORT under the base path)
#   cwd            "target" (default: the directory, or a file's directory),
#                  "parent", or an absolute directory
#   ready_path     service only: path under the base path that answers when
#                  the service is ready (default "/")
#   ready_timeout  service only (default "60s")
#   stop_grace     time between SIGTERM and SIGKILL on stop (default "5s")

# TensorBoard is built in: rotari registers it without an entry here, as
#   tensorboard --logdir "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1
#     --port "$ROTARI_LAUNCHER_PORT" --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"
# for directories, in service mode. An entry named "TensorBoard" replaces it,
# for example to use the TensorBoard of a particular environment:
#
# [[launcher]]
# name = "TensorBoard"
# command = '''conda run --no-capture-output -n ENV tensorboard --logdir "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"'''
# targets = ["directory"]
# mode = "service"
#
# A command that runs on a file and exits:
#
# [[launcher]]
# name = "Line count"
# command = 'wc -l "$ROTARI_LAUNCHER_ARTIFACT"'
# targets = ["file"]
# mode = "command"
`)
}
