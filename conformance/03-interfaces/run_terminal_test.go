package interfaces

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// TestRunTerminalControlsPrintOutcomeOnce sends Ctrl-D and Ctrl-C through a
// terminal to a synchronous run and expects one complete outcome line.
func TestRunTerminalControlsPrintOutcomeOnce(t *testing.T) {
	covers(t, "CLI-19")
	for _, test := range []struct {
		name     string
		key      byte
		message  string
		exitCode int
	}{
		{name: "Ctrl-D detaches", key: 0x04, message: "Run detached; it continues in the background.", exitCode: 0},
		{name: "Ctrl-C cancels", key: 0x03, message: "Cancellation requested; stopping running jobs...", exitCode: 130},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := support.NewEnv(t)
			project := "terminal-controls"
			gate := filepath.Join(e.Root, "release")
			t.Cleanup(func() {
				_ = os.WriteFile(gate, nil, 0o600)
				_ = e.Rotari("cancel", "-p", project, "--wait")
			})
			e.MustRotari("add", "-p", project, "--", "sh", "-c", "while [ ! -f "+gate+" ]; do sleep 0.05; done")
			output, code := runInTerminal(t, e.Command("run", "-p", project), "=== Run started ===", test.key)
			if code != test.exitCode {
				t.Fatalf("exit = %d, want %d; output:\n%s", code, test.exitCode, output)
			}
			if count := strings.Count(output, test.message); count != 1 {
				t.Fatalf("%q printed %d times, want once; output:\n%s", test.message, count, output)
			}
			if !strings.HasSuffix(output, "\n") {
				t.Fatalf("output does not end with a newline:\n%q", output)
			}
		})
	}
}

// runInTerminal runs cmd on a new pseudo-terminal, writes key once ready
// appears in its output, and returns the output with carriage returns removed.
func runInTerminal(t *testing.T, cmd *exec.Cmd, ready string, key byte) (string, int) {
	t.Helper()
	master, slave, err := openTerminal()
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	defer master.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		slave.Close()
		t.Fatal(err)
	}
	slave.Close()
	var output bytes.Buffer
	done := make(chan struct{})
	readyCh := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 4096)
		signalled := false
		for {
			n, err := master.Read(buffer)
			output.Write(buffer[:n])
			if !signalled && strings.Contains(output.String(), ready) {
				signalled = true
				close(readyCh)
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-readyCh:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		<-done
		t.Fatalf("command did not print %q:\n%s", ready, output.String())
	}
	// Give the client time to start following the run before the key.
	time.Sleep(300 * time.Millisecond)
	if _, err := master.Write([]byte{key}); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	<-done
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(output.String(), "\r", "")
	text = strings.ReplaceAll(text, "^C", "")
	return text, code
}

func openTerminal() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	unlock := 0
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		master.Close()
		return nil, nil, errno
	}
	var number uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&number))); errno != 0 {
		master.Close()
		return nil, nil, errno
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
