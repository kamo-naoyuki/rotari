package main

import (
	"flag"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/diagnose"
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	webprojection "github.com/kamo-naoyuki/rotari/internal/web"
)

func TestCLIFlagSpecTracksRepeatedMetadata(t *testing.T) {
	spec := cliFlagSpec{Name: "executor-option", Repeated: true}
	if !spec.Repeated {
		t.Fatal("cliFlagSpec repeated metadata was not retained")
	}
}

func TestCmdSchemaValidAndInvalidArguments(t *testing.T) {
	if code := cmdSchema(nil); code != 1 {
		t.Fatalf("cmdSchema(nil) = %d, want 1", code)
	}

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdSchema([]string{"--json"})
	_ = writer.Close()
	os.Stdout = oldStdout
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || !strings.Contains(string(data), `"commands"`) || !strings.Contains(string(data), `"project-name"`) || !strings.Contains(string(data), `"slurm-submit-interval"`) {
		t.Fatalf("cmdSchema(--json) = %d, output = %s", code, data)
	}
}

func TestRunDispatchesTopLevelCommands(t *testing.T) {
	if code := run(nil); code != 1 {
		t.Fatalf("run(nil) = %d, want 1", code)
	}
	for _, command := range []string{"version", "--version"} {
		if code := run([]string{command}); code != 0 {
			t.Fatalf("run(%q) = %d, want 0", command, code)
		}
	}
	if code := run([]string{"schema", "--json"}); code != 0 {
		t.Fatalf("run(schema --json) = %d, want 0", code)
	}
	if code := run([]string{"clear", "--basedir", t.TempDir()}); code != 1 {
		t.Fatalf("run(unknown command) = %d, want 1", code)
	}
}

func TestCmdCompletionValidatesArgumentsAndGeneratesScripts(t *testing.T) {
	if code := cmdCompletion(nil); code != 1 {
		t.Fatalf("cmdCompletion(nil) = %d, want 1", code)
	}
	if code := cmdCompletion([]string{"bash", "extra"}); code != 1 {
		t.Fatalf("cmdCompletion with extra arguments = %d, want 1", code)
	}
	if code := cmdCompletion([]string{"unknown"}); code != 1 {
		t.Fatalf("cmdCompletion with unknown shell = %d, want 1", code)
	}
	if code := cmdCompletion([]string{"install", "bash", "extra"}); code != 1 {
		t.Fatalf("cmdCompletion install with extra arguments = %d, want 1", code)
	}

	for _, shell := range []string{"bash", "zsh", "fish"} {
		if code := cmdCompletion([]string{shell}); code != 0 {
			t.Fatalf("cmdCompletion(%q) = %d, want 0", shell, code)
		}
	}
}

func captureStderr(t *testing.T, run func() int) (int, string) {
	t.Helper()
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	code := run()
	os.Stderr = oldStderr
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(output)
}

func TestSubcommandCommandsPrintHelp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cases := []struct {
		name        string
		run         func([]string) int
		args        []string
		subcommands []string
	}{
		{"completion --help", cmdCompletion, []string{"--help"}, []string{"bash", "zsh", "fish", "install"}},
		{"completion -h", cmdCompletion, []string{"-h"}, []string{"bash", "install"}},
		{"completion install --help", cmdCompletion, []string{"install", "--help"}, []string{"install"}},
		{"server --help", cmdServer, []string{"--help"}, []string{"status", "list", "shutdown"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, text := captureStderr(t, func() int { return tc.run(tc.args) })
			if code != 1 {
				t.Fatalf("exit code = %d, want 1 like FlagSet help", code)
			}
			if strings.Contains(text, "unsupported shell") || strings.Contains(text, "unknown server command") {
				t.Fatalf("help argument was treated as a subcommand:\n%s", text)
			}
			if !strings.Contains(text, "usage: rotari ") || !strings.Contains(text, "Subcommands:") {
				t.Fatalf("help output missing usage or subcommands:\n%s", text)
			}
			for _, subcommand := range tc.subcommands {
				if !strings.Contains(text, "  "+subcommand+" ") {
					t.Fatalf("help output missing subcommand %q:\n%s", subcommand, text)
				}
			}
		})
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("help must not install completion; HOME contains %d entries", len(entries))
	}
}

func TestColorMessageCoversStatusAndFailureBranches(t *testing.T) {
	message := strings.Join([]string{
		"Run failed: exit status 1",
		"Run finished: yes",
		"Retrying job: job=one",
		"Failed job output:",
		"stderr: failed",
		"Run started at: now",
		"Inspect logs",
		"plain: value",
	}, "\n")
	if got := colorMessage(message); got != message {
		t.Fatalf("colorMessage changed non-terminal output without a TTY: %q", got)
	}
}

func TestFormatRunAIReportIncludesFailedJobsOnly(t *testing.T) {
	run := webprojection.Run{
		RunSummary: model.RunSummary{RunID: "run-1", Status: "failed", ExitCode: 1, StartedAt: "start", FinishedAt: "finish"},
		CWD:        "/work/project",
		Jobs: []webprojection.Job{
			{ID: "failed", Name: "failed-job", Result: &model.JobResult{ID: "failed", ExitCode: 1, Error: "boom"}},
			{ID: "success", Name: "success-job", Result: &model.JobResult{ID: "success", ExitCode: 0}},
		},
	}
	paths := state.ProjectPaths{ProjectName: "demo"}
	report := formatRunAIReport(paths, run, true)
	if !strings.Contains(report, "failed-job") || strings.Contains(report, "success-job") {
		t.Fatalf("failed-only report = %s", report)
	}
}

func TestConfigOptionNamesExcludesConfigOnlyOptions(t *testing.T) {
	names := configOptionNames()
	if !sort.StringsAreSorted(names) {
		t.Fatalf("config option names are not sorted: %v", names)
	}
	for _, name := range names {
		if name == "output" {
			t.Fatalf("config-only option %q leaked into configOptionNames", name)
		}
	}
	if len(names) == 0 {
		t.Fatal("configOptionNames returned no options")
	}
}

func TestCLIStringVarRegistersLongAndShortFlags(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	value := ""
	cliStringVar(flags, &value, "project-name", "default")
	if err := flags.Parse([]string{"-p", "demo"}); err != nil {
		t.Fatal(err)
	}
	if value != "demo" {
		t.Fatalf("parsed project name = %q, want demo", value)
	}
}

func TestConfigSectionsGroupsNonCommonOptions(t *testing.T) {
	sections, common := configSections()
	if len(common) != 3 || common[0] != "basedir" || common[1] != "project-name" || common[2] != "quiet" {
		t.Fatalf("common config options = %v", common)
	}
	if _, ok := sections["config"]; ok {
		t.Fatal("config command appeared as a config section")
	}
	if len(sections["run"]) == 0 {
		t.Fatal("run section has no options")
	}
	for _, name := range sections["run"] {
		if name == "quiet" {
			return
		}
	}
	t.Fatal("run section has no quiet option")
}

func TestCLIChoiceValueAndDescription(t *testing.T) {
	target := ""
	choice := &cliChoiceValue{target: &target, choices: []string{"local", "slurm"}}
	if err := choice.Set("slurm"); err != nil || target != "slurm" || choice.String() != "slurm" {
		t.Fatalf("choice = %q, target = %q, err = %v", choice.String(), target, err)
	}
	if err := choice.Set("unknown"); err == nil {
		t.Fatal("invalid choice was accepted")
	}
	description := cliFlagDescription(cliFlagSpec{Name: "executor", Description: "executor", Values: []string{"local", "slurm"}})
	if !strings.Contains(description, "choices: local, slurm") || !strings.Contains(description, "ROTARI_EXECUTOR") {
		t.Fatalf("description = %q", description)
	}
}

func TestPagerAndLogFollowDecisions(t *testing.T) {
	if shouldFollowLogs(false, true, false) || !shouldFollowLogs(false, true, true) || !shouldFollowLogs(true, false, false) {
		t.Fatal("shouldFollowLogs returned an unexpected decision")
	}
}

func TestPagerLineLimitAndLoopbackHostDecisions(t *testing.T) {
	if !exceedsPagerLineLimit([]byte("data"), pagerLineLimit+1) {
		t.Fatal("pager line limit was not detected")
	}
	if exceedsPagerLineLimit([]byte("data\n"), pagerLineLimit) {
		t.Fatal("completed line at the limit was incorrectly exceeded")
	}
	for _, host := range []string{"", "localhost", "127.0.0.1", "::1"} {
		if !isLoopbackWebHost(host) {
			t.Fatalf("host %q was not recognized as loopback", host)
		}
	}
	if isLoopbackWebHost("example.com") {
		t.Fatal("non-loopback host was accepted")
	}
}

func TestZshCompletionEscapingAndOptions(t *testing.T) {
	if got := zshEscapeSpec("a:b[c]"); !strings.Contains(got, `\:`) || !strings.Contains(got, `\[`) {
		t.Fatalf("escaped zsh spec = %q", got)
	}
	options := zshOptionNames([]cliFlagSpec{{Name: "project-name"}, {Name: "run-id"}})
	if options != "--project-name -p --run-id -r" {
		t.Fatalf("zsh option names = %q", options)
	}
}

func TestDiagnosisFormattingAndLanguageValidation(t *testing.T) {
	if got := formatRuleDiagnoses(nil); !strings.Contains(got, "No known rule-based diagnosis") {
		t.Fatalf("empty diagnosis output = %q", got)
	}
	got := formatRuleDiagnoses([]model.RuleDiagnosis{{Name: "Rule", Evidence: "evidence", Suggestion: "next"}})
	if !strings.Contains(got, "Rule\nEvidence: evidence\nNext: next") {
		t.Fatalf("diagnosis output = %q", got)
	}
	for _, tag := range []string{"en", "ja-JP", "zh-Hant"} {
		if !diagnose.IsLanguageTag(tag) {
			t.Fatalf("language tag %q was rejected", tag)
		}
	}
	for _, tag := range []string{"e", "en_", "en--US", "english"} {
		if diagnose.IsLanguageTag(tag) {
			t.Fatalf("invalid language tag %q was accepted", tag)
		}
	}
}

func TestReportStatusAndValueHelpers(t *testing.T) {
	if got := reportJobStatus(webprojection.Job{SchedulerState: "pending"}, false); got != "pending" {
		t.Fatalf("scheduler status = %q", got)
	}
	if got := reportJobStatus(webprojection.Job{}, true); got != "running" {
		t.Fatalf("running status = %q", got)
	}
	if got := reportJobStatus(webprojection.Job{}, false); got != "pending" {
		t.Fatalf("pending status = %q", got)
	}
	if got := reportJobStatus(webprojection.Job{Result: &model.JobResult{Error: "blocked by dependency"}}, false); got != "blocked" {
		t.Fatalf("blocked status = %q", got)
	}
	if got := reportJobStatus(webprojection.Job{Result: &model.JobResult{ExitCode: 0}}, false); got != "success" {
		t.Fatalf("success status = %q", got)
	}
	if got := reportJobStatus(webprojection.Job{Result: &model.JobResult{ExitCode: 1}}, false); got != "failed" {
		t.Fatalf("failed status = %q", got)
	}
	if reportValue("") != "-" || reportValue("value") != "value" || firstNonEmpty("", "value") != "value" || firstNonEmpty("", "") != "" {
		t.Fatal("report value helpers returned unexpected results")
	}
}

func TestShellQuoteAndConfigTemplateFormats(t *testing.T) {
	if got := executor.ShellQuote("it's safe"); got != `'it'\''s safe'` {
		t.Fatalf("shellQuote = %q", got)
	}
	for _, format := range []string{"yaml", "json", "toml"} {
		data, err := configTemplate(format)
		if err != nil || len(data) == 0 {
			t.Fatalf("configTemplate(%q) = %q, %v", format, data, err)
		}
	}
	if _, err := configTemplate("ini"); err == nil {
		t.Fatal("unsupported config format was accepted")
	}
}

func TestConfigFormatMatchesAndNewlineHelpers(t *testing.T) {
	for output, want := range map[string]string{
		"config.yaml": "yaml",
		"config.yml":  "yaml",
		"config.toml": "toml",
		"config.json": "json",
		"config":      "toml",
	} {
		if got := configFormatFromOutput(output); got != want {
			t.Fatalf("configFormatFromOutput(%q) = %q, want %q", output, got, want)
		}
	}
	patterns := []*regexp.Regexp{regexp.MustCompile(`error`), regexp.MustCompile(`failed`)}
	if !diagnose.MatchesAny("job failed", patterns) || diagnose.MatchesAny("job succeeded", patterns) {
		t.Fatal("matchesAny returned an unexpected result")
	}
	if newline("line\n") != "\n" || newline("line") != "" {
		t.Fatal("newline returned an unexpected result")
	}
}
