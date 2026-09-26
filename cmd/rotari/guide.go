package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"
)

// agentGuideIntro holds the hand-written workflow rules. The command
// reference that follows it is generated from cliCommandSpecs so it cannot
// drift from the CLI.
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
	builder.WriteString("Commands that accept both are marked below instead of repeating them.\n\n")
	common := make(map[string]bool)
	for _, flagSpec := range commonCLIFlags() {
		common[agentGuideFlag(flagSpec)] = true
		builder.WriteString("- " + agentGuideFlag(flagSpec) + "\n")
	}
	builder.WriteString("\n## Command reference\n")
	for _, command := range cliCommandSpecs {
		fmt.Fprintf(&builder, "\n### %s\n\n", command.Name)
		if command.Description != "" {
			fmt.Fprintf(&builder, "%s.\n\n", upperFirst(command.Description))
		}
		fmt.Fprintf(&builder, "```\n%s\n```\n", cliUsage(command.Name))
		if len(command.Subcommands) > 0 {
			builder.WriteString("\n")
			for _, subcommand := range command.Subcommands {
				fmt.Fprintf(&builder, "- `%s`: %s\n", subcommand.Name, subcommand.Description)
			}
		}
		commonCount := 0
		for _, flagSpec := range command.Flags {
			if common[agentGuideFlag(flagSpec)] {
				commonCount++
			}
		}
		hasCommon := commonCount == len(common)
		if hasCommon {
			builder.WriteString("\nAccepts the common options.\n")
		}
		flagLines := make([]string, 0, len(command.Flags))
		for _, flagSpec := range command.Flags {
			if hasCommon && common[agentGuideFlag(flagSpec)] {
				continue
			}
			flagLines = append(flagLines, "- "+agentGuideFlag(flagSpec)+"\n")
		}
		if len(flagLines) > 0 {
			builder.WriteString("\n" + strings.Join(flagLines, ""))
		}
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
