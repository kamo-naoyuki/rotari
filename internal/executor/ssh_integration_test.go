package executor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// These tests use a real ssh client and a loopback sshd with a shared
// filesystem. The config must connect without prompting and pin the host key.
func sshIntegrationOptions(t *testing.T) []string {
	t.Helper()
	config := os.Getenv("ROTARI_SSH_INTEGRATION_CONFIG")
	if config == "" {
		t.Skip("set ROTARI_SSH_INTEGRATION_CONFIG to run against a test sshd")
	}
	if _, err := os.Stat(config); err != nil {
		t.Fatalf("SSH client config: %v", err)
	}
	return []string{"rotari-ssh-test", "-F", config}
}

func TestSSHIntegrationRunsRemoteCommand(t *testing.T) {
	options := sshIntegrationOptions(t)
	runDir := t.TempDir()
	job := model.JobSpec{
		ID: "ssh-result", Command: []string{"sh", "-c", "printf 'remote:%s' \"$TEST_VALUE\"; printf 'remote-error' >&2; exit 7"},
		Environment: []string{"TEST_VALUE=connected"}, LogMode: model.LogModeSeparate,
	}
	ssh := NewSSH(testStore())
	handle, err := ssh.Submit(runDir, job, options)
	if err != nil {
		t.Fatal(err)
	}
	result := ssh.Wait(runDir, handle)
	if result.ExitCode != 7 || len(result.Hosts) != 1 || result.Hosts[0] != "rotari-ssh-test" {
		t.Fatalf("result = %#v", result)
	}
	for name, want := range map[string]string{state.StdoutFileName: "remote:connected", state.StderrFileName: "remote-error"} {
		data, err := os.ReadFile(filepath.Join(runDir, job.ID, name))
		if err != nil || string(data) != want {
			t.Errorf("%s = %q, err = %v; want %q", name, data, err, want)
		}
	}
}

func TestSSHIntegrationCancelsRemoteProcessGroup(t *testing.T) {
	options := sshIntegrationOptions(t)
	runDir := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready")
	job := model.JobSpec{ID: "ssh-cancel", Command: []string{"sh", "-c", "trap 'exit 0' TERM; touch " + ShellQuote(ready) + "; while :; do sleep 1; done"}}
	ssh := NewSSH(testStore())
	handle, err := ssh.Submit(runDir, job, options)
	if err != nil {
		t.Fatal(err)
	}
	jobDir := filepath.Join(runDir, job.ID)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("remote SSH command did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := ssh.Cancel(jobDir); err != nil {
		t.Fatal(err)
	}
	result := ssh.Wait(runDir, handle)
	if result.ExitCode != 0 || result.Error != "" {
		t.Fatalf("result after cancellation = %#v", result)
	}
}
