package state

import (
	"fmt"
	"path/filepath"
)

type ProjectPaths struct {
	BaseDir         string
	BaseDirExplicit bool
	ProjectName     string
	ProjectDir      string
	QueueFile       string
	MetaFile        string
	StateLockFile   string
	LockFile        string
	RunsDir         string
}

func ResolveProjectPaths(cliBaseDir, projectName string) (ProjectPaths, error) {
	if !IsValidPathElement(projectName) {
		return ProjectPaths{}, fmt.Errorf("invalid project name %q", projectName)
	}
	baseDir, explicit, err := ResolveBaseDir(cliBaseDir)
	if err != nil {
		return ProjectPaths{}, err
	}
	projectDir := filepath.Join(baseDir, "projects", projectName)
	return ProjectPaths{
		BaseDir:         baseDir,
		BaseDirExplicit: explicit,
		ProjectName:     projectName,
		ProjectDir:      projectDir,
		QueueFile:       filepath.Join(projectDir, "queue.json"),
		MetaFile:        filepath.Join(projectDir, "meta.json"),
		StateLockFile:   filepath.Join(projectDir, "state.lock"),
		LockFile:        filepath.Join(projectDir, "running.lock"),
		RunsDir:         filepath.Join(projectDir, "runs"),
	}, nil
}
