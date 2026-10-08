package interfaces

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestCLIFlagEnvironmentCoverage(t *testing.T) {
	covers(t, "CLI-20")
	e := support.NewEnv(t)
	schema := readEnvironmentCoverageSchema(t, e)
	if len(schema.Commands) == 0 || len(schema.Environments) == 0 {
		t.Fatal("schema must enumerate commands and environment definitions")
	}
	definitions := cliDefaultEnvironmentDefinitions(schema.Environments)
	envOutput := rotariEnvCLIDefinitions(e.MustRotari("env").Stdout)
	for _, command := range schema.Commands {
		assertCommandEnvironmentCoverage(t, command.Name, command.Flags, definitions, envOutput)
	}
}

type environmentCoverageSchema struct {
	Commands []struct {
		Name  string `json:"name"`
		Flags []struct {
			Name            string `json:"name"`
			Environment     string `json:"environment"`
			CommandLineOnly bool   `json:"command_line_only"`
		} `json:"flags"`
	} `json:"commands"`
	Environments []struct {
		Name       string `json:"name"`
		CLIDefault bool   `json:"cli_default"`
	} `json:"environments"`
}

func readEnvironmentCoverageSchema(t *testing.T, e *support.Env) environmentCoverageSchema {
	t.Helper()
	var schema environmentCoverageSchema
	if err := json.Unmarshal([]byte(e.MustRotari("schema", "--json").Stdout), &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

func cliDefaultEnvironmentDefinitions(items []struct {
	Name       string `json:"name"`
	CLIDefault bool   `json:"cli_default"`
}) map[string]bool {
	definitions := make(map[string]bool, len(items))
	for _, item := range items {
		definitions[item.Name] = item.CLIDefault
	}
	return definitions
}

func rotariEnvCLIDefinitions(output string) map[string]bool {
	definitions := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 3 && fields[0] != "VARIABLE" {
			definitions[fields[0]] = fields[2] == "yes"
		}
	}
	return definitions
}

func assertCommandEnvironmentCoverage(t *testing.T, command string, flags []struct {
	Name            string `json:"name"`
	Environment     string `json:"environment"`
	CommandLineOnly bool   `json:"command_line_only"`
}, definitions, envOutput map[string]bool) {
	t.Helper()
	for _, flag := range flags {
		if flag.CommandLineOnly {
			if flag.Environment != "" {
				t.Errorf("%s --%s is command-line-only but maps to %s", command, flag.Name, flag.Environment)
			}
			continue
		}
		if flag.Environment == "" {
			t.Errorf("%s --%s has no environment variable and is not an explicit exception", command, flag.Name)
			continue
		}
		if !definitions[flag.Environment] {
			t.Errorf("%s --%s maps to %s, which is not a CLI-default environment definition", command, flag.Name, flag.Environment)
		}
		if !envOutput[flag.Environment] {
			t.Errorf("%s --%s maps to %s, which is missing from rotari env", command, flag.Name, flag.Environment)
		}
		if command == "wait" && flag.Name == "quiet" && flag.Environment != "ROTARI_WAIT_QUIET" {
			t.Errorf("wait --quiet environment = %q, want ROTARI_WAIT_QUIET", flag.Environment)
		}
	}
}
