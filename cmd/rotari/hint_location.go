package main

import (
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// hintLocation returns the options that make a printed command target paths'
// project: --project-name always, and --basedir only when paths' state
// directory is not the one a command started here without --basedir would
// use.
func hintLocation(paths state.ProjectPaths) string {
	location := "--project-name " + executor.ShellQuote(paths.ProjectName)
	if isImplicitBaseDir(paths.BaseDir) {
		return location
	}
	return "--basedir " + executor.ShellQuote(paths.BaseDir) + " " + location
}

// runHintLocation returns the options, each followed by a space, that a
// printed command naming one of paths' runs needs: none when paths' state
// directory is the implicit one, and otherwise hintLocation's, so the command
// works even for a run the master registry does not list.
func runHintLocation(paths state.ProjectPaths) string {
	if isImplicitBaseDir(paths.BaseDir) {
		return ""
	}
	return hintLocation(paths) + " "
}

// isImplicitBaseDir reports whether baseDir is the state directory a command
// uses without --basedir: ROTARI_BASEDIR, then the configured basedir, then
// the default, in the order cliString applies them.
func isImplicitBaseDir(baseDir string) bool {
	value := configString("basedir", "")
	if env, ok := os.LookupEnv(envBaseDir); ok {
		value = env
	}
	implicit, _, err := state.ResolveBaseDir(value)
	return err == nil && filepath.Clean(implicit) == filepath.Clean(baseDir)
}
