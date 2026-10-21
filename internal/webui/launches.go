package webui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/launcher"
	stateinternal "github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/tableview"
)

// LaunchOptions enables launching registered commands from artifacts
// (rotari web --allow-exec).
type LaunchOptions struct {
	Manager  *launcher.Manager
	Registry launcher.Registry
	// transports serves /_launch/ (launch_proxy.go).
	transports launchTransports
}

// Bounds on one /api/launch-log page.
const (
	launchLogDefaultLimit = 64 << 10
	launchLogMaxLimit     = 1 << 20
)

// webLaunchCommand is a registered command as the UI lists it.
type webLaunchCommand struct {
	Name    string                `json:"name"`
	Command string                `json:"command"`
	Targets []launcher.TargetKind `json:"targets"`
	// Patterns limit the command to matching file names.
	Patterns []string      `json:"patterns,omitempty"`
	Mode     launcher.Mode `json:"mode"`
	CWD      string        `json:"cwd"`
	Builtin  bool          `json:"builtin,omitempty"`
	// Unavailable says why the command cannot run on this host, such as
	// its program missing from PATH; the UI shows it disabled.
	Unavailable string `json:"unavailable,omitempty"`
}

// launchAvailability is the /api/launchers response: whether this server
// can launch, and if not, why and how to enable it. The server words every
// explanation so the UI and the API say the same thing.
type launchAvailability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Hint      string `json:"hint,omitempty"`
	// Path is the launcher file.
	Path     string             `json:"path,omitempty"`
	Commands []webLaunchCommand `json:"commands,omitempty"`
	// Unmatched explains, per target kind, that no registered command
	// accepts it.
	Unmatched map[launcher.TargetKind]string `json:"unmatched,omitempty"`
	// Viewers are the Web UI's own viewers that accept the target; they run
	// no command, so they are listed whether or not launching is available.
	Viewers []webViewer `json:"viewers,omitempty"`
	// Host, User, and Shell are where and how launched commands run.
	Host  string `json:"host,omitempty"`
	User  string `json:"user,omitempty"`
	Shell string `json:"shell,omitempty"`
}

// launchAvailability describes what this server can launch. With a kind,
// and optionally a target's base name, it lists only the commands that
// accept such a target, and explains when none does.
func (s site) launchAvailability(kind launcher.TargetKind, name string) launchAvailability {
	path := s.LauncherFile
	if path == "" {
		path = "the launcher file in rotari's global config directory"
	}
	register := "Register commands in " + path + "; `rotari config --launchers` writes a template there."
	switch {
	case !s.AllowControl:
		return launchAvailability{Path: s.LauncherFile,
			Reason: "Launching is disabled: this server is read-only.",
			Hint:   "Restart rotari web without --allow-control=false and with --allow-exec; launching also requires job control. " + register}
	case s.Launches == nil:
		return launchAvailability{Path: s.LauncherFile,
			Reason: "Launching is disabled on this server.",
			Hint:   "Restart with `rotari web --allow-exec`, which requires signing in with a token (printed at startup unless ROTARI_WEB_AUTH_TOKEN is set). " + register}
	case len(s.Launches.Registry.Commands) == 0:
		return launchAvailability{Path: s.Launches.Registry.Path,
			Reason: "No commands are registered.",
			Hint:   "Register commands in " + s.Launches.Registry.Path + " (`rotari config --launchers` writes a template), then restart rotari web."}
	}
	availability := launchAvailability{Available: true, Path: s.Launches.Registry.Path, Shell: launcher.Shell}
	byKind := map[launcher.TargetKind][]string{}
	for _, command := range s.Launches.Registry.Commands {
		if kind != "" && (!command.Accepts(kind) || (name != "" && !command.Matches(name))) {
			continue
		}
		availability.Commands = append(availability.Commands, webLaunchCommand{
			Name: command.Name, Command: command.Command, Targets: command.Targets, Patterns: command.Patterns,
			Mode: command.Mode, CWD: command.CWD, Builtin: command.Builtin, Unavailable: command.Unavailable(),
		})
		for _, accepted := range command.Targets {
			byKind[accepted] = append(byKind[accepted], command.Name)
		}
	}
	kinds := []launcher.TargetKind{launcher.TargetFile, launcher.TargetDirectory}
	if kind != "" {
		kinds = []launcher.TargetKind{kind}
	}
	for _, unmatched := range kinds {
		if len(byKind[unmatched]) > 0 {
			continue
		}
		if availability.Unmatched == nil {
			availability.Unmatched = map[launcher.TargetKind]string{}
		}
		what := "a " + string(unmatched)
		if name != "" {
			what = fmt.Sprintf("the %s %s", unmatched, name)
		}
		availability.Unmatched[unmatched] = fmt.Sprintf("No registered command accepts %s. Commands accept: %s.", what, describeTargets(s.Launches.Registry.Commands))
	}
	availability.Host, _ = os.Hostname()
	if current, err := user.Current(); err == nil {
		availability.User = current.Username
	}
	return availability
}

// webViewer is a viewer the Web UI provides itself: it opens an artifact in
// a page of this server and runs no command, so it needs no --allow-exec.
type webViewer struct {
	Name string `json:"name"`
	// Path is the viewer's page on this server; the artifact's content
	// parameters name what it shows.
	Path     string                `json:"path"`
	Targets  []launcher.TargetKind `json:"targets"`
	Patterns []string              `json:"patterns,omitempty"`
}

// webViewers are the viewers Launch with offers besides commands.
// The Table viewer reads table files by name, and any directory.
var webViewers = []webViewer{{
	Name: "Table viewer", Path: "/tableview/", Targets: []launcher.TargetKind{launcher.TargetFile},
	Patterns: []string{"*.csv", "*.CSV", "*.tsv", "*.TSV", "*.tab", "*.psv", "*.txt", "*.scp", "*.lst", "*.list", "*.dat"},
}, {
	Name: "Table viewer", Path: "/tableview/", Targets: []launcher.TargetKind{launcher.TargetDirectory},
}}

// viewersFor lists the viewers that accept a target of kind named name, as
// commands are matched: by kind, then by name pattern.
func viewersFor(kind launcher.TargetKind, name string) []webViewer {
	var viewers []webViewer
	for _, viewer := range webViewers {
		accepts := launcher.Command{Targets: viewer.Targets, Patterns: viewer.Patterns}
		if kind != "" && (!accepts.Accepts(kind) || (name != "" && !accepts.Matches(name))) {
			continue
		}
		viewers = append(viewers, viewer)
	}
	return viewers
}

func describeTargets(commands []launcher.Command) string {
	parts := make([]string, 0, len(commands))
	for _, command := range commands {
		kinds := make([]string, 0, len(command.Targets))
		for _, kind := range command.Targets {
			kinds = append(kinds, string(kind))
		}
		accepts := strings.Join(kinds, ", ")
		if len(command.Patterns) > 0 {
			accepts += ": " + strings.Join(command.Patterns, ", ")
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", command.Name, accepts))
	}
	return strings.Join(parts, "; ")
}

// staticLaunchAvailability is what a static export answers.
func staticLaunchAvailability() launchAvailability {
	return launchAvailability{
		Reason: "Launching is unavailable in a static export.",
		Hint:   "Open the live `rotari web --allow-exec` server to launch commands.",
	}
}

// webLaunchError is the body of a rejected launch request.
type webLaunchError struct {
	Error string `json:"error"`
	Hint  string `json:"hint,omitempty"`
	// Launch is the recorded launch when the command could not start.
	Launch *launcher.Launch `json:"launch,omitempty"`
}

func writeLaunchError(writer http.ResponseWriter, status int, body webLaunchError) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}

// launchesUnavailable rejects a request when this server cannot launch,
// with the same explanation /api/launchers gives.
func (s site) launchesUnavailable(writer http.ResponseWriter) bool {
	if s.AllowControl && s.Launches != nil {
		return false
	}
	availability := s.launchAvailability("", "")
	writeLaunchError(writer, http.StatusForbidden, webLaunchError{Error: availability.Reason, Hint: availability.Hint})
	return true
}

// webLaunchRequest names an artifact the way the artifact content requests
// do, and a registered command by name. Nothing else is accepted: unknown
// fields such as command text or a working directory are rejected.
type webLaunchRequest struct {
	ProjectName string         `json:"project_name"`
	RunID       string         `json:"run_id"`
	JobID       string         `json:"job_id"`
	AttemptID   string         `json:"attempt_id"`
	Entry       webLaunchEntry `json:"entry"`
	Child       string         `json:"child"`
	Name        string         `json:"name"`
	NewInstance bool           `json:"new_instance"`
}

// webLaunchEntry is a launch request's entry: an index of the listing, or
// artifactWorkingDirectoryEntry, kept as the content requests' entry
// parameter so both resolve through resolveArtifact, or
// launchBasedirParentEntry. "" means absent.
type webLaunchEntry string

// launchBasedirParentEntry names, in a launch request with a run and no
// job, the directory holding the basedir: the experiment directory, where
// a run's jobs often share one tracker store or TensorBoard log directory.
// It is launched on under the same roots as an entry (artifactRoots), so
// only for a trusted run or under --artifact-root.
const launchBasedirParentEntry = "basedir_parent"

func (entry *webLaunchEntry) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var index int
	if err := json.Unmarshal(data, &index); err == nil {
		*entry = webLaunchEntry(strconv.Itoa(index))
		return nil
	}
	var name string
	if err := json.Unmarshal(data, &name); err != nil || (name != artifactWorkingDirectoryEntry && name != launchBasedirParentEntry) {
		return fmt.Errorf("entry must be an index of the listing, %q, or %q", artifactWorkingDirectoryEntry, launchBasedirParentEntry)
	}
	*entry = webLaunchEntry(name)
	return nil
}

// decodeLaunchJSON decodes a launch request body strictly.
func decodeLaunchJSON(request *http.Request, value any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		if field, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			return fmt.Errorf("unknown field %s: a launch names a registered command; command text, working directory, mode, and service settings come only from the launcher file", field)
		}
		return fmt.Errorf("invalid launch request: %w", err)
	}
	return nil
}

func (s site) registerLaunchRoutes(mux *http.ServeMux, baseDir string) {
	mux.HandleFunc("/api/launchers", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		query := request.URL.Query()
		kind := launcher.TargetKind(query.Get("kind"))
		if kind != "" && kind != launcher.TargetFile && kind != launcher.TargetDirectory {
			writeLaunchError(writer, http.StatusBadRequest, webLaunchError{Error: "kind must be file or directory"})
			return
		}
		availability := s.launchAvailability(kind, query.Get("name"))
		availability.Viewers = viewersFor(kind, query.Get("name"))
		writeWebJSON(writer, availability)
	})
	mux.HandleFunc("/api/launch", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if s.launchesUnavailable(writer) {
			return
		}
		var launch webLaunchRequest
		if err := decodeLaunchJSON(request, &launch); err != nil {
			writeLaunchError(writer, http.StatusBadRequest, webLaunchError{Error: err.Error()})
			return
		}
		s.startLaunch(writer, baseDir, launch)
	})
	mux.HandleFunc("/api/launch-preview", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		if s.launchesUnavailable(writer) {
			return
		}
		s.previewLaunch(writer, baseDir, request.URL.Query())
	})
	mux.HandleFunc("/api/launches", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		if s.launchesUnavailable(writer) {
			return
		}
		writeWebJSON(writer, map[string]any{"launches": s.Launches.Manager.List()})
	})
	mux.HandleFunc("/api/launch-log", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer)
			return
		}
		if s.launchesUnavailable(writer) {
			return
		}
		s.serveLaunchLog(writer, request.URL.Query())
	})
	mux.HandleFunc("/api/launch-stop", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer)
			return
		}
		if s.launchesUnavailable(writer) {
			return
		}
		var stop struct {
			ID string `json:"id"`
		}
		if err := decodeLaunchJSON(request, &stop); err != nil {
			writeLaunchError(writer, http.StatusBadRequest, webLaunchError{Error: err.Error()})
			return
		}
		launch, err := s.Launches.Manager.Stop(stop.ID)
		if err != nil {
			writeLaunchError(writer, launchErrorStatus(err), webLaunchError{Error: err.Error()})
			return
		}
		writeWebJSON(writer, launch)
	})
}

// launchStart resolves a launch request into what the manager starts:
// the registered command, and the artifact resolved like the previews
// resolve it. Preview and start share it, so the dialog shows exactly what
// a start would run.
func (s site) launchStart(baseDir string, request webLaunchRequest) (launcher.StartRequest, error) {
	command, ok := s.Launches.Registry.Lookup(request.Name)
	if !ok {
		return launcher.StartRequest{}, fmt.Errorf("no command named %q is registered in %s", request.Name, s.Launches.Registry.Path)
	}
	if reason := command.Unavailable(); reason != "" {
		return launcher.StartRequest{}, fmt.Errorf("cannot launch %q: %s; install it, or register a command that names its path in %s", command.Name, reason, s.Launches.Registry.Path)
	}
	if request.Entry == "" {
		return launcher.StartRequest{}, errors.New("entry must name an artifact candidate of the attempt")
	}
	var served servedArtifact
	var err error
	if request.Entry == launchBasedirParentEntry {
		served, err = s.resolveBasedirParent(baseDir, request)
	} else {
		served, err = s.resolveArtifact(baseDir, url.Values{
			"project_name": {request.ProjectName}, "run_id": {request.RunID}, "job_id": {request.JobID},
			"attempt_id": {request.AttemptID}, "entry": {string(request.Entry)}, "child": {request.Child},
		})
	}
	if err != nil {
		return launcher.StartRequest{}, err
	}
	kind, err := artifactKind(served)
	if err != nil {
		return launcher.StartRequest{}, err
	}
	if !command.Accepts(kind) {
		return launcher.StartRequest{}, fmt.Errorf("launcher %q does not accept a %s", command.Name, kind)
	}
	if name := filepath.Base(filepath.Join(served.root, served.relative)); !command.Matches(name) {
		return launcher.StartRequest{}, fmt.Errorf("launcher %q accepts only %s, not %s", command.Name, strings.Join(command.Patterns, ", "), name)
	}
	labels := map[string]string{"basedir": baseDir, "project": request.ProjectName, "run": request.RunID}
	if request.JobID != "" {
		labels["job"] = request.JobID
	}
	if request.AttemptID != "" {
		labels["attempt"] = request.AttemptID
	}
	return launcher.StartRequest{
		Command: command, Target: filepath.Join(served.root, served.relative), Kind: kind, Labels: labels, NewInstance: request.NewInstance,
	}, nil
}

// resolveBasedirParent resolves launchBasedirParentEntry for a run of the
// basedir under the roots its jobs' entries are served from.
func (s site) resolveBasedirParent(baseDir string, request webLaunchRequest) (servedArtifact, error) {
	if request.JobID != "" || request.AttemptID != "" || request.Child != "" {
		return servedArtifact{}, fmt.Errorf("entry %q names the directory holding the basedir: give project_name and run_id only", launchBasedirParentEntry)
	}
	paths, err := stateinternal.ResolveProjectPaths(baseDir, request.ProjectName)
	if err != nil {
		return servedArtifact{}, err
	}
	runDir, err := stateinternal.SafeJoin(paths.RunsDir, request.RunID)
	if err != nil {
		return servedArtifact{}, err
	}
	if _, err := os.Stat(runDir); err != nil {
		return servedArtifact{}, fmt.Errorf("run %s of project %s: %w", request.RunID, request.ProjectName, err)
	}
	parent, ok := basedirParentRoot(baseDir)
	if !ok {
		return servedArtifact{}, fmt.Errorf("%w: the basedir %s is directly under the filesystem root", errNotServed, baseDir)
	}
	roots, note := s.artifactRoots(jobstatus.ArtifactListing{}, baseDir, request.ProjectName, request.RunID)
	root, relative, inside := tableview.RootOf(parent, roots)
	if !inside {
		return servedArtifact{}, fmt.Errorf("%w: %s is not served. %s", errNotServed, parent, note)
	}
	return servedArtifact{root: root, relative: relative}, nil
}

func (s site) startLaunch(writer http.ResponseWriter, baseDir string, request webLaunchRequest) {
	start, err := s.launchStart(baseDir, request)
	if err != nil {
		writeLaunchError(writer, http.StatusBadRequest, webLaunchError{Error: err.Error()})
		return
	}
	launch, err := s.Launches.Manager.Start(start)
	if err != nil {
		body := webLaunchError{Error: err.Error()}
		if launch.ID != "" {
			body.Launch = &launch
		}
		writeLaunchError(writer, launchErrorStatus(err), body)
		return
	}
	writeWebJSON(writer, launch)
}

// webLaunchPreview is what the confirmation dialog shows before a start.
type webLaunchPreview struct {
	Name    string              `json:"name"`
	Command string              `json:"command"`
	Shell   string              `json:"shell"`
	Mode    launcher.Mode       `json:"mode"`
	Target  string              `json:"target"`
	Kind    launcher.TargetKind `json:"kind"`
	CWD     string              `json:"cwd"`
	Host    string              `json:"host"`
	User    string              `json:"user"`
	Labels  map[string]string   `json:"labels"`
}

func (s site) previewLaunch(writer http.ResponseWriter, baseDir string, query url.Values) {
	request := webLaunchRequest{ProjectName: query.Get("project_name"), RunID: query.Get("run_id"), JobID: query.Get("job_id"),
		AttemptID: query.Get("attempt_id"), Entry: webLaunchEntry(query.Get("entry")), Child: query.Get("child"), Name: query.Get("name")}
	start, err := s.launchStart(baseDir, request)
	if err != nil {
		writeLaunchError(writer, http.StatusBadRequest, webLaunchError{Error: err.Error()})
		return
	}
	availability := s.launchAvailability("", "")
	writeWebJSON(writer, webLaunchPreview{
		Name: start.Command.Name, Command: start.Command.Command, Shell: launcher.Shell, Mode: start.Command.Mode,
		Target: start.Target, Kind: start.Kind, CWD: start.Command.WorkingDirectory(start.Target, start.Kind),
		Host: availability.Host, User: availability.User, Labels: start.Labels,
	})
}

// artifactKind reports whether the resolved artifact is a file or a
// directory, looking through os.Root so a symlink leaving the root is
// refused. The target must exist on this host: a path recorded on another
// node is not evidence that it does.
func artifactKind(served servedArtifact) (launcher.TargetKind, error) {
	file, err := served.open()
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s does not exist on this host; a path recorded on another node may only exist there", filepath.Join(served.root, served.relative))
	}
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", filepath.Join(served.root, served.relative), err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	switch {
	case info.IsDir():
		return launcher.TargetDirectory, nil
	case info.Mode().IsRegular():
		return launcher.TargetFile, nil
	}
	return "", fmt.Errorf("%s is neither a regular file nor a directory", filepath.Join(served.root, served.relative))
}

func launchErrorStatus(err error) int {
	switch {
	case errors.Is(err, launcher.ErrUnknownLaunch):
		return http.StatusNotFound
	case errors.Is(err, launcher.ErrTooManyLaunches):
		return http.StatusConflict
	case errors.Is(err, launcher.ErrClosed):
		return http.StatusServiceUnavailable
	}
	return http.StatusUnprocessableEntity
}

// webLaunchLog is one page of a launch's output as text.
type webLaunchLog struct {
	Offset  int64  `json:"offset"`
	Next    int64  `json:"next"`
	Total   int64  `json:"total"`
	Dropped bool   `json:"dropped"`
	Text    string `json:"text"`
}

func (s site) serveLaunchLog(writer http.ResponseWriter, query url.Values) {
	offset, limit := int64(0), launchLogDefaultLimit
	if value := query.Get("offset"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			writeLaunchError(writer, http.StatusBadRequest, webLaunchError{Error: "offset must be a non-negative integer"})
			return
		}
		offset = parsed
	}
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > launchLogMaxLimit {
			writeLaunchError(writer, http.StatusBadRequest, webLaunchError{Error: fmt.Sprintf("limit must be between 1 and %d", launchLogMaxLimit)})
			return
		}
		limit = parsed
	}
	// from=end reads the last page instead, for a view that follows output.
	if query.Get("from") == "end" {
		if query.Get("offset") != "" {
			writeLaunchError(writer, http.StatusBadRequest, webLaunchError{Error: "offset cannot be combined with from=end"})
			return
		}
		total, err := s.Launches.Manager.ReadLog(query.Get("id"), 0, 1)
		if err != nil {
			writeLaunchError(writer, launchErrorStatus(err), webLaunchError{Error: err.Error()})
			return
		}
		offset = max(0, total.Total-int64(limit))
	}
	chunk, err := s.Launches.Manager.ReadLog(query.Get("id"), offset, limit)
	if err != nil {
		writeLaunchError(writer, launchErrorStatus(err), webLaunchError{Error: err.Error()})
		return
	}
	writeWebJSON(writer, webLaunchLog{
		Offset: chunk.Offset, Next: chunk.Next, Total: chunk.Total, Dropped: chunk.Dropped,
		Text: strings.ToValidUTF8(string(chunk.Data), "�"),
	})
}
