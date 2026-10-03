package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"
)

// agentGuideIntro holds the hand-written workflow rules. The command index
// that follows it is generated from cliCommandSpecs so it cannot drift from
// the CLI; each command's options are in its --help (writeCommandHelp).
//
//go:embed assets/agent_guide.md
var agentGuideIntro string

// cmdGuide prints the usage guide for coding agents as Markdown.
func cmdGuide(args []string) int {
	fs := flag.NewFlagSet("guide", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: "+cliUsage("guide")) }
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("guide"))
		return 1
	}
	fmt.Print(agentGuide())
	return 0
}

func agentGuide() string {
	var builder strings.Builder
	builder.WriteString(strings.TrimRight(agentGuideIntro, "\n"))
	builder.WriteString("\n\n## Common options\n\n")
	builder.WriteString("Most commands accept these:\n\n")
	for _, flagSpec := range commonCLIFlags() {
		builder.WriteString("- " + agentGuideFlag(flagSpec) + "\n")
	}
	builder.WriteString("\n## Commands\n\n")
	builder.WriteString("Run `rotari COMMAND --help` for a command's usage and every option, with its default.\n\n")
	for _, command := range cliCommandSpecs {
		fmt.Fprintf(&builder, "- `%s`: %s\n", command.Name, command.Description)
	}
	return builder.String()
}

func agentGuideFlag(spec cliFlagSpec) string {
	name := "`--" + spec.Name
	if spec.ValueName != "" {
		name += " " + spec.ValueName
	}
	name += "`"
	if short := cliShortFlagNames[spec.Name]; short != "" {
		name += " (`-" + short + "`)"
	}
	return name + ": " + cliFlagDescription(spec)
}

func upperFirst(text string) string {
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}
