package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestSSHExecutorRunsRemoteCommandAndRecordsResult(t *testing.T) {
	binDir := t.TempDir()
	helperDir := installTestRotari(t)
	oldSSHCommandPath := SSHCommandPath
	SSHCommandPath = filepath.Join(binDir, "ssh")
	t.Cleanup(func() { SSHCommandPath = oldSSHCommandPath })
	argumentsPath := filepath.Join(t.TempDir(), "ssh-args")
	writeExecutable(t, binDir, "ssh", fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
cat > "$ROTARI_SSH_SCRIPT"
env -i PATH=%q:/usr/local/bin:/usr/bin:/bin sh "$ROTARI_SSH_SCRIPT"
`, argumentsPath, helperDir))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	remoteDir := t.TempDir()
	scriptPath := filepath.Join(remoteDir, "remote.sh")
	t.Setenv("ROTARI_SSH_SCRIPT", scriptPath)
	t.Setenv("XDG_RUNTIME_DIR", remoteDir)
	t.Setenv("HOST_ONLY_VALUE", "remote-host-only")

	runDir := filepath.Join(t.TempDir(), "runs", "run-1")
	job := model.JobSpec{
		ID: "job-1", EnvMode: model.EnvModeAll, LogMode: model.LogModeSeparate,
		Command:     []string{"sh", "-c", "printf '%s:%s:%s' \"$ROTARI_JOB_ID\" \"$CALLER_VALUE\" \"$HOST_ONLY_VALUE\"; printf ssh-error >&2"},
		Environment: []string{"ROTARI_JOB_ID=job-1", "PATH=" + helperDir + ":/usr/local/bin:/usr/bin:/bin"}, InheritedEnvironment: []string{"CALLER_VALUE=caller"}, WorkingDirectory: remoteDir,
		Output: []string{"nested/stdout.log", "archive/stdout.log"}, Error: []string{"nested/stderr.log"},
	}
	ssh := SSH{Store: testStore()}
	handle, err := ssh.Submit(runDir, job, []string{"builder@example.test", "-p 2222"})
	if err != nil {
		t.Fatal(err)
	}
	result := ssh.Wait(runDir, handle)
	if result.ExitCode != 0 || len(result.Hosts) != 1 || result.Hosts[0] != "builder@example.test" {
		t.Fatalf("result = %#v", result)
	}
	stdout, err := os.ReadFile(filepath.Join(runDir, "job-1", state.StdoutFileName))
	if err != nil || string(stdout) != "job-1:caller:" {
		t.Fatalf("stdout = %q, err = %v", stdout, err)
	}
	stderr, err := os.ReadFile(filepath.Join(runDir, "job-1", state.StderrFileName))
	if err != nil || string(stderr) != "ssh-error" {
		t.Fatalf("stderr = %q, err = %v", stderr, err)
	}
	for path, want := range map[string]string{
		filepath.Join(remoteDir, "nested", "stdout.log"):  "job-1:caller:",
		filepath.Join(remoteDir, "archive", "stdout.log"): "job-1:caller:",
		filepath.Join(remoteDir, "nested", "stderr.log"):  "ssh-error",
	} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Errorf("external destination %s = %q, err=%v; want %q", path, data, err, want)
		}
	}
	noneJob := model.JobSpec{
		ID: "job-none", EnvMode: model.EnvModeNone, LogMode: model.LogModeSeparate,
		Command:     []string{"sh", "-c", "printf '%s:%s' \"$ROTARI_JOB_ID\" \"$HOST_ONLY_VALUE\""},
		Environment: []string{"ROTARI_JOB_ID=job-none"}, WorkingDirectory: remoteDir,
	}
	noneHandle, err := ssh.Submit(runDir, noneJob, []string{"builder@example.test", "-p", "2222"})
	if err != nil {
		t.Fatal(err)
	}
	if result := ssh.Wait(runDir, noneHandle); result.ExitCode != 0 {
		t.Fatalf("NONE SSH result = %#v", result)
	}
	noneOutput, err := os.ReadFile(filepath.Join(runDir, "job-none", state.StdoutFileName))
	if err != nil || string(noneOutput) != "job-none:" {
		t.Fatalf("NONE SSH stdout = %q, err=%v", noneOutput, err)
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
	oldSSHCommandPath := SSHCommandPath
	SSHCommandPath = filepath.Join(binDir, "ssh")
	t.Cleanup(func() { SSHCommandPath = oldSSHCommandPath })
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
	job := model.JobSpec{ID: "job-1", Command: []string{"sh", "-c", "trap 'exit 0' TERM; while :; do sleep 1; done"}}
	ssh := SSH{Store: testStore()}
	handle, err := ssh.Submit(runDir, job, []string{"builder@example.test", "-p 2222"})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := readSSHMetadata(ssh.Store, filepath.Join(runDir, job.ID))
	if err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(remoteDir, "rotari-"+fmt.Sprint(os.Getuid()), metadata.RemoteToken, "process")
	if err := ssh.Cancel(filepath.Join(runDir, job.ID)); err != nil {
		t.Fatal(err)
	}
	result := ssh.Wait(runDir, handle)
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
	if err := state.WriteJSON(filepath.Join(jobDir, "job.json"), metadata); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHMetadata(testStore(), jobDir); err == nil {
		t.Fatal("readSSHMetadata accepted an invalid remote token")
	}
}

func TestReadSSHMetadataSeparatesMissingAndUnreadableState(t *testing.T) {
	missingJobDir := t.TempDir()
	if _, err := readSSHMetadata(testStore(), missingJobDir); err == nil || !strings.Contains(err.Error(), "job is not running") {
		t.Fatalf("missing metadata error = %v, want job is not running", err)
	}

	jobDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(jobDir, "job.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := readSSHMetadata(testStore(), jobDir); err == nil || strings.Contains(err.Error(), "job is not running") {
		t.Fatalf("unreadable metadata error = %v, want distinct read failure", err)
	}
}

func TestSSHTargetRequiresHost(t *testing.T) {
	if _, _, err := SSHTarget(nil); err == nil {
		t.Fatal("SSHTarget accepted no target host")
	}
}

func TestSSHWrapperScriptChangesWorkingDirectory(t *testing.T) {
	script := sshWrapperScript([]string{"pwd"}, nil, "/remote/work", "0123456789abcdef", "")
	if !strings.Contains(script, "cd '/remote/work' || exit 1") {
		t.Fatalf("script = %q", script)
	}
}
