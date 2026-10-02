package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
	"github.com/kamo-naoyuki/rotari/internal/workflowstate"
)

type (
	importPlan       = workflowstate.Plan
	importPlanJob    = workflowstate.PlanJob
	importPlanSource = workflowstate.PlanSource
)

func cmdImport(args []string) int {
	manifestPath := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		manifestPath = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	overwrite := cliBool(fs, "overwrite", false)
	jsonOutput := cliBool(fs, "json", false)
	guard := cliGuardFlags(fs)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	positional := fs.Args()
	if manifestPath == "" && len(positional) > 0 {
		manifestPath = positional[0]
		positional = positional[1:]
	}
	if manifestPath == "" || len(positional) > 1 {
		printError("usage: " + cliUsage("import"))
		return 1
	}
	if len(positional) == 1 && cliOptionSet(fs, "project-name") {
		printError("usage: " + cliUsage("import"))
		return 1
	}
	if len(positional) == 1 {
		*projectName = positional[0]
	}
	var data []byte
	var err error
	if manifestPath == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(manifestPath)
	}
	if err != nil {
		printErrorf("failed to read workflow manifest: %v", err)
		return 1
	}
	format, err := workflowFormatFromPath(manifestPath)
	if err != nil {
		printError(err)
		return 1
	}
	manifest, err := workflow.Decode(strings.NewReader(string(data)), format)
	if err != nil {
		printError(err)
		return 1
	}
	baseDir, _, err := state.ResolveBaseDir(*basedir)
	if err != nil {
		printError(err)
		return 1
	}
	resolvedProject, err := state.ResolveProjectName(baseDir, *projectName)
	if err != nil {
		printError(err)
		return 1
	}
	plan, err := workflowstate.Import{
		Store: jsonStore(), BaseDir: baseDir, Project: resolvedProject, Manifest: manifest,
		Overwrite: *overwrite, Guard: guard.guard(), NewJobID: makeJobID,
		Validate: func(queue model.Queue) error { return projectRunner().ValidateQueue(queue, "", nil, nil) },
		Register: registerBasedir,
	}.Apply()
	if err != nil {
		printError(err)
		return 1
	}
	if err := writeImportPlan(plan, *jsonOutput); err != nil {
		printErrorf("failed to encode import plan: %v", err)
		return 1
	}
	return 0
}

func writeImportPlan(plan importPlan, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	}
	for _, job := range plan.Jobs {
		fields := append([]string{job.Status, "job_id=" + job.ID}, optionalField("job_name", job.Name)...)
		fields = append(fields, importSourceFields(job.Source)...)
		printImportPlanLine(job.Status, append(fields, importCommandField(job.Command)))
		for _, task := range job.Tasks {
			fields := append([]string{" ", task.Status, "task_id=" + task.ID}, importSourceFields(task.Source)...)
			printImportPlanLine(task.Status, fields)
		}
	}
	for _, removed := range plan.Removed {
		fields := append([]string{"remove", "job_id=" + removed.JobID}, optionalField("job_name", removed.Name)...)
		fields = append(fields, "source_run_id="+removed.RunID)
		printImportPlanLine("remove", append(fields, importCommandField(removed.Command)))
	}
	fmt.Printf("revision=%s\n", plan.Revision)
	return nil
}

// printImportPlanLine colors a plan line by the job's status. Successful
// results are cyan because they are informational; marked statuses and
// removals are yellow because they need attention; other jobs are green like
// other queue changes such as add.
func printImportPlanLine(status string, fields []string) {
	labelColor := green
	switch {
	case status == model.StatusSuccess:
		labelColor = cyan
	case status == "remove" || strings.HasSuffix(status, ")"):
		labelColor = yellow
	}
	fmt.Println(colorKeyValueMessage(strings.Join(fields, " "), labelColor))
}

// importCommandField must be the last field: its value runs to the end of the
// line and may contain spaces.
func importCommandField(command []string) string {
	return "command=" + strings.Join(command, " ")
}

func importSourceFields(source *importPlanSource) []string {
	if source == nil {
		return nil
	}
	// The attempt ID encodes the source run and job IDs, so the human view
	// omits them; JSON output keeps every field.
	return append(optionalField("source_attempt_id", source.AttemptID), "source_status="+source.Status)
}

func optionalField(key, value string) []string {
	if value == "" {
		return nil
	}
	return []string{key + "=" + value}
}

func workflowFormatFromPath(path string) (string, error) {
	if path == "-" {
		return "json", nil
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return "yaml", nil
	case ".toml":
		return "toml", nil
	case ".json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported workflow manifest extension %q", filepath.Ext(path))
	}
}
