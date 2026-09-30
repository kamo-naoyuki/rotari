package webui

import (
	"net/http"

	"github.com/kamo-naoyuki/rotari/internal/jobcontrol"
	"github.com/kamo-naoyuki/rotari/internal/notification"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/web"
)

// Options configures the Web UI for one base directory.
type Options struct {
	BaseDir string
	// RootBaseDir is the Web process's original state directory and anchors
	// the unprefixed routes while the browser switches to other basedirs.
	RootBaseDir string
	// BaseDirs are previously registered state directories available to switch
	// to from the sidebar. BaseDir is always included.
	BaseDirs []string
	// AllowControl enables the endpoints that edit queues, configs, and runs.
	AllowControl bool
	// Notifications is the default of the desktop notification toggle.
	Notifications bool
	// NotificationSettings controls which browser events and fields are shown.
	NotificationSettings notification.ChannelSettings

	Store      state.Store
	Editor     queueops.Editor
	Controller jobcontrol.Controller
	// Executors lists the executor names the UI offers.
	Executors []string
	// Environments are included in the Web state projection.
	Environments []web.EnvironmentDefinition
	// ConfigTemplate returns the TOML config template that "generate config"
	// writes.
	ConfigTemplate func() ([]byte, error)
}

// site serves one Options.
type site struct {
	Options
	notificationSession string
}

func (s site) environments() []web.EnvironmentDefinition {
	return append([]web.EnvironmentDefinition(nil), s.Environments...)
}

// Handler serves the Web UI and its JSON API.
func Handler(options Options) http.Handler {
	return site{Options: options, notificationSession: newNotificationSession()}.handler()
}

// GenerateStatic writes a read-only static export of the Web UI to
// outputDir, replacing what is there.
func GenerateStatic(outputDir string, options Options) error {
	return site{Options: options, notificationSession: newNotificationSession()}.generateStaticWeb(outputDir)
}
