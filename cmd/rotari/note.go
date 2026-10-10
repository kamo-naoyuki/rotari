package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/queueops"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdNote adds a note to a run, or to one job attempt of a run, named by its
// exact run ID or attempt ID. Words after the target are joined with spaces,
// so an unquoted note is kept whole.
func cmdNote(args []string) int {
	fs := flag.NewFlagSet("note", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if fs.NArg() < 2 {
		printError("usage: " + cliUsage("note"))
		return 1
	}
	target, text := fs.Arg(0), strings.Join(fs.Args()[1:], " ")
	var baseDir, project, runID, attemptID string
	var err error
	if strings.HasPrefix(target, "att_") {
		baseDir, project, runID, _, err = resolveCLIAttempt(target, *basedir, *projectName, "")
		attemptID = target
	} else {
		baseDir, project, runID, err = resolveCLIExistingRunID(*basedir, *projectName, target)
	}
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	note, err := queueops.AddNote(paths, runID, attemptID, text, time.Now())
	if err != nil {
		printError(err)
		return 1
	}
	if note.AttemptID != "" {
		fmt.Printf("Added a note to attempt %s (job %s) of run %s.\n", note.AttemptID, note.JobID, runID)
	} else {
		fmt.Printf("Added a note to run %s.\n", runID)
	}
	return 0
}
