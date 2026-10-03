package main

import (
	"bytes"
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
				if !strings.Contains(help, "  --"+flagSpec.Name+"\n") && !strings.Contains(help, "  --"+flagSpec.Name+" ") && !strings.Contains(help, "  --"+flagSpec.Name+",") {
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
		if !strings.Contains(output.String(), "run `rotari guide` first") || !strings.Contains(output.String(), "Usage:") {
			t.Fatalf("run(%q) usage does not point to the guide:\n%s", args, output.String())
		}
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
