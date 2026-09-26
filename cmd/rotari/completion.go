package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdCompletion prints static shell completion scripts for supported shells.
func cmdCompletion(args []string) int {
	if len(args) == 0 {
		printError("usage: rotari completion <bash|zsh|fish|install [bash|zsh|fish]>")
		return 1
	}
	if isHelpArgument(args[0]) || (args[0] == "install" && len(args) == 2 && isHelpArgument(args[1])) {
		printSubcommandHelp("completion")
		return 1
	}
	if args[0] == "install" {
		if len(args) > 2 {
			printError("usage: rotari completion install [bash|zsh|fish]")
			return 1
		}
		shell := ""
		if len(args) == 2 {
			shell = args[1]
		}
		if err := installCompletion(shell); err != nil {
			printError(err)
			return 1
		}
		return 0
	}
	if len(args) != 1 {
		printError("usage: " + cliUsage("completion"))
		return 1
	}
	switch args[0] {
	case "bash":
		fmt.Print(generateBashCompletion())
	case "zsh":
		fmt.Print(generateZshCompletion())
	case "fish":
		fmt.Print(generateFishCompletion())
	default:
		printErrorf("unsupported shell: %s", args[0])
		return 1
	}
	return 0
}

// cmdComplete serves dynamic completion requests from generated shell scripts.
func cmdComplete(args []string) int {
	if len(args) == 0 || (args[0] != "project-name" && args[0] != "run-id" && args[0] != "job-id") {
		return 1
	}
	basedir, projectName, runID := "", "", ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--basedir", "-b":
			if i+1 >= len(args) {
				return 1
			}
			basedir = args[i+1]
			i++
		case "--project-name", "-p":
			if i+1 >= len(args) {
				return 1
			}
			projectName = args[i+1]
			i++
		case "--run-id", "-r":
			if i+1 >= len(args) {
				return 1
			}
			runID = args[i+1]
			i++
		}
	}
	if args[0] != "job-id" {
		runID = ""
	}
	baseDir := ""
	var err error
	if runID != "" {
		baseDir, projectName, err = resolve.ExistingRun(basedir, projectName, runID)
	} else {
		baseDir, _, err = state.ResolveBaseDir(basedir)
	}
	if err != nil {
		return 1
	}
	if args[0] == "project-name" {
		entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
		if err != nil {
			return 0
		}
		values := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				values = append(values, entry.Name())
			}
		}
		if len(values) > 0 {
			fmt.Println(strings.Join(values, "\n"))
		}
		return 0
	}
	projectName, err = state.ResolveProjectName(baseDir, projectName)
	if err != nil {
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		return 1
	}

	values := make(map[string]struct{})
	if args[0] == "run-id" {
		values["latest"] = struct{}{}
		entries, err := os.ReadDir(paths.RunsDir)
		if err != nil {
			return 0
		}
		for _, entry := range entries {
			if entry.IsDir() {
				values[entry.Name()] = struct{}{}
			}
		}
	} else if runID != "" {
		addRunJobIDs(values, filepath.Join(paths.RunsDir, runID))
	} else {
		queue, err := state.LoadQueue(paths.QueueFile)
		if err == nil {
			for _, command := range queue.Commands {
				if command.ID != "" {
					values[command.ID] = struct{}{}
				}
			}
		}
		entries, err := os.ReadDir(paths.RunsDir)
		if err == nil {
			for _, run := range entries {
				if run.IsDir() {
					addRunJobIDs(values, filepath.Join(paths.RunsDir, run.Name()))
				}
			}
		}
	}

	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) > 0 {
		fmt.Println(strings.Join(result, "\n"))
	}
	return 0
}

// addRunJobIDs adds the job directories of a run, skipping the run's config
// snapshot directory.
func addRunJobIDs(values map[string]struct{}, runDir string) {
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "configs" {
			values[entry.Name()] = struct{}{}
		}
	}
}

func installCompletion(shell string) error {
	if shell == "" {
		shell = filepath.Base(os.Getenv("SHELL"))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to find home directory: %w", err)
	}
	switch shell {
	case "bash":
		path := filepath.Join(home, ".bashrc")
		return appendCompletionBlock(path, "bash", `if command -v rotari >/dev/null 2>&1; then
    eval "$(rotari completion bash)"
fi`)
	case "zsh":
		zfuncDir := filepath.Join(home, ".zfunc")
		if err := os.MkdirAll(zfuncDir, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", zfuncDir, err)
		}
		completionPath := filepath.Join(zfuncDir, "_rotari")
		newCompletion := generateZshCompletion()
		completionChanged := true
		if existing, err := os.ReadFile(completionPath); err == nil && string(existing) == newCompletion {
			completionChanged = false
		} else {
			if err := os.WriteFile(completionPath, []byte(newCompletion), 0o644); err != nil {
				return fmt.Errorf("failed to write %s: %w", completionPath, err)
			}
		}
		rcPath := filepath.Join(home, ".zshrc")
		if err := appendCompletionBlock(rcPath, "zsh", `fpath=("$HOME/.zfunc" $fpath)
autoload -Uz compinit && compinit
autoload -Uz _rotari && compdef _rotari rotari`); err != nil {
			return err
		}
		if err := ensureZshCompletionRegistration(rcPath); err != nil {
			return err
		}
		if completionChanged {
			fmt.Printf("installed zsh completion: %s\n", completionPath)
		} else {
			fmt.Printf("zsh completion already installed: %s\n", completionPath)
		}
		return nil
	case "fish":
		completionPath := filepath.Join(home, ".config", "fish", "completions", "rotari.fish")
		if err := os.MkdirAll(filepath.Dir(completionPath), 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", filepath.Dir(completionPath), err)
		}
		newCompletion := generateFishCompletion()
		if existing, err := os.ReadFile(completionPath); err == nil && string(existing) == newCompletion {
			fmt.Printf("fish completion already installed: %s\n", completionPath)
			return nil
		}
		if err := os.WriteFile(completionPath, []byte(newCompletion), 0o644); err != nil {
			return fmt.Errorf("failed to write %s: %w", completionPath, err)
		}
		fmt.Printf("installed fish completion in %s\n", completionPath)
		return nil
	default:
		return fmt.Errorf("unsupported shell %q; specify bash, zsh, or fish", shell)
	}
}

func ensureZshCompletionRegistration(path string) error {
	const registration = "autoload -Uz _rotari && compdef _rotari rotari"
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	if strings.Contains(string(data), registration) {
		return nil
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += registration + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

func appendCompletionBlock(path, shell, block string) error {
	marker := "# rotari completion (" + shell + ")"
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	if strings.Contains(string(data), marker) {
		fmt.Printf("completion already installed in %s\n", path)
		return nil
	}
	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + marker + "\n" + block + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	fmt.Printf("installed %s completion in %s\n", shell, path)
	return nil
}

func generateBashCompletion() string {
	var builder strings.Builder
	builder.WriteString("# bash completion for rotari\n_rotari_completion() {\n")
	builder.WriteString("    local cur prev command\n    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	builder.WriteString("    prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n    command=\"${COMP_WORDS[1]}\"\n\n")
	builder.WriteString("    if [[ ${COMP_CWORD} -eq 1 ]]; then\n")
	fmt.Fprintf(&builder, "        COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", strings.Join(cliCommandNames(), " "))
	builder.WriteString("        return\n    fi\n\n    case \"$prev\" in\n")
	builder.WriteString("        --project-name|-p)\n            local -a __rotari_completion_args=()\n            local __i\n            for (( __i = 2; __i < ${#COMP_WORDS[@]}; __i++ )); do\n                case \"${COMP_WORDS[__i]}\" in\n                    --basedir|-b)\n                        if (( __i + 1 < ${#COMP_WORDS[@]} )); then\n                            __rotari_completion_args+=(\"${COMP_WORDS[__i]}\" \"${COMP_WORDS[__i+1]}\")\n                            (( __i++ ))\n                        fi\n                        ;;\n                esac\n            done\n            COMPREPLY=($(compgen -W \"$(rotari __complete project-name \"${__rotari_completion_args[@]}\" 2>/dev/null)\" -- \"$cur\"))\n            return\n            ;;\n")
	for _, command := range cliCommandSpecs {
		for _, option := range command.Flags {
			if len(option.Values) == 0 {
				continue
			}
			fmt.Fprintf(&builder, "        %s)\n            COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n            return\n            ;;\n", shellOptionPattern(option.Name), strings.Join(option.Values, " "))
		}
	}
	builder.WriteString("        --run-id|-r)\n            local -a __rotari_completion_args=()\n            local __i\n            for (( __i = 2; __i < ${#COMP_WORDS[@]}; __i++ )); do\n                case \"${COMP_WORDS[__i]}\" in\n                    --basedir|-b|--project-name|-p)\n                        if (( __i + 1 < ${#COMP_WORDS[@]} )); then\n                            __rotari_completion_args+=(\"${COMP_WORDS[__i]}\" \"${COMP_WORDS[__i+1]}\")\n                            (( __i++ ))\n                        fi\n                        ;;\n                esac\n            done\n            COMPREPLY=($(compgen -W \"$(rotari __complete run-id \"${__rotari_completion_args[@]}\" 2>/dev/null)\" -- \"$cur\"))\n            return\n            ;;\n        --job-id|-j)\n            local -a __rotari_completion_args=()\n            local __i\n            for (( __i = 2; __i < ${#COMP_WORDS[@]}; __i++ )); do\n                case \"${COMP_WORDS[__i]}\" in\n                    --basedir|-b|--project-name|-p)\n                        if (( __i + 1 < ${#COMP_WORDS[@]} )); then\n                            __rotari_completion_args+=(\"${COMP_WORDS[__i]}\" \"${COMP_WORDS[__i+1]}\")\n                            (( __i++ ))\n                        fi\n                        ;;\n                esac\n            done\n            COMPREPLY=($(compgen -W \"$(rotari __complete job-id \"${__rotari_completion_args[@]}\" 2>/dev/null)\" -- \"$cur\"))\n            return\n            ;;\n")
	completionValues := cliSubcommandNames("completion")
	fmt.Fprintf(&builder, "        completion)\n            COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n            return\n            ;;\n    esac\n\n    case \"$command\" in\n", strings.Join(completionValues, " "))
	for _, command := range cliCommandSpecs {
		if len(command.Flags) == 0 && len(command.Subcommands) == 0 {
			continue
		}
		fmt.Fprintf(&builder, "        %s)\n", command.Name)
		if len(command.Subcommands) > 0 {
			values := make([]string, 0, len(command.Subcommands))
			for _, subcommand := range command.Subcommands {
				values = append(values, subcommand.Name)
			}
			fmt.Fprintf(&builder, "            if [[ ${COMP_CWORD} -eq 2 ]]; then\n                COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n            else\n", strings.Join(values, " "))
			if len(command.Flags) > 0 {
				fmt.Fprintf(&builder, "                COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", bashOptions(command.Flags))
			} else {
				builder.WriteString("                COMPREPLY=()\n")
			}
			builder.WriteString("            fi\n")
		} else {
			fmt.Fprintf(&builder, "            COMPREPLY=($(compgen -W \"%s\" -- \"$cur\"))\n", bashOptions(command.Flags))
		}
		builder.WriteString("            ;;\n")
	}
	builder.WriteString("    esac\n}\ncomplete -F _rotari_completion rotari\n")
	completion := builder.String()
	if jobCase := strings.Index(completion, "        --job-id|-j)"); jobCase >= 0 {
		before, after := completion[:jobCase], completion[jobCase:]
		after = strings.Replace(after, "--basedir|-b|--project-name|-p)", "--basedir|-b|--project-name|-p|--run-id|-r)", 1)
		completion = before + after
	}
	return completion
}

func bashOptions(flags []cliFlagSpec) string {
	options := make([]string, 0, len(flags))
	for _, flag := range flags {
		options = append(options, "--"+flag.Name)
		if short := cliShortFlagNames[flag.Name]; short != "" {
			options = append(options, "-"+short)
		}
	}
	return strings.Join(options, " ")
}

func shellOptionPattern(name string) string {
	if short := cliShortFlagNames[name]; short != "" {
		return "--" + name + "|-" + short
	}
	return "--" + name
}

// dynamicCompletionFlags name the options whose values come from
// `rotari __complete`.
var dynamicCompletionFlags = map[string]bool{"project-name": true, "run-id": true, "job-id": true}

func generateFishCompletion() string {
	var builder strings.Builder
	builder.WriteString(fishCompletionHeader)
	for _, command := range cliCommandSpecs {
		fmt.Fprintf(&builder, "complete -c rotari -n '__rotari_needs_command' -a %s -d %s\n", fishQuote(command.Name), fishQuote(command.Description))
	}
	for _, command := range cliCommandSpecs {
		for _, subcommand := range command.Subcommands {
			fmt.Fprintf(&builder, "complete -c rotari -n %s -a %s -d %s\n", fishQuote("__rotari_needs_subcommand "+command.Name), fishQuote(subcommand.Name), fishQuote(subcommand.Description))
		}
		for _, flag := range command.Flags {
			fmt.Fprintf(&builder, "complete -c rotari -n %s ", fishQuote("__rotari_using_command "+command.Name))
			if short := cliShortFlagNames[flag.Name]; short != "" {
				fmt.Fprintf(&builder, "-s %s ", short)
			}
			fmt.Fprintf(&builder, "-l %s ", flag.Name)
			switch {
			case len(flag.Values) > 0:
				fmt.Fprintf(&builder, "-x -a %s ", fishQuote(strings.Join(flag.Values, " ")))
			case dynamicCompletionFlags[flag.Name]:
				fmt.Fprintf(&builder, "-x -a %s ", fishQuote("(__rotari_complete "+flag.Name+")"))
			case flag.ValueName != "":
				builder.WriteString("-r ")
			}
			fmt.Fprintf(&builder, "-d %s\n", fishQuote(flag.Description))
		}
	}
	return builder.String()
}

// fishCompletionHeader scopes candidates by the command in the second word,
// like the Bash and Zsh scripts. __rotari_complete passes the location
// options already on the command line to `rotari __complete`; job IDs also
// follow --run-id/-r.
const fishCompletionHeader = `# rotari completion (fish)
function __rotari_needs_command
    test (count (commandline -opc)) -eq 1
end
function __rotari_needs_subcommand
    set -l tokens (commandline -opc)
    test (count $tokens) -eq 2; and test "$tokens[2]" = "$argv[1]"
end
function __rotari_using_command
    set -l tokens (commandline -opc)
    test (count $tokens) -ge 2; and test "$tokens[2]" = "$argv[1]"
end
function __rotari_complete
    set -l tokens (commandline -opc)
    set -l args
    set -l i 3
    while test $i -lt (count $tokens)
        set -l next (math $i + 1)
        switch $tokens[$i]
            case --basedir -b --project-name -p
                set -a args $tokens[$i] $tokens[$next]
                set i $next
            case --run-id -r
                if test "$argv[1]" = job-id
                    set -a args $tokens[$i] $tokens[$next]
                end
                set i $next
        end
        set i (math $i + 1)
    end
    rotari __complete $argv[1] $args 2>/dev/null
end
complete -c rotari -f
`

// fishQuote single-quotes s for fish, which escapes only backslash and
// single quote inside single quotes.
func fishQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

func generateZshCompletion() string {
	var builder strings.Builder
	builder.WriteString(zshCompletionHeader)
	writeZshCommandDescriptions(&builder)
	writeZshCommandCases(&builder)
	builder.WriteString(zshCompletionFooter)
	return builder.String()
}

// zshCompletionHeader defines the dynamic value actions.
// _rotari_complete_values passes the location options already on the command
// line to `rotari __complete`; job IDs also follow --run-id/-r.
const zshCompletionHeader = `#compdef rotari

_rotari_complete_values() {
    local -a args values
    local i
    for (( i = 1; i + 1 < CURRENT; i++ )); do
        case $words[i] in
            --basedir|-b|--project-name|-p)
                args+=("$words[i]" "$words[i+1]")
                (( i++ ))
                ;;
            --run-id|-r)
                [[ $1 == job-id ]] && args+=("$words[i]" "$words[i+1]")
                (( i++ ))
                ;;
        esac
    done
    values=("${(@f)$(rotari __complete "$1" "${args[@]}" 2>/dev/null)}")
    compadd -- "${(@)values:#}"
}
_rotari_project_names() { _rotari_complete_values project-name }
_rotari_run_ids() { _rotari_complete_values run-id }
_rotari_job_ids() { _rotari_complete_values job-id }

_rotari() {
    local -a commands
    commands=(
`

func writeZshCommandDescriptions(builder *strings.Builder) {
	for _, command := range cliCommandSpecs {
		fmt.Fprintf(builder, "        '%s:%s'\n", zshQuote(command.Name), zshQuote(command.Description))
	}
	// Dropping the program name lets _arguments see the command as its
	// words[1], as it expects.
	builder.WriteString("    )\n\n    if (( CURRENT == 2 )); then\n        _describe 'command' commands\n        return\n    fi\n    local command=$words[2]\n    shift words\n    (( CURRENT-- ))\n\n    case $command in\n")
}

func writeZshCommandCases(builder *strings.Builder) {
	subcommandIndex := 0
	for _, command := range cliCommandSpecs {
		if len(command.Flags) == 0 && len(command.Subcommands) == 0 {
			continue
		}
		fmt.Fprintf(builder, "        %s)\n", command.Name)
		if len(command.Subcommands) > 0 {
			writeZshSubcommandCase(builder, command, subcommandIndex)
			subcommandIndex++
		} else {
			writeZshFlagCase(builder, command)
		}
		builder.WriteString("            ;;\n")
	}
	builder.WriteString("    esac\n}\n")
}

func writeZshSubcommandCase(builder *strings.Builder, command cliCommandSpec, subcommandIndex int) {
	arrayName := fmt.Sprintf("subcommands%d", subcommandIndex)
	fmt.Fprintf(builder, "            local -a %s\n            %s=(\n", arrayName, arrayName)
	for _, subcommand := range command.Subcommands {
		fmt.Fprintf(builder, "                '%s:%s'\n", zshQuote(subcommand.Name), zshQuote(subcommand.Description))
	}
	fmt.Fprintf(builder, "            )\n            if (( CURRENT == 2 )); then\n                _describe 'subcommand' %s\n                return\n            fi\n", arrayName)
	if len(command.Flags) > 0 {
		fmt.Fprintf(builder, "            shift words\n            (( CURRENT-- ))\n            _arguments %s\n", zshArguments(command.Flags))
	}
}

func writeZshFlagCase(builder *strings.Builder, command cliCommandSpec) {
	arguments := zshArguments(command.Flags)
	if strings.HasPrefix(command.Positional, "<command") {
		arguments += " '*:command:_command_names'"
	} else if command.Positional != "" {
		arguments += " '*:" + zshEscapeSpec(command.Positional) + ":'"
	}
	fmt.Fprintf(builder, "            _arguments %s\n", arguments)
}

// zshCompletionFooter completes on the first call when compinit autoloads
// this file as the body of _rotari, and registers _rotari when the file is
// sourced instead.
const zshCompletionFooter = `
if [[ $zsh_eval_context[-1] == loadautofunc ]]; then
    _rotari "$@"
elif (( $+functions[compdef] )); then
    compdef _rotari rotari
fi
`

func zshArguments(flags []cliFlagSpec) string {
	arguments := make([]string, 0, len(flags))
	for _, flag := range flags {
		option := "'--" + flag.Name
		if short := cliShortFlagNames[flag.Name]; short != "" {
			option = "{-" + short + ",--" + flag.Name + "}'"
		}
		if flag.Repeated {
			// A repeatable option stays offered after its first use.
			option = "'*'" + strings.TrimPrefix(option, "'")
		}
		valueName := zshEscapeSpec(flag.ValueName)
		argument := fmt.Sprintf("%s[%s]", option, zshEscapeSpec(flag.Description))
		if len(flag.Values) > 0 {
			argument += ":" + valueName + ":(" + strings.Join(flag.Values, " ") + ")"
		} else if dynamicCompletionFlags[flag.Name] {
			action := "_rotari_" + strings.ReplaceAll(flag.Name, "-", "_") + "s"
			argument += ":" + valueName + ":" + action
		} else if flag.ValueName != "" {
			argument += ":" + valueName + ":"
		}
		arguments = append(arguments, argument+"'")
	}
	return strings.Join(arguments, " ")
}

// zshEscapeSpec escapes characters that are structurally significant either
// to zsh's _arguments option-spec parser ("[", "]", ":") or to the enclosing
// single-quoted shell string ("'") so arbitrary flag descriptions and value
// names cannot break completion script generation.
func zshEscapeSpec(s string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"[", "\\[",
		"]", "\\]",
		":", "\\:",
	)
	return zshQuote(replacer.Replace(s))
}

// zshQuote escapes a single quote for safe embedding inside a zsh
// single-quoted string, using the standard close-escape-reopen technique.
func zshQuote(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}
