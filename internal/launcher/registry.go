package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
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
	// Patterns, when set, limit the command to targets whose base name
	// matches one of these filepath.Match patterns, such as "*.py".
	Patterns []string
	// ProxyHeaders are request headers the Web server's proxy adds when it
	// forwards to the service, with $ROTARI_LAUNCHER_TOKEN replaced by the
	// launch's token, so a service can require a secret that only the
	// proxy presents. Service mode only.
	ProxyHeaders map[string]string
}

// Matches reports whether a target named name (its base name) fits the
// command's patterns; a command without patterns accepts every name.
func (command Command) Matches(name string) bool {
	if len(command.Patterns) == 0 {
		return true
	}
	for _, pattern := range command.Patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

// The built-in services' commands. Each listens only on 127.0.0.1 and
// serves under ROTARI_LAUNCHER_BASE_PATH, so it works behind the Web
// server's proxy; each was checked against the version named.
const (
	// TensorBoard 2.21.0.
	tensorBoardCommand = `tensorboard --logdir "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --path_prefix "$ROTARI_LAUNCHER_BASE_PATH"`
	// JupyterLab 4: the launch token is its login token, which only the
	// proxy presents (jupyterProxyHeaders), so other local users who reach
	// its port cannot run code. The proxy keeps cross-site requests out
	// (WithExecGuard), so Jupyter's own Origin check, which would compare
	// the browser's Origin with the proxy's upstream Host, is turned off.
	jupyterCommand = `jupyter lab --no-browser --ip=127.0.0.1 --port="$ROTARI_LAUNCHER_PORT" --ServerApp.base_url="$ROTARI_LAUNCHER_BASE_PATH" --IdentityProvider.token="$ROTARI_LAUNCHER_TOKEN" --ServerApp.root_dir="$ROTARI_LAUNCHER_ARTIFACT" --ServerApp.allow_remote_access=True --ServerApp.allow_origin='*'`
	// MLflow 3.17 on a file-store tracking directory (mlruns), which it
	// serves only when MLFLOW_ALLOW_FILE_STORE is set.
	mlflowCommand = `MLFLOW_ALLOW_FILE_STORE=true mlflow ui --backend-store-uri "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --static-prefix "$ROTARI_LAUNCHER_BASE_PATH" --workers 1`
	// MLflow 3.17 on a SQLite tracking database (mlflow.db).
	mlflowSQLiteCommand = `mlflow ui --backend-store-uri "sqlite:///$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --static-prefix "$ROTARI_LAUNCHER_BASE_PATH" --workers 1`
	// Aim 3: the artifact is a directory holding an Aim repository (.aim).
	aimCommand = `aim up --repo "$ROTARI_LAUNCHER_ARTIFACT" --host 127.0.0.1 --port "$ROTARI_LAUNCHER_PORT" --base-path "$ROTARI_LAUNCHER_BASE_PATH"`
	// Streamlit 1.65: the artifact is the app's .py file.
	// Development mode, which Streamlit assumes when it is not installed in
	// site-packages, would refuse --server.port.
	streamlitCommand = `streamlit run "$ROTARI_LAUNCHER_ARTIFACT" --global.developmentMode false --server.address 127.0.0.1 --server.port "$ROTARI_LAUNCHER_PORT" --server.baseUrlPath "$ROTARI_LAUNCHER_BASE_PATH" --server.headless true --browser.gatherUsageStats false`
)

// BuiltinCommands are registered without a launcher file. An entry in the
// file with the same name replaces one, for example to run a tool from a
// particular environment. A built-in whose program is not installed is
// shown unavailable (Command.Unavailable).
func BuiltinCommands() []Command {
	service := func(name, command string, targets []TargetKind, readyPath string) Command {
		return Command{Name: name, Command: command, Targets: targets, Mode: ModeService, CWD: CWDTarget,
			ReadyPath: readyPath, ReadyTimeout: DefaultReadyTimeout, StopGrace: DefaultStopGrace, Builtin: true}
	}
	directory := []TargetKind{TargetDirectory}
	jupyter := service("Jupyter Lab", jupyterCommand, directory, "/api")
	jupyter.ProxyHeaders = map[string]string{"Authorization": "token $" + EnvToken}
	streamlit := service("Streamlit", streamlitCommand, []TargetKind{TargetFile}, "/_stcore/health")
	streamlit.Patterns = []string{"*.py"}
	// MLflow creates its default experiment, or its tables, in whatever it
	// opens, so it is offered only for the names MLflow itself uses.
	mlflow := service("MLflow", mlflowCommand, directory, "/health")
	mlflow.Patterns = []string{"mlruns"}
	mlflowSQLite := service("MLflow (SQLite)", mlflowSQLiteCommand, []TargetKind{TargetFile}, "/health")
	mlflowSQLite.Patterns = []string{"mlflow.db"}
	return []Command{
		service("TensorBoard", tensorBoardCommand, directory, DefaultReadyPath),
		jupyter,
		mlflow,
		mlflowSQLite,
		service("Aim", aimCommand, directory, DefaultReadyPath),
		streamlit,
	}
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
	Patterns     []string `toml:"patterns"`
	// ProxyHeaders is a table, so it decodes into a map.
	ProxyHeaders map[string]string `toml:"proxy_headers"`
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

// ErrConfigChanged reports that the launcher file changed since the editor
// loaded it.
var ErrConfigChanged = errors.New("launcher config changed since it was loaded")

// SaveConfig validates and atomically replaces an existing trusted launcher
// file only when its current contents still match expected.
func SaveConfig(path string, expected, contents []byte) error {
	if len(contents) > maxRegistryBytes {
		return fmt.Errorf("launcher file is larger than %d bytes", maxRegistryBytes)
	}
	if _, err := ParseCommands(contents); err != nil {
		return fmt.Errorf("invalid launcher config: %w", err)
	}
	resolved, err := resolveTrusted(path, os.Getuid())
	if err != nil {
		return fmt.Errorf("launcher file %s: %w", path, err)
	}
	file, err := OpenTrustedFile(path)
	if err != nil {
		return fmt.Errorf("launcher file %s: %w", path, err)
	}
	current, readErr := io.ReadAll(io.LimitReader(file, maxRegistryBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return fmt.Errorf("read launcher file %s: %w", path, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close launcher file %s: %w", path, closeErr)
	}
	if len(current) > maxRegistryBytes {
		return fmt.Errorf("launcher file %s is larger than %d bytes", path, maxRegistryBytes)
	}
	if !bytes.Equal(current, expected) {
		return ErrConfigChanged
	}
	temporary, err := os.CreateTemp(filepath.Dir(resolved), ".launchers-*.toml")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, resolved)
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
	for _, pattern := range entry.Patterns {
		if _, err := filepath.Match(pattern, ""); err != nil || pattern == "" || strings.Contains(pattern, "/") {
			return Command{}, fmt.Errorf("pattern %q must be a file name pattern such as \"*.py\"", pattern)
		}
		command.Patterns = append(command.Patterns, pattern)
	}
	if len(entry.ProxyHeaders) > 0 {
		if command.Mode != ModeService {
			return Command{}, fmt.Errorf("proxy_headers apply only to mode %q", ModeService)
		}
		command.ProxyHeaders = map[string]string{}
		for name, value := range entry.ProxyHeaders {
			if err := checkProxyHeader(name, value); err != nil {
				return Command{}, err
			}
			command.ProxyHeaders[name] = value
		}
	}
	return command, nil
}

// reservedProxyHeaders are request headers the proxy itself controls.
var reservedProxyHeaders = map[string]bool{
	"host": true, "connection": true, "content-length": true, "transfer-encoding": true, "upgrade": true,
	"te": true, "trailer": true, "keep-alive": true, "proxy-connection": true, "x-rotari-token": true,
}

func checkProxyHeader(name, value string) error {
	if name == "" || strings.ContainsFunc(name, func(r rune) bool {
		return r <= ' ' || r >= 0x7f || strings.ContainsRune("()<>@,;:\\\"/[]?={}", r)
	}) {
		return fmt.Errorf("proxy_headers name %q is not a valid HTTP header name", name)
	}
	if reservedProxyHeaders[strings.ToLower(name)] {
		return fmt.Errorf("proxy_headers cannot set %s", name)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("proxy_headers value for %s must be one line", name)
	}
	return nil
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
#   ROTARI_LAUNCHER_TOKEN      service mode: a secret made for this launch
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
#   patterns       only targets whose name matches one of these, such as
#                  ["*.py"] (default: any name)
#   proxy_headers  service only: a table of headers rotari's proxy adds to
#                  every request, in which $ROTARI_LAUNCHER_TOKEN stands for
#                  the launch's token (for example, an Authorization header
#                  of "token $ROTARI_LAUNCHER_TOKEN"). A service that requires
#                  this token cannot be used by other users who connect to
#                  its port directly.
#
# Built in, without an entry here: TensorBoard, Jupyter Lab (requires the
# launch token), MLflow (a directory named mlruns), MLflow (SQLite) (a file
# named mlflow.db), Aim, and Streamlit (*.py files). Each is shown disabled
# while its program is not installed. An entry with the same name replaces
# one.

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

// ErrTemplateExists is returned by WriteTemplate when the file exists.
var ErrTemplateExists = errors.New("launcher file already exists")

// WriteTemplate creates the launcher file at path with Template's contents,
// private (0600, in a directory created 0700) so it passes OpenTrustedFile.
// It never replaces an existing file.
func WriteTemplate(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%w: %s", ErrTemplateExists, path)
	}
	if err != nil {
		return fmt.Errorf("write launcher file %s: %w", path, err)
	}
	if _, err := file.Write(Template()); err != nil {
		file.Close()
		return fmt.Errorf("write launcher file %s: %w", path, err)
	}
	return file.Close()
}
