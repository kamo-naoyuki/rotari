package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamo-naoyuki/rotari/internal/config"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func cmdInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 2 {
		printError("usage: " + cliUsage("init"))
		return 1
	}
	base := ".rotari-state"
	if len(fs.Args()) > 0 {
		base = fs.Args()[0]
	}
	if base == "" || filepath.IsAbs(base) {
		printError("init basedir must be a non-empty relative path")
		return 1
	}
	values := map[string]any{"basedir": base}
	if len(fs.Args()) == 2 {
		name := fs.Args()[1]
		if !state.IsValidPathElement(name) {
			printErrorf("invalid project name %q", name)
			return 1
		}
		if err := model.ValidateReservedName("project", name); err != nil {
			printError(err.Error())
			return 1
		}
		values["project-name"] = name
	}
	data, err := config.Marshal(values)
	if err != nil {
		printError(err.Error())
		return 1
	}
	// Link an already complete temporary file without replacing an existing
	// workspace file, including symlinks. Never touch state or registries.
	tmp, err := os.CreateTemp(".", ".rotari-init-*")
	if err != nil {
		printError(err.Error())
		return 1
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(state.FileMode()); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Link(tmp.Name(), config.WorkspaceFile)
	}
	if err != nil {
		printErrorf("refusing to replace or failed to write %s: %v", config.WorkspaceFile, err)
		return 1
	}
	fmt.Println("Created " + config.WorkspaceFile)
	return 0
}
