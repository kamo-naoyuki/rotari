package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Warnf reports a config file that cannot be inspected, read, or parsed; such
// a file is skipped or treated as empty. It writes to stderr unless replaced.
var Warnf = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

var extensions = []string{".yaml", ".toml", ".json"}

// HomeDir returns the global config directory: $XDG_CONFIG_HOME/rotari, or
// ~/.config/rotari.
func HomeDir() (string, error) {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return filepath.Join(value, "rotari"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "rotari"), nil
}

// LoadFile parses the config file in directory, or returns an empty config
// when there is none. More than one config file there is an error.
func LoadFile(directory string) (map[string]any, error) {
	paths := FilePaths(directory)
	if len(paths) > 1 {
		return nil, fmt.Errorf("multiple config files found in %s: %s", directory, strings.Join(paths, ", "))
	}
	if len(paths) == 0 {
		return map[string]any{}, nil
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		Warnf("WARNING: cannot read config %s: %v", paths[0], err)
		return map[string]any{}, nil
	}
	config, err := Parse(paths[0], data)
	if err != nil {
		Warnf("WARNING: cannot parse config %s: %v", paths[0], err)
		return map[string]any{}, nil
	}
	return config, nil
}

// Parse parses config data in the format that path's extension names.
func Parse(path string, data []byte) (map[string]any, error) {
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

// FilePaths returns the config files that exist in directory.
func FilePaths(directory string) []string {
	paths := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		path := filepath.Join(directory, "config"+extension)
		// codeql[go/path-injection]: path is built from the trusted config directory and fixed suffix.
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			Warnf("WARNING: cannot inspect config %s: %v", path, err)
		}
	}
	return paths
}

// EffectivePath returns the config file that applies to projectName in
// baseDir, or "" when there is none; see PathsForRun.
func EffectivePath(baseDir, projectName string) string {
	paths := PathsForRun(baseDir, projectName)
	if len(paths) == 0 {
		return ""
	}
	return paths[len(paths)-1]
}

// GlobalPath returns the global config file, or "" when there is none.
func GlobalPath() string {
	configHome, err := HomeDir()
	if err != nil {
		return ""
	}
	paths := FilePaths(configHome)
	if len(paths) == 0 {
		return ""
	}
	return paths[len(paths)-1]
}

// PathsForRun returns the config files of the first scope that has any: the
// project, then baseDir, then the global config directory.
func PathsForRun(baseDir, projectName string) []string {
	if projectName != "" {
		if projectDir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), projectName); err == nil {
			if paths := FilePaths(projectDir); len(paths) > 0 {
				return paths
			}
		}
	}
	if paths := FilePaths(baseDir); len(paths) > 0 {
		return paths
	}
	if configHome, err := HomeDir(); err == nil {
		return FilePaths(configHome)
	}
	return nil
}

// ListPaths returns every config file found across all scopes for
// `config --list`. Unlike PathsForRun, it does not stop at the first
// scope that has files: --list is meant to show the user everything, not just
// the one scope that would take effect.
func ListPaths(baseDir, projectName string) (common []string, projects map[string][]string) {
	projects = make(map[string][]string)
	if configHome, err := HomeDir(); err == nil {
		common = append(common, FilePaths(configHome)...)
	}
	common = append(common, FilePaths(baseDir)...)
	if projectName != "" {
		projectDir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), projectName)
		if err != nil {
			return common, projects
		}
		if paths := FilePaths(projectDir); len(paths) > 0 {
			projects[projectName] = paths
		}
		return common, projects
	}
	entries, err := os.ReadDir(filepath.Join(baseDir, "projects"))
	if err != nil {
		return common, projects
	}
	for _, entry := range entries {
		if !entry.IsDir() || !state.IsValidPathElement(entry.Name()) {
			continue
		}
		projectDir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), entry.Name())
		if err != nil {
			continue
		}
		if paths := FilePaths(projectDir); len(paths) > 0 {
			projects[entry.Name()] = paths
		}
	}
	return common, projects
}
