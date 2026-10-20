package launcher

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Manager limits.
const (
	DefaultMaxLive  = 16
	DefaultMaxEnded = 50
)

// Environment variables a launched command receives.
const (
	EnvArtifact = "ROTARI_LAUNCHER_ARTIFACT"
	EnvCWD      = "ROTARI_LAUNCHER_CWD"
	EnvPort     = "ROTARI_LAUNCHER_PORT"
	EnvBasePath = "ROTARI_LAUNCHER_BASE_PATH"
)

// noProxyLoopback is added to NO_PROXY and no_proxy: loopback traffic never
// needs a proxy, and TensorBoard's own gRPC to localhost fails through one.
const noProxyLoopback = "127.0.0.1,localhost,::1"

var (
	// ErrClosed is returned by Start after Close.
	ErrClosed = errors.New("the launcher is shutting down")
	// ErrTooManyLaunches is returned when MaxLive launches are running.
	ErrTooManyLaunches = errors.New("too many launches are running; stop one first")
	// ErrUnknownLaunch is returned for an ID the manager does not know.
	ErrUnknownLaunch = errors.New("unknown launch")
	// ErrNotServing is returned by ServiceUpstream for a launch that is not
	// a ready service.
	ErrNotServing = errors.New("launch is not serving")
)

// State is a launch's lifecycle state.
type State string

const (
	// StateStarting is a service that has not passed its readiness check.
	StateStarting State = "starting"
	// StateRunning is a running command, or a service that is ready.
	StateRunning  State = "running"
	StateStopping State = "stopping"
	// StateExited means the command ended by itself; Result says how.
	StateExited State = "exited"
	// StateStopped means Stop ended the command.
	StateStopped State = "stopped"
	// StateFailed means the command could not start; Error says why.
	StateFailed State = "failed"
)

// live reports whether a launch in this state still owns processes.
func (state State) live() bool {
	return state == StateStarting || state == StateRunning || state == StateStopping
}

// ManagerConfig configures a Manager. Zero limits use the defaults.
type ManagerConfig struct {
	// Wrapper is the argv that runs RunWrapper.
	Wrapper []string
	// Env is the environment launches inherit, normally os.Environ().
	Env             []string
	MaxLive         int
	MaxEnded        int
	LogSegmentBytes int64
	// ServicePath returns the URL path a service launch is served under,
	// which it receives as ROTARI_LAUNCHER_BASE_PATH. Without it, services
	// cannot be launched.
	ServicePath func(id string) string
}

// StartRequest asks for a registered command to run against a target.
type StartRequest struct {
	Command Command
	// Target is the absolute path of the selected file or directory.
	Target string
	Kind   TargetKind
	// Labels describe where the target came from, for display only.
	Labels map[string]string
	// NewInstance starts another instance even if a matching one is live.
	NewInstance bool
}

// Launch is a snapshot of one launch.
type Launch struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Mode      Mode              `json:"mode"`
	Target    string            `json:"target"`
	Kind      TargetKind        `json:"kind"`
	CWD       string            `json:"cwd"`
	Labels    map[string]string `json:"labels,omitempty"`
	State     State             `json:"state"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   *time.Time        `json:"ended_at,omitempty"`
	ExitCode  *int              `json:"exit_code,omitempty"`
	Signal    string            `json:"signal,omitempty"`
	Error     string            `json:"error,omitempty"`
	Reused    bool              `json:"reused,omitempty"`
	// Port and BasePath are a service's upstream port and URL path.
	Port       int    `json:"port,omitempty"`
	BasePath   string `json:"base_path,omitempty"`
	reuseKey   string
	stopGrace  time.Duration
	process    *Process
	log        *rotatingLog
	logDir     string
	stopCalled bool
	// recorded is closed once watch has recorded how the launch ended.
	recorded chan struct{}
}

func (launch *Launch) snapshot() Launch {
	copied := *launch
	copied.Labels = cloneLabels(launch.Labels)
	copied.process, copied.log = nil, nil
	return copied
}

// Manager owns every launch of one Web server.
type Manager struct {
	config   ManagerConfig
	lifetime *Lifetime
	dir      string
	// lock holds dir's flock so other managers' sweeps leave dir alone.
	lock *os.File

	mu       sync.Mutex
	closed   bool
	launches []*Launch // oldest first
}

// NewManager creates the manager's lifetime pipe and private runtime
// directory, removing runtime directories that managers which died without
// Close left behind.
func NewManager(config ManagerConfig) (*Manager, error) {
	if len(config.Wrapper) == 0 {
		return nil, errors.New("launcher wrapper command is empty")
	}
	if config.MaxLive <= 0 {
		config.MaxLive = DefaultMaxLive
	}
	if config.MaxEnded <= 0 {
		config.MaxEnded = DefaultMaxEnded
	}
	if config.LogSegmentBytes <= 0 {
		config.LogSegmentBytes = DefaultLogSegmentBytes
	}
	lifetime, err := NewLifetime()
	if err != nil {
		return nil, err
	}
	dir, lock, err := createRuntimeDir(os.TempDir())
	if err != nil {
		lifetime.Close()
		return nil, err
	}
	return &Manager{config: config, lifetime: lifetime, dir: dir, lock: lock}, nil
}

// Start runs request's command, or returns the matching live launch with
// Reused set. A command that cannot start is recorded as StateFailed and
// returned together with the error.
func (manager *Manager) Start(request StartRequest) (Launch, error) {
	command := request.Command
	if command.Mode == ModeService && manager.config.ServicePath == nil {
		return Launch{}, fmt.Errorf("launcher %q: this server cannot serve services", command.Name)
	}
	if !filepath.IsAbs(request.Target) {
		return Launch{}, fmt.Errorf("target %q is not an absolute path", request.Target)
	}
	if !command.Accepts(request.Kind) {
		return Launch{}, fmt.Errorf("launcher %q does not accept a %s", command.Name, request.Kind)
	}
	cwd := command.WorkingDirectory(request.Target, request.Kind)
	reuseKey := strings.Join([]string{command.Name, request.Target, cwd}, "\x00")

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return Launch{}, ErrClosed
	}
	if !request.NewInstance {
		for _, launch := range manager.launches {
			if launch.reuseKey == reuseKey && (launch.State == StateRunning || launch.State == StateStarting) {
				reused := launch.snapshot()
				reused.Reused = true
				return reused, nil
			}
		}
	}
	if manager.liveCount() >= manager.config.MaxLive {
		return Launch{}, ErrTooManyLaunches
	}

	id, err := newLaunchID()
	if err != nil {
		return Launch{}, err
	}
	launch := &Launch{
		ID: id, Name: command.Name, Command: command.Command, Mode: command.Mode,
		Target: request.Target, Kind: request.Kind, CWD: cwd, Labels: cloneLabels(request.Labels),
		StartedAt: time.Now(), reuseKey: reuseKey, stopGrace: command.StopGrace,
		logDir: filepath.Join(manager.dir, id), recorded: make(chan struct{}),
	}
	if command.Mode == ModeService {
		launch.BasePath = manager.config.ServicePath(id)
	}
	err = manager.prepare(launch)
	if err == nil {
		err = manager.spawn(launch)
	}
	manager.launches = append(manager.launches, launch)
	if err != nil {
		launch.fail(err)
		close(launch.recorded)
		manager.trimEnded()
		return launch.snapshot(), err
	}
	if command.Mode == ModeService {
		launch.State = StateStarting
		go manager.serve(launch, command)
	} else {
		launch.State = StateRunning
		go manager.watch(launch, nil)
	}
	return launch.snapshot(), nil
}

// prepare checks the launch's paths and creates its log.
func (manager *Manager) prepare(launch *Launch) error {
	if err := checkDirectory(launch.CWD); err != nil {
		return fmt.Errorf("working directory: %w", err)
	}
	if _, err := os.Stat(launch.Target); err != nil {
		return fmt.Errorf("target: %w", err)
	}
	if err := os.Mkdir(launch.logDir, 0o700); err != nil {
		return fmt.Errorf("create launch log directory: %w", err)
	}
	log, err := newRotatingLog(launch.logDir, manager.config.LogSegmentBytes)
	if err != nil {
		return fmt.Errorf("create launch log: %w", err)
	}
	launch.log = log
	return nil
}

// spawn starts the launch's process; a service first gets a new port.
func (manager *Manager) spawn(launch *Launch) error {
	if launch.Mode == ModeService {
		port, err := AllocatePort()
		if err != nil {
			return err
		}
		launch.Port = port
	}
	process, err := Start(manager.lifetime, Spec{
		Wrapper: manager.config.Wrapper,
		Command: launch.Command,
		Dir:     launch.CWD,
		Env:     launchEnvironment(manager.config.Env, launch),
		Output:  launch.log,
		Grace:   launch.stopGrace,
	})
	if err == nil {
		launch.process = process
	}
	return err
}

func checkDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

// launchEnvironment is env with the launch's variables set and loopback
// added to the no-proxy lists. Inherited ROTARI_LAUNCHER_* values are
// dropped so a command never sees a stale one.
func launchEnvironment(env []string, launch *Launch) []string {
	result := make([]string, 0, len(env)+4)
	noProxy := map[string]string{"NO_PROXY": "", "no_proxy": ""}
	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "ROTARI_LAUNCHER_") {
			continue
		}
		if _, ok := noProxy[name]; ok {
			noProxy[name] = value
			continue
		}
		result = append(result, entry)
	}
	for _, name := range []string{"NO_PROXY", "no_proxy"} {
		value := noProxyLoopback
		if inherited := noProxy[name]; inherited != "" {
			value = inherited + "," + noProxyLoopback
		}
		result = append(result, name+"="+value)
	}
	result = append(result, EnvArtifact+"="+launch.Target, EnvCWD+"="+launch.CWD)
	if launch.Mode == ModeService {
		result = append(result, EnvPort+"="+strconv.Itoa(launch.Port), EnvBasePath+"="+launch.BasePath)
	}
	return result
}

func (launch *Launch) fail(err error) {
	now := time.Now()
	launch.State, launch.Error, launch.EndedAt = StateFailed, err.Error(), &now
	if launch.log != nil {
		launch.log.Close()
	}
}

// watch records how a launch ended. A failure, when given and the launch
// was not stopped, makes it StateFailed with that reason.
func (manager *Manager) watch(launch *Launch, failure error) {
	manager.mu.Lock()
	process := launch.process
	manager.mu.Unlock()
	result, err := process.Wait()
	launch.log.Close()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	defer close(launch.recorded)
	now := time.Now()
	launch.EndedAt = &now
	switch {
	case err != nil:
		launch.State, launch.Error = StateFailed, err.Error()
	case launch.stopCalled:
		launch.State = StateStopped
	case failure != nil:
		launch.State, launch.Error = StateFailed, failure.Error()
	default:
		launch.State = StateExited
	}
	if err == nil {
		if result.Signal != 0 {
			launch.Signal = result.Signal.String()
		} else {
			code := result.ExitCode
			launch.ExitCode = &code
		}
	}
	manager.trimEnded()
}

func (manager *Manager) liveCount() int {
	count := 0
	for _, launch := range manager.launches {
		if launch.State.live() {
			count++
		}
	}
	return count
}

// trimEnded removes the oldest ended launches and their logs beyond
// MaxEnded.
func (manager *Manager) trimEnded() {
	ended := 0
	for _, launch := range manager.launches {
		if !launch.State.live() {
			ended++
		}
	}
	manager.launches = slices.DeleteFunc(manager.launches, func(launch *Launch) bool {
		if ended <= manager.config.MaxEnded || launch.State.live() {
			return false
		}
		ended--
		_ = os.RemoveAll(launch.logDir)
		return true
	})
}

// List returns every known launch, oldest first.
func (manager *Manager) List() []Launch {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	launches := make([]Launch, 0, len(manager.launches))
	for _, launch := range manager.launches {
		launches = append(launches, launch.snapshot())
	}
	return launches
}

// Get returns one launch.
func (manager *Manager) Get(id string) (Launch, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	launch, err := manager.find(id)
	if err != nil {
		return Launch{}, err
	}
	return launch.snapshot(), nil
}

func (manager *Manager) find(id string) (*Launch, error) {
	for _, launch := range manager.launches {
		if launch.ID == id {
			return launch, nil
		}
	}
	return nil, fmt.Errorf("%w %q", ErrUnknownLaunch, id)
}

// ReadLog returns a page of a launch's output; see rotatingLog.Read.
func (manager *Manager) ReadLog(id string, offset int64, limit int) (LogChunk, error) {
	manager.mu.Lock()
	launch, err := manager.find(id)
	manager.mu.Unlock()
	if err != nil {
		return LogChunk{}, err
	}
	if launch.log == nil {
		return LogChunk{}, nil
	}
	return launch.log.Read(offset, limit)
}

// Stop stops a live launch and waits for it to end. Stopping an ended
// launch returns it unchanged.
func (manager *Manager) Stop(id string) (Launch, error) {
	manager.mu.Lock()
	launch, err := manager.find(id)
	if err != nil {
		manager.mu.Unlock()
		return Launch{}, err
	}
	process := launch.process
	if launch.State == StateRunning || launch.State == StateStarting {
		launch.State, launch.stopCalled = StateStopping, true
	}
	live := launch.State.live()
	manager.mu.Unlock()
	if live {
		_, _ = process.Stop()
		<-launch.recorded
	}
	return manager.Get(id)
}

// Close rejects new launches, stops every live one, and removes the
// runtime directory. The lifetime pipe is closed last, so a wrapper that
// Stop could not reach still stops its command.
func (manager *Manager) Close() error {
	manager.mu.Lock()
	manager.closed = true
	var live []*Launch
	for _, launch := range manager.launches {
		if launch.State == StateRunning || launch.State == StateStarting {
			launch.State, launch.stopCalled = StateStopping, true
		}
		if launch.State.live() {
			live = append(live, launch)
		}
	}
	processes := make([]*Process, len(live))
	for i, launch := range live {
		processes[i] = launch.process
	}
	manager.mu.Unlock()
	var wg sync.WaitGroup
	for i, launch := range live {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = processes[i].Stop()
			<-launch.recorded
		}()
	}
	wg.Wait()
	err := errors.Join(manager.lifetime.Close(), os.RemoveAll(manager.dir))
	manager.lock.Close()
	return err
}

func newLaunchID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate launch ID: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func cloneLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	copied := make(map[string]string, len(labels))
	for key, value := range labels {
		copied[key] = value
	}
	return copied
}
