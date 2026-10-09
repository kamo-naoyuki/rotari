package main

import (
	"bytes"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestAgentGuideIndexesEveryCommand(t *testing.T) {
	guide := agentGuide()
	for _, command := range cliCommandSpecs {
		if !strings.Contains(guide, "\n- `"+command.Name+"`: "+command.Description+"\n") {
			t.Fatalf("guide does not index command %q", command.Name)
		}
	}
	if !strings.Contains(guide, "rotari COMMAND --help") {
		t.Fatal("guide does not point to the commands' help for their options")
	}
}

// TestCommandHelpCoversEveryOptionAndExitsZero asks every command for its
// help, which the guide points to: it exits 0, names the command the user
// typed, and describes every option of the command's spec.
func TestCommandHelpCoversEveryOptionAndExitsZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, command := range cliCommandSpecs {
		t.Run(command.Name, func(t *testing.T) {
			var output bytes.Buffer
			code := captureShowStdout(t, &output, func() int { return run([]string{command.Name, "--help"}) })
			help := output.String()
			if code != 0 || !strings.Contains(help, "usage: rotari "+command.Name) {
				t.Fatalf("%s --help exit %d:\n%s", command.Name, code, help)
			}
			for _, flagSpec := range command.Flags {
				// An option heads its entry, or follows another name in a
				// per-executor entry.
				named := regexp.MustCompile(`(?m)^  (--[a-z-]+, )*--` + regexp.QuoteMeta(flagSpec.Name) + `[ ,\n]`)
				if !named.MatchString(help) {
					t.Errorf("%s --help does not describe --%s:\n%s", command.Name, flagSpec.Name, help)
				}
			}
		})
	}
	var output bytes.Buffer
	captureShowStdout(t, &output, func() int { return run([]string{"retry", "--help"}) })
	if !strings.HasPrefix(output.String(), "rotari retry: ") {
		t.Fatalf("retry --help is not titled by retry:\n%s", output.String())
	}
}

// TestCommandHelpStatesEachNoteOnce checks that run --help lists the
// per-executor options once per kind, not once per executor, and states an
// option's default and choices once.
func TestCommandHelpStatesEachNoteOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var output bytes.Buffer
	captureShowStdout(t, &output, func() int { return run([]string{"run", "--help"}) })
	help := output.String()
	for _, want := range []string{
		"\n  --ssh-concurrency, --slurm-concurrency, --pbs-concurrency, --lsf-concurrency, --sge-concurrency N\n      executor concurrency (env: ROTARI_RUN_<EXECUTOR>_CONCURRENCY)\n",
		"\n  --slurm-submit-interval, --pbs-submit-interval, --lsf-submit-interval, --sge-submit-interval DURATION\n      minimum submission interval (env: ROTARI_RUN_<EXECUTOR>_SUBMIT_INTERVAL) (default 0s)\n",
		"\n  --local-concurrency N\n",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("run --help lacks %q:\n%s", want, help)
		}
	}
	for _, line := range strings.Split(help, "\n") {
		if strings.Count(line, "(default ") > 1 || strings.Count(line, "(choices: ")+strings.Count(line, "valid values") > 1 {
			t.Errorf("run --help repeats a note: %q", line)
		}
	}
}

func TestAgentGuideRecommendsAsyncRunWithWait(t *testing.T) {
	guide := agentGuide()
	for _, want := range []string{"rotari run --async", "rotari wait", "import --dry-run", "rotari check", "wait --until-failure", "rotari lineage RUN_ID", "rotari show -j ATTEMPT_ID --report"} {
		if !strings.Contains(guide, want) {
			t.Fatalf("guide does not mention %q", want)
		}
	}
}

func TestCmdGuidePrintsGuideAndRejectsArguments(t *testing.T) {
	var output bytes.Buffer
	if code := captureShowStdout(t, &output, func() int { return run([]string{"guide"}) }); code != 0 {
		t.Fatalf("guide exit code = %d, want 0", code)
	}
	if output.String() != agentGuide() {
		t.Fatalf("guide output differs from agentGuide()")
	}
	output.Reset()
	if code := captureShowStdout(t, &output, func() int { return run([]string{"guide", "extra"}) }); code != 1 {
		t.Fatalf("guide with an argument exit code = %d, want 1", code)
	}
}

func TestTopLevelUsagePointsAgentsToGuide(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}, {"help"}} {
		wantCode := 0
		if len(args) == 0 {
			wantCode = 1
		}
		var output bytes.Buffer
		code := captureShowStdout(t, &output, func() int { return run(args) })
		if code != wantCode {
			t.Fatalf("run(%q) exit code = %d, want %d", args, code, wantCode)
		}
		if !strings.Contains(output.String(), "run `rotari guide` first") || !strings.Contains(output.String(), "Commands:") {
			t.Fatalf("run(%q) usage does not point to the guide:\n%s", args, output.String())
		}
	}
}

// TestTopLevelUsageIndexesCommandsWithoutOptions checks that top-level help
// is a one-line index of every command and leaves each command's options to
// its own --help: agents read it first, and the full synopsis of every
// command cost them several times the guide.
func TestTopLevelUsageIndexesCommandsWithoutOptions(t *testing.T) {
	var output bytes.Buffer
	if code := captureShowStdout(t, &output, func() int { return run([]string{"--help"}) }); code != 0 {
		t.Fatalf("run(--help) exit code = %d", code)
	}
	text := output.String()
	for _, command := range cliCommandSpecs {
		pattern := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(command.Name) + ` +` + regexp.QuoteMeta(command.Description) + `$`)
		if got := len(pattern.FindAllString(text, -1)); got != 1 {
			t.Errorf("help indexes command %q %d times, want once:\n%s", command.Name, got, text)
		}
	}
	if strings.Contains(text, "--config FILE") {
		t.Errorf("help lists command options:\n%s", text)
	}
}

// TestRetryDocumentsEveryOptionRunTakes compares the specs of run and retry:
// retry parses run's options, so its help, schema, and completion must list
// each of them.
func TestRetryDocumentsEveryOptionRunTakes(t *testing.T) {
	flags := func(name string) []string {
		for _, command := range cliCommandSpecs {
			if command.Name == name {
				names := make([]string, 0, len(command.Flags))
				for _, flagSpec := range command.Flags {
					names = append(names, flagSpec.Name)
				}
				sort.Strings(names)
				return names
			}
		}
		t.Fatalf("no spec for %s", name)
		return nil
	}
	if run, retry := flags("run"), flags("retry"); strings.Join(run, " ") != strings.Join(retry, " ") {
		t.Fatalf("retry documents\n  %v\nrun documents\n  %v", retry, run)
	}
}
