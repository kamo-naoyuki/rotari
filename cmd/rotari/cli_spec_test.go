package main

import (
	"flag"
	"os"
	"strings"
	"testing"
)

func TestCliParseColorsFlagErrors(t *testing.T) {
	oldTerminalCheck := terminalCheck
	terminalCheck = func(*os.File) bool { return true }
	t.Cleanup(func() { terminalCheck = oldTerminalCheck })

	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.String("known", "", "known option")
	_, output := captureStderr(t, func() int {
		if err := cliParse(fs, []string{"--matri"}); err == nil {
			return 0
		}
		return 1
	})
	if !strings.HasPrefix(output, ansiRed+"flag provided but not defined: -matri"+ansiReset+"\n") {
		t.Fatalf("flag error = %q, want a red error line", output)
	}
}

func TestCliSimilarCommand(t *testing.T) {
	if got := cliSimilarCommand("lieneage"); got != "lineage" {
		t.Fatalf("similar command = %q, want lineage", got)
	}
	if got := cliSimilarCommand("shwo"); got != "show" {
		t.Fatalf("similar command = %q, want show", got)
	}
	if got := cliSimilarCommand("completely-unrelated"); got != "" {
		t.Fatalf("unrelated command suggestion = %q, want none", got)
	}
}
