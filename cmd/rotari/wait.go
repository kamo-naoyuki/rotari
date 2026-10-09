package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/attachment"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdWait waits concurrently for selected runs and returns the greatest run
// exit code, unless the user detaches or requests cancellation.
func cmdWait(args []string) int {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	var explicitRunIDs stringSliceFlag
	cliValue(fs, &explicitRunIDs, "run-id")
	timeout := cliDuration(fs, "timeout", 0)
	untilFailure := cliBool(fs, "until-failure", false)
	jsonOutput := cliBool(fs, "json", false)
	quiet := cliBool(fs, "quiet", false)
	disconnectAction := cliString(fs, "disconnect-action", serverinternal.DisconnectActionDetach)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if *disconnectAction != serverinternal.DisconnectActionDetach && *disconnectAction != serverinternal.DisconnectActionCancel {
		printErrorf("invalid disconnect action %q (choose detach or cancel)", *disconnectAction)
		return 1
	}
	if *timeout < 0 {
		printError("--timeout must be >= 0")
		return 1
	}
	selectors := fs.Args()
	targets := make([]resolve.Run, 0, len(explicitRunIDs)+len(selectors))
	for _, runID := range explicitRunIDs {
		targets = append(targets, resolve.Run{BaseDir: *basedir, ProjectName: *queueNameOption, RunID: runID})
	}
	for _, selector := range selectors {
		target, err := resolveWaitTarget(*basedir, *queueNameOption, selector)
		if err != nil {
			printError(err)
			return 1
		}
		targets = append(targets, target)
	}
	implicitSelection := len(targets) == 0 && *queueNameOption == ""
	if len(targets) == 0 {
		activeTargets, err := resolveActiveWaitTargets(*basedir, *queueNameOption)
		if err != nil {
			printError(err)
			return 1
		}
		if implicitSelection {
			if err := warnInterruptedWaitRuns(*basedir); err != nil {
				printError(err)
				return 1
			}
		}
		if len(activeTargets) == 0 {
			return 0
		}
		targets = append(targets, activeTargets...)
	}
	deadline := time.Time{}
	if *timeout > 0 {
		deadline = time.Now().Add(*timeout)
	}
	waitTargets := make([]resolve.Run, 0, len(targets))
	for _, target := range targets {
		if target.RunID == "" {
			continue
		}
		baseDir, projectName, runID, err := resolveCLIExistingRunID(target.BaseDir, target.ProjectName, target.RunID)
		if err != nil {
			printError(err)
			return 1
		}
		waitTargets = append(waitTargets, resolve.Run{BaseDir: baseDir, ProjectName: projectName, RunID: runID})
	}
	if len(waitTargets) == 0 {
		return 0 // A project that does not exist yet has nothing to wait for.
	}
	reservations := map[string]*attachment.Session(nil)
	if implicitSelection {
		var reserveErr error
		waitTargets, reservations, reserveErr = reserveImplicitWaitTargets(waitTargets, *disconnectAction)
		if reserveErr != nil {
			printError(reserveErr)
			return 1
		}
		if len(waitTargets) == 0 {
			return 0
		}
	}
	if !implicitSelection {
		if err := warnAttachedWaitTargets(waitTargets); err != nil {
			printError(err)
			return 1
		}
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	detach := make(chan struct{}, 1)
	if isTerminal(os.Stdin) {
		go watchClientInput(os.Stdin, detach, interrupt, *disconnectAction)
	}
	return waitTargetsControlled(waitTargets, deadline, *untilFailure, *jsonOutput, *quiet, interrupt, detach, false, *disconnectAction, reservations).exitCode
}

func implicitWaitKey(target resolve.Run) string {
	return target.BaseDir + "\x00" + target.ProjectName + "\x00" + target.RunID
}

// reserveImplicitWaitTargets rechecks and registers candidates under each
// project's state lock. Concurrent implicit waiters cannot both reserve one
// run that was unattached during their initial directory scans.
func reserveImplicitWaitTargets(candidates []resolve.Run, disconnectAction string) ([]resolve.Run, map[string]*attachment.Session, error) {
	selected := make([]resolve.Run, 0, len(candidates))
	reservations := make(map[string]*attachment.Session)
	for _, target := range candidates {
		paths, err := state.ResolveProjectPaths(target.BaseDir, target.ProjectName)
		if err != nil {
			closeWaitReservations(reservations)
			return nil, nil, err
		}
		release, err := state.AcquireStateLock(paths.StateLockFile)
		if err != nil {
			closeWaitReservations(reservations)
			return nil, nil, err
		}
		phase, err := project.RunPhaseOf(paths, target.RunID)
		if err != nil {
			release()
			closeWaitReservations(reservations)
			return nil, nil, err
		}
		if phase == project.RunPhaseRunning {
			attached, attachErr := attachment.IsAttached(paths, target.RunID)
			if attachErr != nil {
				release()
				closeWaitReservations(reservations)
				return nil, nil, attachErr
			}
			if attached {
				release()
				continue
			}
			sessionID, idErr := attachment.NewID()
			if idErr != nil {
				release()
				closeWaitReservations(reservations)
				return nil, nil, idErr
			}
			session, openErr := attachment.Open(paths, target.RunID, sessionID, disconnectAction)
			if openErr != nil {
				release()
				closeWaitReservations(reservations)
				return nil, nil, openErr
			}
			reservations[implicitWaitKey(target)] = session
		}
		release()
		selected = append(selected, target)
	}
	return selected, reservations, nil
}

func closeWaitReservations(sessions map[string]*attachment.Session) {
	for _, session := range sessions {
		_ = session.Close("")
	}
}

func warnAttachedWaitTargets(targets []resolve.Run) error {
	for _, target := range targets {
		if err := warnAttachedWaitTarget(target); err != nil {
			return err
		}
	}
	return nil
}

func warnAttachedWaitTarget(target resolve.Run) error {
	paths, err := state.ResolveProjectPaths(target.BaseDir, target.ProjectName)
	if err != nil {
		return err
	}
	phase, err := project.RunPhaseOf(paths, target.RunID)
	if err != nil || phase != project.RunPhaseRunning {
		return err
	}
	lock, err := state.LoadLock(paths.LockFile)
	if err != nil {
		return fmt.Errorf("failed to inspect run %s client: %w", target.RunID, err)
	}
	attached, err := attachment.IsAttached(paths, target.RunID)
	if err != nil {
		return fmt.Errorf("failed to inspect run %s attachment: %w", target.RunID, err)
	}
	if lock.RunID == target.RunID && attached {
		printWarningf("run %s in project %q is attached to a client; wait will attach to it", target.RunID, target.ProjectName)
	}
	return nil
}

type waitControlOutcome struct {
	exitCode     int
	detached     bool
	interrupted  bool
	disconnected bool
}

func waitTargetsWithControl(waitTargets []resolve.Run, deadline time.Time, untilFailure, jsonOutput, quiet bool, interrupt <-chan os.Signal, detach <-chan struct{}) int {
	return waitTargetsControlled(waitTargets, deadline, untilFailure, jsonOutput, quiet, interrupt, detach, false, serverinternal.DisconnectActionDetach, nil).exitCode
}

func waitTargetsControlled(waitTargets []resolve.Run, deadline time.Time, untilFailure, jsonOutput, quiet bool, interrupt <-chan os.Signal, detach <-chan struct{}, initial bool, disconnectAction string, reservations map[string]*attachment.Session) waitControlOutcome {
	stop := make(chan struct{})

	// Multiple runs are monitored concurrently. Their output is serialized at
	// event boundaries and tagged so interleaved progress remains attributable.
	var outputMu sync.Mutex
	type indexedResult struct {
		index  int
		result waitResult
	}
	results := make(chan indexedResult, len(waitTargets))
	outputs := make([]*waitOutput, len(waitTargets))
	usedColors := make(map[int]bool)
	controlHint := newWaitControlHint(len(waitTargets))
	for index, target := range waitTargets {
		var bufferedJSON bytes.Buffer
		output := newWaitOutput(target, len(waitTargets) > 1, &outputMu)
		output.color = availableWaitColor(output.color, usedColors)
		output.controlHint = controlHint
		usedColors[output.color] = true
		if jsonOutput {
			output.stdout = &bufferedJSON
			output.stdoutLabel = ""
			output.stderrLabel = output.label
		}
		outputs[index] = output
		go func(index int, target resolve.Run, output *waitOutput) {
			result := followRunWithOutput(target.BaseDir, target.ProjectName, target.RunID, deadline, untilFailure, jsonOutput, quiet, output, stop, initial, disconnectAction, reservations[implicitWaitKey(target)])
			results <- indexedResult{index: index, result: result}
		}(index, target, output)
	}
	exitCode := 0
	timedOut := false
	completedCount := 0
	for completedCount < len(waitTargets) {
		var completed indexedResult
		select {
		case completed = <-results:
			completedCount++
		case <-detach:
			close(stop)
			for completedCount < len(waitTargets) {
				<-results
				completedCount++
			}
			flushWaitJSON(outputs, jsonOutput)
			if !quiet && !jsonOutput {
				message := "Stopped waiting; runs continue in the background."
				if initial {
					message = serverinternal.DetachedMessage
				}
				fmt.Fprintln(os.Stdout, cyan(message))
			}
			return waitControlOutcome{detached: true}
		case receivedSignal := <-interrupt:
			for index, target := range waitTargets {
				cancelWaitTarget(target, outputs[index])
			}
			close(stop)
			for completedCount < len(waitTargets) {
				<-results
				completedCount++
			}
			flushWaitJSON(outputs, jsonOutput)
			if !quiet && !jsonOutput {
				fmt.Fprintln(os.Stdout, yellow("Cancellation requested; stopping running jobs..."))
			}
			disconnected := receivedSignal == syscall.SIGHUP || receivedSignal == syscall.SIGTERM
			return waitControlOutcome{exitCode: 130, interrupted: true, disconnected: disconnected}
		}
		if completed.result.exitCode > exitCode {
			exitCode = completed.result.exitCode
		}
		timedOut = timedOut || completed.result.timedOut
	}
	flushWaitJSON(outputs, jsonOutput)
	if timedOut {
		return waitControlOutcome{exitCode: 1}
	}
	return waitControlOutcome{exitCode: exitCode}
}

// Completed JSON results survive an explicit detach or cancellation, in
// selector order. Workers must have stopped before their buffers are read.
func flushWaitJSON(outputs []*waitOutput, enabled bool) {
	if !enabled {
		return
	}
	for _, output := range outputs {
		if buffer, ok := output.stdout.(*bytes.Buffer); ok {
			_, _ = os.Stdout.Write(buffer.Bytes())
		}
	}
}

func cancelWaitTarget(target resolve.Run, output *waitOutput) {
	paths, err := state.ResolveProjectPaths(target.BaseDir, target.ProjectName)
	if err != nil {
		output.errorf("failed to cancel run %s: %v", target.RunID, err)
		return
	}
	phase, err := project.RunPhaseOf(paths, target.RunID)
	if err != nil {
		output.errorf("failed to inspect run %s before cancellation: %v", target.RunID, err)
		return
	}
	if phase != project.RunPhaseRunning {
		return
	}
	if _, err := jobController().Cancel(target.BaseDir, target.ProjectName, target.RunID, nil, false); err != nil {
		// A run can finish between phase inspection and cancellation.
		phase, phaseErr := project.RunPhaseOf(paths, target.RunID)
		if phaseErr == nil && phase == project.RunPhaseRunning {
			output.errorf("failed to cancel run %s: %v", target.RunID, err)
		}
	}
}

type waitOutput struct {
	stdout      io.Writer
	stderr      io.Writer
	mu          *sync.Mutex
	label       string
	stdoutLabel string
	stderrLabel string
	color       int
	// controlHint is shared by every run one wait follows, so the terminal
	// controls are explained once.
	controlHint *waitControlHint
}

// waitControlHint is the Ctrl-D/Ctrl-C explanation printed when wait first
// attaches to a run.
type waitControlHint struct {
	once sync.Once
	text string
}

func newWaitControlHint(runs int) *waitControlHint {
	if runs > 1 {
		return &waitControlHint{text: fmt.Sprintf("Press Ctrl-D to stop waiting; Ctrl-C to cancel all %d runs.", runs)}
	}
	return &waitControlHint{text: "Press Ctrl-D to stop waiting; Ctrl-C to cancel the run."}
}

// printControlHint prints the hint once per wait, untagged because it
// applies to every followed run.
func (output *waitOutput) printControlHint() {
	hint := output.controlHint
	if hint == nil {
		hint = newWaitControlHint(1)
	}
	hint.once.Do(func() {
		untagged := *output
		untagged.stdoutLabel = ""
		_, _ = fmt.Fprintln(untagged.stdoutWriter(), cyan(hint.text))
	})
}

var waitIdentityColors = [...]int{33, 63, 69, 99, 105, 129, 135, 141}

func availableWaitColor(preferred int, used map[int]bool) int {
	if !used[preferred] {
		return preferred
	}
	for _, color := range waitIdentityColors {
		if !used[color] {
			return color
		}
	}
	return preferred
}

func newWaitOutput(target resolve.Run, tag bool, mu *sync.Mutex) *waitOutput {
	output := &waitOutput{mu: mu}
	if !tag {
		return output
	}
	suffix := target.RunID
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	output.label = target.ProjectName + "/…" + suffix
	output.stdoutLabel = output.label
	output.stderrLabel = output.label
	hash := fnv.New32a()
	_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%s", target.BaseDir, target.ProjectName, target.RunID)
	output.color = waitIdentityColors[hash.Sum32()%uint32(len(waitIdentityColors))]
	return output
}

type waitOutputStream struct {
	output *waitOutput
	stderr bool
}

func (stream waitOutputStream) Write(data []byte) (int, error) {
	output := stream.output
	writer := output.stdout
	label := output.stdoutLabel
	if stream.stderr {
		writer = output.stderr
		label = output.stderrLabel
	}
	if writer == nil {
		if stream.stderr {
			writer = os.Stderr
		} else {
			writer = os.Stdout
		}
	}
	text := string(data)
	if label != "" {
		file := os.Stdout
		if stream.stderr {
			file = os.Stderr
		}
		text = tagWaitOutput(text, label, output.color, file)
	}
	if output.mu != nil {
		output.mu.Lock()
		defer output.mu.Unlock()
	}
	_, err := io.WriteString(writer, text)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func tagWaitOutput(text, label string, color int, file *os.File) string {
	display := "[" + label + "] "
	prefix := display
	if terminalCheck(file) {
		prefix = fmt.Sprintf("\033[38;5;%dm%s\033[0m", color, display)
	}
	continuation := strings.Repeat(" ", len(display))
	lines := strings.SplitAfter(text, "\n")
	for index, line := range lines {
		if line == "" {
			continue
		}
		if index == 0 {
			lines[index] = prefix + line
		} else {
			lines[index] = continuation + line
		}
	}
	return strings.Join(lines, "")
}

func (output *waitOutput) stdoutWriter() io.Writer { return waitOutputStream{output: output} }

func (output *waitOutput) stderrWriter() io.Writer {
	return waitOutputStream{output: output, stderr: true}
}

func (output *waitOutput) errorf(format string, args ...any) {
	_, _ = fmt.Fprintln(output.stderrWriter(), redError(fmt.Sprintf(format, args...)))
}

func (output *waitOutput) error(err error) {
	output.errorf("%v", err)
}

func resolveWaitTarget(cliBaseDir, cliProjectName, selector string) (resolve.Run, error) {
	if selector == model.Latest {
		baseDir, projectName, runID, err := resolveCLIExistingRunID(cliBaseDir, cliProjectName, selector)
		if err != nil {
			return resolve.Run{}, err
		}
		return resolve.Run{BaseDir: baseDir, ProjectName: projectName, RunID: runID}, nil
	}
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return resolve.Run{}, err
	}
	// A project or run name waits for its active run, or else returns the
	// latest matching run's result, so a run that ends before wait is called
	// is not an error.
	if resolve.ProjectExists(baseDir, selector) {
		return resolveProjectWaitTarget(baseDir, selector)
	}
	selectedProject := cliProjectName
	if selectedProject == "" {
		selectedProject = os.Getenv(envProjectName)
	}
	if selectedProject != "" {
		if err := resolve.RequireProject(baseDir, selectedProject); err != nil {
			return resolve.Run{}, err
		}
	}

	activeTargets, err := resolve.RunsByName(baseDir, cliProjectName, selector, true)
	if err != nil {
		return resolve.Run{}, err
	}
	if len(activeTargets) == 0 {
		named, err := resolve.RunsByName(baseDir, cliProjectName, selector, false)
		if err != nil {
			return resolve.Run{}, err
		}
		activeTargets = latestRunPerProject(named)
	}
	if len(activeTargets) == 1 {
		return activeTargets[0], nil
	}
	if len(activeTargets) > 1 {
		candidates := make([]resolve.Job, 0, len(activeTargets))
		for _, target := range activeTargets {
			candidates = append(candidates, resolve.Job{Run: target})
		}
		return resolve.Run{}, resolve.AmbiguousError(fmt.Sprintf("run name %q", selector), candidates)
	}
	if _, found, registryErr := resolveRunLocation(selector); registryErr != nil {
		return resolve.Run{}, registryErr
	} else if found {
		// A run ID locates its own run, and an explicit base directory or
		// project that disagrees with the registry is an error, as with
		// --run-id and every other command.
		baseDir, projectName, err := resolveCLIExistingRun(cliBaseDir, cliProjectName, selector)
		if err != nil {
			return resolve.Run{}, err
		}
		return resolve.Run{BaseDir: baseDir, ProjectName: projectName, RunID: selector}, nil
	}
	if state.IsValidPathElement(selector) && !resolve.IsRunID(selector) && selectedProject == "" {
		return resolve.Run{}, resolve.RequireProject(baseDir, selector)
	}
	return resolve.Run{}, fmt.Errorf("no project, run name, or run ID matches %q", selector)
}

// resolveProjectWaitTarget applies the same active-then-latest rule to a
// project selected by a positional argument, option, or environment variable.
func resolveProjectWaitTarget(baseDir, projectName string) (resolve.Run, error) {
	runID, err := resolveActiveRunTarget(baseDir, projectName)
	if err != nil {
		baseDir, projectName, err = resolveCLIExistingRun(baseDir, projectName, "")
		if err != nil {
			return resolve.Run{}, err
		}
		paths, err := state.ResolveProjectPaths(baseDir, projectName)
		if err != nil {
			return resolve.Run{}, err
		}
		runID, err = resolve.RunID(paths, "")
		if err != nil {
			return resolve.Run{}, err
		}
	}
	return resolve.Run{BaseDir: baseDir, ProjectName: projectName, RunID: runID}, nil
}

// latestRunPerProject keeps the newest of runs in each project. Run IDs
// start with their creation time, so the greatest ID is the newest.
func latestRunPerProject(runs []resolve.Run) []resolve.Run {
	latest := make(map[string]resolve.Run)
	var order []string
	for _, run := range runs {
		key := run.BaseDir + "\x00" + run.ProjectName
		current, seen := latest[key]
		if !seen {
			order = append(order, key)
		}
		if !seen || run.RunID > current.RunID {
			latest[key] = run
		}
	}
	result := make([]resolve.Run, 0, len(order))
	for _, key := range order {
		result = append(result, latest[key])
	}
	return result
}

func resolveActiveWaitTargets(cliBaseDir, cliProjectName string) ([]resolve.Run, error) {
	if cliProjectName != "" {
		baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
		if err != nil {
			return nil, err
		}
		projectName, err := state.ResolveProjectName(baseDir, cliProjectName)
		if err != nil {
			return nil, err
		}
		if err := resolve.RequireProject(baseDir, projectName); err != nil {
			return nil, err
		}
		target, err := resolveProjectWaitTarget(baseDir, projectName)
		if err != nil {
			return nil, err
		}
		return []resolve.Run{target}, nil
	}
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	active := make([]resolve.Run, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths, pathErr := state.ResolveProjectPaths(baseDir, entry.Name())
		if pathErr != nil {
			return nil, pathErr
		}
		lockState, _, lockErr := state.InspectLock(paths.LockFile, false)
		if lockErr != nil {
			return nil, lockErr
		}
		if lockState != state.LockActive && lockState != state.LockRemote {
			continue
		}
		lock, lockErr := state.LoadLock(paths.LockFile)
		if lockErr != nil {
			return nil, lockErr
		}
		attached, attachErr := attachment.IsAttached(paths, lock.RunID)
		if attachErr != nil {
			return nil, attachErr
		}
		if attached {
			continue
		}
		active = append(active, resolve.Run{BaseDir: baseDir, ProjectName: entry.Name(), RunID: lock.RunID})
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ProjectName < active[j].ProjectName })
	return active, nil
}

// warnInterruptedWaitRuns reports runs that implicit wait cannot follow
// because their supervisor is gone. They do not change wait's exit code.
func warnInterruptedWaitRuns(cliBaseDir string) error {
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		paths, err := state.ResolveProjectPaths(baseDir, entry.Name())
		if err != nil {
			return err
		}
		inspection, err := project.Inspect(paths, false)
		if err != nil {
			return err
		}
		if inspection.State != project.Interrupted {
			continue
		}
		if message, ok := runEndedWithoutSummary(paths, inspection.RunID); ok {
			printWarningf("warning: %s", message)
		}
	}
	return nil
}

func resolveActiveRunTarget(cliBaseDir, cliProjectName string) (string, error) {
	baseDir, _, err := state.ResolveBaseDir(cliBaseDir)
	if err != nil {
		return "", err
	}
	queueName, err := state.ResolveProjectName(baseDir, cliProjectName)
	if err != nil {
		return "", err
	}
	if err := resolve.RequireProject(baseDir, queueName); err != nil {
		return "", err
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		return "", err
	}
	inspection, err := project.Inspect(paths, false)
	state, runID := inspection.State, inspection.RunID
	if err != nil {
		return "", fmt.Errorf("failed to check project state: %w", err)
	}
	if state != project.Running || runID == "" {
		return "", fmt.Errorf("project %q has no active run", queueName)
	}
	return runID, nil
}

type waitResult struct {
	exitCode int
	timedOut bool
}

// waitForRun waits until runID finishes, or with untilFailure until one of
// its jobs has failed with no retry left, whichever comes first.
func waitForRun(basedir, queueNameOption, runID string, deadline time.Time, untilFailure, jsonOutput, quiet bool) waitResult {
	return waitForRunWithOutput(basedir, queueNameOption, runID, deadline, untilFailure, jsonOutput, quiet, &waitOutput{}, nil)
}

func waitForRunWithOutput(basedir, queueNameOption, runID string, deadline time.Time, untilFailure, jsonOutput, quiet bool, output *waitOutput, stop <-chan struct{}) waitResult {
	return followRunWithOutput(basedir, queueNameOption, runID, deadline, untilFailure, jsonOutput, quiet, output, stop, false, serverinternal.DisconnectActionDetach, nil)
}

// followRunWithOutput is the shared post-start attachment operation for sync
// run/retry and wait. initial starts the cursor at the beginning; wait skips
// the progress history that predates its attachment.
func followRunWithOutput(basedir, queueNameOption, runID string, deadline time.Time, untilFailure, jsonOutput, quiet bool, output *waitOutput, stop <-chan struct{}, initial bool, disconnectAction string, existing *attachment.Session) waitResult {
	baseDir, queueName, runID, err := resolveCLIExistingRunID(basedir, queueNameOption, runID)
	if err != nil {
		output.error(err)
		return waitResult{exitCode: 1}
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		output.errorf("failed to resolve paths: %v", err)
		return waitResult{exitCode: 1}
	}
	runDir, err := state.SafeJoin(paths.RunsDir, runID)
	if err != nil {
		output.errorf("invalid run ID %q", runID)
		return waitResult{exitCode: 1}
	}
	// Follow only events emitted after wait attaches. JSON never reads or
	// renders the text progress stream.
	phase, phaseErr := project.RunPhaseOf(paths, runID)
	followProgress := !jsonOutput && (initial || phaseErr == nil && phase == project.RunPhaseRunning)
	clientSession := existing
	if !initial && clientSession == nil && phaseErr == nil && phase == project.RunPhaseRunning {
		release, lockErr := state.AcquireStateLock(paths.StateLockFile)
		if lockErr != nil {
			output.errorf("failed to register attachment to run %s: %v", runID, lockErr)
			return waitResult{exitCode: 1}
		}
		lockedPhase, phaseErr := project.RunPhaseOf(paths, runID)
		if phaseErr == nil && lockedPhase == project.RunPhaseRunning {
			sessionID, idErr := attachment.NewID()
			if idErr != nil {
				release()
				output.errorf("failed to create wait attachment: %v", idErr)
				return waitResult{exitCode: 1}
			}
			clientSession, err = attachment.Open(paths, runID, sessionID, disconnectAction)
		}
		release()
		if err != nil {
			output.errorf("failed to register attachment to run %s: %v", runID, err)
			return waitResult{exitCode: 1}
		}
		if phaseErr == nil && lockedPhase != project.RunPhaseRunning {
			followProgress = false
		}
	}
	if clientSession != nil && !initial {
		defer func() {
			if err := clientSession.Close(""); err != nil {
				output.errorf("warning: failed to release wait attachment: %v", err)
			}
		}()
	}
	var cursor state.ProgressCursor
	printer := runProgressPrinter{quiet: quiet, lastCompleted: -1, lastSucceeded: -1, lastFailed: -1, output: output.stdoutWriter()}
	if initial {
		printer.controlHint = "Press Ctrl-D to detach; Ctrl-C to cancel."
	}
	var attachedSnapshot model.ProgressEvent
	hasAttachedSnapshot := false
	if followProgress && !initial {
		var err error
		attachedSnapshot, hasAttachedSnapshot, err = cursor.SkipExisting(runDir)
		if err != nil {
			output.errorf("warning: failed to read progress for run %s: %v; continuing without progress", runID, err)
			followProgress = false
		}
		if !quiet && !jsonOutput {
			_, _ = fmt.Fprintln(output.stdoutWriter(), cyan("=== Run attached ==="))
			output.printControlHint()
			if hasAttachedSnapshot {
				printer.print(serverinternal.Response{
					Progress: true, Completed: attachedSnapshot.Completed, Total: attachedSnapshot.Total,
					Succeeded: attachedSnapshot.Succeeded, Failed: attachedSnapshot.Failed,
				})
			}
		}
	}
	drainProgress := func() {
		if !followProgress {
			return
		}
		if err := readWaitProgress(&cursor, runDir, &printer, output); err != nil {
			output.errorf("warning: failed to read progress for run %s: %v; continuing without progress", runID, err)
			followProgress = false
		}
	}
	for {
		select {
		case <-stop:
			return waitResult{}
		default:
		}
		drainProgress()
		summaryPath, pathErr := state.ValidatedStateFile(runDir, "summary.json")
		if pathErr != nil {
			output.errorf("invalid run directory %q", runID)
			return waitResult{exitCode: 1}
		}
		summary, err := state.LoadRunSummary(summaryPath)
		if err == nil {
			phase, phaseErr := project.RunPhaseOf(paths, runID)
			if phaseErr != nil {
				output.errorf("failed to check project state: %v", phaseErr)
				return waitResult{exitCode: 1}
			}
			if phase == project.RunPhaseRunning {
				if !deadline.IsZero() && time.Now().After(deadline) {
					output.errorf("timed out waiting for run %s", runID)
					return waitResult{exitCode: 1, timedOut: true}
				}
				if waitStopped(stop) {
					return waitResult{}
				}
				continue
			}
			if phase == project.RunPhaseInterrupted {
				output.errorf("%s", endedRunHint(paths, runID, "was interrupted after writing its summary", true))
				return waitResult{exitCode: 1}
			}
			// Final events can arrive between the poll above and finalization.
			// The supervisor journals them before releasing the run lock.
			drainProgress()
			if jsonOutput {
				_ = json.NewEncoder(output.stdoutWriter()).Encode(summary)
			} else if !quiet {
				_, _ = io.WriteString(output.stdoutWriter(), formatRunCompletion(paths, runID, summary))
			}
			return waitResult{exitCode: summary.ExitCode}
		}
		if runInfo, statErr := os.Stat(runDir); os.IsNotExist(statErr) || (statErr == nil && !runInfo.IsDir()) {
			output.errorf("run %q is registered but its run directory is missing; run 'rotari gc --dry-run' to list stale registry entries and 'rotari gc' to remove them", runID)
			return waitResult{exitCode: 1}
		} else if statErr != nil {
			output.errorf("failed to inspect run directory %s: %v", runDir, statErr)
			return waitResult{exitCode: 1}
		}
		if errors.Is(err, os.ErrNotExist) {
			if message, ended := runEndedWithoutSummary(paths, runID); ended {
				output.errorf("%s", message)
				return waitResult{exitCode: 1}
			}
		} else if errors.Is(err, state.ErrNewerStateVersion) {
			output.error(err)
			return waitResult{exitCode: 1}
		} else if errors.Is(err, state.ErrInvalidJSON) {
			if message, ended := runEndedWithInvalidSummary(paths, runID); ended {
				output.errorf("%s", message)
				return waitResult{exitCode: 1}
			}
		} else {
			output.errorf("failed to read run summary %s: %v", summaryPath, err)
			return waitResult{exitCode: 1}
		}
		if untilFailure {
			if failures := finalFailureGroups(paths, runID); len(failures) > 0 {
				writeEarlyFailures(paths, runID, failures, jsonOutput, output)
				return waitResult{exitCode: 1}
			}
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			output.errorf("timed out waiting for run %s", runID)
			return waitResult{exitCode: 1, timedOut: true}
		}
		if waitStopped(stop) {
			return waitResult{}
		}
	}
}

func waitStopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	case <-time.After(500 * time.Millisecond):
		return false
	}
}

// readWaitProgress shares the attached run's renderer without its terminal
// control hints. The cursor retains partial lines and tolerates old runs with
// no journal. A malformed complete line is consumed: report it, then continue
// draining so it cannot hide later events, including the final ones.
func readWaitProgress(cursor *state.ProgressCursor, runDir string, printer *runProgressPrinter, output *waitOutput) error {
	for {
		events, err := cursor.Read(runDir)
		for _, event := range events {
			printer.print(serverinternal.Response{
				OK: event.OK, Message: event.Message, Progress: event.Progress, RunID: event.RunID, Notice: event.Notice,
				JobID: event.JobID, Completed: event.Completed, Total: event.Total,
				Succeeded: event.Succeeded, Failed: event.Failed,
			})
		}
		if !errors.Is(err, state.ErrInvalidJSON) {
			return err
		}
		output.errorf("skipping malformed run progress: %v", err)
	}
}

// finalFailureGroups groups the jobs of an active run that have failed with
// no retry left; see runview.FinalFailureGroups. A run that cannot be read
// yet has none.
func finalFailureGroups(paths state.ProjectPaths, runID string) []runlineage.FailureGroup {
	groups, err := runview.FinalFailureGroups(paths, runID, jsonStore())
	if err != nil {
		return nil
	}
	return groups
}

// earlyFailureJSON is what wait --until-failure --json prints for a run that
// is still running when a job has failed.
type earlyFailureJSON struct {
	RunID    string                    `json:"run_id"`
	Status   string                    `json:"status"`
	Failures []runlineage.FailureGroup `json:"failures"`
}

// writeEarlyFailures reports that runID, still running, has failed jobs.
func writeEarlyFailures(paths state.ProjectPaths, runID string, failures []runlineage.FailureGroup, jsonOutput bool, output *waitOutput) {
	if jsonOutput {
		_ = json.NewEncoder(output.stdoutWriter()).Encode(earlyFailureJSON{RunID: runID, Status: "running", Failures: failures})
		return
	}
	var message bytes.Buffer
	fmt.Fprintln(&message, red(fmt.Sprintf("Run %s is still running, and jobs have failed.", runID)))
	// The run is still running, so a retry of its failures cannot start yet.
	writeFailureGroups(&message, failures, nil)
	fmt.Fprintln(&message, cyan("To keep waiting:"))
	fmt.Fprintf(&message, "  rotari wait %s--run-id %s\n", runHintLocation(paths), executor.ShellQuote(runID))
	fmt.Fprintln(&message, cyan("To cancel the run:"))
	fmt.Fprintf(&message, "  rotari cancel %s\n", hintLocation(paths))
	_, _ = output.stdoutWriter().Write(message.Bytes())
}

// runEndedWithInvalidSummary reports an invalid summary only after its run is
// no longer active, allowing wait to tolerate a summary being atomically replaced.
func runEndedWithInvalidSummary(paths state.ProjectPaths, runID string) (string, bool) {
	phase, err := project.RunPhaseOf(paths, runID)
	if err != nil || phase == project.RunPhaseRunning || phase == project.RunPhaseFinished {
		return "", false
	}
	if phase == project.RunPhaseInterrupted {
		return endedRunHint(paths, runID, "was interrupted without a valid summary", true), true
	}
	return endedRunHint(paths, runID, "is not active and has no valid summary", false), true
}

// runEndedWithoutSummary reports whether runID is no longer active although it
// never wrote summaryPath, for example because its supervisor exited early.
// It leaves a stale run lock in place for show and unlock.
func runEndedWithoutSummary(paths state.ProjectPaths, runID string) (string, bool) {
	// RunPhaseOf reads the summary again, so a run that finished since the
	// caller's read is not reported as ended.
	phase, err := project.RunPhaseOf(paths, runID)
	if err != nil || phase == project.RunPhaseRunning || phase == project.RunPhaseFinished {
		return "", false
	}
	if phase == project.RunPhaseInterrupted {
		return endedRunHint(paths, runID, "was interrupted before it wrote a summary", true), true
	}
	return endedRunHint(paths, runID, "is not active and has no summary", false), true
}

// endedRunHint describes a run wait cannot follow and lists the commands to
// inspect it and, for an interrupted run, recover it. Each command is on its
// own line so its shell-quoted arguments can be copied as they are.
func endedRunHint(paths state.ProjectPaths, runID, condition string, recover bool) string {
	show := "rotari show " + runHintLocation(paths) + "--run-id " + executor.ShellQuote(runID)
	if !recover {
		return fmt.Sprintf("run %s %s. Inspect it:\n  %s", runID, condition, show)
	}
	unlock := "rotari unlock " + hintLocation(paths) + " --run-id " + executor.ShellQuote(runID)
	// Jobs recorded as unfinished may outlive the supervisor; say so before
	// offering recovery, as commands refusing an interrupted project do.
	detail, stillRunning := project.InterruptedRunDetail(paths, runID)
	if stillRunning {
		return fmt.Sprintf("run %s %s%s.\nInspect it:\n  %s\n%s Then recover:\n  %s", runID, condition, detail, show, project.UnconfirmedStopWarning, unlock)
	}
	return fmt.Sprintf("run %s %s%s. Inspect it, then recover:\n  %s\n  %s", runID, condition, detail, show, unlock)
}
