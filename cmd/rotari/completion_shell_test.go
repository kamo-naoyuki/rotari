package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestShellCompletionCandidates drives the generated bash, zsh, and fish
// scripts through each shell's own completion machinery and compares the
// offered candidates with cliCommandSpecs: every command name, every
// subcommand, every command's options, every fixed option value, and the
// dynamic project, run, and job IDs served by `rotari __complete`.
func TestShellCompletionCandidates(t *testing.T) {
	if testing.Short() {
		t.Skip("builds rotari and starts shells")
	}
	binDir := buildCompletionBinary(t)
	fixture := newCompletionFixture(t, binDir)
	cases := completionCases(fixture)

	for _, shell := range []struct {
		name     string
		complete func(t *testing.T, binDir string, lines [][]string) [][]string
	}{
		{"bash", completeWithBash},
		{"zsh", completeWithZsh},
		{"fish", completeWithFish},
	} {
		t.Run(shell.name, func(t *testing.T) {
			if _, err := exec.LookPath(shell.name); err != nil {
				t.Skipf("%s not installed", shell.name)
			}
			lines := make([][]string, len(cases))
			for i, c := range cases {
				lines[i] = c.words
			}
			results := shell.complete(t, binDir, lines)
			for i, c := range cases {
				got := normalizeCandidates(results[i])
				want := normalizeCandidates(c.want)
				if !slices.Equal(got, want) {
					t.Errorf("%s: rotari %s<TAB>\n got: %q\nwant: %q", c.name, strings.Join(c.words, " "), got, want)
				}
			}
		})
	}
}

type completionCase struct {
	name string
	// words follow "rotari"; the last one is the word being completed.
	words []string
	want  []string
}

func TestListingCompletionScopes(t *testing.T) {
	binDir := buildCompletionBinary(t)
	stateHome := filepath.Join(binDir, "state")
	defaultBase := filepath.Join(stateHome, "rotari")
	envBase := filepath.Join(binDir, "env-state")
	registeredBase := filepath.Join(binDir, "registered-state")
	masterDir := filepath.Join(binDir, "master")
	register := exec.Command(filepath.Join(binDir, "rotari"), "add", "-b", registeredBase, "-p", "registry-only", "--", "true")
	register.Dir = binDir
	register.Env = append(completionShellEnv(binDir), "ROTARI_MASTERDIR="+masterDir)
	if out, err := register.CombinedOutput(); err != nil {
		t.Fatalf("register completion fixture: %v\n%s", err, out)
	}
	for _, item := range []struct{ base, project string }{{defaultBase, "default-only"}, {envBase, "env-only"}} {
		if err := os.MkdirAll(filepath.Join(item.base, "projects", item.project), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// An ignored location must not break completion before candidates load.
	if err := os.WriteFile(filepath.Join(envBase, "config.toml"), []byte("retry = [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "runs default", args: []string{"__complete", "project-name", "--command", "runs"}, want: "default-only\n"},
		{name: "runs explicit", args: []string{"__complete", "project-name", "--command", "runs", "-b", defaultBase}, want: "default-only\n"},
		{name: "runs other explicit", args: []string{"__complete", "project-name", "--command", "runs", "-b", registeredBase}, want: "registry-only\n"},
		{name: "runs all", args: []string{"__complete", "project-name", "--command", "runs", "--all-basedirs"}, want: "registry-only\n"},
		{name: "projects registry and default", args: []string{"__complete", "project-name", "--command", "projects"}, want: "default-only\nregistry-only\n"},
		{name: "projects explicit", args: []string{"__complete", "project-name", "--command", "projects", "-b", defaultBase}, want: "default-only\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command(filepath.Join(binDir, "rotari"), test.args...)
			cmd.Dir = binDir
			cmd.Env = append(completionShellEnv(binDir), "XDG_STATE_HOME="+stateHome, "ROTARI_BASEDIR="+envBase, "ROTARI_PROJECT_NAME=env-only", "ROTARI_MASTERDIR="+masterDir)
			out, err := cmd.CombinedOutput()
			if err != nil || string(out) != test.want {
				t.Fatalf("completion = %q, %v; want %q", out, err, test.want)
			}
		})
	}
	// Exercise the source-command plumbing in all generated scripts as well.
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("ROTARI_BASEDIR", envBase)
	// completionShellEnv deliberately strips ROTARI_*; put the ignored
	// basedir in global config so each shell receives the same default.
	if err := os.WriteFile(filepath.Join(binDir, "config", "rotari", "config.toml"), []byte("basedir = '"+envBase+"'\nmasterdir = '"+masterDir+"'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, shell := range []struct {
		name     string
		complete func(*testing.T, string, [][]string) [][]string
	}{{"bash", completeWithBash}, {"zsh", completeWithZsh}, {"fish", completeWithFish}} {
		t.Run(shell.name, func(t *testing.T) {
			if _, err := exec.LookPath(shell.name); err != nil {
				t.Skipf("%s not installed", shell.name)
			}
			lines := [][]string{{"runs", "-p", ""}, {"runs", "--all-basedirs", "-p", ""}, {"runs", "-b", registeredBase, "-p", ""}}
			got := shell.complete(t, binDir, lines)
			for i, want := range []string{"default-only", "registry-only", "registry-only"} {
				if !slices.Equal(normalizeCandidates(got[i]), []string{want}) {
					t.Errorf("completion %q = %q, want %s", lines[i], got[i], want)
				}
			}
		})
	}
}

type completionFixture struct {
	baseDir string
	// project alpha has one run (runID) of jobs runJobs, then queuedJobs.
	runID      string
	runJobs    []string
	queuedJobs []string
	projects   []string
}

// completionCases derives the expected candidates from cliCommandSpecs, so a
// new command, option, or option value is covered without a new case.
func completionCases(fixture completionFixture) []completionCase {
	b, runID := fixture.baseDir, fixture.runID
	location := []string{"-b", b, "-p", "alpha"}
	allJobs := append(append([]string{}, fixture.runJobs...), fixture.queuedJobs...)
	cases := []completionCase{
		// zsh autoloads _rotari from fpath on the first completion.
		{name: "first completion", words: []string{""}, want: cliCommandNames()},
		{name: "commands", words: []string{""}, want: cliCommandNames()},
	}
	for _, command := range cliCommandSpecs {
		prefix := []string{command.Name}
		if len(command.Subcommands) > 0 {
			names := make([]string, 0, len(command.Subcommands))
			for _, subcommand := range command.Subcommands {
				names = append(names, subcommand.Name)
			}
			cases = append(cases, completionCase{name: command.Name + " subcommands", words: []string{command.Name, ""}, want: names})
			prefix = append(prefix, command.Subcommands[0].Name)
		}
		if len(command.Flags) == 0 {
			continue
		}
		longOptions := make([]string, 0, len(command.Flags))
		for _, flag := range command.Flags {
			longOptions = append(longOptions, "--"+flag.Name)
		}
		cases = append(cases, completionCase{name: command.Name + " options", words: with(prefix, "--"), want: longOptions})
		for _, flag := range command.Flags {
			options := []string{"--" + flag.Name}
			if short := cliShortFlagNames[flag.Name]; short != "" {
				options = append(options, "-"+short)
			}
			for _, option := range options {
				name := command.Name + " " + option
				switch {
				case len(flag.Values) > 0:
					cases = append(cases, completionCase{name: name, words: with(prefix, option, ""), want: flag.Values})
				case flag.Name == "project-name":
					cases = append(cases, completionCase{name: name, words: with(prefix, "-b", b, option, ""), want: fixture.projects})
				case flag.Name == "run-id":
					cases = append(cases, completionCase{name: name, words: with(append(prefix, location...), option, ""), want: []string{"latest", runID}})
				case flag.Name == "job-id":
					cases = append(cases, completionCase{name: name, words: with(append(prefix, location...), option, ""), want: allJobs})
					cases = append(cases, completionCase{name: name + " of a run", words: with(append(prefix, location...), "--run-id", runID, option, ""), want: fixture.runJobs})
				}
			}
		}
	}
	return cases
}

func with(prefix []string, words ...string) []string {
	return append(append([]string{}, prefix...), words...)
}

func normalizeCandidates(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func buildCompletionBinary(t *testing.T) string {
	t.Helper()
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not installed")
	}
	dir := t.TempDir()
	out, err := exec.Command(goPath, "build", "-o", filepath.Join(dir, "rotari"), ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	// A config file makes each run keep a config snapshot directory beside
	// its job directories.
	configDir := filepath.Join(dir, "config", "rotari")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newCompletionFixture(t *testing.T, binDir string) completionFixture {
	t.Helper()
	fixture := completionFixture{baseDir: t.TempDir(), projects: []string{"alpha", "beta"}}
	jobIDPattern := regexp.MustCompile(`job_id=(\S+)`)
	add := func(project string, args ...string) string {
		out := runCompletionRotari(t, binDir, append([]string{"add", "-b", fixture.baseDir, "-p", project}, args...)...)
		match := jobIDPattern.FindStringSubmatch(out)
		if match == nil {
			t.Fatalf("add printed no job ID: %s", out)
		}
		return match[1]
	}
	fixture.runJobs = append(fixture.runJobs, add("alpha", "--job-name", "one", "--", "true"), add("alpha", "--", "true"))
	runCompletionRotari(t, binDir, "run", "-b", fixture.baseDir, "-p", "alpha")
	fixture.queuedJobs = append(fixture.queuedJobs, add("alpha", "--job-name", "two", "--", "true"))
	add("beta", "--", "true")
	runs, err := os.ReadDir(filepath.Join(fixture.baseDir, "projects", "alpha", "runs"))
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs of alpha = %v, %v; want one", runs, err)
	}
	fixture.runID = runs[0].Name()
	return fixture
}

func runCompletionRotari(t *testing.T, binDir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(binDir, "rotari"), args...)
	cmd.Env = completionShellEnv(binDir)
	// IDs are parsed from success output, not from warnings about other jobs.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rotari %s: %v\n%s\n%s", strings.Join(args, " "), err, out, stderr.String())
	}
	return string(out)
}

// completionShellEnv puts the built rotari first on PATH, points HOME and
// the config directory into binDir, and drops rotari settings of the calling
// environment.
func completionShellEnv(binDir string) []string {
	set := []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + binDir,
		"XDG_CONFIG_HOME=" + filepath.Join(binDir, "config"),
		"TERM=dumb",
		"LANG=C",
	}
	env := append([]string{}, set...)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		replaced := slices.ContainsFunc(set, func(value string) bool { return strings.HasPrefix(value, name+"=") })
		if !replaced && !strings.HasPrefix(name, "ROTARI_") {
			env = append(env, entry)
		}
	}
	return env
}

const (
	completionCandidateMarker = "<CAND>"
	completionEndMarker       = "<END>"
)

// runCompletionShell runs a shell script that prints candidates as
// "<CAND>value" and ends each completion with "<END>", and returns the
// candidates of each completion.
func runCompletionShell(t *testing.T, binDir string, want int, name string, args ...string) [][]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = completionShellEnv(binDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
	candidate := regexp.MustCompile(regexp.QuoteMeta(completionCandidateMarker) + `([^\r\n]*)`)
	blocks := strings.Split(strings.ReplaceAll(string(out), "\r", ""), completionEndMarker)
	if len(blocks) != want+1 {
		t.Fatalf("%s ran %d completions, want %d\n%s", name, len(blocks)-1, want, out)
	}
	results := make([][]string, want)
	for i := range results {
		for _, match := range candidate.FindAllStringSubmatch(blocks[i], -1) {
			// fish prints "value<TAB>description".
			value, _, _ := strings.Cut(match[1], "\t")
			results[i] = append(results[i], value)
		}
	}
	return results
}

func completeWithBash(t *testing.T, binDir string, lines [][]string) [][]string {
	scriptPath := filepath.Join(t.TempDir(), "rotari.bash")
	if err := os.WriteFile(scriptPath, []byte(generateBashCompletion()), 0o644); err != nil {
		t.Fatal(err)
	}
	var script strings.Builder
	fmt.Fprintf(&script, "source %s\n", quoteForShell(scriptPath))
	fmt.Fprintf(&script, "__case() { COMP_WORDS=(\"$@\"); COMP_CWORD=$(( $# - 1 )); COMPREPLY=(); _rotari_completion; printf '%s%%s\\n' \"${COMPREPLY[@]}\"; printf '%s\\n'; }\n", completionCandidateMarker, completionEndMarker)
	for _, words := range lines {
		script.WriteString("__case rotari")
		for _, word := range words {
			script.WriteString(" " + quoteForShell(word))
		}
		script.WriteString("\n")
	}
	return runCompletionShell(t, binDir, len(lines), "bash", "--norc", "--noprofile", "-c", script.String())
}

func completeWithFish(t *testing.T, binDir string, lines [][]string) [][]string {
	scriptPath := filepath.Join(t.TempDir(), "rotari.fish")
	if err := os.WriteFile(scriptPath, []byte(generateFishCompletion()), 0o644); err != nil {
		t.Fatal(err)
	}
	fishQuote := func(s string) string {
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
	}
	var script strings.Builder
	fmt.Fprintf(&script, "source %s\n", fishQuote(scriptPath))
	for _, words := range lines {
		line := "rotari " + strings.Join(words, " ")
		fmt.Fprintf(&script, "printf '%s%%s\\n' (complete -C %s); printf '%s\\n'\n", completionCandidateMarker, fishQuote(line), completionEndMarker)
	}
	return runCompletionShell(t, binDir, len(lines), "fish", "--no-config", "-c", script.String())
}

// completeWithZsh installs _rotari the way `rotari completion install zsh`
// does (autoloaded from fpath by compinit) and presses TAB in an interactive
// zsh on a pseudo-terminal, recording what completion functions pass to
// compadd.
func completeWithZsh(t *testing.T, binDir string, lines [][]string) [][]string {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "_rotari"), []byte(generateZshCompletion()), 0o644); err != nil {
		t.Fatal(err)
	}
	var input strings.Builder
	for _, words := range lines {
		input.WriteString("rotari " + strings.Join(words, " ") + "\n")
	}
	inputPath := filepath.Join(dir, "lines")
	if err := os.WriteFile(inputPath, []byte(input.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return runCompletionShell(t, binDir, len(lines), "zsh", "-f", "-c", zshCompletionDriver, "zsh", dir, inputPath)
}

// zshCompletionDriver takes the fpath directory and a file of command lines.
// The markers are split in the typed definitions so the echoed input never
// contains them.
const zshCompletionDriver = `
zmodload zsh/zpty || exit 1
zpty z zsh -f -i
zpty -w z "PROMPT=; RPROMPT=; unset zle_bracketed_paste; unsetopt auto_list; LISTMAX=100000; fpath=(${(q)1} \$fpath); autoload -Uz compinit; compinit -u -D"
zpty -w z 'compadd() { if [[ ${@[1,(i)(-|--)]} == *-(O|A|D)\ * ]]; then builtin compadd "$@"; return; fi; local -a __h; builtin compadd -A __h "$@"; local x; for x in $__h; do print -r -- "<C""AND>$x"; done; builtin compadd "$@" }'
zpty -w z '_rotari_done() { print -r -- "<E""ND>"; BUFFER=; }; zle -N _rotari_done; bindkey "^T" _rotari_done; bindkey "^I" complete-word'
for line in "${(@f)$(<$2)}"; do
    zpty -w -n z "$line"$'\t\x14'
    while zpty -r z chunk; do
        print -rn -- "$chunk"
        [[ $chunk == *'<END>'* ]] && break
    done
done
zpty -d z
`
