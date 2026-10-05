package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdReset clears the queued jobs for the next run without changing an active
// or interrupted run.
func cmdReset(args []string) int {
	fs := flag.NewFlagSet("reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	queueNameOption := cliString(fs, "project-name", "")
	quiet := cliBool(fs, "quiet", false)
	guard := cliGuardFlags(fs)
	if rejectResetRecovery(args) {
		return 1
	}
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) > 1 || (len(fs.Args()) == 1 && cliOptionSet(fs, "project-name")) {
		printError("usage: " + cliUsage("reset"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*queueNameOption = fs.Args()[0]
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printErrorf("failed to resolve state directory: %v", err)
		return 1
	}
	queueName, err := state.ResolveProjectName(baseDir, *queueNameOption)
	if err != nil {
		printError(err)
		return 1
	}
	if !resolve.ProjectExists(baseDir, queueName) {
		if err := model.ValidateReservedName("project", queueName); err != nil {
			printError(err)
			return 1
		}
	}
	paths, err := state.ResolveProjectPaths(baseDir, queueName)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	result, err := project.Reset(paths, guard.guard())
	if err != nil {
		printError(err)
		return 1
	}
	message := fmt.Sprintf("reset project=%s cleared=%d job(s)", queueName, result.Cleared)
	guard.printResult(message, *quiet)
	return 0
}

func rejectResetRecovery(args []string) bool {
	if _, ok := os.LookupEnv(envResetRecover); ok {
		printErrorf("reset no longer accepts %s; recover an interrupted run with rotari unlock", envResetRecover)
		return true
	}
	for _, arg := range args {
		if arg == "--recover" || arg == "-recover" || strings.HasPrefix(arg, "--recover=") || strings.HasPrefix(arg, "-recover=") {
			printError("reset no longer accepts --recover; recover an interrupted run with rotari unlock")
			return true
		}
	}
	return false
}
