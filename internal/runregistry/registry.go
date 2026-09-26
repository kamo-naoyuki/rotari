// Package runregistry indexes where each run lives.
//
// Run IDs do not encode a location, so the master directory keeps one record
// per run, <masterdir>/runs/<run-id>.json, naming the run's base directory and
// project. The registry is only an index: run files remain authoritative, and
// an entry whose run directory is gone is stale rather than a reason to fall
// back to another run.
package runregistry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

// Location is one registry record.
type Location struct {
	BaseDir     string `json:"base_dir"`
	ProjectName string `json:"project_name"`
	RunID       string `json:"run_id"`
}

// Valid reports whether the record is well formed: an absolute base
// directory and path-safe project and run IDs.
func (location Location) Valid() bool {
	return location.BaseDir != "" && filepath.IsAbs(location.BaseDir) &&
		state.IsValidPathElement(location.ProjectName) &&
		state.IsValidPathElement(location.RunID)
}

// Exists reports whether the run directory the record names exists.
func (location Location) Exists() bool {
	if location.BaseDir == "" || !filepath.IsAbs(location.BaseDir) {
		return false
	}
	path := filepath.Clean(location.BaseDir)
	for _, element := range []string{"projects", location.ProjectName, "runs", location.RunID} {
		var err error
		if path, err = state.SafeJoin(path, element); err != nil {
			return false
		}
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Registry is the run index under one master directory.
type Registry struct {
	dir string
}

// Open returns the registry under masterDir.
func Open(masterDir string) Registry {
	return Registry{dir: filepath.Join(masterDir, "runs")}
}

// Default returns the registry under the resolved master directory; see
// state.ResolveMasterDir.
func Default() (Registry, error) {
	masterDir, err := state.ResolveMasterDir("")
	if err != nil {
		return Registry{}, err
	}
	return Open(masterDir), nil
}

func store() state.Store {
	return state.NewStore(state.DirectoryMode(), state.FileMode())
}

// entryPath returns the record path for runID.
func (registry Registry) entryPath(runID string) (string, error) {
	if !state.IsValidPathElement(runID) {
		return "", fmt.Errorf("invalid run id %q", runID)
	}
	return state.SafeJoin(registry.dir, runID+".json")
}

// Register records location. Registering the same mapping again is a no-op;
// mapping a registered run ID to another location fails.
func (registry Registry) Register(location Location) error {
	baseDir, err := filepath.Abs(location.BaseDir)
	if err != nil {
		return err
	}
	location.BaseDir = baseDir
	if err := os.MkdirAll(registry.dir, state.DirectoryMode()); err != nil {
		return err
	}
	path, err := registry.entryPath(location.RunID)
	if err != nil {
		return err
	}
	var existing Location
	if err := store().ReadJSON(path, &existing); err == nil {
		if existing != location {
			return fmt.Errorf("run id %q is already registered to another location", location.RunID)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("run id %q is already registered to another location", location.RunID)
	}
	return state.WriteJSON(path, location)
}

// Unregister removes the record for runID if there is one.
func (registry Registry) Unregister(runID string) error {
	path, err := registry.entryPath(runID)
	if err != nil {
		return err
	}
	// codeql[go/path-injection]: path is the validated registry file path.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Lookup returns the record for runID. It reports false when the run is not
// registered and an error when the record is unreadable or inconsistent.
func (registry Registry) Lookup(runID string) (Location, bool, error) {
	path, err := registry.entryPath(runID)
	if err != nil {
		return Location{}, false, err
	}
	var location Location
	if err := store().ReadJSON(path, &location); err != nil {
		if os.IsNotExist(err) {
			return Location{}, false, nil
		}
		return Location{}, false, fmt.Errorf("invalid run registry entry for %q: %w", runID, err)
	}
	if location.BaseDir == "" || location.ProjectName == "" || location.RunID != runID {
		return Location{}, false, fmt.Errorf("invalid run registry entry for %q", runID)
	}
	return location, true, nil
}

// Orphans lists well-formed records whose run directory is missing, and the
// paths of malformed records, which are left for manual inspection.
func (registry Registry) Orphans() ([]Location, []string, error) {
	entries, err := os.ReadDir(registry.dir)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	orphans := make([]Location, 0)
	skipped := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(registry.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		var location Location
		if err := json.Unmarshal(data, &location); err != nil || !location.Valid() {
			skipped = append(skipped, path)
			continue
		}
		if !location.Exists() {
			orphans = append(orphans, location)
		}
	}
	return orphans, skipped, nil
}

// RemoveOrphan removes planned if its record is unchanged and its run
// directory is still missing. It reports whether the record was removed.
func (registry Registry) RemoveOrphan(planned Location) (bool, error) {
	path, err := registry.entryPath(planned.RunID)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read registry entry %q: %w", planned.RunID, err)
	}
	var current Location
	if err := json.Unmarshal(data, &current); err != nil {
		return false, fmt.Errorf("failed to read registry entry %q: %w", planned.RunID, err)
	}
	if current != planned || current.Exists() {
		return false, nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to remove registry entry %q: %w", planned.RunID, err)
	}
	return true, nil
}

// BaseDirs lists the base directories named by readable records, with
// duplicates. Unreadable records are ignored; discovery is not exhaustive.
func (registry Registry) BaseDirs() ([]string, error) {
	entries, err := os.ReadDir(registry.dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	baseDirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var location Location
		if err := store().ReadJSON(filepath.Join(registry.dir, entry.Name()), &location); err != nil {
			continue
		}
		if location.BaseDir != "" {
			baseDirs = append(baseDirs, location.BaseDir)
		}
	}
	return baseDirs, nil
}
