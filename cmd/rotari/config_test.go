package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestProjectConfigIgnoresLowerPriorityScopes(t *testing.T) {
	baseDir := t.TempDir()
	projectDir := filepath.Join(baseDir, "projects", "demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "config.json"), []byte(`{"executor":`), 0o644); err != nil {
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
	if got := configString("executor", ""); got != "slurm" || configInt("retry", 0) != 0 {
		t.Fatalf("config = %#v", cliConfig)
	}
}

func TestBasedirConfigIgnoresGlobalCommandSections(t *testing.T) {
	configHome := t.TempDir()
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "projects", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	if err := os.MkdirAll(filepath.Join(configHome, "rotari"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configHome, "rotari", "config.yaml"), []byte("run: [invalid\n"), 0o644); err != nil {
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
	if got := configString("executor", ""); got != "local" || configInt("retry", 0) != 0 {
		t.Fatalf("basedir command config = %#v", cliConfig)
	}
	cliConfigCommand = "show"
	if got := configString("project-name", ""); got != "" {
		t.Fatalf("project name = %q, want empty", got)
	}
}

func TestQuietEnvironmentVariablesSupportGlobalAndCommandDefaults(t *testing.T) {
	t.Setenv(envQuiet, "true")
	t.Setenv(envAddQuiet, "false")
	t.Setenv(envRunQuiet, "true")
	for _, test := range []struct {
		command string
		want    bool
	}{
		{command: "add", want: false},
		{command: "copy", want: true},
		{command: "run", want: true},
	} {
		fs := flag.NewFlagSet(test.command, flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		if got := *cliBool(fs, "quiet", false); got != test.want {
			t.Errorf("%s quiet = %t, want %t", test.command, got, test.want)
		}
	}
}

func TestQuietConfigSupportsGlobalAndCommandDefaults(t *testing.T) {
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig = map[string]any{
		"quiet": true,
		"add":   map[string]any{"quiet": false},
	}
	cliConfigCommand = "add"
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})
	t.Setenv(envQuiet, "")
	t.Setenv(envAddQuiet, "")

	addFlags := flag.NewFlagSet("add", flag.ContinueOnError)
	addFlags.SetOutput(io.Discard)
	if got := *cliBool(addFlags, "quiet", false); got {
		t.Fatal("add quiet = true, want command config false")
	}
	runFlags := flag.NewFlagSet("run", flag.ContinueOnError)
	runFlags.SetOutput(io.Discard)
	cliConfigCommand = "run"
	if got := *cliBool(runFlags, "quiet", false); !got {
		t.Fatal("run quiet = false, want global config true")
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

func TestConfigPathsForRunRejectsUnsafeProjectName(t *testing.T) {
	baseDir := t.TempDir()
	paths := configPathsForRun(baseDir, "../outside")
	for _, path := range paths {
		if strings.Contains(path, "outside") {
			t.Fatalf("configPathsForRun returned path outside the project root: %q", path)
		}
	}
}

func TestConfigCommandGeneratesFile(t *testing.T) {
	output := filepath.Join(t.TempDir(), "config.yaml")
	if code := run([]string{"config", "--output", output}); code != 0 {
		t.Fatalf("config exit code = %d", code)
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
	if !strings.Contains(stdout, "Common:\n") || !strings.Contains(stdout, "Projects:\n") || !strings.Contains(stdout, "  demo:\n    "+filepath.Join(projectDir, "config.yaml")+"\n") {
		t.Fatalf("config list has unexpected format:\n%s", stdout)
	}
	for _, path := range paths {
		if !strings.Contains(stdout, path) {
			t.Errorf("config list does not contain %q:\n%s", path, stdout)
		}
	}
}

func TestConfigListIncludesAllProjectConfigs(t *testing.T) {
	baseDir := t.TempDir()
	paths := map[string]string{}
	for _, project := range []string{"alpha", "beta"} {
		projectDir := filepath.Join(baseDir, "projects", project)
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(projectDir, "config.yaml")
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[project] = path
	}
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdConfig([]string{"--basedir", baseDir, "--list"})
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
	for project, path := range paths {
		if !strings.Contains(stdout, "  "+project+":\n    "+path+"\n") {
			t.Errorf("config list does not contain %s config %q:\n%s", project, path, stdout)
		}
	}
}

func TestConfigListLimitsProjectsToProjectName(t *testing.T) {
	baseDir := t.TempDir()
	paths := map[string]string{}
	for _, project := range []string{"alpha", "beta"} {
		projectDir := filepath.Join(baseDir, "projects", project)
		if err := os.MkdirAll(projectDir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(projectDir, "config.yaml")
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[project] = path
	}
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	code := cmdConfig([]string{"--basedir", baseDir, "--project-name", "alpha", "--list"})
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
	if !strings.Contains(stdout, "Projects:\n  alpha:\n    "+paths["alpha"]+"\n") {
		t.Errorf("config list does not contain alpha config %q:\n%s", paths["alpha"], stdout)
	}
	if strings.Contains(stdout, paths["beta"]) {
		t.Errorf("config list includes beta config %q:\n%s", paths["beta"], stdout)
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
	runSection, ok := values["run"].(map[string]any)
	if !ok || runSection["slurm-submit-interval"] != nil || runSection["slurm-submit-retry-limit"] != nil {
		t.Fatalf("run template scheduler submit settings = %#v, want null entries", runSection)
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
			"ssh-concurrency":          3,
			"ssh-options":              []any{"builder@worker-01", "-p", "2222"},
			"slurm-concurrency":        12,
			"slurm-submit-interval":    "750ms",
			"slurm-submit-retry-limit": 4,
		},
	}
	cliConfigCommand = "run"
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})

	settings := cliExecutorRunSettings(flag.NewFlagSet("run", flag.ContinueOnError))()
	if settings["ssh"].Concurrency != 3 || len(settings["ssh"].Options) != 3 {
		t.Fatalf("SSH settings = %#v", settings["ssh"])
	}
	if settings["slurm"].Concurrency != 12 || settings["slurm"].SubmitInterval != 750*time.Millisecond || settings["slurm"].SubmitRetryLimit != 4 {
		t.Fatalf("Slurm settings = %#v", settings["slurm"])
	}
}

func TestExecutorRunSettingsEnvironmentOverridesConfig(t *testing.T) {
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig = map[string]any{"run": map[string]any{"ssh-concurrency": 3, "ssh-options": "config-host", "slurm-submit-interval": "750ms", "slurm-submit-retry-limit": 4}}
	cliConfigCommand = "run"
	t.Setenv(envRunSSHConc, "5")
	t.Setenv(envRunSSHOptions, "env-host")
	t.Setenv(envRunSlurmSubmitInterval, "250ms")
	t.Setenv(envRunSlurmSubmitRetryLimit, "1")
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})

	settings := cliExecutorRunSettings(flag.NewFlagSet("run", flag.ContinueOnError))()
	if settings["ssh"].Concurrency != 5 || len(settings["ssh"].Options) != 1 || settings["ssh"].Options[0] != "env-host" {
		t.Fatalf("SSH settings = %#v, want environment values", settings["ssh"])
	}
	if settings["slurm"].SubmitInterval != 250*time.Millisecond || settings["slurm"].SubmitRetryLimit != 1 {
		t.Fatalf("Slurm settings = %#v, want environment values", settings["slurm"])
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

func TestExecutorRunSettingsIncludeCommandLineValues(t *testing.T) {
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig = map[string]any{"run": map[string]any{"slurm-concurrency": 3}}
	cliConfigCommand = "run"
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})

	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	settings := cliExecutorRunSettings(fs)
	if err := fs.Parse([]string{"--slurm-concurrency", "7", "--slurm-options", "--partition=gpu", "--slurm-submit-interval", "2s", "--slurm-submit-retry-limit", "5", "--ssh-concurrency", "2"}); err != nil {
		t.Fatal(err)
	}
	got := settings()
	if got["slurm"].Concurrency != 7 || len(got["slurm"].Options) != 1 || got["slurm"].Options[0] != "--partition=gpu" || got["slurm"].SubmitInterval != 2*time.Second || got["slurm"].SubmitRetryLimit != 5 {
		t.Fatalf("Slurm settings = %#v, want command-line values", got["slurm"])
	}
	if got["ssh"].Concurrency != 2 {
		t.Fatalf("SSH settings = %#v, want command-line concurrency", got["ssh"])
	}
}

func TestCommandLineOnlyFlagIgnoresConfigAndStaysOutOfTemplate(t *testing.T) {
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig = map[string]any{"output": "top.yaml", "export": map[string]any{"output": "section.yaml"}}
	cliConfigCommand = "export"
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})

	output := cliString(flag.NewFlagSet("export", flag.ContinueOnError), "output", "")
	if *output != "" {
		t.Fatalf("export --output default = %q, want no config value", *output)
	}
	sections, _ := configSections()
	for _, name := range sections["export"] {
		if name == "output" {
			t.Fatalf("config template lists command-line-only export option %q", name)
		}
	}
}

func TestFlagHelpUsesTheParsingCommandsDescription(t *testing.T) {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	cliString(fs, "format", "yaml")
	var runIDs stringSliceFlag
	cliValue(fs, &runIDs, "run-id")
	if usage := fs.Lookup("format").Usage; !strings.Contains(usage, "manifest format") {
		t.Fatalf("export --format usage = %q, want the export description", usage)
	}
	if usage := fs.Lookup("run-id").Usage; !strings.Contains(usage, "run ID to export") {
		t.Fatalf("export --run-id usage = %q, want the export description", usage)
	}
	template, err := configTemplate("toml")
	if err != nil {
		t.Fatal(err)
	}
	exportSection := string(template)[strings.Index(string(template), "[export]"):]
	if end := strings.Index(exportSection[1:], "\n["); end >= 0 {
		exportSection = exportSection[:end+1]
	}
	if !strings.Contains(exportSection, "manifest format") || strings.Contains(exportSection, "config format") {
		t.Fatalf("config template [export] section uses another command's description:\n%s", exportSection)
	}
}

// TestCommandLineOnlyBoolIgnoresConfig checks that a config file cannot make
// delete remove every run.
func TestCommandLineOnlyBoolIgnoresConfig(t *testing.T) {
	oldConfig, oldCommand := cliConfig, cliConfigCommand
	cliConfig = map[string]any{"all": true, "delete": map[string]any{"all": true}}
	cliConfigCommand = "delete"
	t.Cleanup(func() {
		cliConfig = oldConfig
		cliConfigCommand = oldCommand
	})
	if all := cliBool(flag.NewFlagSet("delete", flag.ContinueOnError), "all", false); *all {
		t.Fatal("delete --all was set from config")
	}
}
