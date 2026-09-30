package interfaces

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type cliSchemaFlag struct {
	Name string `json:"name"`
}

type cliSchemaCommand struct {
	Name  string          `json:"name"`
	Flags []cliSchemaFlag `json:"flags"`
}

type cliSchemaForPrecedence struct {
	Commands []cliSchemaCommand `json:"commands"`
}

type configCommandCase struct {
	name, optionName, envName, configDefault string
	args                                     []string
}

func TestCLIOptionPrecedence(t *testing.T) {
	covers(t, "CLI-3")
	e := support.NewEnv(t)
	configPath, baseDir, masterDir := writePrecedenceConfig(t, e)
	for _, test := range configCommandCases(t, e, configPath, baseDir, masterDir) {
		t.Run(test.name, func(t *testing.T) {
			assertConfigAndEnvironmentPrecedence(t, e, configPath, test)
		})
	}
	assertExplicitCLIValueWins(t, e, configPath)
	assertQuietPrecedence(t, e)
}

func writePrecedenceConfig(t *testing.T, e *support.Env) (path, baseDir, masterDir string) {
	t.Helper()
	path = filepath.Join(e.Root, "selected.yaml")
	baseDir = filepath.Join(e.Root, "config-base")
	masterDir = filepath.Join(e.Root, "config-master")
	data := "basedir: " + strconv.Quote(baseDir) + "\nmasterdir: " + strconv.Quote(masterDir) + "\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, baseDir, masterDir
}

func configCommandCases(t *testing.T, e *support.Env, configPath, configBaseDir, configMasterDir string) []configCommandCase {
	t.Helper()
	var schema cliSchemaForPrecedence
	if err := json.Unmarshal([]byte(e.MustRotari("schema", "--json").Stdout), &schema); err != nil {
		t.Fatal(err)
	}
	var cases []configCommandCase
	for _, command := range schema.Commands {
		cases = append(cases, commandConfigCases(t, command, configPath, configBaseDir, configMasterDir)...)
	}
	if len(cases) == 0 {
		t.Fatal("CLI schema has no config-loading commands to check")
	}
	return cases
}

func commandConfigCases(t *testing.T, command cliSchemaCommand, configPath, configBaseDir, configMasterDir string) []configCommandCase {
	t.Helper()
	if !schemaHasFlag(command, "config") {
		return nil
	}
	var cases []configCommandCase
	for _, subcommand := range precedenceSubcommands(command.Name) {
		optionName, configDefault, envName := configLocationOption(command, subcommand, configBaseDir, configMasterDir)
		if optionName == "" {
			t.Errorf("schema command %q accepts --config but has no basedir/masterdir option", command.Name)
			continue
		}
		args := []string{command.Name}
		if subcommand != "" {
			args = append(args, subcommand)
		}
		args = append(args, "--config", configPath, "--help")
		name := command.Name
		if subcommand != "" {
			name += " " + subcommand
		}
		cases = append(cases, configCommandCase{name: name, optionName: optionName, envName: envName, configDefault: configDefault, args: args})
	}
	return cases
}

func configLocationOption(command cliSchemaCommand, subcommand, configBaseDir, configMasterDir string) (optionName, configDefault, envName string) {
	useBaseDir := schemaHasFlag(command, "basedir") && !(command.Name == "server" && subcommand == "list")
	if useBaseDir {
		return "basedir", configBaseDir, "ROTARI_BASEDIR"
	}
	if schemaHasFlag(command, "masterdir") {
		return "masterdir", configMasterDir, "ROTARI_MASTERDIR"
	}
	return "", "", ""
}

func schemaHasFlag(command cliSchemaCommand, name string) bool {
	for _, flag := range command.Flags {
		if flag.Name == name {
			return true
		}
	}
	return false
}

func precedenceSubcommands(command string) []string {
	if command == "server" {
		return []string{"status", "list", "shutdown"}
	}
	return []string{""}
}

func assertConfigAndEnvironmentPrecedence(t *testing.T, e *support.Env, configPath string, test configCommandCase) {
	t.Helper()
	for _, source := range []struct {
		name, envValue, want string
	}{
		{name: "config beats built-in default", want: test.configDefault},
		{name: "environment beats config", envValue: filepath.Join(e.Root, "environment", test.name), want: filepath.Join(e.Root, "environment", test.name)},
	} {
		t.Run(source.name, func(t *testing.T) {
			commandEnv := e.Without(test.envName)
			if source.envValue != "" {
				commandEnv = commandEnv.WithVar(test.envName, source.envValue)
			}
			result := commandEnv.Rotari(test.args...)
			output := result.Stdout + result.Stderr
			if strings.Contains(output, "failed to load config") || strings.Contains(output, "flag provided but not defined") {
				t.Fatalf("command did not load or accept --config: %s", result)
			}
			want := "(default \"" + source.want + "\")"
			if !strings.Contains(output, want) {
				t.Fatalf("default for --%s does not contain %q: %s", test.optionName, want, result)
			}
		})
	}
}

func assertExplicitCLIValueWins(t *testing.T, e *support.Env, configPath string) {
	t.Helper()
	cliBaseDir := filepath.Join(e.Root, "command-line")
	commandEnv := e.WithVar("ROTARI_BASEDIR", filepath.Join(e.Root, "environment"))
	result := commandEnv.Rotari("show", "--config", configPath, "--basedir", cliBaseDir, "--project-name", "demo", "--json")
	if !strings.Contains(result.Stdout+result.Stderr, cliBaseDir) {
		t.Fatalf("explicit --basedir did not override environment and config: %s", result)
	}
}

func assertQuietPrecedence(t *testing.T, e *support.Env) {
	t.Helper()
	configPath := filepath.Join(e.Root, "quiet.yaml")
	if err := os.WriteFile(configPath, []byte("quiet: false\ncheck:\n  quiet: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sectionHelp := e.Without("ROTARI_QUIET").Without("ROTARI_CHECK_QUIET").Rotari("check", "--config", configPath, "--help")
	if !strings.Contains(sectionHelp.Stdout+sectionHelp.Stderr, "(default true)") {
		t.Fatalf("command config did not override root config for quiet: %s", sectionHelp)
	}
	commandEnvHelp := e.WithVar("ROTARI_QUIET", "false").WithVar("ROTARI_CHECK_QUIET", "true").Rotari("check", "--config", configPath, "--help")
	if !strings.Contains(commandEnvHelp.Stdout+commandEnvHelp.Stderr, "(default true)") {
		t.Fatalf("command quiet environment did not override global quiet environment and config: %s", commandEnvHelp)
	}
	e.MustRotari("add", "--project-name", "demo", "--", "true")
	cliResult := e.WithVar("ROTARI_QUIET", "true").WithVar("ROTARI_CHECK_QUIET", "true").Rotari("check", "--config", configPath, "--project-name", "demo", "--quiet=false")
	if cliResult.Code != 0 || cliResult.Stdout == "" {
		t.Fatalf("explicit --quiet=false did not override config and environment: %s", cliResult)
	}
}
