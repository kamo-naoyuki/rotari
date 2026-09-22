package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadConfigFileSupportsAllFormats(t *testing.T) {
	tests := []struct {
		name      string
		extension string
		content   string
	}{
		{name: "yaml", extension: ".yaml", content: "executor: slurm\nlocal-concurrency: 4\n"},
		{name: "toml", extension: ".toml", content: "executor = \"slurm\"\nlocal-concurrency = 4\n"},
		{name: "json", extension: ".json", content: `{"executor":"slurm","local-concurrency":4}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			if err := os.WriteFile(filepath.Join(directory, "config"+test.extension), []byte(test.content), 0o644); err != nil {
				t.Fatal(err)
			}
			config, err := loadConfigFile(directory)
			if err != nil {
				t.Fatal(err)
			}
			if configStringFrom(config, "executor") != "slurm" || configIntFrom(config, "local-concurrency") != 4 {
				t.Fatalf("config = %#v", config)
			}
		})
	}
}

func TestLoadConfigFileRejectsMultipleFormats(t *testing.T) {
	directory := t.TempDir()
	for _, extension := range []string{".yaml", ".toml"} {
		if err := os.WriteFile(filepath.Join(directory, "config"+extension), []byte("executor = \"local\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := loadConfigFile(directory)
	if err == nil || !strings.Contains(err.Error(), "multiple config files") {
		t.Fatalf("loadConfigFile error = %v", err)
	}
}

func TestLoadConfigFileWarnsAndIgnoresInvalidFormat(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte("run: [invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfigFile(directory)
	if err != nil {
		t.Fatalf("loadConfigFile returned error: %v", err)
	}
	if len(config) != 0 {
		t.Fatalf("config = %#v, want empty config", config)
	}
}

func TestProjectConfigOverridesBaseConfig(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := filepath.Join(baseDir, "projects", "demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "config.json"), []byte(`{"executor":"local","retry":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "config.json"), []byte(`{"executor":"slurm"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	oldConfig := cliConfig
	t.Cleanup(func() { cliConfig = oldConfig })
	if err := loadCLIConfig([]string{"--basedir", baseDir, "--project-name", "demo"}); err != nil {
		t.Fatal(err)
	}
	if got := configString("executor", ""); got != "slurm" || configInt("retry", 0) != 1 {
		t.Fatalf("config = %#v", cliConfig)
	}
}

func TestGlobalConfigSupportsCommandSections(t *testing.T) {
	configHome := t.TempDir()
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if err := os.MkdirAll(filepath.Join(configHome, "rotari"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configHome, "rotari", "config.yaml"), []byte("project-name: global-project\nrun:\n  executor: slurm\n  retry: 4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "config.yaml"), []byte("run:\n  executor: local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})
	if err := loadCLIConfig([]string{"--basedir", baseDir, "--project-name", "demo"}); err != nil {
		t.Fatal(err)
	}
	cliConfigCommand = "run"
	if got := configString("executor", ""); got != "local" || configInt("retry", 0) != 4 {
		t.Fatalf("global command config = %#v", cliConfig)
	}
	cliConfigCommand = "show"
	if got := configString("project-name", ""); got != "global-project" {
		t.Fatalf("global root config = %q", got)
	}
}

func TestConfigLoadDoesNotRejectAmbiguousProjects(t *testing.T) {
	baseDir := t.TempDir()
	for _, project := range []string{"demo", "example"} {
		if err := os.MkdirAll(filepath.Join(baseDir, "projects", project), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldConfig := cliConfig
	t.Cleanup(func() { cliConfig = oldConfig })
	if err := loadCLIConfig([]string{"--basedir", baseDir}); err != nil {
		t.Fatalf("loadCLIConfig rejected ambiguous projects: %v", err)
	}
}

func TestRunConfigCommandGeneratesFile(t *testing.T) {
	output := filepath.Join(t.TempDir(), "config.yaml")
	if code := run([]string{"run", "config", "--output", output}); code != 0 {
		t.Fatalf("run config exit code = %d", code)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("generated config: %v", err)
	}
}

func TestConfigListIncludesMixedFormatsAcrossScopes(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	globalDir := filepath.Join(configHome, "rotari")
	baseDir := t.TempDir()
	projectDir := filepath.Join(baseDir, "projects", "demo")
	for _, directory := range []string{globalDir, baseDir, projectDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{
		filepath.Join(globalDir, "config.yaml"),
		filepath.Join(globalDir, "config.json"),
		filepath.Join(baseDir, "config.toml"),
		filepath.Join(projectDir, "config.yaml"),
		filepath.Join(projectDir, "config.toml"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdConfig([]string{"--basedir", baseDir, "--project-name", "demo", "--list"})
	os.Stdout = oldStdout
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("cmdConfig exit code = %d", code)
	}
	stdout := string(data)
	for _, path := range paths {
		if !strings.Contains(stdout, path) {
			t.Errorf("config list does not contain %q:\n%s", path, stdout)
		}
	}
}

func TestConfigLocationsDeepMergeCommandSections(t *testing.T) {
	destination := map[string]any{
		"run": map[string]any{"retry": 2, "executor": "local"},
	}
	mergeConfig(destination, map[string]any{
		"run": map[string]any{"executor": "slurm"},
	})
	runConfig, ok := destination["run"].(map[string]any)
	if !ok || runConfig["executor"] != "slurm" || runConfig["retry"] != 2 {
		t.Fatalf("merged config = %#v", destination)
	}
}

func TestConfigTemplateIncludesAllOptionsAsNull(t *testing.T) {
	if got := configFormatFromOutput(""); got != "toml" {
		t.Fatalf("default config format = %q, want toml", got)
	}
	data, err := configTemplate("yaml")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	sections, common := configSections()
	for _, name := range common {
		if value, ok := values[name]; !ok || value != nil {
			t.Errorf("template[%q] = %#v, want null", name, value)
		}
	}
	for sectionName, names := range sections {
		section, ok := values[sectionName].(map[string]any)
		if !ok {
			t.Errorf("template[%q] = %#v, want a section", sectionName, values[sectionName])
			continue
		}
		for _, name := range names {
			if value, exists := section[name]; !exists || value != nil {
				t.Errorf("template[%s.%q] = %#v, want null", sectionName, name, value)
			}
		}
	}
	if !strings.Contains(string(data), "# state directory") || !strings.Contains(string(data), "# replace job executor") {
		t.Fatalf("template is missing option descriptions:\n%s", data)
	}
	tomlData, err := configTemplate("toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tomlData), "# state directory") || !strings.Contains(string(tomlData), "# basedir = \"\"") {
		t.Fatalf("TOML template is missing option comments:\n%s", tomlData)
	}
	if strings.Contains(string(tomlData), "TOML has no null value") {
		t.Fatal("TOML template contains the removed introductory comment")
	}
}

func TestNullConfigValuesAreIgnored(t *testing.T) {
	oldConfig := cliConfig
	oldCommand := cliConfigCommand
	cliConfig = map[string]any{
		"executor":     nil,
		"retry":        nil,
		"async":        nil,
		"project-name": "shared",
		"run":          map[string]any{"job-id": "from-run", "retry": nil},
	}
	cliConfigCommand = "run"
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})
	if got := configString("executor", "local"); got != "local" {
		t.Fatalf("configString = %q, want local", got)
	}
	if got := configInt("retry", 3); got != 3 {
		t.Fatalf("configInt = %d, want 3", got)
	}
	if got := configBool("async", true); !got {
		t.Fatal("configBool changed the default for null")
	}
	if got := configString("job-id", ""); got != "from-run" {
		t.Fatalf("command config = %q, want from-run", got)
	}
	if got := configString("project-name", ""); got != "shared" {
		t.Fatalf("root config = %q, want shared", got)
	}
	if got := configInt("retry", 3); got != 3 {
		t.Fatalf("null command config = %d, want 3", got)
	}
}

func TestExecutorRunSettingsLoadFromRunConfig(t *testing.T) {
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig = map[string]any{
		"run": map[string]any{
			"ssh-concurrency":   3,
			"ssh-options":       []any{"builder@worker-01", "-p", "2222"},
			"slurm-concurrency": 12,
		},
	}
	cliConfigCommand = "run"
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})

	settings := cliExecutorRunSettings(flag.NewFlagSet("run", flag.ContinueOnError))
	if settings["ssh"].Concurrency != 3 || len(settings["ssh"].Options) != 3 {
		t.Fatalf("SSH settings = %#v", settings["ssh"])
	}
	if settings["slurm"].Concurrency != 12 {
		t.Fatalf("Slurm settings = %#v", settings["slurm"])
	}
}

func TestExecutorRunSettingsEnvironmentOverridesConfig(t *testing.T) {
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig = map[string]any{"run": map[string]any{"ssh-concurrency": 3, "ssh-options": "config-host"}}
	cliConfigCommand = "run"
	t.Setenv(envRunSSHConc, "5")
	t.Setenv(envRunSSHOptions, "env-host")
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})

	settings := cliExecutorRunSettings(flag.NewFlagSet("run", flag.ContinueOnError))
	if settings["ssh"].Concurrency != 5 || len(settings["ssh"].Options) != 1 || settings["ssh"].Options[0] != "env-host" {
		t.Fatalf("SSH settings = %#v, want environment values", settings["ssh"])
	}
}

func TestPrintConfigCandidates(t *testing.T) {
	var output strings.Builder
	path, ok := chooseConfigOutput(strings.NewReader("3\n"), &output, "/state", "demo", "yaml")
	if !ok || path != "/state/projects/demo/config.yaml" {
		t.Fatalf("chooseConfigOutput = %q, %t", path, ok)
	}
	if !strings.Contains(output.String(), "2) basedir") || !strings.Contains(output.String(), "3) project demo") {
		t.Fatalf("candidate output:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "4) stdout") || !strings.Contains(output.String(), "project > basedir > global") {
		t.Fatalf("candidate output is missing stdout:\n%s", output.String())
	}
	output.Reset()
	path, ok = chooseConfigOutput(strings.NewReader("5\n/tmp/custom-config.yaml\n"), &output, "/state", "demo", "yaml")
	if !ok || path != "/tmp/custom-config.yaml" {
		t.Fatalf("custom chooseConfigOutput = %q, %t", path, ok)
	}
	if !strings.Contains(output.String(), "5) other path") || !strings.Contains(output.String(), "Enter config file path:") {
		t.Fatalf("custom candidate output:\n%s", output.String())
	}
}

func configStringFrom(config map[string]any, name string) string {
	return strings.TrimSpace(strings.Trim(fmt.Sprint(config[name]), `"`))
}

func configIntFrom(config map[string]any, name string) int {
	value := fmt.Sprint(config[name])
	var result int
	_, _ = fmt.Sscanf(value, "%d", &result)
	return result
}
