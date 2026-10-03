package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const (
	// failureMemberLimit caps the jobs listed for one cause.
	failureMemberLimit = 10
	// failureEvidenceLimit caps the characters of a cause's example line.
	failureEvidenceLimit = 160
)

// runFailureGroups groups the failed jobs of runID by cause. A non-nil
// selected keeps only the jobs it names, so the groups match a filtered
// table.
func runFailureGroups(paths state.ProjectPaths, runID string, selected map[string]bool) ([]runlineage.FailureGroup, error) {
	run, err := runview.LoadRun(paths, runID, jsonStore())
	if err != nil {
		return nil, err
	}
	if selected != nil {
		jobs := run.Jobs[:0]
		for _, job := range run.Jobs {
			if selected[job.Spec.ID] {
				jobs = append(jobs, job)
			}
		}
		run.Jobs = jobs
	}
	return runlineage.FailureGroups(run), nil
}

// writeFailureGroups prints one entry per cause: its count, exit codes, and
// jobs, the first job's evidence and how to show that job, and the cause's
// suggestion.
func writeFailureGroups(writer io.Writer, groups []runlineage.FailureGroup, retryHint func(runlineage.FailureGroup) string) {
	if len(groups) == 0 {
		return
	}
	fmt.Fprintln(writer, cyan("Failures by cause:"))
	for _, group := range groups {
		count := strconv.Itoa(group.Count)
		if group.Carried > 0 {
			count += fmt.Sprintf(" (%d carried)", group.Carried)
		}
		exitCodes := make([]string, 0, len(group.ExitCodes))
		for _, code := range group.ExitCodes {
			exitCodes = append(exitCodes, strconv.Itoa(code))
		}
		fmt.Fprintf(writer, "  %s %s (exit %s): %s\n", count, red(group.Cause), strings.Join(exitCodes, ","), failureMemberLabels(group.Jobs, failureMemberLimit))
		if group.Example.Evidence != "" {
			fmt.Fprintf(writer, "    e.g. %s\n", truncateText(group.Example.Evidence, failureEvidenceLimit))
		}
		if group.Example.AttemptID != "" {
			fmt.Fprintf(writer, "    show: rotari show -j %s\n", group.Example.AttemptID)
		}
		if group.Suggestion != "" {
			fmt.Fprintf(writer, "    fix: %s\n", group.Suggestion)
		}
		if retryHint != nil {
			if hint := retryHint(group); hint != "" {
				fmt.Fprintf(writer, "    retry: %s\n", hint)
			}
		}
	}
}

// failureMemberLabels lists up to limit jobs, joining consecutive tasks of
// one array as name[1,2,3], and counts the rest.
func failureMemberLabels(members []runlineage.FailureMember, limit int) string {
	labels := make([]string, 0)
	shown := 0
	for index := 0; index < len(members) && shown < limit; {
		member := members[index]
		if member.ArrayTaskID == nil {
			labels = append(labels, firstNonEmpty(member.Name, member.ID))
			shown++
			index++
			continue
		}
		base := arrayBaseLabel(member)
		tasks := make([]string, 0)
		for ; index < len(members) && shown < limit && members[index].ArrayTaskID != nil && arrayBaseLabel(members[index]) == base; index++ {
			tasks = append(tasks, strconv.Itoa(*members[index].ArrayTaskID))
			shown++
		}
		labels = append(labels, fmt.Sprintf("%s[%s]", base, strings.Join(tasks, ",")))
	}
	text := strings.Join(labels, " ")
	if rest := len(members) - shown; rest > 0 {
		text += fmt.Sprintf(" +%d more", rest)
	}
	return text
}

// arrayBaseLabel names the array an array task belongs to: its job name
// without the "[N]" task suffix, or for an unnamed array its job ID without
// the "-N" suffix.
func arrayBaseLabel(member runlineage.FailureMember) string {
	task := strconv.Itoa(*member.ArrayTaskID)
	if member.Name != "" {
		return strings.TrimSuffix(member.Name, "["+task+"]")
	}
	return strings.TrimSuffix(member.ID, "-"+task)
}

// truncateText shortens text to limit characters, marking the cut.
func truncateText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-3]) + "..."
}

// failureRetryHints returns, for runID, a function that gives the command
// that previews a retry of one failure group's jobs, as printed: groups of a
// rule diagnosis select by --filter-diagnosis, and the others by
// --filter-failure-kind. retry acts on a project's last run, so it returns
// nil unless runID is the project's last run and has finished.
func failureRetryHints(paths state.ProjectPaths, runID string) func(runlineage.FailureGroup) string {
	meta, err := state.LoadMeta(paths.MetaFile)
	if err != nil || meta.LastRunID != runID {
		return nil
	}
	if phase, err := project.RunPhaseOf(paths, runID); err != nil || phase != project.RunPhaseFinished {
		return nil
	}
	target := fmt.Sprintf("rotari retry --basedir %s --project-name %s", executor.ShellQuote(paths.BaseDir), executor.ShellQuote(paths.ProjectName))
	return func(group runlineage.FailureGroup) string {
		if group.Kind == runlineage.FailureKindDiagnosis {
			return fmt.Sprintf("%s --filter-diagnosis %s --dry-run", target, executor.ShellQuote(group.Cause))
		}
		return fmt.Sprintf("%s --filter-failure-kind %s --dry-run", target, executor.ShellQuote(group.Kind))
	}
}
