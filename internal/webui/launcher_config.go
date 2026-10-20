package webui

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/launcher"
)

// maxLauncherConfigView bounds the launcher file text /api/launcher-config
// returns.
const maxLauncherConfigView = 1 << 20

// webLauncherConfig is the /api/launcher-config response: the global
// launcher file as it is on disk now, which the running server may not
// have loaded yet (it reads the file at startup).
type webLauncherConfig struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Content string `json:"content,omitempty"`
	// Error is why `rotari web --allow-exec` would refuse the file now.
	Error string `json:"error,omitempty"`
	// Commands are what the file registers with the built-ins, as a
	// restarted server would load them.
	Commands []webLaunchCommand `json:"commands,omitempty"`
	// Enabled reports whether this server launches commands.
	Enabled bool `json:"enabled"`
}

func (s site) launcherConfig() (webLauncherConfig, error) {
	if s.LauncherFile == "" {
		return webLauncherConfig{}, errors.New("this server does not know where the launcher file is")
	}
	view := webLauncherConfig{Path: s.LauncherFile, Enabled: s.Launches != nil}
	file, err := os.Open(s.LauncherFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		view.Error = err.Error()
	default:
		view.Exists = true
		data, readErr := io.ReadAll(io.LimitReader(file, maxLauncherConfigView))
		file.Close()
		if readErr != nil {
			view.Error = readErr.Error()
		}
		view.Content = string(data)
	}
	registry, err := launcher.LoadRegistry(s.LauncherFile)
	if err != nil {
		view.Error = err.Error()
		return view, nil
	}
	for _, command := range registry.Commands {
		view.Commands = append(view.Commands, webLaunchCommand{
			Name: command.Name, Command: command.Command, Targets: command.Targets, Mode: command.Mode, CWD: command.CWD,
			Builtin: command.Builtin, Unavailable: command.Unavailable(),
		})
	}
	return view, nil
}

func (s site) registerLauncherConfigRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/launcher-config", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		view, err := s.launcherConfig()
		if err != nil {
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, view)
	})
	// Generating writes only the fixed template; the Web UI never writes
	// command text into the launcher file.
	mux.HandleFunc("/api/generate-launcher-config", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !s.AllowControl {
			forbiddenReadOnly(writer)
			return
		}
		if s.LauncherFile == "" {
			writeWebError(writer, errors.New("this server does not know where the launcher file is"))
			return
		}
		if err := launcher.WriteTemplate(s.LauncherFile); errors.Is(err, launcher.ErrTemplateExists) {
			http.Error(writer, err.Error(), http.StatusConflict)
			return
		} else if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		message := "launcher config generated; edit it, then restart rotari web --allow-exec"
		writeWebJSON(writer, map[string]string{"message": message, "path": s.LauncherFile})
	})
}
