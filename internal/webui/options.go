package webui

import (
	"net/http"

	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/web"
)

// Options configures the Web UI for one base directory.
type Options struct {
	BaseDir string
	// ProjectFilter limits the UI to one project when not empty.
	ProjectFilter string
	// AllowControl enables the endpoints that edit queues, configs, and runs.
	AllowControl bool
	// Notifications is the default of the desktop notification toggle.
	Notifications bool

	Store      state.Store
	Editor     queueops.Editor
	Controller jobcontrol.Controller
	// Executors lists the executor names the UI offers.
	Executors []string
	// Environments lists the environment variables the UI documents; whether
	// each is set is filled in when state is loaded.
	Environments []web.EnvironmentDefinition
	// Commands documents the CLI on the docs page.
	Commands []CommandDoc
	// ConfigTemplate returns the TOML config template that "generate config"
	// writes.
	ConfigTemplate func() ([]byte, error)
}

// CommandDoc documents one CLI command on the docs page.
type CommandDoc struct {
	Name        string
	Description string
	Usage       string
	Flags       []FlagDoc
	Subcommands []SubcommandDoc
}

// FlagDoc documents one option of a command.
type FlagDoc struct {
	Name        string
	Description string
	// Values names the option's value or lists its choices.
	Values string
}

// SubcommandDoc documents one subcommand of a command.
type SubcommandDoc struct {
	Name        string
	Description string
}

// site serves one Options.
type site struct {
	Options
}

// Handler serves the Web UI and its JSON API.
func Handler(options Options) http.Handler {
	return site{options}.handler()
}

// GenerateStatic writes a read-only static export of the Web UI to
// outputDir, replacing what is there.
func GenerateStatic(outputDir string, options Options) error {
	return site{options}.generateStaticWeb(outputDir)
}

// environments returns a copy of the documented environment variables.
func (s site) environments() []web.EnvironmentDefinition {
	return append([]web.EnvironmentDefinition(nil), s.Environments...)
}
