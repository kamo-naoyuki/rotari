// Package basedirregistry indexes state directories for discovery.
package basedirregistry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const recordSuffix = ".json"

type record struct {
	BaseDir string `json:"base_dir"`
}

// Registry stores one discovery record per basedir under a master directory.
type Registry struct {
	dir string
}

// Open returns the basedir registry under masterDir.
func Open(masterDir string) Registry {
	return Registry{dir: filepath.Join(masterDir, "basedirs")}
}

// Default returns the registry under the resolved master directory.
func Default() (Registry, error) {
	masterDir, err := state.ResolveMasterDir("")
	if err != nil {
		return Registry{}, err
	}
	return Open(masterDir), nil
}

func recordPath(dir, baseDir string) string {
	return filepath.Join(dir, ref(baseDir)+recordSuffix)
}

// Ref returns a short, stable reference to an absolute baseDir: the key of
// its registry record. It names a basedir without revealing its path.
func Ref(baseDir string) string {
	return ref(baseDir)
}

func ref(baseDir string) string {
	sum := sha256.Sum256([]byte(baseDir))
	return hex.EncodeToString(sum[:])[:32]
}

// Discover lists the basedirs registered under masterDir without writing.
// A master directory from before the basedir registry existed has only run
// records; their basedirs are listed instead and fromRuns is set, so that a
// caller allowed to write can register them.
func Discover(masterDir string) (baseDirs []string, fromRuns bool, err error) {
	baseDirs, err = Open(masterDir).BaseDirs()
	if err != nil {
		return nil, false, fmt.Errorf("failed to read basedir registry: %w", err)
	}
	if len(baseDirs) > 0 {
		return baseDirs, false, nil
	}
	baseDirs, err = runregistry.Open(masterDir).BaseDirs()
	if err != nil {
		return nil, false, err
	}
	return baseDirs, len(baseDirs) > 0, nil
}

// Register adds baseDir to the discovery index. Registration is idempotent.
func (registry Registry) Register(baseDir string) error {
	baseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return err
	}
	path := recordPath(registry.dir, baseDir)
	var existing record
	if err := state.NewStore(state.DirectoryMode(), state.FileMode()).ReadJSON(path, &existing); err == nil {
		if existing.BaseDir != baseDir {
			return fmt.Errorf("basedir registry hash collision for %q", baseDir)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return state.WriteJSON(path, record{BaseDir: baseDir})
}

// BaseDirs lists registered basedirs. Missing or malformed records are skipped.
func (registry Registry) BaseDirs() ([]string, error) {
	entries, err := os.ReadDir(registry.dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	store := state.NewStore(state.DirectoryMode(), state.FileMode())
	baseDirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != recordSuffix {
			continue
		}
		var value record
		if err := store.ReadJSON(filepath.Join(registry.dir, entry.Name()), &value); err != nil || value.BaseDir == "" {
			continue
		}
		baseDirs = append(baseDirs, value.BaseDir)
	}
	return baseDirs, nil
}

// Missing returns registered basedirs whose directories no longer exist.
func (registry Registry) Missing() ([]string, error) {
	baseDirs, err := registry.BaseDirs()
	if err != nil {
		return nil, err
	}
	missing := make([]string, 0)
	for _, baseDir := range baseDirs {
		if _, err := os.Stat(baseDir); os.IsNotExist(err) {
			missing = append(missing, baseDir)
		} else if err != nil {
			return nil, err
		}
	}
	return missing, nil
}

// Remove unregisters baseDir if its record still refers to that exact path.
func (registry Registry) Remove(baseDir string) (bool, error) {
	baseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return false, err
	}
	path := recordPath(registry.dir, baseDir)
	var existing record
	if err := state.NewStore(state.DirectoryMode(), state.FileMode()).ReadJSON(path, &existing); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if existing.BaseDir != baseDir {
		return false, fmt.Errorf("basedir registry hash collision for %q", baseDir)
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
