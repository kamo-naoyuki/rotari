package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/projectrun"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

type projectCheck = projectrun.Check

// cmdCheck validates whether the selected project state is ready for queue
// edits, runs, and optional deep local execution checks.
func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectNameOption := cliString(fs, "project-name", "")
	jsonOutput := cliBool(fs, "json", false)
	deep := cliBool(fs, "deep", false)
	quiet := cliBool(fs, "quiet", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && cliOptionSet(fs, "project-name")) {
		printError("usage: " + cliUsage("check"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*projectNameOption = fs.Args()[0]
	}

	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	projectName, err := state.ResolveProjectName(baseDir, *projectNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, projectName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	result, err := checkProjectWithOptions(paths, *deep)
	if err != nil {
		printErrorf("failed to check project: %v", err)
		return 1
	}

	if *jsonOutput || !*quiet || !result.Runnable {
		if err := writeProjectCheck(os.Stdout, projectName, result, *jsonOutput); err != nil {
			printErrorf("failed to print project check: %v", err)
			return 1
		}
	}
	if result.Runnable {
		return 0
	}
	return 1
}

func writeProjectCheck(writer io.Writer, projectName string, result projectCheck, jsonOutput bool) error {
	if jsonOutput {
		var queued *int
		if result.QueuedKnown {
			queued = &result.Queued
		}
		return json.NewEncoder(writer).Encode(struct {
			Project  string `json:"project"`
			State    string `json:"state"`
			Runnable bool   `json:"runnable"`
			Queued   *int   `json:"queued"`
			Lock     string `json:"lock"`
			RunID    string `json:"run_id,omitempty"`
			Revision string `json:"revision"`
		}{projectName, result.State, result.Runnable, queued, result.Lock, result.RunID, result.Revision})
	}

	fmt.Fprintf(writer, "project=%s state=%s runnable=%t queued=", projectName, result.State, result.Runnable)
	if result.QueuedKnown {
		fmt.Fprintf(writer, "%d", result.Queued)
	} else {
		fmt.Fprint(writer, "unknown")
	}
	fmt.Fprintf(writer, " lock=%s", result.Lock)
	if result.RunID != "" {
		fmt.Fprintf(writer, " run_id=%s", result.RunID)
	}
	fmt.Fprintf(writer, " revision=%s", result.Revision)
	_, err := fmt.Fprintln(writer)
	return err
}

func checkProject(paths state.ProjectPaths) (projectCheck, error) {
	return checkProjectWithOptions(paths, false)
}

func checkProjectWithOptions(paths state.ProjectPaths, deep bool) (projectCheck, error) {
	var deepCheck func(model.Queue) error
	if deep {
		deepCheck = validateLocalExecutionEnvironment
	}
	return projectRunner().Check(paths, deepCheck)
}
