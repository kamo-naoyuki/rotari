package webui

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"

	"github.com/rotari-dev/rotari/internal/launcher"
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
	// Editable reports whether this server allows edits to this trusted file.
	Editable bool `json:"editable"`
}

func (s site) launcherConfig() (webLauncherConfig, error) {
	if s.LauncherFile == "" {
		return webLauncherConfig{}, errors.New("this server does not know where the launcher file is")
	}
	view := webLauncherConfig{Path: s.LauncherFile, Enabled: s.Launches != nil}
	tooLarge := false
	file, err := os.Open(s.LauncherFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		view.Error = err.Error()
	default:
		view.Exists = true
		data, readErr := io.ReadAll(io.LimitReader(file, maxLauncherConfigView+1))
		file.Close()
		if readErr != nil {
			view.Error = readErr.Error()
		}
		if len(data) > maxLauncherConfigView {
			data = data[:maxLauncherConfigView]
			view.Error = "launcher config is larger than the Web editor limit"
			tooLarge = true
		}
		view.Content = string(data)
	}
	view.Editable = s.AllowControl && view.Exists && !tooLarge
	if view.Editable && view.Exists {
		trusted, trustErr := launcher.OpenTrustedFile(s.LauncherFile)
		if trustErr != nil {
			view.Editable = false
		} else if closeErr := trusted.Close(); closeErr != nil {
			view.Editable = false
			view.Error = closeErr.Error()
		}
	}
	registry, err := launcher.LoadRegistry(s.LauncherFile)
	if err != nil {
		view.Error = err.Error()
		return view, nil
	}
	for _, command := range registry.Commands {
		view.Commands = append(view.Commands, webLaunchCommand{
			Name: command.Name, Command: command.Command, Targets: command.Targets, Patterns: command.Patterns, Mode: command.Mode, CWD: command.CWD,
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
	mux.HandleFunc("/api/save-launcher-config", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if !s.AllowControl {
			forbiddenReadOnly(writer)
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, 8<<20)
		var save struct {
			Expected string `json:"expected"`
			Content  string `json:"content"`
		}
		if err := json.NewDecoder(request.Body).Decode(&save); err != nil {
			writeWebError(writer, err)
			return
		}
		if s.LauncherFile == "" {
			writeWebError(writer, errors.New("this server does not know where the launcher file is"))
			return
		}
		if err := launcher.SaveConfig(s.LauncherFile, []byte(save.Expected), []byte(save.Content)); err != nil {
			if errors.Is(err, launcher.ErrConfigChanged) {
				http.Error(writer, err.Error(), http.StatusConflict)
				return
			}
			writeWebError(writer, err)
			return
		}
		writeWebJSON(writer, map[string]string{
			"message": "launcher config saved; restart rotari web --allow-exec to apply edits",
			"path":    s.LauncherFile,
		})
	})
}
