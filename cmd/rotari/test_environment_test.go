package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Isolate discovery records even for tests that only set --basedir. The
// subprocesses they launch inherit this masterdir, and individual tests can
// still override it with t.Setenv. Clean up before os.Exit, also on failure.
func TestMain(m *testing.M) {
	masterDir, err := os.MkdirTemp("", "rotari-test-master-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := 1
	if err := os.Setenv("ROTARI_MASTERDIR", masterDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		code = m.Run()
	}
	if err := os.RemoveAll(masterDir); err != nil {
		fmt.Fprintln(os.Stderr, "remove test masterdir:", err)
		code = 1
	}
	os.Exit(code)
}

const registryProbeEnv = "ROTARI_TEST_REGISTRY_PROBE"
const registryProbeMarker = "test masterdir: "

func TestTestEnvironmentCleansRegistry(t *testing.T) {
	if probe := os.Getenv(registryProbeEnv); probe != "" {
		probeTestRegistry(t, probe)
		return
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"success", "failure"} {
		t.Run(outcome, func(t *testing.T) {
			checkTestRegistryCleanup(t, executable, outcome)
		})
	}
}

func probeTestRegistry(t *testing.T, outcome string) {
	t.Helper()
	masterDir, err := state.ResolveMasterDir("")
	if err != nil {
		t.Fatal(err)
	}
	baseDir := t.TempDir()
	if code := cmdAdd([]string{"--basedir", baseDir, "--project-name", "demo", "echo", "hello"}); code != 0 {
		t.Fatalf("cmdAdd exit code = %d, want 0", code)
	}
	baseDirs, err := basedirregistry.Open(masterDir).BaseDirs()
	if err != nil || len(baseDirs) != 1 || baseDirs[0] != baseDir {
		t.Fatalf("registered basedirs = %v, %v; want [%s]", baseDirs, err, baseDir)
	}
	fmt.Println(registryProbeMarker + masterDir)
	if outcome == "failure" {
		t.Fatal("intentional failure to check cleanup")
	}
}

func checkTestRegistryCleanup(t *testing.T, executable, outcome string) {
	t.Helper()
	userMaster := t.TempDir()
	command := exec.Command(executable, "-test.run=^TestTestEnvironmentCleansRegistry$", "-test.v")
	command.Env = append(os.Environ(), "ROTARI_MASTERDIR="+userMaster, registryProbeEnv+"="+outcome)
	output, err := command.CombinedOutput()
	if command.ProcessState == nil {
		t.Fatalf("could not start child test: %v", err)
	}
	if outcome == "success" && err != nil {
		t.Fatalf("child test failed: %v\n%s", err, output)
	}
	if outcome == "failure" && command.ProcessState.ExitCode() != 1 {
		t.Fatalf("child exit = %d, want 1: %v\n%s", command.ProcessState.ExitCode(), err, output)
	}
	masterDir := reportedTestMasterDir(t, output)
	if masterDir == userMaster {
		t.Fatal("test used the user's masterdir instead of an isolated temporary directory")
	}
	if _, err := os.Stat(masterDir); !os.IsNotExist(err) {
		t.Fatalf("test masterdir remains after child exit: %s (%v)", masterDir, err)
	}
	entries, err := os.ReadDir(userMaster)
	if err != nil || len(entries) != 0 {
		t.Fatalf("user masterdir was modified: %v, %v", entries, err)
	}
}

func reportedTestMasterDir(t *testing.T, output []byte) string {
	t.Helper()
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, registryProbeMarker) {
			return strings.TrimPrefix(line, registryProbeMarker)
		}
	}
	t.Fatalf("child did not report its masterdir:\n%s", output)
	return ""
}
