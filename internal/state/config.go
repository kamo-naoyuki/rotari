package state

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	baseDirEnv      = "ROTARI_BASEDIR"
	masterDirEnv    = "ROTARI_MASTERDIR"
	privateStateEnv = "ROTARI_PRIVATE_STATE"
	xdgStateHomeEnv = "XDG_STATE_HOME"
)

func ResolveBaseDir(cliBaseDir string) (string, bool, error) {
	baseDir, explicit, err := resolveBaseDir(cliBaseDir)
	if err != nil {
		return "", false, err
	}
	baseDir, err = absoluteDir(baseDir)
	return baseDir, explicit, err
}

func resolveBaseDir(cliBaseDir string) (string, bool, error) {
	if cliBaseDir != "" {
		return cliBaseDir, true, nil
	}
	if value := os.Getenv(baseDirEnv); value != "" {
		return value, true, nil
	}
	if cwd, err := os.Getwd(); err == nil {
		localState := filepath.Join(cwd, ".rotari-state")
		if info, err := os.Stat(localState); err == nil && info.IsDir() {
			return localState, false, nil
		}
	}
	if value := os.Getenv(xdgStateHomeEnv); value != "" {
		return filepath.Join(value, "rotari"), false, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, err
	}
	return filepath.Join(home, ".local", "state", "rotari"), false, nil
}

// ResolveMasterDir resolves the master directory that holds the run and
// server registries: cliMasterDir, then ROTARI_MASTERDIR, then
// $XDG_STATE_HOME/rotari/master, then ~/.local/state/rotari/master.
func ResolveMasterDir(cliMasterDir string) (string, error) {
	masterDir, err := resolveMasterDir(cliMasterDir)
	if err != nil {
		return "", err
	}
	return absoluteDir(masterDir)
}

func resolveMasterDir(cliMasterDir string) (string, error) {
	if cliMasterDir != "" {
		return cliMasterDir, nil
	}
	if value := os.Getenv(masterDirEnv); value != "" {
		return value, nil
	}
	if value := os.Getenv(xdgStateHomeEnv); value != "" {
		return filepath.Join(value, "rotari", "master"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "rotari", "master"), nil
}

func privateStateEnabled() bool {
	return os.Getenv(privateStateEnv) == "true"
}

func mode(privateMode, sharedMode os.FileMode) os.FileMode {
	if privateStateEnabled() {
		return privateMode
	}
	return sharedMode
}

func DirectoryMode() os.FileMode { return mode(0o700, 0o755) }
func FileMode() os.FileMode      { return mode(0o600, 0o644) }
func ScriptMode() os.FileMode    { return mode(0o700, 0o755) }

// absoluteDir resolves a state directory given relative to the working
// directory, as `--basedir ../lab` is, to an absolute, clean path: state file
// paths are validated as absolute paths without "..", and registries record
// state directories by their absolute path.
func absoluteDir(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve state directory %q: %w", dir, err)
	}
	return absolute, nil
}
