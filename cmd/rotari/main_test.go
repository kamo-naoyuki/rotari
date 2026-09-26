package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/jobstatus"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/queueops"
	runcontract "github.com/kamo-naoyuki/rotari/internal/run"
	serverinternal "github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestResolveProjectNamePriority(t *testing.T) {
	const envName = "ROTARI_PROJECT_NAME"
	old, existed := os.LookupEnv(envName)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(envName, old)
		} else {
			_ = os.Unsetenv(envName)
		}
	})

	baseDir := t.TempDir()

	if err := os.Setenv(envName, "from-env"); err != nil {
		t.Fatal(err)
	}
	got, err := state.ResolveProjectName(baseDir, "from-option")
	if err != nil || got != "from-option" {
		t.Fatalf("option priority: got %q, err %v", got, err)
	}
	got, err = state.ResolveProjectName(baseDir, "")
	if err != nil || got != "from-env" {
		t.Fatalf("environment priority: got %q, err %v", got, err)
	}
	if err := os.Unsetenv(envName); err != nil {
		t.Fatal(err)
	}
	got, err = state.ResolveProjectName(baseDir, "")
	if err != nil || got != defaultProjectName {
		t.Fatalf("default priority: got %q, want %q, err %v", got, defaultProjectName, err)
	}

	// Test automatically selecting a single queue if only one exists
	q1Dir := filepath.Join(baseDir, "projects", "q1")
	if err := os.MkdirAll(q1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	got, err = state.ResolveProjectName(baseDir, "")
	if err != nil || got != "q1" {
		t.Fatalf("auto select single project: got %q, want q1, err %v", got, err)
	}

	// Test returning an error if multiple projects exist and none is specified
	q2Dir := filepath.Join(baseDir, "projects", "q2")
	if err := os.MkdirAll(q2Dir, 0755); err != nil {
		t.Fatal(err)
	}
	_, err = state.ResolveProjectName(baseDir, "")
	if err == nil {
		t.Fatal("expected error for multiple projects when project-name is empty, got nil")
	}
	if !strings.Contains(err.Error(), `state directory "`+baseDir+`"`) {
		t.Fatalf("ambiguous project error = %q, want state directory", err)
	}
}
func TestCmdAddExpandsMatrixIntoIndependentJobs(t *testing.T) {
	baseDir := t.TempDir()
	code := cmdAdd([]string{
		"--basedir", baseDir, "--project-name", "demo", "--job-name", "train",
		"--matrix", "python=3.10,3.11", "--matrix", "cuda=cpu,cuda", "echo", "hello",
	})
	if code != 0 {
		t.Fatalf("cmdAdd exit code = %d, want 0", code)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 4 {
		t.Fatalf("queue contains %d commands, want 4", len(queue.Commands))
	}
	if queue.Commands[0].Name != "train-python3.10-cudacpu" || queue.Commands[3].Name != "train-python3.11-cudacuda" {
		t.Fatalf("matrix job names = %q, %q", queue.Commands[0].Name, queue.Commands[3].Name)
	}
	if queue.Commands[0].ID == queue.Commands[1].ID {
		t.Fatal("matrix jobs have the same job ID")
	}
	if queue.Commands[0].Matrix == nil || queue.Commands[0].Matrix.GroupID == "" || queue.Commands[0].Matrix.GroupID != queue.Commands[3].Matrix.GroupID {
		t.Fatalf("matrix provenance = %#v, %#v", queue.Commands[0].Matrix, queue.Commands[3].Matrix)
	}
	if !reflect.DeepEqual(queue.Commands[0].Matrix.Values, []model.MatrixValue{{Name: "python", Value: "3.10"}, {Name: "cuda", Value: "cpu"}}) {
		t.Fatalf("matrix values = %#v", queue.Commands[0].Matrix.Values)
	}
	if !hasEnvironmentEntry(queue.Commands[0].Environment, "python=3.10") || !hasEnvironmentEntry(queue.Commands[0].Environment, "cuda=cpu") {
		t.Fatalf("matrix environment = %#v", queue.Commands[0].Environment)
	}
}

func hasEnvironmentEntry(environment []string, want string) bool {
	for _, entry := range environment {
		if entry == want {
			return true
		}
	}
	return false
}

func TestCmdAddCombinesMatrixWithArray(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--job-name", "train", "--array", "1-3", "--matrix", "python=3.10,3.11", "echo", "hello"}); code != 0 {
		t.Fatalf("cmdAdd exit code = %d, want 0", code)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 {
		t.Fatalf("queue contains %d commands, want 2 matrix commands", len(queue.Commands))
	}
	for _, command := range queue.Commands {
		if command.Array == nil || command.Array.First != 1 || command.Array.Last != 3 {
			t.Fatalf("matrix command array = %#v, want 1-3", command.Array)
		}
	}
	if jobs := model.QueueToJobs(queue.Commands); len(jobs) != 6 {
		t.Fatalf("expanded jobs = %d, want 6 matrix-by-array jobs", len(jobs))
	}
}

func TestCmdAddCombinesMatrixWithSparseArray(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--job-name", "train", "--array", "1,3,4", "--matrix", "python=3.10,3.11", "echo", "hello"}); code != 0 {
		t.Fatalf("cmdAdd exit code = %d, want 0", code)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 2 {
		t.Fatalf("queue contains %d commands, want 2 matrix commands", len(queue.Commands))
	}
	for _, command := range queue.Commands {
		if command.Array == nil || !reflect.DeepEqual(command.Array.Tasks, []int{1, 3, 4}) {
			t.Fatalf("matrix command array = %#v, want sparse tasks 1,3,4", command.Array)
		}
	}
	if jobs := model.QueueToJobs(queue.Commands); len(jobs) != 6 {
		t.Fatalf("expanded jobs = %d, want 6 matrix-by-sparse-array jobs", len(jobs))
	}
}

func TestSanitizeMatrixName(t *testing.T) {
	for value, want := range map[string]string{
		"3.10":       "3.10",
		"cuda/12":    "cuda_12",
		"a b+c":      "a_b_c",
		"日本語":        "___",
		"already_ok": "already_ok",
	} {
		if got := model.SanitizeMatrixName(value); got != want {
			t.Errorf("model.SanitizeMatrixName(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestCmdAddRejectsDuplicateMatrixKey(t *testing.T) {
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--matrix", "python=3.10", "--matrix", "python=3.11", "echo", "hello"}); code != 1 {
		t.Fatalf("cmdAdd exit code = %d, want 1", code)
	}
}

func TestResolvePathsRejectsProjectTraversal(t *testing.T) {
	for _, projectName := range []string{"../outside", ".", "..", "nested/project"} {
		if _, err := state.ResolveProjectPaths(t.TempDir(), projectName); err == nil {
			t.Errorf("state.ResolveProjectPaths accepted unsafe project name %q", projectName)
		}
	}
}

func TestResolveProjectNameRejectsUnsafeProjectNames(t *testing.T) {
	for _, projectName := range []string{"../outside", "/tmp/outside", ".", "..", "nested/project", "subdir/..", "job/with/slash"} {
		if _, err := state.ResolveProjectName(t.TempDir(), projectName); err == nil {
			t.Errorf("state.ResolveProjectName accepted unsafe project name %q", projectName)
		}
	}

	const envName = "ROTARI_PROJECT_NAME"
	old, existed := os.LookupEnv(envName)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(envName, old)
		} else {
			_ = os.Unsetenv(envName)
		}
	})
	if err := os.Setenv(envName, "../outside"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ResolveProjectName(t.TempDir(), ""); err == nil {
		t.Fatal("state.ResolveProjectName accepted unsafe project name from environment")
	}
}

func TestResolvePathsRejectsAbsoluteAndNestedPathVariants(t *testing.T) {
	for _, projectName := range []string{"/tmp/outside", "///tmp/outside", "nested/../outside", "subdir/.", "subdir/..", "job/with/slash", "..\\outside", "nested\\project", "C:\\tmp\\outside"} {
		if _, err := state.ResolveProjectPaths(t.TempDir(), projectName); err == nil {
			t.Errorf("state.ResolveProjectPaths accepted unsafe project name %q", projectName)
		}
	}
}

func TestValidWebIDRejectsTraversalAndDotSegments(t *testing.T) {
	for _, value := range []string{"..", ".", "../outside", "nested/project", "nested\\project", "/tmp/outside", "C:\\tmp\\outside"} {
		if state.IsValidPathElement(value) {
			t.Fatalf("state.IsValidPathElement accepted unsafe value %q", value)
		}
	}
	if !state.IsValidPathElement("demo") || !state.IsValidPathElement("run-2026") || !state.IsValidPathElement("job-1") {
		t.Fatal("validWebID rejected a normal identifier")
	}
}

func TestValidatedStateDirectoriesRejectTraversal(t *testing.T) {
	paths, err := state.ResolveProjectPaths(t.TempDir(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"../outside", "nested/run"} {
		if _, err := validatedRunDir(paths, runID); err == nil {
			t.Errorf("validatedRunDir accepted unsafe run ID %q", runID)
		}
	}
	runDir, err := validatedRunDir(paths, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, jobID := range []string{"../outside", "nested/job"} {
		if _, err := validatedJobDir(runDir, jobID); err == nil {
			t.Errorf("validatedJobDir accepted unsafe job ID %q", jobID)
		}
	}
	jobDir, err := validatedJobDir(runDir, "job-1")
	if err != nil || jobDir != filepath.Join(runDir, "job-1") {
		t.Fatalf("validatedJobDir = %q, err %v", jobDir, err)
	}
}

func TestJoinValidatedPathRejectsTraversal(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "runs")
	got, err := state.SafeJoin(baseDir, "run-1")
	if err != nil {
		t.Fatalf("joinValidatedPath returned error for safe value: %v", err)
	}
	want := filepath.Join(baseDir, "run-1")
	if got != want {
		t.Fatalf("joinValidatedPath = %q, want %q", got, want)
	}
	if _, err := state.SafeJoin(baseDir, "../outside"); err == nil {
		t.Fatal("joinValidatedPath accepted traversal value")
	}
}

func TestResolveBaseDirPriority(t *testing.T) {
	const envName = "ROTARI_BASEDIR"
	old, existed := os.LookupEnv(envName)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(envName, old)
		} else {
			_ = os.Unsetenv(envName)
		}
	})

	_ = os.Unsetenv(envName)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	// 1. Fallback to default (home directory, etc.) if .rotari-state does not exist in current dir
	_, _, err = state.ResolveBaseDir("")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Prefer .rotari-state in current dir if it exists
	localState := filepath.Join(tempDir, ".rotari-state")
	if err := os.Mkdir(localState, 0755); err != nil {
		t.Fatal(err)
	}

	got, _, err := state.ResolveBaseDir("")
	if err != nil {
		t.Fatal(err)
	}
	if got != localState {
		t.Fatalf("current dir priority: got %q, want %q", got, localState)
	}

	// 3. ROTARI_BASEDIR environment variable priority
	if err := os.Setenv(envName, "/env/basedir"); err != nil {
		t.Fatal(err)
	}
	got, _, err = state.ResolveBaseDir("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/env/basedir" {
		t.Fatalf("env priority: got %q, want %q", got, "/env/basedir")
	}

	// 4. CLI option priority
	got, _, err = state.ResolveBaseDir("/cli/basedir")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/cli/basedir" {
		t.Fatalf("cli option priority: got %q, want %q", got, "/cli/basedir")
	}
}

func TestStateModeDefaultsToSharedPermissions(t *testing.T) {
	t.Setenv(envPrivateState, "")
	if got := state.DirectoryMode(); got != 0o755 {
		t.Fatalf("stateDirMode() = %o, want 0755 (shared by default)", got)
	}
	if got := state.FileMode(); got != 0o644 {
		t.Fatalf("stateFileMode() = %o, want 0644 (shared by default)", got)
	}
	if got := state.ScriptMode(); got != 0o755 {
		t.Fatalf("stateScriptMode() = %o, want 0755 (shared by default)", got)
	}

	t.Setenv(envPrivateState, "true")
	if got := state.DirectoryMode(); got != 0o700 {
		t.Fatalf("stateDirMode() with %s=true = %o, want 0700", envPrivateState, got)
	}
	if got := state.FileMode(); got != 0o600 {
		t.Fatalf("stateFileMode() with %s=true = %o, want 0600", envPrivateState, got)
	}
	if got := state.ScriptMode(); got != 0o700 {
		t.Fatalf("stateScriptMode() with %s=true = %o, want 0700", envPrivateState, got)
	}
}

func TestCLIStringUsesRotariEnvironmentDefaults(t *testing.T) {
	t.Setenv("ROTARI_RUN_ID", "run-from-env")
	t.Setenv("ROTARI_JOB_ID", "job-from-env")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	runID := cliString(fs, "run-id", "")
	jobID := cliString(fs, "job-id", "")
	if *runID != "run-from-env" || *jobID != "job-from-env" {
		t.Fatalf("defaults = %q, %q", *runID, *jobID)
	}
	if err := fs.Parse([]string{"--run-id", "run-from-flag"}); err != nil {
		t.Fatal(err)
	}
	if *runID != "run-from-flag" {
		t.Fatalf("flag value = %q, want explicit flag to override environment", *runID)
	}
}

func TestCLICommandSpecificEnvironmentDefaults(t *testing.T) {
	t.Setenv("ROTARI_WEB_HOST", "127.0.0.2")
	t.Setenv("ROTARI_WEB_PORT", "9000")
	t.Setenv(envWebAuthToken, "token-from-env")
	t.Setenv("ROTARI_WAIT_TIMEOUT", "2s")
	web := flag.NewFlagSet("web", flag.ContinueOnError)
	host := cliString(web, "host", "127.0.0.1")
	port := cliInt(web, "port", 8787)
	authToken := cliString(web, "auth-token", "")
	timeout := cliDuration(flag.NewFlagSet("wait", flag.ContinueOnError), "timeout", 0)
	if *host != "127.0.0.2" || *port != 9000 || *authToken != "token-from-env" || *timeout != 2*time.Second {
		t.Fatalf("defaults = %q, %d, %q, %s", *host, *port, *authToken, *timeout)
	}
}

func TestCLIHelpShowsEnvironmentDefaults(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var output strings.Builder
	fs.SetOutput(&output)
	cliString(fs, "basedir", "")
	cliBool(fs, "overwrite", false)
	cliInt(fs, "local-concurrency", 8)
	var executorOptions stringSliceFlag
	cliValue(fs, &executorOptions, "executor-option")

	fs.PrintDefaults()
	help := output.String()
	if !strings.Contains(help, "basedir") || !strings.Contains(help, "env: ROTARI_BASEDIR") {
		t.Fatalf("help does not show basedir environment variable: %q", help)
	}
	if strings.Contains(help, "--overwrite") && strings.Contains(help, "ROTARI_OVERWRITE") {
		t.Fatalf("help advertises unsupported overwrite environment variable: %q", help)
	}
	if !strings.Contains(help, "env: ROTARI_RUN_LOCAL_CONCURRENCY") {
		t.Fatalf("help does not show local concurrency environment variable: %q", help)
	}
	if !strings.Contains(help, "env: ROTARI_EXECUTOR_OPTIONS") {
		t.Fatalf("help does not show executor option environment variable: %q", help)
	}
}

func TestCLIUsageIncludesShortOptions(t *testing.T) {
	usage := cliUsage("add")
	for _, option := range []string{"[-b DIR|--basedir DIR]", "[-p NAME|--project-name NAME]", "[-e EXECUTOR|--executor EXECUTOR]"} {
		if !strings.Contains(usage, option) {
			t.Fatalf("usage %q does not contain %q", usage, option)
		}
	}
	if usage := cliUsage("check"); !strings.Contains(usage, "[--json]") {
		t.Fatalf("usage %q does not contain --json", usage)
	}
	if usage := cliUsage("check"); !strings.Contains(usage, "[--deep]") {
		t.Fatalf("usage %q does not contain --deep", usage)
	}
	for _, command := range []struct {
		name       string
		positional string
	}{
		{name: "delete", positional: "[RUN_ID]"},
		{name: "unlock", positional: "[PROJECT]"},
		{name: "copy", positional: "[RUN_ID]"},
		{name: "gc", positional: "[MASTERDIR]"},
		{name: "jobs", positional: "[PROJECT]"},
		{name: "check", positional: "[PROJECT]"},
		{name: "reset", positional: "[PROJECT]"},
		{name: "remove", positional: "[JOB_ID ...]"},
		{name: "diagnose", positional: "JOB_ID"},
	} {
		if usage := cliUsage(command.name); !strings.Contains(usage, command.positional) {
			t.Fatalf("usage %q does not contain positional %q", usage, command.positional)
		}
	}
}

func TestCLIStringRejectsValuesOutsideChoices(t *testing.T) {
	for _, args := range [][]string{{"--executor", "invalid"}, {"-e", "invalid"}} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		executor := cliString(fs, "executor", "")
		err := fs.Parse(args)
		if err == nil || !strings.Contains(err.Error(), `invalid choice "invalid" (choose from local, lsf, pbs, slurm, ssh)`) {
			t.Fatalf("Parse(%v) error = %v", args, err)
		}
		if *executor != "" {
			t.Fatalf("Parse(%v) executor = %q, want unchanged", args, *executor)
		}
	}
}

func TestCLIStringAcceptsValueFromChoices(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var output strings.Builder
	fs.SetOutput(&output)
	executor := cliString(fs, "executor", "")
	if err := fs.Parse([]string{"--executor", "slurm"}); err != nil {
		t.Fatal(err)
	}
	if *executor != "slurm" {
		t.Fatalf("executor = %q, want slurm", *executor)
	}
	fs.PrintDefaults()
	if !strings.Contains(output.String(), "choices: local, lsf, pbs, slurm, ssh") {
		t.Fatalf("help does not list choices: %q", output.String())
	}
}

func TestCLIStringAppliesChoicesFromFlagSpec(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "format", value: "xml"},
		{name: "provider", value: "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			cliString(fs, test.name, "")
			if err := fs.Parse([]string{"--" + test.name, test.value}); err == nil || !strings.Contains(err.Error(), "invalid choice") {
				t.Fatalf("Parse() error = %v", err)
			}
		})
	}
}

func TestCmdAddRejectsExecutorOutsideChoicesBeforeWritingQueue(t *testing.T) {
	baseDir := t.TempDir()
	code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "--executor", "invalid", "--", "true"})
	if code != 1 {
		t.Fatalf("cmdAdd exit code = %d, want 1", code)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.QueueFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("queue file exists after rejected choice: %v", err)
	}
}

func TestEnvironmentDefinitionsAreUniqueAndIncludeCoreVariables(t *testing.T) {
	definitions := environmentDefinitions()
	seen := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		if definition.Name == "" {
			t.Fatal("environment definition has an empty name")
		}
		if seen[definition.Name] {
			t.Fatalf("duplicate environment definition %q", definition.Name)
		}
		seen[definition.Name] = true
	}
	for _, name := range []string{envBaseDir, envRunID, envJobID, envExecutor, envRunRetry, envRunAsync, envQuiet, envArrayTaskID, envWebPort} {
		if !seen[name] {
			t.Errorf("missing environment definition %q", name)
		}
	}
	for flagName, envName := range cliEnvironmentVariables {
		if !seen[envName] {
			t.Errorf("CLI environment variable %q for --%s is undocumented", envName, flagName)
		}
	}
	for _, definition := range definitions {
		if !definition.CLIDefault {
			continue
		}
		mapped := false
		for _, envName := range cliEnvironmentVariables {
			if envName == definition.Name {
				mapped = true
				break
			}
		}
		if !mapped {
			t.Errorf("CLI default environment variable %q has no flag mapping", definition.Name)
		}
	}
}

func TestCmdEnvironmentListsCurrentValues(t *testing.T) {
	t.Setenv(envRunID, "run-from-env")
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdEnvironment(nil)
	_ = writer.Close()
	os.Stdout = oldStdout
	data, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if code != 0 || readErr != nil {
		t.Fatalf("cmdEnvironment = %d, read error = %v", code, readErr)
	}
	output := string(data)
	if !strings.Contains(output, "VARIABLE\tVALUE\tCLI\tJOB\tARRAY") || !strings.Contains(output, envRunID+"\trun-from-env") {
		t.Fatalf("environment output = %q", output)
	}
}

func TestMakeRunIDFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^\d{8}-\d{6}-[0-9a-f]{8}$`)
	first := makeRunID()
	if !pattern.MatchString(first) {
		t.Fatalf("makeRunID() = %q, want format YYYYMMDD-HHMMSS-XXXXXXXX", first)
	}
	if first == makeRunID() {
		t.Fatal("makeRunID returned the same ID twice")
	}
}

func TestFormatProjectRunningErrorIncludesWaitAndCancelHints(t *testing.T) {
	output := formatProjectRunningError(state.ProjectPaths{BaseDir: "/state", ProjectName: "demo"}, "run-1")
	for _, want := range []string{
		"project 'demo' is running",
		"Run: run-1",
		"rotari wait --basedir /state --project-name demo --run-id run-1",
		"rotari cancel --basedir /state --project-name demo",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("formatProjectRunningError() missing %q; got %q", want, output)
		}
	}
}

func TestCmdResetClearsQueueButKeepsDefaultsAndHistory(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{DefaultExecutor: "slurm", Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"echo", "queued"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	if code := cmdReset([]string{"--basedir", baseDir, "--project-name", "demo"}); code != 0 {
		t.Fatalf("cmdReset exit code = %d, want 0", code)
	}
	gotQueue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotQueue.Commands) != 0 || gotQueue.DefaultExecutor != "slurm" {
		t.Fatalf("queue after reset = %#v, want no commands and preserved defaults", gotQueue)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-1")); err != nil {
		t.Fatalf("run history was removed: %v", err)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" {
		t.Fatalf("metadata phase = %q, want collecting", meta.Phase)
	}
}

func TestCmdResetAcceptsPositionalProjectName(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"echo", "queued"}}}}); err != nil {
		t.Fatal(err)
	}

	if code := cmdReset([]string{"--basedir", baseDir, "demo"}); code != 0 {
		t.Fatalf("cmdReset positional project exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue commands after positional reset = %#v, want empty", queue.Commands)
	}
	if code := cmdReset([]string{"--basedir", baseDir, "--project-name", "demo", "other"}); code != 1 {
		t.Fatalf("cmdReset accepted positional project with --project-name: exit code = %d", code)
	}
}

func TestCmdResetClearsInvalidDuplicateNameQueue(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "old-prepare", Command: []string{"echo", "old"}, Name: "prepare"},
		{ID: "new-prepare", Command: []string{"echo", "new"}, Name: "prepare"},
	}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}

	if code := cmdReset([]string{"--basedir", baseDir, "--project-name", "demo"}); code != 0 {
		t.Fatalf("cmdReset exit code = %d, want 0", code)
	}
	gotQueue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotQueue.Commands) != 0 {
		t.Fatalf("queue commands = %#v, want empty", gotQueue.Commands)
	}
}

func TestCmdResetRejectsRunningProject(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "retained", Command: []string{"retained"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")
	if err := state.AcquireRunLock(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}

	code, output := captureResetStderr(t, []string{"--basedir", baseDir, "--project-name", "demo"})
	if code == 0 {
		t.Fatal("cmdReset accepted a running project")
	}
	for _, want := range []string{"project 'demo' is running", "rotari wait", "rotari cancel"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stderr = %q, want it to contain %q", output, want)
		}
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 {
		t.Fatalf("queue commands = %#v, want unchanged", queue.Commands)
	}
}

func TestCmdResetRecoversInterruptedRunWithoutPrompt(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "retained", Command: []string{"retained"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")

	if code := cmdReset([]string{"--basedir", baseDir, "--project-name", "demo", "--recover"}); code != 0 {
		t.Fatalf("cmdReset exit code = %d, want 0", code)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue commands = %#v, want empty", queue.Commands)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" {
		t.Fatalf("metadata phase = %q, want collecting", meta.Phase)
	}
}

func TestCmdResetRequiresRecoverFlagForInterruptedRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "retained", Command: []string{"retained"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	writeTestRunStateFiles(t, paths, "run-1")
	useNonTerminalStdin(t)

	code, output := captureResetStderr(t, []string{"--basedir", baseDir, "--project-name", "demo"})
	if code == 0 {
		t.Fatal("cmdReset recovered an interrupted run without confirmation")
	}
	if !strings.Contains(output, "reset requires confirmation") {
		t.Fatalf("stderr = %q, want non-interactive confirmation error", output)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 {
		t.Fatalf("queue commands = %#v, want unchanged", queue.Commands)
	}
}

func TestCmdResetRejectsUnexpectedStateWithoutDiscardingQueue(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, paths state.ProjectPaths)
	}{
		{
			name: "malformed queue",
			setup: func(t *testing.T, paths state.ProjectPaths) {
				t.Helper()
				if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths.QueueFile, []byte("not json\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "malformed metadata",
			setup: func(t *testing.T, paths state.ProjectPaths) {
				t.Helper()
				if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths.MetaFile, []byte("not json\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unknown metadata phase",
			setup: func(t *testing.T, paths state.ProjectPaths) {
				t.Helper()
				if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "retained", Command: []string{"echo", "retained"}}}}); err != nil {
					t.Fatal(err)
				}
				if err := writeJSON(paths.MetaFile, model.Meta{Phase: "unknown"}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "malformed run lock",
			setup: func(t *testing.T, paths state.ProjectPaths) {
				t.Helper()
				if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(paths.LockFile, []byte("not json\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "remote run lock",
			setup: func(t *testing.T, paths state.ProjectPaths) {
				t.Helper()
				if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "retained", Command: []string{"echo", "retained"}}}}); err != nil {
					t.Fatal(err)
				}
				if err := writeJSON(paths.LockFile, model.LockInfo{PID: -1, RunID: "run-1", Host: "other-host"}); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseDir := t.TempDir()
			paths, err := state.ResolveProjectPaths(baseDir, "demo")
			if err != nil {
				t.Fatal(err)
			}
			test.setup(t, paths)

			if code := cmdReset([]string{"--basedir", baseDir, "--project-name", "demo"}); code == 0 {
				t.Fatal("cmdReset accepted an unexpected project state")
			}
			if _, err := os.Stat(paths.QueueFile); err == nil {
				queue, loadErr := loadQueue(paths.QueueFile)
				if loadErr == nil && len(queue.Commands) == 0 {
					t.Fatal("cmdReset discarded queued commands")
				}
			}
		})
	}
}

func TestConfirmResetOfInterruptedRun(t *testing.T) {
	paths := state.ProjectPaths{ProjectName: "demo"}
	confirmed, err := confirmResetOfInterruptedRun(strings.NewReader("yes\n"), io.Discard, paths, "run-1")
	if err != nil || !confirmed {
		t.Fatalf("confirmResetOfInterruptedRun(yes) = %v, %v", confirmed, err)
	}
	confirmed, err = confirmResetOfInterruptedRun(strings.NewReader("no\n"), io.Discard, paths, "run-1")
	if err != nil || confirmed {
		t.Fatalf("confirmResetOfInterruptedRun(no) = %v, %v", confirmed, err)
	}
}

func TestRunStatus(t *testing.T) {
	if got := model.RunStatus(0); got != "finished" {
		t.Fatalf("runStatus(0) = %q, want finished", got)
	}
	if got := model.RunStatus(1); got != "failed" {
		t.Fatalf("runStatus(1) = %q, want failed", got)
	}
}

func TestParseArrayRange(t *testing.T) {
	got, err := model.ParseArrayRange("2-4")
	if err != nil || got.First != 2 || got.Last != 4 || len(got.Tasks) != 0 {
		t.Fatalf("parseArrayRange = %#v, %v", got, err)
	}
	got, err = model.ParseArrayRange("1,3,4")
	if err != nil || !reflect.DeepEqual(got, model.ArraySpec{First: 1, Last: 4, Tasks: []int{1, 3, 4}}) {
		t.Fatalf("parseArrayRange sparse = %#v, %v", got, err)
	}
	for _, value := range []string{"", "4-2", "one-2", "1,,3", "1,3,3"} {
		if _, err := model.ParseArrayRange(value); err == nil {
			t.Errorf("parseArrayRange(%q) returned nil error", value)
		}
	}
}

func TestQueueToJobsExpandsSparseArray(t *testing.T) {
	jobs := model.QueueToJobs([]model.QueuedCommand{{
		ID: "array", Command: []string{"echo", "hello"}, Array: &model.ArraySpec{First: 1, Last: 4, Tasks: []int{1, 3, 4}},
	}})
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}
	for index, wantTask := range []int{1, 3, 4} {
		if jobs[index].ID != fmt.Sprintf("array-%d", wantTask) || *jobs[index].ArrayTaskID != wantTask || jobs[index].ArraySize != 3 {
			t.Fatalf("job %d = %#v, want task %d with size 3", index, jobs[index], wantTask)
		}
	}
}

func TestSparseArrayUsesIndividualSubmissions(t *testing.T) {
	jobs := model.QueueToJobs([]model.QueuedCommand{{
		ID: "array", Command: []string{"echo", "hello"}, Array: &model.ArraySpec{First: 1, Last: 4, Tasks: []int{1, 3, 4}},
	}})
	if runcontract.CompleteArrayGroup(jobs, 1, 4) {
		t.Fatal("sparse array must not use a native contiguous scheduler array")
	}
}

func TestMergeEnvironmentOverridesValues(t *testing.T) {
	got := executor.MergeEnvironment([]string{"PATH=/bin", "ROTARI_JOB_ID=old"}, []string{"ROTARI_JOB_ID=new", "ROTARI_TASK=value"})
	want := []string{"PATH=/bin", "ROTARI_JOB_ID=new", "ROTARI_TASK=value"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("mergeEnvironment = %#v, want %#v", got, want)
	}
}

func TestPrepareJobEnvironmentsIncludesRunOptions(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	runID := "run-1"
	if err := writeRunContext(paths, runID, "/work"); err != nil {
		t.Fatal(err)
	}
	jobs := []model.JobSpec{{ID: "job-1", Executor: "slurm"}}
	prepareJobEnvironments(paths, runID, jobs, "nightly", 2, 3, 4, []string{"-p short"})
	values := make(map[string]string)
	for _, entry := range jobs[0].Environment {
		parts := strings.SplitN(entry, "=", 2)
		values[parts[0]] = parts[1]
	}
	for name, want := range map[string]string{
		envRunName: "nightly", envRunLocalConc: "2", envRunBatchConc: "3", envRunRetry: "4", envExecutorOpts: "-p short",
	} {
		if values[name] != want {
			t.Errorf("%s = %q, want %q", name, values[name], want)
		}
	}
}

func TestAssignAttemptIDsAreUniquePerRunAttempt(t *testing.T) {
	jobs := []model.JobSpec{{ID: "job-1"}, {ID: "job-2"}}
	runID := makeRunID()
	assignAttemptIDs(jobs, runID, 0)
	firstAttempt := []string{jobs[0].AttemptID, jobs[1].AttemptID}
	for index, job := range jobs {
		if job.AttemptID == "" {
			t.Fatalf("job %d has no attempt ID", index)
		}
		if got, want := runcontract.EnvironmentEntry(job.Environment, envAttemptID); got != envAttemptID+"="+job.AttemptID || !want {
			t.Fatalf("job %d attempt environment = %q, want %q", index, got, envAttemptID+"="+job.AttemptID)
		}
		payload, err := decodeAttemptID(job.AttemptID)
		if err != nil {
			t.Fatalf("decode attempt ID %q: %v", job.AttemptID, err)
		}
		if payload.RunID != runID || payload.JobID != job.ID || payload.Number != 0 {
			t.Fatalf("attempt payload = %#v", payload)
		}
	}
	if firstAttempt[0] == firstAttempt[1] {
		t.Fatalf("attempt IDs are not unique: %#v", firstAttempt)
	}

	assignAttemptIDs(jobs, runID, 1)
	for index, job := range jobs {
		if job.AttemptID == firstAttempt[index] {
			t.Fatalf("job %d attempt ID did not change on retry: %q", index, job.AttemptID)
		}
	}
}

func TestLatestAttemptJobDirUsesAttemptDirectories(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "diagnosed-run")
	job := model.JobSpec{ID: "job-1", AttemptID: makeAttemptID("diagnosed-run", "job-1", 1)}
	jobDir, err := state.AttemptJobDir(runDir, model.JobSpec(job))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(runDir, "job-1", "attempts", job.AttemptID)
	if jobDir != want {
		t.Fatalf("attempt job dir = %q, want %q", jobDir, want)
	}
	if err := os.MkdirAll(jobDir, 0o700); err != nil {
		t.Fatal(err)
	}
	latest, err := latestAttemptJobDir(runDir, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest != want {
		t.Fatalf("latest attempt dir = %q, want %q", latest, want)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(jobDir)), "latest_attempt")); !os.IsNotExist(err) {
		t.Fatalf("latest attempt marker exists: %v", err)
	}
}

func TestLatestAttemptIDIgnoresOtherRunsAndJobs(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "current-run")
	jobDir, err := validatedJobDir(runDir, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	attemptIDs := []string{
		makeAttemptID("current-run", "job-1", 1),
		makeAttemptID("other-run", "job-1", 99),
		makeAttemptID("current-run", "other-job", 100),
	}
	for _, attemptID := range attemptIDs {
		if err := os.MkdirAll(filepath.Join(jobDir, "attempts", attemptID), 0o700); err != nil {
			t.Fatal(err)
		}
	}

	got, err := latestAttemptID(runDir, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	want := attemptIDs[0]
	if got != want {
		t.Fatalf("latest attempt ID = %q, want %q", got, want)
	}
}

func TestEnqueueCommandPersistsStableJobID(t *testing.T) {
	baseDir := t.TempDir()
	message, err := enqueueCommand(baseDir, "default", []string{"echo", "old"}, "", nil, nil, "job", nil)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 1 || queue.Commands[0].ID == "" {
		t.Fatalf("queue command ID = %q, want a persisted ID", queue.Commands[0].ID)
	}
	id := queue.Commands[0].ID
	if !strings.Contains(message, "added project=default job_id="+id+" job_name=job command=[echo old]") {
		t.Fatalf("enqueue message = %q, want job metadata and command", message)
	}
	queue.Commands[0].Command = []string{"echo", "new"}
	jobs := model.QueueToJobs(queue.Commands)
	if len(jobs) != 1 || jobs[0].ID != id {
		t.Fatalf("changed command ID = %q, want %q", jobs[0].ID, id)
	}
}

func TestEnqueueCommandKeepsPerJobExecutorOutOfQueueDefault(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "job"}, "slurm", []string{"-p short"}, nil, "job", nil); err != nil {
		t.Fatal(err)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if queue.DefaultExecutor != "" || len(queue.Commands) != 1 || queue.Commands[0].Executor != "slurm" {
		t.Fatalf("queue = %#v, want only the job to use slurm", queue)
	}
}

func TestEnqueueCommandPersistsEnvironment(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "job"}, "", nil, []string{"TOKEN=secret", "MODE=test"}, "job", nil); err != nil {
		t.Fatal(err)
	}
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(queue.Commands[0].Environment, "\x00"); got != "TOKEN=secret\x00MODE=test" {
		t.Fatalf("environment = %q", got)
	}
	jobs := model.QueueToJobs(queue.Commands)
	prepareJobEnvironments(paths, "run-1", jobs, "", 1, 1, 0, nil)
	values := make(map[string]string)
	for _, entry := range jobs[0].Environment {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	if values["TOKEN"] != "secret" || values["MODE"] != "test" {
		t.Fatalf("job environment = %#v", values)
	}
}

func TestValidateEnvironment(t *testing.T) {
	if err := model.ValidateEnvironment([]string{"KEY=value", "EMPTY=", "_PRIVATE=yes", "VALUE_2=ok"}); err != nil {
		t.Fatal(err)
	}
	for _, environment := range [][]string{{"not-an-assignment"}, {"9KEY=value"}, {"BAD-NAME=value"}, {"KEY=value\x00tail"}} {
		if err := model.ValidateEnvironment(environment); err == nil {
			t.Errorf("validateEnvironment(%q) returned nil error", environment)
		}
	}
}

func TestEnqueueCommandKeepsFinishedRunHistory(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	if _, err := enqueueCommand(baseDir, "default", []string{"echo", "new"}, "", nil, nil, "new-job", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-1", "summary.json")); err != nil {
		t.Fatalf("finished run history was removed: %v", err)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "collecting" || meta.LastRunID != "run-1" {
		t.Fatalf("meta = %#v, want collecting with run-1 history", meta)
	}
}

func TestAppendCompletionBlockIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bashrc")
	if err := appendCompletionBlock(path, "bash", "eval completion"); err != nil {
		t.Fatal(err)
	}
	if err := appendCompletionBlock(path, "bash", "eval completion"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(data), "# rotari completion (bash)"); count != 1 {
		t.Fatalf("completion marker count = %d, want 1", count)
	}
}

func TestZshArgumentsIncludeValueNames(t *testing.T) {
	got := zshArguments([]cliFlagSpec{{Name: "basedir", Description: "state directory", ValueName: "DIR"}})
	if got != "{-b,--basedir}'[state directory]:DIR:'" {
		t.Fatalf("zsh argument = %q, want value name in specification", got)
	}
}

func TestCLIShortOptions(t *testing.T) {
	fs := flag.NewFlagSet("short-options", flag.ContinueOnError)
	baseDir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	executor := cliString(fs, "executor", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")

	if err := fs.Parse([]string{"-b", "/state", "-p", "build", "-r", "run-1", "-j", "job-1", "-j", "job-2", "-e", "local"}); err != nil {
		t.Fatal(err)
	}
	if *baseDir != "/state" || *projectName != "build" || *runID != "run-1" || *executor != "local" {
		t.Fatalf("short option values = %q, %q, %q, %q", *baseDir, *projectName, *runID, *executor)
	}
	if strings.Join(jobIDs, ",") != "job-1,job-2" {
		t.Fatalf("short job IDs = %q", jobIDs)
	}
}

func TestCompletionScriptsContainCommandOptions(t *testing.T) {
	for _, option := range []string{"--basedir", "-b", "--project-name", "-p", "--run-id", "-r", "--job-id", "-j", "--executor", "-e", "--failed-logs", "--no-pager", "--job-name"} {
		if !strings.Contains(generateBashCompletion(), option) {
			t.Errorf("Bash completion does not contain %s", option)
		}
		if !strings.Contains(generateZshCompletion(), option) {
			t.Errorf("Zsh completion does not contain %s", option)
		}
	}
	for _, option := range []string{"basedir", "project-name", "run-id", "job-id", "executor", "failed-logs", "no-pager", "job-name"} {
		if !strings.Contains(generateFishCompletion(), option) {
			t.Errorf("Fish completion does not contain %s", option)
		}
	}
	if !strings.Contains(generateBashCompletion(), "bash zsh fish install") {
		t.Error("Bash completion does not contain the install subcommand")
	}
	if !strings.Contains(generateZshCompletion(), "'install:install completion") {
		t.Error("Zsh completion does not contain the install subcommand")
	}
	if !strings.Contains(generateFishCompletion(), "install") {
		t.Error("Fish completion does not contain the install subcommand")
	}
	if !strings.Contains(generateZshCompletion(), "compdef _rotari rotari") {
		t.Error("Zsh completion does not register rotari")
	}
	if !strings.Contains(generateFishCompletion(), "complete -c rotari") {
		t.Error("Fish completion does not register rotari")
	}
	if !strings.Contains(generateBashCompletion(), "__complete project-name") {
		t.Error("Bash completion does not dynamically complete project names")
	}
	if !strings.Contains(generateZshCompletion(), "_rotari_project_names") {
		t.Error("Zsh completion does not dynamically complete project names")
	}
	if !strings.Contains(generateFishCompletion(), "__rotari_complete project-name") {
		t.Error("Fish completion does not dynamically complete project names")
	}
}

func TestGenerateZshCompletionIsValidSyntax(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	script := generateZshCompletion()
	scriptPath := filepath.Join(t.TempDir(), "_rotari")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(zshPath, "-n", scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("generated zsh completion has a syntax error: %v\n%s", err, out)
	}
}

func TestGenerateBashCompletionIsValidSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	script := generateBashCompletion()
	scriptPath := filepath.Join(t.TempDir(), "rotari-completion.bash")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bashPath, "-n", scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("generated bash completion has a syntax error: %v\n%s", err, out)
	}
}

func TestGenerateFishCompletionIsValidSyntax(t *testing.T) {
	fishPath, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not installed")
	}
	script := generateFishCompletion()
	scriptPath := filepath.Join(t.TempDir(), "rotari-completion.fish")
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(fishPath, "-n", scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("generated fish completion has a syntax error: %v\n%s", err, out)
	}
}

func quoteForShell(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func TestBashCompletionIntegration(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	scriptPath := filepath.Join(t.TempDir(), "rotari-completion.bash")
	if err := os.WriteFile(scriptPath, []byte(generateBashCompletion()), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bashPath, "-ic", fmt.Sprintf("source %q; COMP_WORDS=(rotari); COMP_CWORD=1; _rotari_completion; printf '%%s\\n' \"${COMPREPLY[*]}\"", scriptPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash completion did not run: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "config") || !strings.Contains(string(out), "show") {
		t.Fatalf("bash completion output = %q, want command candidates", out)
	}
}

func TestZshCompletionIntegration(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not installed")
	}
	scriptPath := filepath.Join(t.TempDir(), "_rotari")
	if err := os.WriteFile(scriptPath, []byte(generateZshCompletion()), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(zshPath, "-c", fmt.Sprintf("autoload -Uz compinit && compinit; source %s; whence -w _rotari", quoteForShell(scriptPath)))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zsh completion did not register: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "_rotari") {
		t.Fatalf("zsh registration output = %q, want _rotari function", out)
	}
}

func TestFishCompletionIntegration(t *testing.T) {
	fishPath, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not installed")
	}
	scriptPath := filepath.Join(t.TempDir(), "rotari-completion.fish")
	if err := os.WriteFile(scriptPath, []byte(generateFishCompletion()), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(fishPath, "-C", fmt.Sprintf("source %q; complete -C 'rotari '", scriptPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fish completion did not run: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "config") || !strings.Contains(string(out), "show") {
		t.Fatalf("fish completion output = %q, want command candidates", out)
	}
}

func TestCompleteProjectNames(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "z-last"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "a-first"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "projects", "not-a-project"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdComplete([]string{"project-name", "--basedir", baseDir})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdComplete exit code = %d, want 0", code)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "a-first\nz-last\n" {
		t.Fatalf("project-name completion = %q, want sorted project names", output)
	}
}

func TestCompleteJobIDsForRun(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runID := "completion-run-1"
	for _, jobID := range []string{"job-b", "job-a"} {
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, runID, jobID), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "other-run", "job-other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := registerRun(paths, runID); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdComplete([]string{"job-id", "--run-id", runID})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdComplete exit code = %d, want 0", code)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "job-a\njob-b\n" {
		t.Fatalf("job-id completion = %q, want only jobs from %s", output, runID)
	}
}

func TestShouldFollowLogs(t *testing.T) {
	if !shouldFollowLogs(true, true, true) {
		t.Fatal("explicit follow should be enabled")
	}
	if !shouldFollowLogs(false, true, true) {
		t.Fatal("single-job auto follow should activate for running jobs on TTY")
	}
	if shouldFollowLogs(false, false, true) {
		t.Fatal("auto follow should be disabled when the job is no longer running")
	}
	if shouldFollowLogs(false, true, false) {
		t.Fatal("auto follow should be disabled for non-TTY output")
	}
}

func TestShowWithPagerDisabledWritesDirectly(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = oldStdout }()

	if code := showWithPager(false, func(writer io.Writer) int {
		_, _ = fmt.Fprint(writer, "log output")
		return 0
	}); code != 0 {
		t.Fatalf("showWithPager exit code = %d, want 0", code)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "log output" {
		t.Fatalf("output = %q, want log output", output)
	}
}

func TestShowRunIncludesCarriedJobFromCommands(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-2")
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "carried", Name: "carried-job", Command: []string{"echo", "done"},
		Origin: &model.JobOrigin{RunID: "run-1", JobID: "carried", Status: "success"},
	}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "summary.json"), model.RunSummary{RunID: "run-2", Status: "finished", Results: []model.JobResult{{ID: "carried", ExitCode: 0}}}); err != nil {
		t.Fatal(err)
	}
	sourceJobDir := filepath.Join(paths.RunsDir, "run-1", "carried")
	if err := os.MkdirAll(sourceJobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceJobDir, "submitted_at"), []byte("2026-09-16T00:00:01Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceJobDir, "finished_at"), []byte("2026-09-16T00:00:02Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := showRun(paths, "run-2", showJobFilter{})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("showRun exit code = %d, want 0", code)
	}
	for _, want := range []string{"carried", "carried-job", "2026-09-16 09:00:01 JST", "2026-09-16 09:00:02 JST", "echo done"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("showRun output does not contain %q:\n%s", want, output)
		}
	}
	if !strings.Contains(string(output), "rotari delete --run-id run-2") {
		t.Fatalf("showRun output does not contain short delete command:\n%s", output)
	}
	if strings.Contains(string(output), "rotari delete --basedir") {
		t.Fatalf("showRun output contains verbose delete command:\n%s", output)
	}
}

func TestPrintChangeHintsUsesRetryLabel(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	printChangeHints(state.ProjectPaths{BaseDir: "/tmp/rotari", ProjectName: "demo"}, "run-1", model.Queue{}, []model.JobSpec{{ID: "job-1"}})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	if !strings.Contains(text, "Retry:") || strings.Contains(text, "Rerun:") {
		t.Fatalf("change hints have incorrect retry label:\n%s", text)
	}
}

func TestExceedsPagerLineLimit(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		output   string
		newlines int
		want     bool
	}{
		{name: "24 complete lines", output: strings.Repeat("line\n", pagerLineLimit), newlines: pagerLineLimit},
		{name: "25 complete lines", output: strings.Repeat("line\n", pagerLineLimit+1), newlines: pagerLineLimit + 1, want: true},
		{name: "25th partial line", output: strings.Repeat("line\n", pagerLineLimit) + "line", newlines: pagerLineLimit, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := exceedsPagerLineLimit([]byte(testCase.output), testCase.newlines); got != testCase.want {
				t.Fatalf("exceedsPagerLineLimit() = %t, want %t", got, testCase.want)
			}
		})
	}
}

func TestInstallCompletionForBash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := installCompletion("bash"); err != nil {
		t.Fatal(err)
	}
	if err := installCompletion("bash"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Count(content, "# rotari completion (bash)") != 1 {
		t.Fatal("Bash completion block was installed more than once")
	}
	if !strings.Contains(content, `eval "$(rotari completion bash)"`) {
		t.Fatal("Bash completion command is missing")
	}
}

func TestInstallCompletionForZsh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := installCompletion("zsh"); err != nil {
		t.Fatal(err)
	}
	if err := installCompletion("zsh"); err != nil {
		t.Fatal(err)
	}

	completion, err := os.ReadFile(filepath.Join(home, ".zfunc", "_rotari"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(completion), "#compdef rotari") {
		t.Fatal("Zsh completion header is missing")
	}
	if err := os.WriteFile(filepath.Join(home, ".zfunc", "_rotari"), []byte("stale zsh completion"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installCompletion("zsh"); err != nil {
		t.Fatal(err)
	}
	completion, err = os.ReadFile(filepath.Join(home, ".zfunc", "_rotari"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(completion), "#compdef rotari") {
		t.Fatal("Zsh completion was not refreshed after stale install")
	}
	rc, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(rc), "# rotari completion (zsh)") != 1 {
		t.Fatal("Zsh completion block was installed more than once")
	}
	if !strings.Contains(string(rc), "autoload -Uz _rotari && compdef _rotari rotari") {
		t.Fatal("Zsh completion function was not registered")
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("# rotari completion (zsh)\nfpath=(\"$HOME/.zfunc\" $fpath)\nautoload -Uz compinit && compinit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installCompletion("zsh"); err != nil {
		t.Fatal(err)
	}
	rc, err = os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rc), "autoload -Uz _rotari && compdef _rotari rotari") {
		t.Fatal("Existing Zsh completion block was not upgraded")
	}
}

func TestInstallCompletionForFish(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := installCompletion("fish"); err != nil {
		t.Fatal(err)
	}
	if err := installCompletion("fish"); err != nil {
		t.Fatal(err)
	}

	completionPath := filepath.Join(home, ".config", "fish", "completions", "rotari.fish")
	data, err := os.ReadFile(completionPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "complete -c rotari") {
		t.Fatal("Fish completion script is missing")
	}
	if strings.Count(string(data), "# rotari completion (fish)") != 1 {
		t.Fatal("Fish completion block was installed more than once")
	}
}

func TestPlanRerunSelectionWithoutPreviousRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.MetaFile), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = planRerunSelection(paths, model.Queue{Commands: []model.QueuedCommand{{ID: "alpha"}}}, "failed", nil, "", true)
	if !errors.Is(err, errNoPreviousRun) {
		t.Fatalf("error = %v, want errNoPreviousRun", err)
	}
}

func TestPlanRerunSelectionCarriesForwardNonMatchingResults(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "alpha", Command: []string{"echo", "alpha"}, Name: "alpha"},
		{ID: "beta", Command: []string{"echo", "beta"}, Name: "beta"},
		{ID: "gamma", Command: []string{"echo", "gamma"}, Name: "gamma"},
		{ID: "delta", Command: []string{"echo", "delta"}, Name: "delta"},
	}}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{
		RunID: "run-1",
		Results: []model.JobResult{
			{ID: "alpha", ExitCode: 0},
			{ID: "beta", ExitCode: 1},
			{ID: "gamma", ExitCode: 0},
		},
	}); err != nil {
		t.Fatal(err)
	}
	alphaDir := filepath.Join(paths.RunsDir, "run-1", "alpha")
	if err := os.MkdirAll(alphaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alphaDir, "submitted_at"), []byte("2026-09-16T00:00:01Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alphaDir, "finished_at"), []byte("2026-09-16T00:00:02Z\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := defaultMeta()
	meta.LastRunID = "run-1"
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	plan, err := planRerunSelection(paths, queue, "failed", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["beta"] || plan.Execute["alpha"] || plan.Execute["gamma"] || plan.Execute["delta"] {
		t.Fatalf("execute set = %#v, want only beta", plan.Execute)
	}
	if _, ok := plan.CarriedResults["alpha"]; !ok {
		t.Fatal("expected alpha to be carried forward")
	}
	if _, ok := plan.CarriedResults["gamma"]; !ok {
		t.Fatal("expected gamma to be carried forward")
	}
	if _, ok := plan.CarriedResults["delta"]; ok {
		t.Fatal("delta has no previous result and must not be carried forward")
	}
	origin := plan.CarriedOrigins["alpha"]
	if origin == nil || origin.RunID != "run-1" || origin.JobID != "alpha" || origin.Status != "success" || origin.SubmittedAt != "2026-09-16T00:00:01Z" || origin.FinishedAt != "2026-09-16T00:00:02Z" {
		t.Fatalf("origin = %#v, want run-1/alpha success with timestamps", origin)
	}
}

func TestPlanRerunSelectionAggregatesArrayTaskResults(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "array", Command: []string{"echo", "array"}, Name: "array", Array: &model.ArraySpec{First: 1, Last: 2}},
	}}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{
		RunID: "run-1",
		Results: []model.JobResult{
			{ID: "array-1", ExitCode: 0},
			{ID: "array-2", ExitCode: 0},
		},
	}); err != nil {
		t.Fatal(err)
	}
	meta := defaultMeta()
	meta.LastRunID = "run-1"
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	// A fully successful array job must not be re-executed by --unfinished,
	// with or without partialArray (both tasks already match "finished").
	plan, err := planRerunSelection(paths, queue, "unfinished", nil, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array"] {
		t.Fatalf("execute set = %#v, want array carried forward, not re-executed", plan.Execute)
	}
	if _, ok := plan.CarriedResults["array-1"]; !ok {
		t.Fatal("expected array-1 to be carried forward")
	}
	if _, ok := plan.CarriedResults["array-2"]; !ok {
		t.Fatal("expected array-2 to be carried forward")
	}
	if origin := plan.CarriedOrigins["array"]; origin == nil || origin.Status != "success" {
		t.Fatalf("origin = %#v, want status=success", origin)
	}

	// With partialArray disabled, a single failed task still re-executes
	// the whole array as one unit (the pre-partial-array behavior).
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{
		RunID: "run-1",
		Results: []model.JobResult{
			{ID: "array-1", ExitCode: 0},
			{ID: "array-2", ExitCode: 1},
		},
	}); err != nil {
		t.Fatal(err)
	}
	plan, err = planRerunSelection(paths, queue, "failed", nil, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["array"] {
		t.Fatal("expected array with a failed task to be re-executed as a whole when partialArray is false")
	}
}

func TestPlanRerunSelectionPartialArrayReexecutesOnlyFailedTasks(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "array", Command: []string{"echo", "array"}, Name: "array", Array: &model.ArraySpec{First: 1, Last: 2}},
	}}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{
		RunID: "run-1",
		Results: []model.JobResult{
			{ID: "array-1", ExitCode: 0},
			{ID: "array-2", ExitCode: 1},
		},
	}); err != nil {
		t.Fatal(err)
	}
	meta := defaultMeta()
	meta.LastRunID = "run-1"
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	// partialArray=true (the default): only the failed task re-executes,
	// the successful one carries its previous result forward instead.
	plan, err := planRerunSelection(paths, queue, "failed", nil, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute["array"] {
		t.Fatal("did not expect the whole array command ID to be marked for execution")
	}
	if plan.Execute["array-1"] {
		t.Fatal("array-1 already succeeded and must not be re-executed")
	}
	if !plan.Execute["array-2"] {
		t.Fatal("array-2 failed and must be re-executed")
	}
	if result, ok := plan.CarriedResults["array-1"]; !ok || result.ExitCode != 0 {
		t.Fatalf("array-1 carried result = %#v, want carried success", result)
	}
	if _, ok := plan.CarriedResults["array-2"]; ok {
		t.Fatal("array-2 is being re-executed and must not also be carried forward")
	}
}

func TestDeleteRemovesOnlySelectedRun(t *testing.T) {
	t.Setenv("ROTARI_MASTERDIR", t.TempDir())
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.RunsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, runID := range []string{"run-1", "run-2"} {
		if err := os.Mkdir(filepath.Join(paths.RunsDir, runID), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := registerRun(paths, runID); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-2", LastRunExitCode: 1}); err != nil {
		t.Fatal(err)
	}

	if code := cmdDelete([]string{"--basedir", baseDir, "run-2"}); code != 0 {
		t.Fatalf("cmdDelete exit = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-1")); err != nil {
		t.Fatalf("run-1 was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-2")); !os.IsNotExist(err) {
		t.Fatalf("run-2 still exists, stat error = %v", err)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastRunID != "run-1" || meta.LastRunExitCode != 1 || meta.Phase != "collecting" {
		t.Fatalf("metadata = %#v, want latest remaining run-1", meta)
	}
	if _, found, err := resolveRunLocation("run-2"); err != nil || found {
		t.Fatalf("deleted run registry entry: found=%v, err=%v; want removed", found, err)
	}
	if _, found, err := resolveRunLocation("run-1"); err != nil || !found {
		t.Fatalf("remaining run registry entry: found=%v, err=%v; want retained", found, err)
	}
}

func TestClearCommandIsRejected(t *testing.T) {
	if code := run([]string{"clear", "--basedir", t.TempDir()}); code != 1 {
		t.Fatalf("run clear exit = %d, want 1", code)
	}
}

func TestPlanRerunSelectionByJobID(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare", Command: []string{"echo", "prepare"}, Name: "prepare"},
		{ID: "alpha", Command: []string{"echo", "alpha"}, Name: "alpha", DependsOn: []string{"prepare"}},
		{ID: "beta", Command: []string{"echo", "beta"}, Name: "beta"},
	}}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{RunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	meta := defaultMeta()
	meta.LastRunID = "run-1"
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	plan, err := planRerunSelection(paths, queue, "job-id", []string{"beta", "alpha"}, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Execute["alpha"] || !plan.Execute["beta"] || plan.Execute["prepare"] {
		t.Fatalf("execute set = %#v, want alpha and beta only", plan.Execute)
	}
	if len(plan.CarriedResults) != 0 {
		t.Fatalf("carried results = %#v, want none (prepare has no previous result)", plan.CarriedResults)
	}
}

func TestCompareQueueWithRun(t *testing.T) {
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "queue.json")
	runPath := filepath.Join(dir, "commands.json")
	if err := writeJSON(queuePath, model.Queue{Commands: []model.QueuedCommand{
		{ID: "same", Command: []string{"echo", "same"}, Name: "same"},
		{ID: "changed", Command: []string{"echo", "changed"}, Name: "new-name"},
		{ID: "added", Command: []string{"echo", "added"}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(runPath, model.Queue{Commands: []model.QueuedCommand{
		{ID: "same", Command: []string{"echo", "same"}, Name: "same"},
		{ID: "changed", Command: []string{"echo", "changed"}, Name: "old-name"},
		{ID: "removed", Command: []string{"echo", "removed"}},
	}}); err != nil {
		t.Fatal(err)
	}

	diff, err := compareQueueWithRun(queuePath, runPath)
	if err != nil {
		t.Fatal(err)
	}
	if diff.Added != 1 || diff.Removed != 1 || diff.Changed != 1 {
		t.Fatalf("diff = %#v, want added=1 removed=1 changed=1", diff)
	}
}

func TestResolveQueueExecutorUsesDefaultExecutor(t *testing.T) {
	baseDir := t.TempDir()
	queueDir := filepath.Join(baseDir, "projects", "default")
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(queueDir, "queue.json"), model.Queue{
		DefaultExecutor: "slurm",
		Commands:        []model.QueuedCommand{{ID: "hello", Command: []string{"echo", "hello"}}},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := resolveQueueExecutor(baseDir, "default", "")
	if err != nil {
		t.Fatalf("resolveQueueExecutor returned error: %v", err)
	}
	if got != "slurm" {
		t.Fatalf("resolved executor = %q, want slurm", got)
	}

	if _, err := resolveQueueExecutor(baseDir, "default", "invalid"); err == nil {
		t.Fatal("resolveQueueExecutor accepted unsupported executor")
	}
}

func TestCmdWaitRejectsNegativeTimeout(t *testing.T) {
	if code := cmdWait([]string{"--run-id", "run-1", "--timeout", "-1s"}); code != 1 {
		t.Fatalf("cmdWait exit = %d, want 1", code)
	}
}

func TestCmdCancelRejectsWaitWithJobID(t *testing.T) {
	if code := cmdCancel([]string{"--basedir", t.TempDir(), "--job-id", "job-1", "--wait"}); code != 1 {
		t.Fatalf("cmdCancel exit = %d, want 1", code)
	}
}

func TestCmdCancelRejectsPositionalCombinedWithJobIDFlag(t *testing.T) {
	if code := cmdCancel([]string{"--basedir", t.TempDir(), "--job-id", "job-1", "job-2"}); code != 1 {
		t.Fatalf("cmdCancel exit = %d, want 1", code)
	}
}

func TestCmdJobSignalRejectsPositionalCombinedWithJobIDFlag(t *testing.T) {
	if code := cmdJobSignal([]string{"--basedir", t.TempDir(), "--job-id", "job-1", "job-2"}, "suspend"); code != 1 {
		t.Fatalf("cmdJobSignal exit = %d, want 1", code)
	}
}

func TestCmdCancelRejectsWaitWithPositionalJobID(t *testing.T) {
	if code := cmdCancel([]string{"--basedir", t.TempDir(), "--wait", "job-1"}); code != 1 {
		t.Fatalf("cmdCancel exit = %d, want 1", code)
	}
}

func TestFollowJobLogReadsAppendedOutputUntilFinished(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(paths.RunsDir, "run-1", "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(jobDir, "output")
	if err := os.WriteFile(outputPath, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = os.WriteFile(outputPath, []byte("first\nsecond\n"), 0o644)
		_ = os.WriteFile(filepath.Join(jobDir, "status"), []byte("0\n"), 0o644)
	}()

	var output bytes.Buffer
	if code := followJobLog(&output, paths, "run-1", "job-1"); code != 0 {
		t.Fatalf("followJobLog exit = %d, want 0", code)
	}
	if got := output.String(); got != "first\nsecond\n" {
		t.Fatalf("followed output = %q, want appended log", got)
	}
}

func TestFinishRunClearsQueueAndKeepsRunHistory(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{DefaultExecutor: "slurm", Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"echo", "queued"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, defaultMeta()); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{RunID: "run-1", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}

	if err := finishRun(paths, "run-1", 1); err != nil {
		t.Fatal(err)
	}

	gotQueue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotQueue.Commands) != 0 {
		t.Fatalf("queue commands = %d, want 0", len(gotQueue.Commands))
	}
	if gotQueue.DefaultExecutor != "slurm" {
		t.Fatalf("queue default executor = %q, want slurm", gotQueue.DefaultExecutor)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, "run-1", "commands.json")); err != nil {
		t.Fatalf("run snapshot was removed: %v", err)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "finished" || meta.LastRunID != "run-1" || meta.LastRunExitCode != 1 {
		t.Fatalf("metadata = %#v, want finished run-1 exit 1", meta)
	}
}

func TestFinishRunDoesNotFinalizeAnotherRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "queued", Command: []string{"echo", "queued"}}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: "run-2"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{RunID: "run-2", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}

	if err := finishRun(paths, "run-1", 1); err == nil {
		t.Fatal("finishRun finalized a different run")
	}

	gotQueue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotQueue.Commands) != 1 {
		t.Fatalf("queue commands = %d, want 1", len(gotQueue.Commands))
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastRunID != "run-2" || meta.Phase != "running" {
		t.Fatalf("metadata = %#v, want active run-2", meta)
	}
}

func TestChangeBatchRestoresAndEditsPreviousRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Command: []string{"echo", "prepare"}, Name: "prepare"},
		{ID: "train-id", Command: []string{"echo", "train"}, Name: "train", DependsOn: []string{"prepare"}},
	}}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	message, err := queueEditor().Change(baseDir, "default", "run-1", model.CommandSelector{IDs: []string{"train-id"}}, queueops.Mutation{Executor: "slurm", ExecutorOptions: []string{"-p gpu"}, DependsOn: []string{"prepare"}, Command: []string{"./train-v2"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "job=train-id") {
		t.Fatalf("change message = %q, want train-id", message)
	}
	changed, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Commands[1].ID != "train-id" || changed.Commands[1].Executor != "slurm" ||
		changed.Commands[1].Command[0] != "./train-v2" || len(changed.Commands[1].ExecutorOptions) != 1 {
		t.Fatalf("changed queue = %#v", changed)
	}
	original, err := loadQueue(filepath.Join(paths.RunsDir, "run-1", "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	if original.Commands[1].Command[0] != "echo" || original.Commands[1].Executor != "" {
		t.Fatalf("snapshot was modified: %#v", original.Commands[1])
	}
}

func TestRemoveBatchRemovesJobsAndRejectsDependencies(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "prepare-id", Command: []string{"echo", "prepare"}, Name: "prepare"},
		{ID: "train-id", Command: []string{"echo", "train"}, Name: "train", DependsOn: []string{"prepare"}},
		{ID: "other-id", Command: []string{"echo", "other"}, Name: "other"},
	}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, defaultMeta()); err != nil {
		t.Fatal(err)
	}

	if _, err := queueEditor().Remove(baseDir, "default", "", model.CommandSelector{IDs: []string{"other-id"}}); err != nil {
		t.Fatal(err)
	}
	remaining, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Commands) != 2 || remaining.Commands[0].ID != "prepare-id" || remaining.Commands[1].ID != "train-id" {
		t.Fatalf("remaining queue = %#v", remaining.Commands)
	}

	if _, err := queueEditor().Remove(baseDir, "default", "", model.CommandSelector{Name: "prepare"}); err == nil {
		t.Fatal("removing a job referenced by a dependency succeeded")
	}
	remaining, err = loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Commands) != 2 {
		t.Fatalf("queue changed after rejected removal: %#v", remaining.Commands)
	}
}

func TestRemoveBatchRestoresOnlyAnExplicitRun(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.Queue{Commands: []model.QueuedCommand{
		{ID: "one-id", Command: []string{"echo", "one"}, Name: "one"},
		{ID: "two-id", Command: []string{"echo", "two"}, Name: "two"},
	}}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "commands.json"), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "finished", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}

	if _, err := queueEditor().Remove(baseDir, "default", "", model.CommandSelector{IDs: []string{"one-id"}}); err == nil || !strings.Contains(err.Error(), "no queued jobs") {
		t.Fatalf("remove from an empty queue error = %v, want no queued jobs", err)
	}
	if queue, err := loadQueue(paths.QueueFile); err != nil || len(queue.Commands) != 0 {
		t.Fatalf("remove from an empty queue restored %#v (err %v)", queue.Commands, err)
	}

	message, err := queueEditor().Remove(baseDir, "default", "latest", model.CommandSelector{IDs: []string{"one-id"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "removed 1 job") {
		t.Fatalf("remove message = %q", message)
	}
	remaining, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining.Commands) != 1 || remaining.Commands[0].ID != "two-id" {
		t.Fatalf("restored queue = %#v", remaining.Commands)
	}
}

func TestExecuteMixedRunBlocksWhenDependencyFails(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{
		{ID: "job1-id", Command: []string{"/bin/sh", "-c", "exit 1"}, Name: "job1"},
		{ID: "job2-id", Command: []string{"/bin/sh", "-c", "echo ok"}, Name: "job2", DependsOn: []string{"job1"}},
	}}); err != nil {
		t.Fatal(err)
	}

	if code := executeMixedRun(paths, "blocked-run", "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 1 {
		t.Fatalf("executeMixedRun exit = %d, want 1", code)
	}

	data, err := os.ReadFile(filepath.Join(paths.RunsDir, "blocked-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary model.RunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 2 {
		t.Fatalf("summary len = %d, want 2", len(summary.Results))
	}
	if summary.Results[0].ExitCode != 1 || summary.Results[1].ExitCode != 1 {
		t.Fatalf("results = %#v, want both jobs to fail", summary.Results)
	}
	if summary.Results[1].Error != "blocked by failed dependency" {
		t.Fatalf("job2 error = %q, want blocked by failed dependency", summary.Results[1].Error)
	}
}

func TestExecuteMixedRunCarriesForwardNonSelectedResults(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "alpha", Command: []string{"/bin/sh", "-c", "exit 0"}, Name: "alpha"},
		{ID: "beta", Command: []string{"/bin/sh", "-c", "exit 1"}, Name: "beta"},
	}}
	run1ID := makeRunID()
	run2ID := makeRunID()
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, run1ID, "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); code != 1 {
		t.Fatalf("first run exit = %d, want 1", code)
	}
	meta := defaultMeta()
	meta.LastRunID = run1ID
	if err := writeJSON(paths.MetaFile, meta); err != nil {
		t.Fatal(err)
	}

	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, run2ID, "", 1, 1, 0, "", nil, "failed", nil, "", true, nil, nil); code != 1 {
		t.Fatalf("second run exit = %d, want 1 (beta still fails)", code)
	}

	if _, err := os.Stat(filepath.Join(paths.RunsDir, run2ID, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("alpha should not have been re-executed, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RunsDir, run2ID, "beta")); err != nil {
		t.Fatalf("beta should have been re-executed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(paths.RunsDir, run2ID, "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var summary model.RunSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	results := make(map[string]model.JobResult, len(summary.Results))
	for _, result := range summary.Results {
		results[result.ID] = result
	}
	if results["alpha"].ExitCode != 0 || results["beta"].ExitCode != 1 {
		t.Fatalf("summary results = %#v, want alpha carried success and beta re-run failed", results)
	}

	commandsData, err := os.ReadFile(filepath.Join(paths.RunsDir, run2ID, "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	var commands model.Queue
	if err := json.Unmarshal(commandsData, &commands); err != nil {
		t.Fatal(err)
	}
	var alphaOrigin *model.JobOrigin
	for _, command := range commands.Commands {
		if command.ID == "alpha" {
			alphaOrigin = command.Origin
		}
	}
	if alphaOrigin == nil || alphaOrigin.RunID != run1ID || alphaOrigin.JobID != "alpha" || alphaOrigin.Status != "success" {
		t.Fatalf("alpha origin = %#v, want %s/alpha success", alphaOrigin, run1ID)
	}
}

func TestValidateDependencies(t *testing.T) {
	valid := []model.JobSpec{
		{Name: "job1", Command: []string{"echo", "1"}},
		{Name: "job2", Command: []string{"echo", "2"}, DependsOn: []string{"job1"}},
	}
	if err := model.ValidateDependencies(valid); err != nil {
		t.Fatalf("valid dependencies returned error: %v", err)
	}
	cases := []struct {
		name string
		jobs []model.JobSpec
	}{
		{
			name: "unknown dependency",
			jobs: []model.JobSpec{{Name: "job2", DependsOn: []string{"missing"}}},
		},
		{
			name: "duplicate name",
			jobs: []model.JobSpec{{Name: "same"}, {Name: "same"}},
		},
		{
			name: "cycle",
			jobs: []model.JobSpec{
				{Name: "job1", DependsOn: []string{"job2"}},
				{Name: "job2", DependsOn: []string{"job1"}},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := model.ValidateDependencies(testCase.jobs); err == nil {
				t.Fatal("validateDependencies returned nil")
			}
		})
	}
}

func TestFinalizeCompletedCancellationRemovesStaleServerLock(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.MetaFile, model.Meta{Phase: "cancelling", LastRunID: "run-1"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{ID: "job-1", Command: []string{"echo", "stale"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(paths.RunsDir, "run-1", "summary.json"), model.RunSummary{
		RunID: "run-1", Status: "failed", FinishedAt: nowRFC3339(), ExitCode: 143,
	}); err != nil {
		t.Fatal(err)
	}

	finalized, err := finalizeCompletedCancellation(paths)
	if err != nil {
		t.Fatal(err)
	}
	if !finalized {
		t.Fatal("finalizeCompletedCancellation returned false")
	}
	if _, err := os.Stat(paths.LockFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock still exists, stat error = %v", err)
	}
	meta, err := loadMeta(paths.MetaFile)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Phase != "finished" {
		t.Fatalf("meta phase = %q, want finished", meta.Phase)
	}
	queue, err := loadQueue(paths.QueueFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 0 {
		t.Fatalf("queue commands = %d, want 0", len(queue.Commands))
	}
}

func TestExecuteMixedRunSupportsLSFExecutor(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "bsub", `#!/bin/sh
cat >/dev/null
printf 'Job <999> is submitted to default queue.\n'
`)
	writeExecutable(t, binDir, "bjobs", `#!/bin/sh
if [ "$1" = "-a" ]; then
    printf 'DONE 0\n'
    exit 0
fi
exit 1
`)
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "lsf-job", Command: []string{"echo", "hi"}, Executor: "lsf"}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}

	if exitCode := executeMixedRun(paths, makeRunID(), "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil); exitCode != 0 {
		t.Fatalf("executeMixedRun exit code = %d, want 0", exitCode)
	}
}

func TestExecuteMixedRunSupportsPBSExecutor(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "qsub", `#!/bin/sh
shift $(($#-1))
sh "$1" >/dev/null 2>&1 &
printf '999.headnode\n'
`)
	writeExecutable(t, binDir, "qstat", `#!/bin/sh
exit 1
`)
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })

	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.ProjectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "pbs-job", Command: []string{"echo", "hi"}, Executor: "pbs"}}}
	if err := writeJSON(paths.QueueFile, queue); err != nil {
		t.Fatal(err)
	}

	exitCode := executeMixedRun(paths, makeRunID(), "", 1, 1, 0, "", nil, "", nil, "", true, nil, nil)
	if exitCode != 0 {
		t.Fatalf("executeMixedRun exit code = %d, want 0", exitCode)
	}
}

func TestExecuteMixedRunSubmitsAndCompletesPBSArrayTasks(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, binDir, "qsub", `#!/bin/sh
wrapper=
for arg in "$@"; do wrapper=$arg; done
PBS_ARRAY_INDEX=1 sh "$wrapper"
PBS_ARRAY_INDEX=2 sh "$wrapper"
printf '999[].headnode\n'
`)
	writeExecutable(t, binDir, "qstat", "#!/bin/sh\nexit 1\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Name: "array", Executor: "pbs", Command: []string{"sh", "-c", "test \"$ROTARI_ARRAY_TASK_ID\" = \"${PBS_ARRAY_INDEX}\""}, Array: &model.ArraySpec{First: 1, Last: 2},
	}}}); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "array-run", "", 1, 2, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit = %d, want 0", code)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "array-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 2 || summary.Results[0].ID != "array-1" || summary.Results[1].ID != "array-2" {
		t.Fatalf("summary results = %#v, want both array tasks", summary.Results)
	}
	for _, id := range []string{"array-1", "array-2"} {
		jobDir, err := latestAttemptJobDir(filepath.Join(paths.RunsDir, "array-run"), id)
		if err != nil {
			t.Fatal(err)
		}
		status, ok := loadWrapperStatus(filepath.Join(jobDir, "status.json"))
		if !ok || status.Phase != "finished" || status.ExitCode != 0 {
			t.Fatalf("task %s status = %#v, ok=%v", id, status, ok)
		}
	}
}

func TestExecuteMixedRunSubmitsAndCompletesSlurmArrayTasks(t *testing.T) {
	binDir := t.TempDir()
	argumentsPath := filepath.Join(t.TempDir(), "sbatch-args")
	writeExecutable(t, binDir, "sbatch", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
wrapper=
for arg in "$@"; do wrapper=$arg; done
SLURM_ARRAY_TASK_ID=1 sh "$wrapper"
SLURM_ARRAY_TASK_ID=2 sh "$wrapper"
printf '54321;fake-host\n'
`, argumentsPath))
	writeExecutable(t, binDir, "squeue", "#!/bin/sh\nexit 0\n")
	writeExecutable(t, binDir, "sacct", "#!/bin/sh\nprintf 'COMPLETED|0:0\n'\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.QueueFile, model.Queue{Commands: []model.QueuedCommand{{
		ID: "array", Name: "array", Executor: "slurm", Command: []string{"sh", "-c", "exit 0"}, Array: &model.ArraySpec{First: 1, Last: 2},
	}}}); err != nil {
		t.Fatal(err)
	}
	if code := executeMixedRun(paths, "array-run", "", 1, 2, 0, "", nil, "", nil, "", true, nil, nil); code != 0 {
		t.Fatalf("executeMixedRun exit = %d, want 0", code)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(arguments), "--array=1-2\n") != 1 {
		t.Fatalf("sbatch arguments = %q, want one native array submission", arguments)
	}
	summary, err := loadRunSummary(filepath.Join(paths.RunsDir, "array-run", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Results) != 2 || summary.Results[0].ID != "array-1" || summary.Results[1].ID != "array-2" {
		t.Fatalf("summary results = %#v, want both array tasks", summary.Results)
	}
	for _, id := range []string{"array-1", "array-2"} {
		jobDir, err := latestAttemptJobDir(filepath.Join(paths.RunsDir, "array-run"), id)
		if err != nil {
			t.Fatal(err)
		}
		status, ok := loadWrapperStatus(filepath.Join(jobDir, "status.json"))
		if !ok || status.Phase != "finished" {
			t.Fatalf("task %s status = %#v, ok=%v", id, status, ok)
		}
	}
}

func TestCancelJobsCancelsSelectedLocalJob(t *testing.T) {
	runDir := filepath.Join(t.TempDir(), "run-1")
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sleep", "30")
	// runOneJob starts jobs as their own process group leader (Setpgid) so
	// signal(-pid) reaches the wrapped command too; mirror that here.
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), fmt.Appendf(nil, "%d\n", child.Process.Pid), 0o644); err != nil {
		t.Fatal(err)
	}

	message, err := jobController().CancelJobs(runDir, "default", "run-1", []string{"job-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "Jobs: 1") {
		t.Fatalf("message = %q, want one cancelled job", message)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("cancelled process exited successfully")
	}
}

func TestControlQueueJobsSuspendsAndResumesSelectedLocalJob(t *testing.T) {
	baseDir := t.TempDir()
	outputPath := filepath.Join(baseDir, "progress")
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sh", "-c", fmt.Sprintf("while :; do printf x >> %q; done", outputPath))
	// runOneJob starts jobs as their own process group leader (Setpgid) so
	// signal(-pid) reaches the wrapped command too; mirror that here.
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), fmt.Appendf(nil, "%d\n", child.Process.Pid), 0o644); err != nil {
		t.Fatal(err)
	}
	waitForFileSize(t, outputPath, 1)

	if _, err := controlQueueJobs(baseDir, "default", "", []string{"job-1"}, "suspend"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	suspendedSize := len(data)
	time.Sleep(100 * time.Millisecond)
	data, err = os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != suspendedSize {
		t.Fatalf("suspended process continued writing: size changed from %d to %d", suspendedSize, len(data))
	}
	if _, err := controlQueueJobs(baseDir, "default", "", []string{"job-1"}, "resume"); err != nil {
		t.Fatal(err)
	}
	waitForFileSize(t, outputPath, suspendedSize+1)
}

func TestControlQueueJobsReportsHostMismatchForLocalJob(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(paths.RunsDir, "run-1")
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(runDir, "context.json"), model.RunContext{Hostname: "other-host"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), fmt.Appendf(nil, "%d\n", os.Getpid()), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = controlQueueJobs(baseDir, "default", "", []string{"job-1"}, "suspend")
	if err == nil {
		t.Fatal("suspend across hosts unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "other-host") {
		t.Fatalf("error = %q, want it to mention the recorded host", err)
	}
	if _, err := jobController().CancelJobs(runDir, "default", "run-1", []string{"job-1"}); err == nil {
		t.Fatal("cancel across hosts unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "other-host") {
		t.Fatalf("error = %q, want it to mention the recorded host", err)
	}
}

func TestCancelQueueRejectsWholeRunFromWrongHost(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	// A PID that isn't this test process's own, recorded as owned by
	// another host: the pre-fix code would signal -pid locally, get ESRCH,
	// swallow it, and report success without cancelling anything remote.
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid() + 1, RunID: "run-1", StartedAt: nowRFC3339(), Host: "other-host"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = cancelQueue(baseDir, "default", false)
	if err == nil {
		t.Fatal("cancel of a whole run on another host unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "other-host") {
		t.Fatalf("error = %q, want it to mention the recorded host", err)
	}
}

func waitForFileSize(t *testing.T, path string, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("file %s did not reach size %d", path, want)
}

func TestControlQueueJobsControlsAllRunningJobsAndSkipsFinishedJobs(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "default")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(paths.LockFile, model.LockInfo{PID: os.Getpid(), RunID: "run-1", StartedAt: nowRFC3339()}); err != nil {
		t.Fatal(err)
	}
	for _, jobID := range []string{"running-1", "running-2", "finished"} {
		if err := os.MkdirAll(filepath.Join(paths.RunsDir, "run-1", jobID), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(paths.RunsDir, "run-1", "finished", "finished_at"), []byte(nowRFC3339()), 0o644); err != nil {
		t.Fatal(err)
	}
	children := make([]*exec.Cmd, 0, 2)
	for _, jobID := range []string{"running-1", "running-2"} {
		child := exec.Command("sleep", "30")
		// runOneJob starts jobs as their own process group leader (Setpgid) so
		// signal(-pid) reaches the wrapped command too; mirror that here.
		child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
		jobDir := filepath.Join(paths.RunsDir, "run-1", jobID)
		if err := os.WriteFile(filepath.Join(jobDir, "pid"), fmt.Appendf(nil, "%d\n", child.Process.Pid), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, child := range children {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	})

	message, err := controlQueueJobs(baseDir, "default", "", nil, "suspend")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "Jobs: 2") {
		t.Fatalf("message = %q, want two controlled jobs", message)
	}
}

func TestControlQueueJobsRejectsInvalidOrUnavailableRequests(t *testing.T) {
	if _, err := controlQueueJobs(t.TempDir(), "default", "", nil, "pause"); err == nil {
		t.Fatal("unsupported operation succeeded")
	}
	baseDir := t.TempDir()
	if _, err := controlQueueJobs(baseDir, "default", "", nil, "suspend"); err == nil {
		t.Fatal("suspend without a running queue succeeded")
	}
}

func TestLocalExecutorSignalRejectsMissingPID(t *testing.T) {
	err := (executor.Local{}).Cancel(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "job is not running") {
		t.Fatalf("error = %v, want job is not running", err)
	}
}

func TestLocalExecutorSignalRejectsMalformedPID(t *testing.T) {
	jobDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), []byte("not-a-pid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := (executor.Local{}).Suspend(jobDir)
	if err == nil || !strings.Contains(err.Error(), "job is not running") {
		t.Fatalf("error = %v, want job is not running", err)
	}
}

func TestLocalExecutorSignalRejectsExitedProcess(t *testing.T) {
	command := exec.Command("sh", "-c", "exit 0")
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	jobDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), []byte(stringPID(command.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	err := (executor.Local{}).Resume(jobDir)
	if err == nil || !strings.Contains(err.Error(), "job is not running") {
		t.Fatalf("error = %v, want job is not running", err)
	}
}

func stringPID(pid int) string {
	return fmt.Sprintf("%d\n", pid)
}

// runOneJob wraps local commands in the same self-reporting statusWrapperScript
// used by scheduler executors, so status.json stays authoritative even if the
// coordinator process dies before it can call cmd.Wait() itself.
func TestRunOneJobSelfReportsStatusJSON(t *testing.T) {
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job-1", Command: []string{"sh", "-c", "exit 3"}}
	result := runOneJob(runDir, job)
	if result.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", result.ExitCode)
	}
	status, ok := loadWrapperStatus(filepath.Join(runDir, "job-1", "status.json"))
	if !ok {
		t.Fatal("expected local job to self-report status.json like scheduler executors")
	}
	if status.Phase != "finished" || status.ExitCode != 3 {
		t.Fatalf("status.json = %+v, want phase=finished exit_code=3", status)
	}
}

func TestLocalJobWrapperSelfReportsStatusEvenIfCoordinatorNeverWaits(t *testing.T) {
	jobDir := t.TempDir()
	wrapperPath := filepath.Join(jobDir, "local-wrapper.sh")
	wrapper := executor.StatusWrapperScript([]string{"sh", "-c", "exit 7"}, jobDir, nil, "", "")
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", wrapperPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Simulate the coordinator dying before it calls cmd.Wait(); reap the
	// process afterward only to avoid leaking a zombie in this test.
	pid := cmd.Process.Pid
	t.Cleanup(func() { _, _ = syscall.Wait4(pid, nil, 0, nil) })

	deadline := time.Now().Add(2 * time.Second)
	var status executor.WrapperStatus
	var ok bool
	for time.Now().Before(deadline) {
		status, ok = loadWrapperStatus(filepath.Join(jobDir, "status.json"))
		if ok && jobstatus.WrapperTerminal(status) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok || !jobstatus.WrapperTerminal(status) {
		t.Fatalf("status.json was not self-reported by the orphaned job: %+v", status)
	}
	if status.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", status.ExitCode)
	}
}

func TestRunOneJobSkipsCancelledPendingJob(t *testing.T) {
	runDir := t.TempDir()
	job := model.JobSpec{ID: "job-1", Command: []string{"sh", "-c", "exit 0"}}
	jobDir := filepath.Join(runDir, job.ID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "cancelled"), []byte("requested\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runOneJob(runDir, job)
	if result.ExitCode != 143 || result.Error != "cancelled before start" {
		t.Fatalf("result = %+v, want cancelled result", result)
	}
	if _, err := os.Stat(filepath.Join(jobDir, "pid")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pid file exists or stat failed: %v", err)
	}
	if output, err := os.ReadFile(filepath.Join(jobDir, "output")); err != nil || !strings.Contains(string(output), "cancelled before start") {
		t.Fatalf("output = %q, err = %v", output, err)
	}
}

func TestJobWasExplicitlyCancelledUsesCancellationStateNotExitCode(t *testing.T) {
	runDir := t.TempDir()
	jobDir := filepath.Join(runDir, "job-1")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(jobDir, "status.json"), executor.WrapperStatus{Phase: "cancelled", ExitCode: 143}); err != nil {
		t.Fatal(err)
	}
	if !jobWasExplicitlyCancelled(runDir, "job-1", model.JobResult{ID: "job-1", ExitCode: 143}) {
		t.Fatal("cancelled status was not recognized")
	}
	if jobWasExplicitlyCancelled(runDir, "job-2", model.JobResult{ID: "job-2", ExitCode: 143}) {
		t.Fatal("exit code 143 alone was treated as cancellation")
	}
}

func TestSendRunRequestReadsProgressThenFinalResponse(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-socket-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", serverinternal.SocketPath(baseDir))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	defer os.Remove(serverinternal.SocketPath(baseDir))

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var request serverinternal.Request
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if request.Op != "run" {
			t.Errorf("request op = %q, want run", request.Op)
		}
		encoder := json.NewEncoder(conn)
		if err := encoder.Encode(serverinternal.Response{Progress: true, Completed: 1, Total: 2, Succeeded: 1, Failed: 0, Message: "progress"}); err != nil {
			t.Errorf("encode progress: %v", err)
			return
		}
		if err := encoder.Encode(serverinternal.Response{OK: true, Message: "Run finished", ExitCode: 0}); err != nil {
			t.Errorf("encode final response: %v", err)
		}
	}()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	response, err := sendRunRequest(baseDir, serverinternal.Request{Op: "run"})
	_ = w.Close()
	output, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatalf("sendRunRequest returned error: %v", err)
	}
	if !response.OK || response.Message != "Run finished" || response.ExitCode != 0 {
		t.Fatalf("response = %#v, want OK=true message=Run finished exit_code=0", response)
	}
	if !strings.Contains(string(output), "progress") && !strings.Contains(string(output), "progress:") {
		t.Fatalf("progress output missing; got %q", string(output))
	}
}

func TestServerSyncRunDisconnectCancelsRunningJob(t *testing.T) {
	baseDir := t.TempDir()
	queueDir := filepath.Join(baseDir, "projects", "default")
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(queueDir, "queue.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "slow-id", Command: []string{"sleep", "30"}, Name: "slow"}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(queueDir, "meta.json"), defaultMeta()); err != nil {
		t.Fatal(err)
	}

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		newRotariServer(baseDir).Handle(serverConn)
	}()
	startAttachedTestRun(t, client)

	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		matches, err := filepath.Glob(filepath.Join(queueDir, "runs", "*", "slow-id", "attempts", "*", "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 1 {
			data, err := os.ReadFile(matches[0])
			if err == nil {
				pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			}
		}
		if pid == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if pid == 0 {
		t.Fatal("job did not start")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("running job was not cancelled after disconnect")
	}
	if state.ProcessAlive(pid) {
		t.Fatalf("job process %d is still running after disconnect", pid)
	}
}

func TestServerSyncRunDetachLeavesJobRunning(t *testing.T) {
	baseDir := t.TempDir()
	queueDir := filepath.Join(baseDir, "projects", "default")
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(queueDir, "queue.json"), model.Queue{Commands: []model.QueuedCommand{{ID: "slow-id", Command: []string{"sleep", "1"}, Name: "slow"}}}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(queueDir, "meta.json"), defaultMeta()); err != nil {
		t.Fatal(err)
	}

	client, serverConn := net.Pipe()
	defer client.Close()
	defer serverConn.Close()
	done := make(chan struct{})
	server := newRotariServer(baseDir)
	go func() {
		defer close(done)
		server.Handle(serverConn)
	}()
	responses := startAttachedTestRun(t, client)

	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		matches, err := filepath.Glob(filepath.Join(queueDir, "runs", "*", "slow-id", "attempts", "*", "pid"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 1 {
			data, err := os.ReadFile(matches[0])
			if err == nil {
				pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			}
		}
		if pid == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if pid == 0 {
		t.Fatal("job did not start")
	}
	if _, err := client.Write([]byte{serverinternal.DetachControl}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("detach did not return promptly")
	}
	if !state.ProcessAlive(pid) {
		t.Fatalf("running job %d was stopped by detach", pid)
	}
	if final := <-responses; !final.OK || final.Message != serverinternal.DetachedMessage {
		t.Fatalf("final response = %#v, want detached message", final)
	}
	deadline = time.Now().Add(5 * time.Second)
	for server.Busy() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if server.Busy() || !server.Stopped() {
		t.Fatal("server did not end the detached run after it completed")
	}
}

// startAttachedTestRun sends a synchronous run request for the default
// project and drains progress, returning the final response.
func startAttachedTestRun(t *testing.T, client net.Conn) <-chan serverinternal.Response {
	t.Helper()
	if err := json.NewEncoder(client).Encode(serverinternal.Request{Op: serverinternal.OpRun, QueueName: "default", LocalConcurrency: 1, BatchMaxActive: 1, PartialArray: true}); err != nil {
		t.Fatal(err)
	}
	final := make(chan serverinternal.Response, 1)
	go func() {
		decoder := json.NewDecoder(client)
		for {
			var response serverinternal.Response
			if err := decoder.Decode(&response); err != nil {
				close(final)
				return
			}
			if !response.Progress {
				final <- response
				return
			}
		}
	}()
	return final
}

func writeExecutable(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}
