package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type cliSchema struct {
	Version  int                `json:"version"`
	Commands []cliSchemaCommand `json:"commands"`
}

type cliSchemaCommand struct {
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Flags       []cliSchemaFlag     `json:"flags,omitempty"`
	Subcommands []cliSubcommandSpec `json:"subcommands,omitempty"`
	Positional  string              `json:"positional,omitempty"`
}

type cliSchemaFlag struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	ValueName   string   `json:"value_name,omitempty"`
	Values      []string `json:"values,omitempty"`
	Short       string   `json:"short,omitempty"`
	Environment string   `json:"environment,omitempty"`
	Repeated    bool     `json:"repeated,omitempty"`
}

// cmdSchema prints the machine-readable CLI configuration schema.
func cmdSchema(args []string) int {
	if len(args) != 1 || args[0] != "--json" {
		printError("usage: rotari schema --json")
		return 1
	}

	schema := cliSchema{Version: 1, Commands: make([]cliSchemaCommand, 0, len(cliCommandSpecs))}
	for _, command := range cliCommandSpecs {
		value := cliSchemaCommand{
			Name: command.Name, Description: command.Description,
			Subcommands: command.Subcommands, Positional: command.Positional,
		}
		for _, flag := range command.Flags {
			value.Flags = append(value.Flags, cliSchemaFlag{
				Name: flag.Name, Description: flag.Description, ValueName: flag.ValueName,
				Values: flag.Values, Short: cliShortFlagNames[flag.Name],
				Environment: cliEnvironmentVariables[flag.Name],
				Repeated:    flag.Repeated || strings.Contains(flag.Description, "may be repeated"),
			})
		}
		schema.Commands = append(schema.Commands, value)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(schema); err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode CLI schema: %v\n", err)
		return 1
	}
	return 0
}
