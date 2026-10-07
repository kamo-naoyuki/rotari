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

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/notification"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"gopkg.in/yaml.v3"
)

var cliConfig map[string]any
var cliConfigCommand string
var cliConfigPath string
var cliFileConfig config.Loaded
var cliLocationExplicit map[string]bool
var cliLocationDefaults map[string]string
var cliIgnoreImplicitLocationDefaults map[string]bool

func init() {
	// Config warnings use the CLI's error style.
	config.Warnf = printErrorf
}

func loadCLIConfig(args []string) error {
	if cliConfigCommand == "__complete" {
		command, locationArgs, err := completionConfigInvocation(args)
		if err != nil {
			return err
		}
		if command != "" {
			cliConfigCommand = command
			args = locationArgs
		}
	}
	cliConfig = nil
	cliConfigPath = ""
	cliLocationDefaults = nil
	cliFileConfig = config.Loaded{Values: map[string]any{}}
	baseDir, projectName := configLocationArgs(args)
	invocationOptions, positional := configInvocationOptions(args)
	aggregate := aggregateCommandInvocation(cliConfigCommand, invocationOptions, positional)
	cliIgnoreImplicitLocationDefaults = ignoredImplicitLocationDefaults(cliConfigCommand, aggregate)
	// Help reports ordinary option provenance; it does not execute a listing.
	if _, help := invocationOptions["help"]; help {
		cliIgnoreImplicitLocationDefaults = nil
	} else if _, help := invocationOptions["h"]; help {
		cliIgnoreImplicitLocationDefaults = nil
	}
	if len(positional) > 0 && (cliConfigCommand == "check" || cliConfigCommand == "reset" || cliConfigCommand == "unlock" || cliConfigCommand == "jobs" || cliConfigCommand == "runs") && projectName == "" {
		projectName = positional[0]
	}
	cliLocationExplicit = map[string]bool{
		"basedir":      baseDir != "" || !cliIgnoreImplicitLocationDefaults["basedir"] && os.Getenv(envBaseDir) != "",
		"project-name": projectName != "" || !cliIgnoreImplicitLocationDefaults["project-name"] && os.Getenv(envProjectName) != "",
	}
	if path, specified := configFileArg(args); specified {
		values, err := config.LoadPath(path)
		if err != nil {
			return err
		}
		cliConfig = values
		cliConfigPath, err = filepath.Abs(path)
		if err != nil {
			return err
		}
		cliFileConfig = config.Loaded{Values: config.Merge(nil, values), Sources: []config.Source{{Scope: "explicit", Path: cliConfigPath}}}
		rememberConfigLocations()
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := config.HomeDir()
	if err != nil {
		return err
	}
	for _, item := range []struct{ scope, dir string }{{"global", home}, {"workspace", cwd}} {
		layer, err := config.LoadScope(item.scope, item.dir)
		if err != nil {
			return err
		}
		cliFileConfig.Add(layer)
	}
	cliConfig = config.Merge(nil, cliFileConfig.Values)
	// Registry-wide listings have no single basedir or project config scope.
	// Global/workspace defaults (e.g. masterdir) still apply.
	if cliConfigCommand == "basedirs" || cliConfigCommand == "projects" && baseDir == "" || cliConfigCommand == "runs" && listingAllBaseDirs(invocationOptions) {
		for _, source := range cliFileConfig.Sources {
			cliConfigPath = source.Path
		}
		rememberConfigLocations()
		return nil
	}
	if baseDir == "" {
		if !cliIgnoreImplicitLocationDefaults["basedir"] {
			baseDir = os.Getenv(envBaseDir)
		}
	}
	if projectName == "" {
		if !cliIgnoreImplicitLocationDefaults["project-name"] {
			projectName = os.Getenv(envProjectName)
		}
	}
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
	if baseDir == "" {
		if !cliIgnoreImplicitLocationDefaults["basedir"] {
			baseDir = configString("basedir", "")
		}
	}
	var resolvedBaseDir string
	if cliIgnoreImplicitLocationDefaults["basedir"] && baseDir == "" {
		resolvedBaseDir, err = state.ResolveBaseDirDefault()
	} else {
		resolvedBaseDir, _, err = state.ResolveBaseDir(baseDir)
	}
	if err != nil {
		return err
	}
	_, projectFlagGiven := invocationOptions["project-name"]
	if !projectFlagGiven && len(positional) > 0 && (cliConfigCommand == "show" || cliConfigCommand == "export" || cliConfigCommand == "wait") {
		candidate := positional[0]
		if candidate != "latest" && state.IsValidPathElement(candidate) && !resolve.IsRunID(candidate) {
			if info, err := os.Stat(filepath.Join(resolvedBaseDir, "projects", candidate)); err == nil && info.IsDir() {
				projectName = candidate
				cliLocationExplicit["project-name"] = true
			}
		}
	}
	skipAggregateBaseLayer := aggregate && cliConfigCommand == "show" && len(invocationOptions["basedir"]) == 0
	if !skipAggregateBaseLayer {
		layer, err := config.LoadScope("basedir", resolvedBaseDir)
		if err != nil {
			return err
		}
		cliFileConfig.Add(layer)
	}
	cliConfig = config.Merge(nil, cliFileConfig.Values)
	if projectName == "" && !cliIgnoreImplicitLocationDefaults["project-name"] {
		projectName = configString("project-name", "")
	}
	if cliIgnoreImplicitLocationDefaults["project-name"] && projectName == "" && cliConfigCommand == "lineage" {
		projects, listErr := resolve.ExistingProjectNames(resolvedBaseDir)
		if listErr != nil {
			return listErr
		}
		if len(projects) == 1 {
			projectName = projects[0]
		}
	} else if !aggregate && !cliIgnoreImplicitLocationDefaults["project-name"] {
		if projectName, err = configProjectName(resolvedBaseDir, projectName); err != nil {
			return err
		}
	}
	if projectName != "" && cliConfigCommand != "projects" {
		if aggregate && cliConfigCommand == "jobs" && len(positional) == 0 && len(invocationOptions["project-name"]) == 0 {
			projectName = ""
		} else {
			projectDir, err := state.SafeJoin(filepath.Join(resolvedBaseDir, "projects"), projectName)
			if err != nil {
				return err
			}
			layer, err := config.LoadScope("project", projectDir)
			if err != nil {
				return err
			}
			cliFileConfig.Add(layer)
		}
	}
	cliConfig = config.Merge(nil, cliFileConfig.Values)
	// Runtime location facts are separate from the file-only snapshot.
	if configString("basedir", "") != "" {
		setConfigLocation("basedir", resolvedBaseDir)
	}
	if configString("project-name", "") != "" {
		setConfigLocation("project-name", projectName)
	}
	for _, source := range cliFileConfig.Sources {
		cliConfigPath = source.Path
	}
	rememberConfigLocations()
	return nil
}

func aggregateCommandInvocation(command string, options map[string][]string, positional []string) bool {
	for arg := range options {
		if arg == "help" || arg == "h" {
			return false
		}
	}
	switch command {
	case "basedirs", "projects", "runs":
		return len(positional) == 0 && len(options["project-name"]) == 0
	case "jobs":
		return true
	case "lineage":
		return len(positional) == 0 && len(options["run-id"]) == 0 && len(options["project-name"]) == 0
	case "config":
		return len(options["list"]) > 0
	default:
		return false
	}
}

func ignoredImplicitLocationDefaults(command string, aggregate bool) map[string]bool {
	ignored := map[string]bool{}
	// These list scopes never use implicit locations, even when a project
	// filter makes aggregateCommandInvocation return false.
	if command == "basedirs" || command == "projects" || command == "runs" {
		ignored["basedir"] = true
		ignored["project-name"] = true
		return ignored
	}
	if !aggregate {
		return ignored
	}
	switch command {
	case "jobs":
		ignored["basedir"] = true
		ignored["project-name"] = true
	case "lineage", "config":
		ignored["project-name"] = true
	}
	return ignored
}

func listingAllBaseDirs(options map[string][]string) bool {
	if _, specified := options["all-basedirs"]; specified {
		return optionEnabled(options, "all-basedirs")
	}
	return configBool("all-basedirs", false)
}

func optionEnabled(options map[string][]string, name string) bool {
	values := options[name]
	if len(values) == 0 {
		return false
	}
	value := values[len(values)-1]
	if value == "" {
		return true
	}
	enabled, err := strconv.ParseBool(value)
	return err != nil || enabled
}

func rememberConfigLocations() {
	cliLocationDefaults = make(map[string]string)
	for _, key := range []string{"basedir", "project-name"} {
		if !cliLocationExplicit[key] {
			cliLocationDefaults[key] = configString(key, "")
		}
	}
}

func setConfigLocation(name, value string) {
	cliConfig[name] = value
	if section, ok := cliConfig[cliConfigCommand].(map[string]any); ok {
		delete(section, name)
	}
}

func configFileArg(args []string) (string, bool) {
	options, _ := configInvocationOptions(args)
	values, specified := options["config"]
	if len(values) == 0 {
		return "", false
	}
	return values[len(values)-1], specified
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
	if cliConfigCommand == "add" || cliConfigCommand == "config" || cliConfigCommand == "init" || cliConfigCommand == "basedirs" || cliConfigCommand == "projects" || cliConfigCommand == "runs" {
		return ""
	}
	options, positional := configInvocationOptions(args)
	if values := options["run-id"]; len(values) > 0 {
		return values[0]
	}
	selectors := append(append([]string(nil), positional...), options["job-id"]...)
	for _, value := range selectors {
		if strings.HasPrefix(value, "att_") {
			if payload, err := state.DecodeAttemptID(value); err == nil {
				return payload.RunID
			}
		}
		if resolve.IsRunID(value) {
			return value
		}
	}
	return ""
}

func mergeConfig(destination, source map[string]any) {
	for key, value := range config.Merge(destination, source) {
		destination[key] = value
	}
}

func configLocationArgs(args []string) (baseDir, projectName string) {
	options, _ := configInvocationOptions(args)
	if values := options["basedir"]; len(values) > 0 {
		baseDir = values[len(values)-1]
	}
	if values := options["project-name"]; len(values) > 0 {
		projectName = values[len(values)-1]
	}
	return baseDir, projectName
}

// Read option shapes from the same metadata as parsing/help. Option values
// that happen to look like run IDs are not location selectors, and add/change
// job arguments must never be interpreted as rotari options.
func configInvocationOptions(args []string) (map[string][]string, []string) {
	options := make(map[string][]string)
	var positional []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			if cliConfigCommand != "add" && cliConfigCommand != "change" {
				positional = append(positional, args[index+1:]...)
			}
			break
		}
		if !strings.HasPrefix(arg, "-") {
			if cliConfigCommand == "add" || cliConfigCommand == "change" {
				break
			}
			positional = append(positional, arg)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		name = cliCanonicalFlagName(name)
		spec := cliCommandFlag(cliConfigCommand, name)
		takesValue := spec.ValueName != "" || name == "config" || name == "basedir" || name == "project-name" || name == "run-id" || name == "job-id"
		if !hasValue && takesValue && index+1 < len(args) {
			index++
			value = args[index]
		}
		options[name] = append(options[name], value)
	}
	return options, positional
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
			if !flagSpec.CommandLineOnly {
				names[flagSpec.Name] = true
			}
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
			if commonSet[flagSpec.Name] || flagSpec.CommandLineOnly {
				continue
			}
			sections[command.Name] = append(sections[command.Name], flagSpec.Name)
		}
		sort.Strings(sections[command.Name])
	}
	return sections, common
}

func configTemplate(format string, scopes ...string) ([]byte, error) {
	return configTemplateWithDefaults(format, nil, scopes...)
}

func configTemplateWithDefaults(format string, defaults map[string]any, scopes ...string) ([]byte, error) {
	sections, common := configSections()
	if len(scopes) > 0 {
		filtered := common[:0]
		for _, key := range common {
			if (scopes[0] == "basedir" || scopes[0] == "project") && key == "basedir" || scopes[0] == "project" && key == "project-name" {
				continue
			}
			filtered = append(filtered, key)
		}
		common = filtered
	}
	switch format {
	case "yaml":
		root := yaml.Node{Kind: yaml.MappingNode}
		for _, name := range common {
			root.Content = append(root.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: name, HeadComment: configOptionDescription("", name)},
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
					&yaml.Node{Kind: yaml.ScalarNode, Value: name, HeadComment: configOptionDescription(sectionName, name)},
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
			fmt.Fprintf(&output, "# %s\n", configOptionDescription("", name))
			if value, ok := defaults[name]; ok {
				data, err := config.Marshal(map[string]any{name: value})
				if err != nil {
					return nil, err
				}
				output.Write(data)
			} else {
				fmt.Fprintf(&output, "# %s = \"\"\n", name)
			}
		}
		sectionNames := make([]string, 0, len(sections))
		for name := range sections {
			sectionNames = append(sectionNames, name)
		}
		sort.Strings(sectionNames)
		for _, sectionName := range sectionNames {
			fmt.Fprintf(&output, "\n[%s]\n", sectionName)
			for _, name := range sections[sectionName] {
				fmt.Fprintf(&output, "# %s\n# %s = \"\"\n", configOptionDescription(sectionName, name), name)
			}
		}
		return []byte(output.String()), nil
	default:
		return nil, fmt.Errorf("unsupported config format %q (want yaml, toml, or json)", format)
	}
}

// configOptionDescription describes option name as defined by command; pass
// an empty command for top-level options shared by several commands.
func configOptionDescription(command, name string) string {
	spec := cliCommandFlag(command, name)
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
	if err := loadConfigCommandDefaults(args); err != nil {
		printErrorf("failed to load location defaults: %v", err)
		return 1
	}
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	list := cliBool(fs, "list", false)
	notifications := cliBool(fs, "notifications", false)
	format := cliString(fs, "format", "")
	output := cliString(fs, "output", "")
	if err := cliParse(fs, args); err != nil || len(fs.Args()) != 0 {
		printError("usage: " + cliUsage("config"))
		return 1
	}
	_ = basedir
	_ = projectName
	selectedFormat := *format
	if selectedFormat == "" {
		selectedFormat = configFormatFromOutput(*output)
	}
	if *notifications {
		if *list {
			printError("--notifications cannot be combined with --list")
			return 1
		}
		if *format != "" && *format != "toml" {
			printError("notification configuration only supports TOML")
			return 1
		}
		if *output != "" && *output != "-" && filepath.Ext(*output) != ".toml" {
			printError("notification configuration output must use the .toml extension")
			return 1
		}
		selectedFormat = "toml"
	}
	if *list {
		if *format != "" || *output != "" {
			printError("--list cannot be combined with --format or --output")
			return 1
		}
		projectFilter := ""
		if cliOptionSet(fs, "project-name") {
			projectFilter = *projectName
		}
		if projectFilter != "" && !state.IsValidPathElement(projectFilter) {
			printErrorf("invalid project name %q", projectFilter)
			return 1
		}
		resolvedBaseDir, _, err := state.ResolveBaseDir(*basedir)
		if err != nil {
			printErrorf("failed to resolve basedir: %v", err)
			return 1
		}
		common, projects := config.ListPaths(resolvedBaseDir, projectFilter, notification.FileName)
		if len(common) > 0 {
			fmt.Println("Common:")
			for _, path := range common {
				fmt.Printf("  %s\n", path)
			}
		}
		if len(projects) > 0 {
			fmt.Println("Projects:")
			projectNames := make([]string, 0, len(projects))
			for projectName := range projects {
				projectNames = append(projectNames, projectName)
			}
			sort.Strings(projectNames)
			for _, projectName := range projectNames {
				fmt.Printf("  %s:\n", projectName)
				for _, path := range projects[projectName] {
					fmt.Printf("    %s\n", path)
				}
			}
		}
		return 0
	}
	resolvedBaseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve basedir: %v", err)
		return 1
	}
	if *output == "" {
		var selectedOutput string
		var ok bool
		if *notifications {
			selectedOutput, ok = chooseConfigOutputNamed(os.Stdin, os.Stderr, resolvedBaseDir, *projectName, "toml", notification.FileName)
		} else {
			selectedOutput, ok = chooseConfigOutput(os.Stdin, os.Stderr, resolvedBaseDir, *projectName, selectedFormat)
		}
		if !ok {
			return 1
		}
		*output = selectedOutput
	}
	data, err := configTemplate(selectedFormat, configOutputScope(*output, resolvedBaseDir))
	if *notifications {
		data = notification.Template()
		err = nil
	}
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
	if err := os.MkdirAll(filepath.Dir(*output), state.DirectoryMode()); err != nil {
		printErrorf("failed to create config directory for %s: %v", *output, err)
		return 1
	}
	if err := os.WriteFile(*output, data, state.FileMode()); err != nil {
		printErrorf("failed to write config file %s: %v", *output, err)
		return 1
	}
	return 0
}

// Config inventory may list duplicate formats. Do not choose one of them as
// a defaults file, and do not let stale defaults from another command leak in.
func loadConfigCommandDefaults(args []string) error {
	cliConfig = map[string]any{}
	cliIgnoreImplicitLocationDefaults = nil
	inventory := false
	for _, arg := range args {
		if arg == "--list" || arg == "--list=true" {
			inventory = true
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := config.HomeDir()
	if err != nil {
		return err
	}
	for _, item := range []struct{ scope, dir string }{{"global", home}, {"workspace", cwd}} {
		if item.scope != "workspace" && len(config.FilePaths(item.dir)) > 1 {
			continue
		}
		layer, err := config.LoadScope(item.scope, item.dir)
		if err != nil {
			if inventory {
				continue
			}
			return err
		}
		cliConfig = config.Merge(cliConfig, layer.Values)
	}
	if inventory {
		cliIgnoreImplicitLocationDefaults = ignoredImplicitLocationDefaults("config", true)
		// Config inventory is an aggregate view. Its target base/project are
		// selected explicitly by cmdConfig, not by location defaults.
		return nil
	}
	base, _ := configLocationArgs(args)
	if base == "" {
		base = os.Getenv(envBaseDir)
	}
	if base == "" {
		base = configString("basedir", "")
	}
	resolved, _, err := state.ResolveBaseDir(base)
	if err != nil {
		return err
	}
	if len(config.FilePaths(resolved)) == 1 {
		layer, err := config.LoadScope("basedir", resolved)
		if err != nil {
			if !inventory {
				return err
			}
		} else {
			cliConfig = config.Merge(cliConfig, layer.Values)
		}
	}
	return nil
}

func configOutputScope(output, baseDir string) string {
	if filepath.Base(output) == config.WorkspaceFile {
		return "workspace"
	}
	absolute, err := filepath.Abs(output)
	if err != nil {
		return "global"
	}
	if filepath.Dir(absolute) == baseDir {
		return "basedir"
	}
	if filepath.Dir(filepath.Dir(absolute)) == filepath.Join(baseDir, "projects") {
		return "project"
	}
	return "global"
}

func chooseConfigOutput(reader io.Reader, writer io.Writer, baseDir, projectName, format string) (string, bool) {
	return chooseConfigOutputNamed(reader, writer, baseDir, projectName, format, "config."+format)
}

func chooseConfigOutputNamed(reader io.Reader, writer io.Writer, baseDir, projectName, format, fileName string) (string, bool) {
	configHome, err := config.HomeDir()
	if err != nil {
		printErrorf("failed to resolve config home: %v", err)
		return "", false
	}
	type candidate struct {
		label string
		path  string
	}
	candidates := []candidate{
		{label: "global", path: filepath.Join(configHome, fileName)},
		{label: "basedir", path: filepath.Join(baseDir, fileName)},
	}
	if fileName != notification.FileName && format == "toml" {
		if cwd, err := os.Getwd(); err == nil {
			candidates = append(candidates, candidate{label: "workspace", path: filepath.Join(cwd, config.WorkspaceFile)})
		}
	}
	if projectName != "" {
		if projectDir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), projectName); err == nil {
			candidates = append(candidates, candidate{label: "project " + projectName, path: filepath.Join(projectDir, fileName)})
		}
	} else if entries, err := os.ReadDir(filepath.Join(baseDir, "projects")); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && state.IsValidPathElement(entry.Name()) {
				candidates = append(candidates, candidate{label: "project " + entry.Name(), path: filepath.Join(baseDir, "projects", entry.Name(), fileName)})
			}
		}
	}
	fmt.Fprintln(writer, "Config resolution priority (highest to lowest):")
	if fileName == notification.FileName {
		fmt.Fprintln(writer, "  first existing notification file: project > basedir > global (no merge)")
	} else {
		fmt.Fprintln(writer, "  CLI option > environment variable > project > basedir > workspace > global > built-in default")
	}
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
