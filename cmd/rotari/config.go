package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

var cliConfig map[string]any
var cliConfigCommand string

var configExtensions = []string{".yaml", ".toml", ".json"}

func loadCLIConfig(args []string) error {
	cliConfig = nil
	baseDir, projectName := configLocationArgs(args)
	if runID := configRunIDArg(args); runID != "" {
		location, found, err := resolveRunLocation(runID)
		if err != nil {
			return err
		}
		if found {
			if baseDir == "" {
				baseDir = location.BaseDir
			}
			if projectName == "" {
				projectName = location.ProjectName
			}
		}
	}
	resolvedBaseDir, _, err := resolveBaseDir(baseDir)
	if err != nil {
		return err
	}
	if projectName, err = configProjectName(resolvedBaseDir, projectName); err != nil {
		return err
	}
	config := map[string]any{}
	if path := effectiveConfigPath(resolvedBaseDir, projectName); path != "" {
		config, err = loadConfigFile(filepath.Dir(path))
		if err != nil {
			return err
		}
	}
	cliConfig = config
	return nil
}

func configProjectName(baseDir, requested string) (string, error) {
	if requested != "" {
		if !state.IsValidPathElement(requested) {
			return "", fmt.Errorf("invalid project name %q", requested)
		}
		return requested, nil
	}
	if value := os.Getenv(envProjectName); value != "" {
		if !state.IsValidPathElement(value) {
			return "", fmt.Errorf("invalid project name %q", value)
		}
		return value, nil
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		return "", nil
	}
	var projects []string
	for _, entry := range entries {
		if entry.IsDir() {
			projects = append(projects, entry.Name())
		}
	}
	if len(projects) == 1 {
		return projects[0], nil
	}
	return "", nil
}

func configRunIDArg(args []string) string {
	for index := 0; index < len(args); index++ {
		name, value, hasValue := strings.Cut(args[index], "=")
		if name != "--run-id" && name != "-r" {
			continue
		}
		if hasValue {
			return value
		}
		if index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func mergeConfig(destination, source map[string]any) {
	for key, value := range source {
		sourceSection, sourceIsSection := value.(map[string]any)
		destinationSection, destinationIsSection := destination[key].(map[string]any)
		if sourceIsSection && destinationIsSection {
			mergeConfig(destinationSection, sourceSection)
			continue
		}
		destination[key] = value
	}
}

func configLocationArgs(args []string) (baseDir, projectName string) {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		name, value, hasValue := strings.Cut(arg, "=")
		if !hasValue && index+1 < len(args) {
			switch arg {
			case "--basedir", "-b", "--project-name", "-p":
				value = args[index+1]
				hasValue = true
				index++
			}
		}
		if !hasValue {
			continue
		}
		switch name {
		case "--basedir", "-b":
			baseDir = value
		case "--project-name", "-p":
			projectName = value
		}
	}
	return baseDir, projectName
}

func configHomeDir() (string, error) {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return filepath.Join(value, "rotari"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rotari"), nil
}

func loadConfigFile(directory string) (map[string]any, error) {
	paths := configFilePaths(directory)
	if len(paths) > 1 {
		return nil, fmt.Errorf("multiple config files found in %s: %s", directory, strings.Join(paths, ", "))
	}
	if len(paths) == 0 {
		return map[string]any{}, nil
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		printErrorf("WARNING: cannot read config %s: %v", paths[0], err)
		return map[string]any{}, nil
	}
	config, err := parseConfigContent(paths[0], data)
	if err != nil {
		printErrorf("WARNING: cannot parse config %s: %v", paths[0], err)
		return map[string]any{}, nil
	}
	return config, nil
}

func parseConfigContent(path string, data []byte) (map[string]any, error) {
	config := make(map[string]any)
	var err error
	switch filepath.Ext(path) {
	case ".json":
		err = json.Unmarshal(data, &config)
	case ".yaml":
		err = yaml.Unmarshal(data, &config)
	case ".toml":
		_, err = toml.Decode(string(data), &config)
	default:
		return nil, fmt.Errorf("unsupported config format %q", filepath.Ext(path))
	}
	if err != nil {
		return nil, err
	}
	return config, nil
}

func configFilePaths(directory string) []string {
	paths := make([]string, 0, len(configExtensions))
	for _, extension := range configExtensions {
		path := filepath.Join(directory, "config"+extension)
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			printErrorf("WARNING: cannot inspect config %s: %v", path, err)
		}
	}
	return paths
}

func effectiveConfigPath(baseDir, projectName string) string {
	paths := configPathsForRun(baseDir, projectName)
	if len(paths) == 0 {
		return ""
	}
	return paths[len(paths)-1]
}

func globalConfigPath() string {
	configHome, err := configHomeDir()
	if err != nil {
		return ""
	}
	paths := configFilePaths(configHome)
	if len(paths) == 0 {
		return ""
	}
	return paths[len(paths)-1]
}

func configPathsForRun(baseDir, projectName string) []string {
	if projectName != "" {
		if projectDir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), projectName); err == nil {
			if paths := configFilePaths(projectDir); len(paths) > 0 {
				return paths
			}
		}
	}
	if paths := configFilePaths(baseDir); len(paths) > 0 {
		return paths
	}
	if configHome, err := configHomeDir(); err == nil {
		return configFilePaths(configHome)
	}
	return nil
}

func configValue(name string) (any, bool) {
	if cliConfigCommand != "" {
		if section, ok := cliConfig[cliConfigCommand].(map[string]any); ok {
			if value, exists := section[name]; exists && value != nil {
				return value, true
			}
		}
	}
	value, ok := cliConfig[name]
	return value, ok && value != nil
}

func configString(name, defaultValue string) string {
	value, ok := configValue(name)
	if !ok || value == nil {
		return defaultValue
	}
	if parsed, ok := value.(string); ok {
		return parsed
	}
	return fmt.Sprint(value)
}

func configOptionNames() []string {
	names := make(map[string]bool)
	for _, command := range cliCommandSpecs {
		if command.Name == "config" {
			continue
		}
		for _, flagSpec := range command.Flags {
			names[flagSpec.Name] = true
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func configSections() (map[string][]string, []string) {
	common := []string{"basedir", "project-name", "quiet"}
	commonSet := map[string]bool{"basedir": true, "project-name": true}
	sections := make(map[string][]string)
	for _, command := range cliCommandSpecs {
		if command.Name == "config" {
			continue
		}
		for _, flagSpec := range command.Flags {
			if commonSet[flagSpec.Name] {
				continue
			}
			sections[command.Name] = append(sections[command.Name], flagSpec.Name)
		}
		sort.Strings(sections[command.Name])
	}
	return sections, common
}

func configTemplate(format string) ([]byte, error) {
	sections, common := configSections()
	switch format {
	case "yaml":
		root := yaml.Node{Kind: yaml.MappingNode}
		for _, name := range common {
			root.Content = append(root.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: name, HeadComment: configOptionDescription(name)},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"},
			)
		}
		sectionNames := make([]string, 0, len(sections))
		for name := range sections {
			sectionNames = append(sectionNames, name)
		}
		sort.Strings(sectionNames)
		for _, sectionName := range sectionNames {
			mapping := &yaml.Node{Kind: yaml.MappingNode, HeadComment: "Options for " + sectionName + ""}
			for _, name := range sections[sectionName] {
				mapping.Content = append(mapping.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: name, HeadComment: configOptionDescription(name)},
					&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"},
				)
			}
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: sectionName}, mapping)
		}
		return yaml.Marshal(&root)
	case "json":
		values := make(map[string]any, len(sections)+len(common))
		for _, name := range common {
			values[name] = nil
		}
		for sectionName, names := range sections {
			section := make(map[string]any, len(names))
			for _, name := range names {
				section[name] = nil
			}
			values[sectionName] = section
		}
		return json.MarshalIndent(values, "", "  ")
	case "toml":
		var output strings.Builder
		for _, name := range common {
			fmt.Fprintf(&output, "# %s\n# %s = \"\"\n", configOptionDescription(name), name)
		}
		sectionNames := make([]string, 0, len(sections))
		for name := range sections {
			sectionNames = append(sectionNames, name)
		}
		sort.Strings(sectionNames)
		for _, sectionName := range sectionNames {
			fmt.Fprintf(&output, "\n[%s]\n", sectionName)
			for _, name := range sections[sectionName] {
				fmt.Fprintf(&output, "# %s\n# %s = \"\"\n", configOptionDescription(name), name)
			}
		}
		return []byte(output.String()), nil
	default:
		return nil, fmt.Errorf("unsupported config format %q (want yaml, toml, or json)", format)
	}
}

func configOptionDescription(name string) string {
	spec := cliFlag(name)
	description := cliFlagDescription(spec)
	if len(spec.Values) > 0 {
		description += " (values: " + strings.Join(spec.Values, ", ") + ")"
	}
	return description
}

func configFormatFromOutput(output string) string {
	switch filepath.Ext(output) {
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".json":
		return "json"
	default:
		return "toml"
	}
}

// cmdConfig writes, reads, validates, and explains Rotari configuration files.
func cmdConfig(args []string) int {
	oldCommand := cliConfigCommand
	cliConfigCommand = "config"
	defer func() { cliConfigCommand = oldCommand }()
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	list := cliBool(fs, "list", false)
	format := cliString(fs, "format", "")
	output := cliString(fs, "output", "")
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("config"))
		return 1
	}
	_ = basedir
	_ = projectName
	selectedFormat := *format
	if selectedFormat == "" {
		selectedFormat = configFormatFromOutput(*output)
	}
	resolvedBaseDir, _, err := resolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve basedir: %v", err)
		return 1
	}
	if *list {
		if *format != "" || *output != "" {
			printError("--list cannot be combined with --format or --output")
			return 1
		}
		project, err := configProjectName(resolvedBaseDir, *projectName)
		if err != nil {
			printError(err.Error())
			return 1
		}
		for _, path := range configPathsForRun(resolvedBaseDir, project) {
			fmt.Println(path)
		}
		return 0
	}
	if *output == "" {
		selectedOutput, ok := chooseConfigOutput(os.Stdin, os.Stderr, resolvedBaseDir, *projectName, selectedFormat)
		if !ok {
			return 1
		}
		*output = selectedOutput
	}
	data, err := configTemplate(selectedFormat)
	if err != nil {
		printError(err.Error())
		return 1
	}
	if *output == "-" {
		_, _ = os.Stdout.Write(data)
		return 0
	}
	if _, err := os.Stat(*output); err == nil {
		printErrorf("refusing to overwrite existing config file %s", *output)
		return 1
	} else if !errors.Is(err, os.ErrNotExist) {
		printErrorf("failed to inspect config file %s: %v", *output, err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(*output), stateDirMode()); err != nil {
		printErrorf("failed to create config directory for %s: %v", *output, err)
		return 1
	}
	if err := os.WriteFile(*output, data, stateFileMode()); err != nil {
		printErrorf("failed to write config file %s: %v", *output, err)
		return 1
	}
	return 0
}

func chooseConfigOutput(reader io.Reader, writer io.Writer, baseDir, projectName, format string) (string, bool) {
	extension := "." + format
	configHome, err := configHomeDir()
	if err != nil {
		printErrorf("failed to resolve config home: %v", err)
		return "", false
	}
	type candidate struct {
		label string
		path  string
	}
	candidates := []candidate{
		{label: "global", path: filepath.Join(configHome, "config"+extension)},
		{label: "basedir", path: filepath.Join(baseDir, "config"+extension)},
	}
	if projectName != "" {
		if projectDir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), projectName); err == nil {
			candidates = append(candidates, candidate{label: "project " + projectName, path: filepath.Join(projectDir, "config"+extension)})
		}
	} else if entries, err := os.ReadDir(filepath.Join(baseDir, "projects")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && state.IsValidPathElement(entry.Name()) {
				candidates = append(candidates, candidate{label: "project " + entry.Name(), path: filepath.Join(baseDir, "projects", entry.Name(), "config"+extension)})
			}
		}
	}
	fmt.Fprintln(writer, "Config resolution priority (highest to lowest):")
	fmt.Fprintln(writer, "  CLI option > environment variable > project > basedir > global > built-in default")
	fmt.Fprintln(writer, "Choose a config file location to create:")
	for index, item := range candidates {
		fmt.Fprintf(writer, "  %d) %-16s %s\n", index+1, item.label, item.path)
	}
	stdoutChoice := len(candidates) + 1
	customChoice := stdoutChoice + 1
	fmt.Fprintf(writer, "  %d) stdout\n", stdoutChoice)
	fmt.Fprintf(writer, "  %d) other path\n", customChoice)
	fmt.Fprintf(writer, "Select [1-%d]: ", customChoice)
	input := bufio.NewReader(reader)
	line, err := input.ReadString('\n')
	if err != nil && len(line) == 0 {
		printError("failed to read config location")
		return "", false
	}
	choice := strings.TrimSpace(line)
	selected, err := strconv.Atoi(choice)
	if err != nil || selected < 1 || selected > customChoice {
		printErrorf("invalid config location %q", choice)
		return "", false
	}
	if selected <= len(candidates) {
		return candidates[selected-1].path, true
	}
	if selected == stdoutChoice {
		return "-", true
	}
	fmt.Fprint(writer, "Enter config file path: ")
	customPath, err := input.ReadString('\n')
	if err != nil && len(customPath) == 0 {
		printError("failed to read config file path")
		return "", false
	}
	customPath = strings.TrimSpace(customPath)
	if customPath == "" || customPath == "-" {
		printError("config file path must not be empty or stdout")
		return "", false
	}
	return customPath, true
}

func configBool(name string, defaultValue bool) bool {
	value, ok := configValue(name)
	if !ok || value == nil {
		return defaultValue
	}
	switch parsed := value.(type) {
	case bool:
		return parsed
	case string:
		if result, err := strconv.ParseBool(parsed); err == nil {
			return result
		}
	}
	return defaultValue
}

func configInt(name string, defaultValue int) int {
	value, ok := configValue(name)
	if !ok || value == nil {
		return defaultValue
	}
	switch parsed := value.(type) {
	case int:
		return parsed
	case int64:
		return int(parsed)
	case uint64:
		return int(parsed)
	case float64:
		return int(parsed)
	case string:
		if result, err := strconv.Atoi(parsed); err == nil {
			return result
		}
	}
	return defaultValue
}

func configStrings(name string) []string {
	value, ok := configValue(name)
	if !ok {
		return nil
	}
	if parsed, ok := value.(string); ok {
		return []string{parsed}
	}
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, fmt.Sprint(value))
	}
	return result
}
