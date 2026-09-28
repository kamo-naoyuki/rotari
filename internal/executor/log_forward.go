package executor

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type repeatedPath []string

func (paths *repeatedPath) String() string { return strings.Join(*paths, ",") }

func (paths *repeatedPath) Set(value string) error {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("destination must be non-empty and contain no NUL byte")
	}
	cleaned := filepath.Clean(value)
	for _, existing := range *paths {
		if filepath.Clean(existing) == cleaned {
			return fmt.Errorf("duplicate destination %q", value)
		}
	}
	*paths = append(*paths, value)
	return nil
}

// RunLogForward is an internal command used by wrappers only when a job has
// external --output or --error destinations. It runs the requested command,
// streaming each fd to the wrapper (for rotari's live attempt log) and to the
// configured destinations without requiring a tee executable on the host.
func RunLogForward(args []string) int {
	flags := flag.NewFlagSet("__log-forward", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	openMode := flags.String("open-mode", model.OpenModeAppend, "")
	var outputs, errorsTo []string
	flags.Var((*repeatedPath)(&outputs), "output", "")
	flags.Var((*repeatedPath)(&errorsTo), "error", "")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	command := flags.Args()
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	if len(command) == 0 {
		fmt.Fprintln(os.Stderr, "rotari __log-forward: missing command")
		return 2
	}
	if *openMode != model.OpenModeAppend && *openMode != model.OpenModeTruncate {
		fmt.Fprintln(os.Stderr, "rotari __log-forward: open mode must be append or truncate")
		return 2
	}
	if len(outputs) == 0 && len(errorsTo) == 0 {
		fmt.Fprintln(os.Stderr, "rotari __log-forward: at least one destination is required")
		return 2
	}

	destinations := make(map[string]*os.File)
	closeDestinations := func() {
		for _, file := range destinations {
			_ = file.Close()
		}
	}
	openDestination := func(path string) (*os.File, error) {
		resolved, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		resolved = filepath.Clean(resolved)
		if file := destinations[resolved]; file != nil {
			return file, nil
		}
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			return nil, err
		}
		mode := os.O_WRONLY | os.O_CREATE | os.O_APPEND
		if *openMode == model.OpenModeTruncate {
			mode = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		}
		file, err := os.OpenFile(resolved, mode, 0o600)
		if err != nil {
			return nil, err
		}
		destinations[resolved] = file
		return file, nil
	}
	outputFiles, err := openDestinations(openDestination, outputs)
	if err != nil {
		closeDestinations()
		fmt.Fprintf(os.Stderr, "rotari __log-forward: open stdout destination: %v\n", err)
		return 1
	}
	errorPaths := errorsTo
	if len(errorPaths) == 0 {
		errorPaths = outputs
	}
	errorFiles, err := openDestinations(openDestination, errorPaths)
	if err != nil {
		closeDestinations()
		fmt.Fprintf(os.Stderr, "rotari __log-forward: open stderr destination: %v\n", err)
		return 1
	}
	defer closeDestinations()

	shared := &sync.Mutex{}
	stdoutSinks := append([]io.Writer{os.Stdout}, filesAsWriters(outputFiles)...)
	stderrSinks := append([]io.Writer{os.Stderr}, filesAsWriters(errorFiles)...)
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "rotari __log-forward: create stdout pipe: %v\n", err)
		return 1
	}
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		fmt.Fprintf(os.Stderr, "rotari __log-forward: create stderr pipe: %v\n", err)
		return 1
	}
	process := exec.Command(command[0], command[1:]...)
	process.Stdout, process.Stderr = stdoutWriter, stderrWriter
	if err := process.Start(); err != nil {
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		_ = stderr.Close()
		_ = stderrWriter.Close()
		fmt.Fprintf(os.Stderr, "rotari __log-forward: start command: %v\n", err)
		return 1
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()

	copyResults := make(chan error, 2)
	go func() {
		_, copyErr := io.Copy(&lockedFanoutWriter{mu: shared, writers: stdoutSinks}, stdout)
		_ = stdout.Close()
		copyResults <- copyErr
	}()
	go func() {
		_, copyErr := io.Copy(&lockedFanoutWriter{mu: shared, writers: stderrSinks}, stderr)
		_ = stderr.Close()
		copyResults <- copyErr
	}()

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Stop(signals)
	waitResult := make(chan error, 1)
	go func() { waitResult <- process.Wait() }()
	var commandErr error
	waiting := true
	for waiting {
		select {
		case commandErr = <-waitResult:
			waiting = false
		case sig := <-signals:
			_ = process.Process.Signal(sig)
		}
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	copyErr1 := <-copyResults
	copyErr2 := <-copyResults
	if commandErr == nil && (copyErr1 != nil || copyErr2 != nil) {
		if copyErr1 != nil {
			commandErr = copyErr1
		} else {
			commandErr = copyErr2
		}
	}
	if commandErr == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(commandErr, &exitError) {
		return exitError.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "rotari __log-forward: command failed: %v\n", commandErr)
	return 1
}

func openDestinations(open func(string) (*os.File, error), paths []string) ([]*os.File, error) {
	files := make([]*os.File, 0, len(paths))
	for _, path := range paths {
		file, err := open(path)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func filesAsWriters(files []*os.File) []io.Writer {
	writers := make([]io.Writer, len(files))
	for index, file := range files {
		writers[index] = file
	}
	return writers
}

type lockedFanoutWriter struct {
	mu      *sync.Mutex
	writers []io.Writer
}

func (writer *lockedFanoutWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	for _, destination := range writer.writers {
		if _, err := destination.Write(data); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}
