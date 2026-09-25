package executor

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// ErrJobNotRunning reports a job directory that no executor owns.
var ErrJobNotRunning = errors.New("job is not running")

// Registry maps executor names to executors.
type Registry map[string]JobExecutor

// NewRegistry returns the built-in executors. logf receives job submission
// and completion lines.
func NewRegistry(store state.Store, logf func(string, ...any)) Registry {
	return Registry{
		"local": NewLocal(store, logf),
		"slurm": NewSlurm(store, logf),
		"pbs":   NewPBS(store, logf),
		"lsf":   NewLSF(store, logf),
		"ssh":   NewSSH(store),
	}
}

// Lookup returns the executor registered under name.
func (registry Registry) Lookup(name string) (JobExecutor, bool) {
	executor, ok := registry[name]
	return executor, ok
}

// Known reports whether name is a registered executor.
func (registry Registry) Known(name string) bool {
	_, ok := registry[name]
	return ok
}

// Names returns the registered executor names, sorted for stable output.
func (registry Registry) Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Owner determines which executor owns the job recorded in jobDir, based on
// the metadata each executor writes on submission. Every scheduler-style
// executor writes job.json with its own "executor" name, so job.json alone
// (not its mere existence) names the owner; a local job has a pid file.
func (registry Registry) Owner(store state.Store, jobDir string) (JobExecutor, error) {
	if path, err := state.ValidatedStateFile(jobDir, "job.json"); err == nil { // NOSONAR: jobDir is restricted to validated job-path boundaries
		var meta struct {
			Executor string `json:"executor"`
		}
		if err := store.ReadJSON(path, &meta); err == nil {
			if executor, ok := registry.Lookup(meta.Executor); ok {
				return executor, nil
			}
		}
	}
	if path, err := state.ValidatedStateFile(jobDir, "pid"); err == nil {
		// codeql[go/path-injection]: path is returned by ValidatedStateFile.
		if _, err := os.Stat(path); err == nil {
			if executor, ok := registry.Lookup("local"); ok {
				return executor, nil
			}
		}
	}
	return nil, ErrJobNotRunning
}

// LocalHostMismatch reports the run's recorded hostname when executor is the
// local executor and this process runs on a different host. The local
// executor signals jobs by PID, which is only meaningful on the host that
// spawned the process; over a shared base directory, a failed PID check on
// the wrong host would misleadingly report "job is not running", so callers
// should surface this instead before trying.
func LocalHostMismatch(store state.Store, executor JobExecutor, runDir string) (recordedHost string, mismatch bool) {
	if executor.Name() != "local" {
		return "", false
	}
	safeRunDir := filepath.Join(filepath.Dir(runDir), filepath.Base(runDir))
	context, err := state.LoadContext(store, safeRunDir)
	if err != nil || context.Hostname == "" {
		return "", false
	}
	host, err := os.Hostname()
	if err != nil || strings.EqualFold(host, context.Hostname) {
		return "", false
	}
	return context.Hostname, true
}
