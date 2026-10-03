package main

import (
	"flag"
	"io"
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

func TestCLIParseRejectsRepeatedSingleValueOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--stage", "single", "--stage", "batch"},
		{"--stage=single", "--stage=batch"},
		{"--basedir", "/tmp/one", "-b", "/tmp/two"},
		{"--yes", "--yes"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			fs := flag.NewFlagSet("suspend", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			cliString(fs, "basedir", "")
			cliJobControlOptions(fs, signalStates)
			if err := cliParse(fs, args); err == nil || !strings.Contains(err.Error(), "specified more than once") {
				t.Fatalf("cliParse(%q) error = %v, want duplicate-option error", args, err)
			}
		})
	}
}

func TestCLIParseAllowsRepeatedOptionsAndRejectsDuplicatesBeforeCommand(t *testing.T) {
	fs := flag.NewFlagSet("suspend", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cliJobControlOptions(fs, signalStates)
	if err := cliParse(fs, []string{"--job-name", "first", "--job-name", "second"}); err != nil {
		t.Fatalf("cliParse repeated --job-name: %v", err)
	}

	add := flag.NewFlagSet("add", flag.ContinueOnError)
	add.SetOutput(io.Discard)
	cliString(add, "stage", "")
	if err := parseLeadingFlags(add, []string{"--stage", "single", "--stage", "batch", "echo", "ok"}); err == nil || !strings.Contains(err.Error(), "specified more than once") {
		t.Fatalf("parseLeadingFlags duplicate --stage error = %v", err)
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
