package state

import (
	"os"
	"path/filepath"
)

const (
	baseDirEnv      = "ROTARI_BASEDIR"
	privateStateEnv = "ROTARI_PRIVATE_STATE"
	xdgStateHomeEnv = "XDG_STATE_HOME"
)

func ResolveBaseDir(cliBaseDir string) (string, bool, error) {
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
