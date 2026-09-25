package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestAgentGuideCoversEveryCommandAndFlag(t *testing.T) {
	guide := agentGuide()
	for _, command := range cliCommandSpecs {
		if !strings.Contains(guide, "\n### "+command.Name+"\n") {
			t.Fatalf("guide does not describe command %q", command.Name)
		}
		if !strings.Contains(guide, "\n"+cliUsage(command.Name)+"\n") {
			t.Fatalf("guide does not contain usage for %q", command.Name)
		}
		for _, flagSpec := range command.Flags {
			if !strings.Contains(guide, "`--"+flagSpec.Name) {
				t.Fatalf("guide does not describe flag --%s of %q", flagSpec.Name, command.Name)
			}
		}
	}
}

func TestAgentGuideListsCommonOptionsOnlyWhenCommandHasAll(t *testing.T) {
	guide := agentGuide()
	section := func(name string) string {
		start := strings.Index(guide, "\n### "+name+"\n")
		if start < 0 {
			t.Fatalf("guide has no section for %q", name)
		}
		rest := guide[start+1:]
		if end := strings.Index(rest, "\n### "); end >= 0 {
			rest = rest[:end]
		}
		return rest
	}
	show := section("show")
	if !strings.Contains(show, "Accepts the common options.") || strings.Contains(show, "- `--basedir DIR`") {
		t.Fatalf("show section should refer to the common options instead of listing them:\n%s", show)
	}
	server := section("server")
	if strings.Contains(server, "Accepts the common options.") || !strings.Contains(server, "- `--basedir DIR`") {
		t.Fatalf("server section should list --basedir because it lacks --project-name:\n%s", server)
	}
}

func TestAgentGuideRecommendsAsyncRunWithWait(t *testing.T) {
	guide := agentGuide()
	for _, want := range []string{"rotari run --async", "rotari wait", "import --dry-run", "rotari check"} {
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
