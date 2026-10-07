package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const WorkspaceFile = ".rotari.toml"

// Source identifies a real configuration file, not a synthesized view.
type Source = model.ConfigSource

// Loaded holds file values captured once, independently of runtime overrides.
type Loaded struct {
	Values  map[string]any
	Sources []Source
}

// LoadScope loads at most one file, failing on invalid discovered files.
func LoadScope(scope, directory string) (Loaded, error) {
	loaded := Loaded{Values: map[string]any{}}
	var paths []string
	if scope == "workspace" {
		path := filepath.Join(directory, WorkspaceFile)
		if _, err := os.Stat(path); err == nil {
			paths = []string{path}
		} else if !errors.Is(err, os.ErrNotExist) {
			return loaded, fmt.Errorf("workspace config %s: %w", path, err)
		}
	} else {
		for _, ext := range extensions {
			path := filepath.Join(directory, "config"+ext)
			if _, err := os.Stat(path); err == nil {
				paths = append(paths, path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return loaded, fmt.Errorf("%s config %s: %w", scope, path, err)
			}
		}
	}
	if len(paths) > 1 {
		return loaded, fmt.Errorf("multiple config files found in %s scope %s: %v", scope, directory, paths)
	}
	if len(paths) == 0 {
		return loaded, nil
	}
	path, err := filepath.Abs(paths[0])
	if err != nil {
		return loaded, err
	}
	values, err := LoadPath(path)
	if err != nil {
		return loaded, fmt.Errorf("%s config %s: %w", scope, path, err)
	}
	if err := Validate(scope, path, values); err != nil {
		return loaded, err
	}
	loaded.Values = Merge(nil, values)
	loaded.Sources = []Source{{Scope: scope, Path: path}}
	return loaded, nil
}

// Validate prevents location cycles, including command-specific location keys.
func Validate(scope, path string, values map[string]any) error {
	for key, value := range values {
		if (scope == "basedir" || scope == "project") && key == "basedir" || scope == "project" && key == "project-name" {
			return fmt.Errorf("%s config %s: forbidden key %q", scope, path, key)
		}
		if section, ok := value.(map[string]any); ok {
			if err := Validate(scope, path, section); err != nil {
				return err
			}
		}
	}
	return nil
}

// Merge makes an independent deep copy. Maps recurse, arrays replace, and null
// is unspecified; false, zero and empty strings are retained.
func Merge(lower, higher map[string]any) map[string]any {
	result := make(map[string]any)
	for key, value := range lower {
		if value != nil {
			result[key] = clone(value)
		}
	}
	for key, value := range higher {
		if value == nil {
			continue
		}
		if section, ok := value.(map[string]any); ok {
			previous, _ := result[key].(map[string]any)
			result[key] = Merge(previous, section)
		} else {
			result[key] = clone(value)
		}
	}
	return result
}

func clone(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return Merge(nil, v)
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = clone(item)
		}
		return result
	default:
		return value
	}
}

func (loaded *Loaded) Add(layer Loaded) {
	loaded.Values = Merge(loaded.Values, layer.Values)
	loaded.Sources = append(loaded.Sources, layer.Sources...)
}

// Marshal produces canonical TOML from file values only.
func Marshal(values map[string]any) ([]byte, error) {
	var output bytes.Buffer
	err := toml.NewEncoder(&output).Encode(Merge(nil, values))
	return output.Bytes(), err
}

func (loaded Loaded) Snapshot() (*model.FileConfigSnapshot, error) {
	data, err := Marshal(loaded.Values)
	if err != nil {
		return nil, err
	}
	return &model.FileConfigSnapshot{Content: string(data), Sources: append([]Source(nil), loaded.Sources...)}, nil
}

// Load reads all ordinary scopes; cwd is explicitly supplied by the caller.
// A Web server captures it at startup and never substitutes a job's cwd.
func Load(cwd, baseDir, projectName string) (Loaded, error) {
	loaded := Loaded{Values: map[string]any{}}
	home, err := HomeDir()
	if err != nil {
		return loaded, err
	}
	scopes := []struct{ scope, dir string }{{"global", home}, {"workspace", cwd}, {"basedir", baseDir}}
	if projectName != "" {
		dir, err := state.SafeJoin(filepath.Join(baseDir, "projects"), projectName)
		if err != nil {
			return loaded, err
		}
		scopes = append(scopes, struct{ scope, dir string }{"project", dir})
	}
	for _, item := range scopes {
		layer, err := LoadScope(item.scope, item.dir)
		if err != nil {
			return loaded, err
		}
		loaded.Add(layer)
	}
	return loaded, nil
}
