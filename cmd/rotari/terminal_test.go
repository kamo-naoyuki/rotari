package main

import (
	"os"
	"testing"
)

// usePromptStdin makes confirmation prompts treat stdin as interactive and
// feeds them input through a pipe.
func usePromptStdin(t *testing.T, input string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	oldStdin, oldStdinIsTerminal := os.Stdin, stdinIsTerminal
	os.Stdin = reader
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() {
		os.Stdin, stdinIsTerminal = oldStdin, oldStdinIsTerminal
		_ = reader.Close()
	})
}

func TestIsTerminalRejectsRegularFile(t *testing.T) {
	regular, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()
	if isTerminal(regular) {
		t.Fatal("regular file was treated as a terminal")
	}
}

func TestIsTerminalRejectsDevNull(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	if isTerminal(devNull) {
		t.Fatal("/dev/null was treated as a terminal")
	}
}

func TestIsTerminalAcceptsPseudoTerminal(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	defer master.Close()
	if !isTerminal(master) {
		t.Fatal("pseudo-terminal was not treated as a terminal")
	}
}
