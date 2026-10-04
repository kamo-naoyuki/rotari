package mcp

import (
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/report"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
	"github.com/kamo-naoyuki/rotari/internal/workflowstate"
)

// redactedValue replaces a value an exported manifest must not reveal.
// rotari_import refuses a manifest that still contains it, so a redacted
// view is never imported as if its placeholders were real values.
const redactedValue = "[REDACTED]"

type ExportRunInput struct {
	RunID  string `json:"run_id" jsonschema:"exact run ID"`
	Format string `json:"format,omitempty" jsonschema:"manifest format: yaml (default), json, or toml"`
}

type ExportRunOutput struct {
	BaseDirRef string `json:"basedir_ref"`
	Project    string `json:"project"`
	Format     string `json:"format"`
	// Manifest is the run as a workflow manifest, with environment values,
	// executor options, and paths redacted.
	Manifest string `json:"manifest"`
}

// exportRun exports a settled run as `rotari export RUN_ID` does, redacted
// for reading: environment values and executor options are replaced, and
// paths in commands, working directories, and output destinations are
// redacted where detected. The full manifest is available from the CLI.
func exportRun(masterDir string, input ExportRunInput) (ExportRunOutput, error) {
	location, paths, err := registeredRun(masterDir, input.RunID)
	if err != nil {
		return ExportRunOutput{}, err
	}
	runner := writeTools{masterDir: masterDir}.runner()
	run, err := workflowstate.LoadSettledRun(paths, location.RunID, func(queue model.Queue) error {
		return runner.ValidateQueue(queue, "", nil, nil)
	}, false)
	if err != nil {
		return ExportRunOutput{}, err
	}
	manifest, err := workflow.MergeRuns(location.ProjectName, []workflow.SourceRun{run})
	if err != nil {
		return ExportRunOutput{}, err
	}
	format := input.Format
	if format == "" {
		format = "yaml"
	}
	data, err := workflow.Encode(redactManifest(manifest), format)
	if err != nil {
		return ExportRunOutput{}, err
	}
	return ExportRunOutput{BaseDirRef: basedirregistry.Ref(location.BaseDir), Project: location.ProjectName, Format: format, Manifest: string(data)}, nil
}

// redactManifest returns manifest with the values the MCP tools do not
// reveal by default replaced.
func redactManifest(manifest workflow.Manifest) workflow.Manifest {
	jobs := make([]workflow.Job, len(manifest.Jobs))
	for index, job := range manifest.Jobs {
		job.Command = redactEach(job.Command)
		job.WorkingDirectory = report.RedactPatterns(job.WorkingDirectory)
		job.Output = redactEach(job.Output)
		job.Error = redactEach(job.Error)
		job.Artifacts = redactEach(job.Artifacts)
		if len(job.ExecutorOptions) > 0 {
			job.ExecutorOptions = []string{redactedValue}
		}
		environment := make([]string, len(job.Environment))
		for position, entry := range job.Environment {
			name, _, _ := strings.Cut(entry, "=")
			environment[position] = name + "=" + redactedValue
		}
		if len(environment) > 0 {
			job.Environment = environment
		}
		jobs[index] = job
	}
	manifest.Jobs = jobs
	return manifest
}

func redactEach(values []string) []string {
	if values == nil {
		return nil
	}
	redacted := make([]string, len(values))
	for index, value := range values {
		redacted[index] = report.RedactPatterns(value)
	}
	return redacted
}

// containsRedaction reports whether manifest text holds a placeholder that a
// redacted export put there.
func containsRedaction(manifest string) bool {
	return strings.Contains(manifest, redactedValue) || strings.Contains(manifest, "[REDACTED_PATH]") || strings.Contains(manifest, "[REDACTED_HOST]")
}
