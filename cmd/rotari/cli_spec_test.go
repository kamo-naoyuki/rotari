package main

import (
	"flag"
	"io"
	"os"
	"reflect"
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
	if !strings.HasPrefix(output, ansiRed+"unknown option --matri"+ansiReset+"\n") {
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

// The flag package names every option with one dash and follows an error
// with every option's description; users see one error line naming the
// option as they would type it and a pointer to the command's help.
func TestCliParseFlagErrorsNameOptionsAndPointToHelp(t *testing.T) {
	oldTerminalCheck := terminalCheck
	terminalCheck = func(*os.File) bool { return false }
	t.Cleanup(func() { terminalCheck = oldTerminalCheck })
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--matri"}, "unknown option --matri"},
		{[]string{"-x"}, "unknown option -x"},
		{[]string{"--stage"}, "option --stage needs a value"},
		{[]string{"--local-concurrency", "many"}, `invalid value "many" for option --local-concurrency`},
		{[]string{"--quiet=maybe"}, `invalid boolean value "maybe" for option --quiet`},
	} {
		t.Run(strings.Join(test.args, "_"), func(t *testing.T) {
			_, output := captureStderr(t, func() int {
				fs := flag.NewFlagSet("run", flag.ContinueOnError)
				fs.SetOutput(os.Stderr)
				cliString(fs, "stage", "")
				cliInt(fs, "local-concurrency", 1)
				cliBool(fs, "quiet", false)
				if err := cliParse(fs, test.args); err == nil {
					return 0
				}
				return 1
			})
			lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
			if len(lines) != 2 || !strings.HasPrefix(lines[0], test.want) || lines[1] != "Run 'rotari run --help' to list its options." {
				t.Fatalf("flag error output = %q, want %q and a help hint", output, test.want)
			}
		})
	}
}

// Short options are kept to the target selectors used across commands;
// output options such as --format have none.
func TestShortOptionsAreOnlyTargetSelectors(t *testing.T) {
	want := map[string]string{"basedir": "b", "project-name": "p", "run-id": "r", "job-id": "j", "executor": "e"}
	if !reflect.DeepEqual(cliShortFlagNames, want) {
		t.Fatalf("short options = %v, want %v", cliShortFlagNames, want)
	}
	for _, command := range []string{"jobs", "config", "export"} {
		_, output := captureStderr(t, func() int { return run([]string{command, "-o", "json"}) })
		if !strings.Contains(output, "unknown option -o") {
			t.Errorf("%s -o output = %q, want unknown option -o", command, output)
		}
	}
}
