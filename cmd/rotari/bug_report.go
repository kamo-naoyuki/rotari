package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rotari-dev/rotari/internal/bugreport"
	"github.com/rotari-dev/rotari/internal/state"
)

// cmdBugReport prints a Markdown bug report, or writes it with the files it
// draws on as a .tar.gz bundle.
func cmdBugReport(args []string) int {
	fs := flag.NewFlagSet("bug-report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectOption := cliString(fs, "project-name", "")
	runID := cliString(fs, "run-id", "")
	var jobIDs stringSliceFlag
	cliValue(fs, &jobIDs, "job-id")
	output := cliString(fs, "output", "")
	fullOutput := cliBool(fs, "full-output", false)
	noOutput := cliBool(fs, "no-output", false)
	noRedact := cliBool(fs, "no-redact", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) != 0 || (*fullOutput && *noOutput) {
		printError("usage: " + cliUsage("bug-report"))
		return 1
	}
	if *output != "" && !strings.HasSuffix(*output, ".tar.gz") && !strings.HasSuffix(*output, ".tgz") {
		printError("--output must name a .tar.gz or .tgz file")
		return 1
	}
	if *runID == "" {
		for _, id := range jobIDs {
			if payload, err := state.DecodeAttemptID(id); err == nil {
				*runID = payload.RunID
				break
			}
		}
	}

	options := bugreport.Options{Version: versionString(), Environ: os.Environ(), Redact: !*noRedact, JobIDs: jobIDs}
	switch {
	case *fullOutput:
		options.Output = bugreport.OutputFull
	case *noOutput:
		options.Output = bugreport.OutputNone
	}
	if err := locateBugReport(&options, *basedir, *projectOption, *runID); err != nil {
		printError(err)
		return 1
	}
	options.Facts = bugReportFacts(options.BaseDir, options.Project)

	bundle, err := bugreport.Collect(options)
	if err != nil {
		printError(err)
		return 1
	}
	if *output == "" {
		fmt.Print(bundle.Summary)
		return 0
	}
	now := time.Now()
	root := "rotari-bug-report-" + now.UTC().Format("20060102-150405")
	if err := bugreport.WriteTarGz(*output, root, bundle, now); err != nil {
		printErrorf("failed to write %s: %v", *output, err)
		return 1
	}
	fmt.Printf("Wrote %s (%d files). Review it before sharing: job output and command lines are not checked for secrets.\n", *output, len(bundle.Files)+1)
	return 0
}

// locateBugReport selects the basedir, project, and run. Without --run-id
// or --job-id, a project without runs, or no project at all, still gives a
// report of the environment.
func locateBugReport(options *bugreport.Options, baseDirOption, projectOption, runID string) error {
	baseDir, _, err := state.ResolveBaseDir(baseDirOption)
	if err != nil {
		return fmt.Errorf("failed to resolve basedir: %w", err)
	}
	options.BaseDir = baseDir
	selector := runID
	if selector == "" {
		selector = "latest"
	}
	resolvedBase, project, resolvedRun, err := resolveCLIExistingRunID(baseDirOption, projectOption, selector)
	if err == nil {
		options.BaseDir, options.Project, options.RunID = resolvedBase, project, resolvedRun
		return nil
	}
	if runID != "" || len(options.JobIDs) > 0 {
		return err
	}
	fmt.Fprintf(os.Stderr, "No run collected: %v\n", err)
	if project, projectErr := state.ResolveProjectName(baseDir, projectOption); projectErr == nil {
		options.Project = project
	}
	return nil
}

// bugReportFacts are rotari's own views of the state: `info --json` and,
// for a project, `check --deep --json`.
func bugReportFacts(baseDir, project string) []bugreport.File {
	var facts []bugreport.File
	masterDir, err := state.ResolveMasterDir("")
	if err == nil {
		var report infoReport
		if report, err = buildInfoReport(baseDir, masterDir, project); err == nil {
			if data, err := json.MarshalIndent(report, "", "  "); err == nil {
				facts = append(facts, bugreport.File{Name: "info.json", Data: append(data, '\n')})
			}
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "info not collected: %v\n", err)
	}
	if project == "" {
		return facts
	}
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		return facts
	}
	result, err := checkProjectWithOptions(paths, true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check not collected: %v\n", err)
		return facts
	}
	var buffer bytes.Buffer
	if writeProjectCheck(&buffer, project, result, true) == nil {
		facts = append(facts, bugreport.File{Name: "check.json", Data: buffer.Bytes()})
	}
	return facts
}
