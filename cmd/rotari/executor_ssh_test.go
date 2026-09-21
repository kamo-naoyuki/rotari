package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHExecutorRunsRemoteCommandAndRecordsResult(t *testing.T) {
	binDir := t.TempDir()
	oldSSHCommandPath := sshCommandPath
	sshCommandPath = filepath.Join(binDir, "ssh")
	t.Cleanup(func() { sshCommandPath = oldSSHCommandPath })
	argumentsPath := filepath.Join(t.TempDir(), "ssh-args")
	writeExecutable(t, binDir, "ssh", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
cat > "$ROTARI_SSH_SCRIPT"
sh "$ROTARI_SSH_SCRIPT"
`, argumentsPath))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	remoteDir := t.TempDir()
	scriptPath := filepath.Join(remoteDir, "remote.sh")
	t.Setenv("ROTARI_SSH_SCRIPT", scriptPath)
	t.Setenv("XDG_RUNTIME_DIR", remoteDir)

	runDir := filepath.Join(t.TempDir(), "runs", "run-1")
	job := JobSpec{ID: "job-1", Command: []string{"sh", "-c", "printf '%s' \"$ROTARI_JOB_ID\""}, Environment: []string{"ROTARI_JOB_ID=job-1"}}
	handle, err := (sshExecutor{}).Submit(runDir, job, []string{"builder@example.test", "-p 2222"})
	if err != nil {
		t.Fatal(err)
	}
	result := (sshExecutor{}).Wait(runDir, handle)
	if result.ExitCode != 0 || len(result.Hosts) != 1 || result.Hosts[0] != "builder@example.test" {
		t.Fatalf("result = %#v", result)
	}
	output, err := os.ReadFile(filepath.Join(runDir, "job-1", "output"))
	if err != nil || string(output) != "job-1" {
		t.Fatalf("output = %q, err = %v", output, err)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "-p\n2222\n--\nbuilder@example.test\nsh\n-s\n") {
		t.Fatalf("ssh arguments = %q", arguments)
	}
}

func TestSSHExecutorCancelsRemoteProcessGroup(t *testing.T) {
	binDir := t.TempDir()
	oldSSHCommandPath := sshCommandPath
	sshCommandPath = filepath.Join(binDir, "ssh")
	t.Cleanup(func() { sshCommandPath = oldSSHCommandPath })
	writeExecutable(t, binDir, "ssh", `#!/bin/sh
script="${ROTARI_SSH_SCRIPT_DIR}/ssh-$$"
cat > "$script"
sh "$script"
status=$?
rm -f "$script"
exit "$status"
`)
	remoteDir := t.TempDir()
	t.Setenv("ROTARI_SSH_SCRIPT_DIR", remoteDir)
	t.Setenv("XDG_RUNTIME_DIR", remoteDir)

	runDir := filepath.Join(t.TempDir(), "runs", "run-1")
	job := JobSpec{ID: "job-1", Command: []string{"sh", "-c", "trap 'exit 0' TERM; while :; do sleep 1; done"}}
	handle, err := (sshExecutor{}).Submit(runDir, job, []string{"builder@example.test", "-p 2222"})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := readSSHMetadata(filepath.Join(runDir, job.ID))
	if err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(remoteDir, "rotari-"+fmt.Sprint(os.Getuid()), metadata.RemoteToken, "process")
	if err := (sshExecutor{}).Cancel(filepath.Join(runDir, job.ID)); err != nil {
		t.Fatal(err)
	}
	result := (sshExecutor{}).Wait(runDir, handle)
	if result.ExitCode != 0 {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("remote process state remains after cancellation: %v", err)
	}
}

func TestSSHCancelRefusesReusedPID(t *testing.T) {
	remoteDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", remoteDir)
	token := "0123456789abcdef0123456789abcdef"
	stateDir := filepath.Join(remoteDir, "rotari-"+fmt.Sprint(os.Getuid()), token)
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "process"), []byte(fmt.Sprintf("%d 0\n", os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-s")
	cmd.Stdin = strings.NewReader(sshCancelScript(token))
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "remote PID has been reused") {
		t.Fatalf("cancel error = %v, output = %q", err, output)
	}
}

func TestReadSSHMetadataRejectsInvalidRemoteToken(t *testing.T) {
	jobDir := t.TempDir()
	metadata := sshJobMetadata{Executor: "ssh", RemoteToken: "'; echo unsafe; '"}
	if err := writeJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHMetadata(jobDir); err == nil {
		t.Fatal("readSSHMetadata accepted an invalid remote token")
	}
}

func TestSSHTargetRequiresHost(t *testing.T) {
	if _, _, err := sshTarget(nil); err == nil {
		t.Fatal("sshTarget accepted no target host")
	}
}

func TestSSHWrapperScriptChangesWorkingDirectory(t *testing.T) {
	script := sshWrapperScript([]string{"pwd"}, nil, "/remote/work", "0123456789abcdef")
	if !strings.Contains(script, "cd '/remote/work' || exit 1") {
		t.Fatalf("script = %q", script)
	}
}
