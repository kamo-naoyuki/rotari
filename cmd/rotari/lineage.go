package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"regexp"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/joblist"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/resolve"
	"github.com/kamo-naoyuki/rotari/internal/runlineage"
	"github.com/kamo-naoyuki/rotari/internal/runview"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdLineage lists runs, summarizes one run, or compares two runs.
func cmdLineage(args []string) int {
	fs := flag.NewFlagSet("lineage", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	basedir := cliString(fs, "basedir", "")
	projectName := cliString(fs, "project-name", "")
	jsonOutput := cliBool(fs, "json", false)
	if err := cliParse(fs, args); err != nil {
		return 1
	}
	if len(fs.Args()) == 0 {
		baseDir, _, err := state.ResolveBaseDir(*basedir)
		if err != nil {
			printErrorf("failed to resolve basedir: %v", err)
			return 1
		}
		project := ""
		if cliOptionSet(fs, "project-name") {
			project = *projectName
			if err := resolve.RequireProject(baseDir, project); err != nil {
				printError(err)
				return 1
			}
		} else {
			projects, err := resolve.ExistingProjectNames(baseDir)
			if err != nil {
				printErrorf("failed to list projects in %s: %v", baseDir, err)
				return 1
			}
			switch len(projects) {
			case 0:
				printErrorf("no projects found in %s; add a project or choose a basedir with --basedir", baseDir)
				return 1
			case 1:
				project = projects[0]
			default:
				fmt.Fprintf(os.Stderr, "multiple projects found in %s; choose one with --project-name:\n", baseDir)
				for _, candidate := range projects {
					fmt.Fprintf(os.Stderr, "  %s\n", candidate)
				}
				return 1
			}
		}
		paths, err := state.ResolveProjectPaths(baseDir, project)
		if err != nil {
			printErrorf("failed to resolve paths: %v", err)
			return 1
		}
		return showLineage(paths, nil, *jsonOutput)
	}
	selectorBase, selectorProject := "", ""
	if cliOptionSet(fs, "basedir") {
		selectorBase = *basedir
	}
	if cliOptionSet(fs, "project-name") {
		selectorProject = *projectName
	}
	baseDir, project, err := resolveCLIExistingRun(selectorBase, selectorProject, firstNonEmpty(fs.Args()...))
	if err != nil {
		printError(err)
		return 1
	}
	paths, err := state.ResolveProjectPaths(baseDir, project)
	if err != nil {
		printErrorf("failed to resolve paths: %v", err)
		return 1
	}
	return showLineage(paths, fs.Args(), *jsonOutput)
}

func writeRunDiff(writer io.Writer, paths state.ProjectPaths, result runlineage.Result, showAll bool) {
	label := func(info runlineage.RunInfo) string { return model.RunLabel(info.ID, info.Name) }
	fmt.Fprintf(writer, "%s %s\n", cyan("Project:"), paths.ProjectName)
	fmt.Fprintf(writer, "%s %s -> %s\n", cyan("Runs:"), label(result.From), label(result.To))
	if result.From.Elapsed != "" || result.To.Elapsed != "" {
		fmt.Fprintf(writer, "%s %s -> %s\n", cyan("Elapsed:"), firstNonEmpty(result.From.Elapsed, "-"), firstNonEmpty(result.To.Elapsed, "-"))
	}
	for _, change := range result.Sources {
		fmt.Fprintf(writer, "%s %s\n", cyan("Source:"), sourceChangeText(change))
	}
	fromNames, toNames := map[string]string{}, map[string]string{}
	for _, job := range result.Jobs {
		fromNames[job.FromID], toNames[job.ToID] = job.Name, job.Name
	}
	for _, side := range []struct {
		label string
		info  runlineage.RunInfo
		names map[string]string
	}{{"Note (from):", result.From, fromNames}, {"Note (to):", result.To, toNames}} {
		for _, note := range side.info.Notes {
			fmt.Fprintf(writer, "%s %s\n", cyan(side.label), model.FormatRunNote(note, side.names[note.JobID]))
		}
	}
	summary := result.Summary
	fmt.Fprintf(writer, "%s fixed %d, still failing %d, newly failing %d, added %d, removed %d, definition changed %d, carried %d, cause changed %d\n",
		cyan("Summary:"), summary.Fixed, summary.StillFailing, summary.NewlyFailing, summary.Added, summary.Removed, summary.Changed, summary.Carried, summary.CauseChanged)

	shown := make([]runlineage.JobDiff, 0, len(result.Jobs))
	for _, job := range result.Jobs {
		if showAll || job.Notable() {
			shown = append(shown, job)
		}
	}
	if len(shown) > 0 {
		rows := groupArrayTasks(shown, sameDiffRow)
		width, resultWidth, changesWidth := len("JOB"), len("RESULT"), len("CHANGES")
		for _, row := range rows {
			width = max(width, len(row.label))
			resultWidth = max(resultWidth, len(diffResult(row.job)))
			changesWidth = max(changesWidth, len(diffChanges(row.job)))
		}
		fmt.Fprintf(writer, "\n%s\n", cyan(fmt.Sprintf("%-*s  %-10s  %-10s  %-*s  %-*s  %s", width, "JOB", "FROM", "TO", resultWidth, "RESULT", changesWidth, "CHANGES", "CAUSE")))
		for _, row := range rows {
			job := row.job
			fmt.Fprintf(writer, "%-*s  %-10s  %-10s  %s  %-*s  %s\n", width, row.label, firstNonEmpty(job.FromStatus, "-"), firstNonEmpty(job.ToStatus, "-"),
				colorTransition(fmt.Sprintf("%-*s", resultWidth, diffResult(job)), job.Transition), changesWidth, diffChanges(job), diffCause(job))
		}
	}
	if hidden := len(result.Jobs) - len(shown); hidden > 0 {
		fmt.Fprintf(writer, "\n%d unchanged job(s) hidden: %s\n", hidden, hiddenJobLabels(result.Jobs, showAll))
	}
	wroteHeader := false
	for _, row := range groupArrayTasks(shown, func(a, b runlineage.JobDiff) bool { return reflect.DeepEqual(a.Changes, b.Changes) }) {
		job := row.job
		if len(job.Changes) == 0 {
			continue
		}
		if !wroteHeader {
			fmt.Fprintf(writer, "\n%s\n", cyan("Definition changes:"))
			wroteHeader = true
		}
		fmt.Fprintf(writer, "  %s\n", row.label)
		for _, change := range job.Changes {
			if len(change.Added) > 0 || len(change.Removed) > 0 {
				parts := make([]string, 0, len(change.Added)+len(change.Removed))
				for _, value := range change.Removed {
					parts = append(parts, red("-"+value))
				}
				for _, value := range change.Added {
					parts = append(parts, green("+"+value))
				}
				fmt.Fprintf(writer, "    %s: %s\n", change.Field, strings.Join(parts, " "))
				continue
			}
			fmt.Fprintf(writer, "    %s: %s -> %s\n", change.Field, firstNonEmpty(change.From, "-"), firstNonEmpty(change.To, "-"))
		}
	}
}

// diffChanges lists the changed definition fields of a compared job.
func diffChanges(job runlineage.JobDiff) string {
	fields := make([]string, 0, len(job.Changes))
	for _, change := range job.Changes {
		fields = append(fields, change.Field)
	}
	return firstNonEmpty(strings.Join(fields, ","), "-")
}

// diffResult names a compared job's transition, marking a result that the
// newer run carried from an earlier one instead of executing the job.
func diffResult(job runlineage.JobDiff) string {
	result := strings.ReplaceAll(job.Transition, "_", " ")
	if job.Carried {
		result += " (carried)"
	}
	return result
}

// diffRow is one line of a comparison: a job, or the tasks of one array
// that read the same, labeled NAME[1,2,3].
type diffRow struct {
	label string
	job   runlineage.JobDiff
}

var arrayTaskName = regexp.MustCompile(`^(.*)\[(\d+)\]$`)

// groupArrayTasks joins the tasks of one array that read the same, as same
// decides against the row's first task, into one row placed where the first
// of them was. Other jobs get rows of their own.
func groupArrayTasks(jobs []runlineage.JobDiff, same func(a, b runlineage.JobDiff) bool) []diffRow {
	type group struct {
		base  string
		job   runlineage.JobDiff
		tasks []string
	}
	groups := make([]*group, 0, len(jobs))
	for _, job := range jobs {
		match := arrayTaskName.FindStringSubmatch(job.Name)
		if match == nil {
			groups = append(groups, &group{job: job})
			continue
		}
		joined := false
		for _, existing := range groups {
			if existing.tasks != nil && existing.base == match[1] && same(existing.job, job) {
				existing.tasks = append(existing.tasks, match[2])
				joined = true
				break
			}
		}
		if !joined {
			groups = append(groups, &group{base: match[1], job: job, tasks: []string{match[2]}})
		}
	}
	rows := make([]diffRow, 0, len(groups))
	for _, group := range groups {
		label := group.job.Name
		if group.tasks != nil {
			label = group.base + "[" + strings.Join(group.tasks, ",") + "]"
		}
		rows = append(rows, diffRow{label: label, job: group.job})
	}
	return rows
}

// hiddenJobLabels names the jobs a comparison hides, an array's tasks joined
// as in the table and carried results marked, up to hiddenJobLimit rows.
func hiddenJobLabels(jobs []runlineage.JobDiff, showAll bool) string {
	hidden := make([]runlineage.JobDiff, 0, len(jobs))
	for _, job := range jobs {
		if !showAll && !job.Notable() {
			hidden = append(hidden, job)
		}
	}
	rows := groupArrayTasks(hidden, func(a, b runlineage.JobDiff) bool { return a.Carried == b.Carried })
	labels := make([]string, 0, min(len(rows), hiddenJobLimit))
	for index, row := range rows {
		if index == hiddenJobLimit {
			break
		}
		label := row.label
		if row.job.Carried {
			label += " (carried)"
		}
		labels = append(labels, label)
	}
	text := strings.Join(labels, ", ")
	if rest := len(rows) - len(labels); rest > 0 {
		text += fmt.Sprintf(" +%d more", rest)
	}
	return text
}

// hiddenJobLimit is how many rows of hidden jobs a comparison names.
const hiddenJobLimit = 10

// sameDiffRow reports whether two compared jobs read the same in the table.
func sameDiffRow(a, b runlineage.JobDiff) bool {
	return a.FromStatus == b.FromStatus && a.ToStatus == b.ToStatus && a.Transition == b.Transition && a.Carried == b.Carried &&
		a.FromCause == b.FromCause && a.ToCause == b.ToCause && reflect.DeepEqual(a.Changes, b.Changes)
}

// diffCause shows a compared job's failure cause in each run: one cause
// when both runs fail for the same one, "FROM -> TO" otherwise, and "-" when
// neither run failed.
func diffCause(job runlineage.JobDiff) string {
	switch {
	case job.FromCause == "" && job.ToCause == "":
		return "-"
	case job.FromCause == job.ToCause:
		return job.FromCause
	}
	return firstNonEmpty(job.FromCause, "-") + " -> " + firstNonEmpty(job.ToCause, "-")
}

func colorTransition(text, transition string) string {
	switch transition {
	case runlineage.TransitionFixed:
		return green(text)
	case runlineage.TransitionNewlyFailing:
		return red(text)
	case runlineage.TransitionStillFailing, runlineage.TransitionAdded, runlineage.TransitionRemoved:
		return yellow(text)
	}
	return text
}

// showLineage lists a project's runs oldest first, summarizes one selected run,
// or compares two selected runs.
func showLineage(paths state.ProjectPaths, runIDs []string, jsonOutput bool) int {
	if len(runIDs) > 0 {
		resolvedIDs := make([]string, len(runIDs))
		for index, runID := range runIDs {
			resolved, err := resolve.RunID(paths, runID)
			if err != nil {
				printError(err)
				return 1
			}
			resolvedIDs[index] = resolved
		}
		if len(resolvedIDs) == 1 {
			summary, err := runview.Summary(paths, resolvedIDs[0], jsonStore())
			if err != nil {
				printError(err)
				return 1
			}
			if jsonOutput {
				return encodeJSON(summary, "run summary")
			}
			writeRunSummary(os.Stdout, paths, summary)
			return 0
		}
		from, err := runview.LoadRun(paths, resolvedIDs[0], jsonStore())
		if err != nil {
			printError(err)
			return 1
		}
		to, err := runview.LoadRun(paths, resolvedIDs[1], jsonStore())
		if err != nil {
			printError(err)
			return 1
		}
		if len(resolvedIDs) > 2 {
			runs := make([]runlineage.Run, 0, len(resolvedIDs))
			for _, runID := range resolvedIDs {
				run, loadErr := runview.LoadRun(paths, runID, jsonStore())
				if loadErr != nil {
					printError(loadErr)
					return 1
				}
				runs = append(runs, run)
			}
			grid := runlineage.CompareGrid(runs)
			if jsonOutput {
				return encodeJSON(grid, "comparison grid")
			}
			writeRunGrid(os.Stdout, paths, grid)
			return 0
		}
		result := runlineage.Compare(from, to)
		if jsonOutput {
			return encodeJSON(result, "comparison")
		}
		writeRunDiff(os.Stdout, paths, result, false)
		return 0
	}
	runIDs, err := runview.RunsByStart(paths)
	if err != nil {
		printError(err)
		return 1
	}
	runs := make([]runlineage.Run, 0, len(runIDs))
	for _, runID := range runIDs {
		run, err := runview.LoadRun(paths, runID, jsonStore())
		if err != nil {
			// An active run may not have written commands.json yet.
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			printError(err)
			return 1
		}
		runs = append(runs, run)
	}
	entries := runlineage.Lineage(runs)
	if jsonOutput {
		return encodeJSON(entries, "lineage")
	}
	writeShowTargetHeaderWithMode(os.Stdout, paths, "lineage")
	fmt.Println("\n" + cyan("Runs (oldest first):"))
	if len(entries) == 0 {
		fmt.Println("No runs found.")
		return 0
	}
	labels := make([]string, len(entries))
	width := len("RUN")
	for index, entry := range entries {
		labels[index] = model.RunLabel(entry.Run.ID, entry.Run.Name)
		width = max(width, len(labels[index]))
	}
	fmt.Println(cyan(fmt.Sprintf("%-*s  %5s  %5s  %6s  %7s  %5s  %8s  %5s  %7s  %7s  %7s  %-9s  %-7s  %s",
		width, "RUN", "JOBS", "OK", "FAILED", "BLOCKED", "FIXED", "NEW FAIL", "ADDED", "REMOVED", "CHANGED", "CARRIED", "CODE", "ELAPSED", "NOTE")))
	for index, entry := range entries {
		counts := entry.Counts
		changeColumns := []string{"-", "-", "-", "-", "-", "-"}
		if changes := entry.Changes; changes != nil {
			changeColumns = []string{
				fmt.Sprint(changes.Fixed), fmt.Sprint(changes.NewlyFailing), fmt.Sprint(changes.Added),
				fmt.Sprint(changes.Removed), fmt.Sprint(changes.Changed), fmt.Sprint(changes.Carried),
			}
		}
		line := fmt.Sprintf("%-*s  %5d  %5d  %6d  %7d  %5s  %8s  %5s  %7s  %7s  %7s  %-9s  %-7s  %s",
			width, labels[index], counts.Jobs, counts.Succeeded, counts.Failed, counts.Blocked,
			changeColumns[0], changeColumns[1], changeColumns[2], changeColumns[3], changeColumns[4], changeColumns[5],
			firstNonEmpty(entry.CodeChange, "-"), firstNonEmpty(entry.Run.Elapsed, "-"), lineageNote(entry.Run.Notes))
		fmt.Println(strings.TrimRight(line, " "))
	}
	if len(entries) >= 2 {
		previous := entries[len(entries)-2].Run.ID
		current := entries[len(entries)-1].Run.ID
		fmt.Printf("\n%s\n  rotari lineage %s %s\n", cyan("To compare the latest two runs:"), previous, current)
	}
	return 0
}

func writeRunGrid(writer io.Writer, paths state.ProjectPaths, grid runlineage.GridResult) {
	fmt.Fprintf(writer, "%s %s\n", cyan("Project:"), paths.ProjectName)
	labels := make([]string, len(grid.Runs))
	for index, run := range grid.Runs {
		labels[index] = model.RunLabel(run.ID, run.Name)
	}
	fmt.Fprintf(writer, "%s %s\n", cyan("Runs:"), strings.Join(labels, " | "))
	width := len("JOB")
	for _, job := range grid.Jobs {
		width = max(width, len(job.Name))
	}
	header := fmt.Sprintf("%-*s", width, "JOB")
	for index := range grid.Runs {
		header += fmt.Sprintf("  %-12s", fmt.Sprintf("RUN_%d", index+1))
	}
	fmt.Fprintln(writer, cyan(header))
	for _, job := range grid.Jobs {
		line := fmt.Sprintf("%-*s", width, job.Name)
		for index, status := range job.Statuses {
			cell := firstNonEmpty(status, "-")
			if index < len(job.DefinitionChanged) && job.DefinitionChanged[index] {
				cell += " *"
			}
			line += fmt.Sprintf("  %-12s", cell)
		}
		fmt.Fprintln(writer, line)
	}
}

func writeRunSummary(writer io.Writer, paths state.ProjectPaths, summary runlineage.RunSummary) {
	fmt.Fprintf(writer, "%s %s\n", cyan("Project:"), paths.ProjectName)
	fmt.Fprintf(writer, "%s %s\n", cyan("Run:"), model.RunLabel(summary.Run.ID, summary.Run.Name))
	for _, source := range summary.Run.Sources {
		fmt.Fprintf(writer, "%s %s\n", cyan("Source:"), model.SourceLabel(source))
	}
	writeRunNotes(writer, summary.Run.Notes, nil)
	counts := summary.Counts
	fmt.Fprintf(writer, "%s jobs %d, succeeded %d, failed %d, blocked %d, unfinished %d\n", cyan("Summary:"),
		counts.Jobs, counts.Succeeded, counts.Failed, counts.Blocked, counts.Unfinished)
	// no_match only says that no rule applied; the failure groups below
	// already list those jobs, so the text omits it while JSON keeps it.
	diagnoses := make([]runlineage.DiagnosisCount, 0, len(summary.Diagnoses))
	for _, diagnosis := range summary.Diagnoses {
		if diagnosis.Name != model.DiagnosisNoMatch {
			diagnoses = append(diagnoses, diagnosis)
		}
	}
	if len(diagnoses) > 0 {
		fmt.Fprintln(writer, cyan("Diagnoses:"))
		for _, diagnosis := range diagnoses {
			fmt.Fprintf(writer, "  %s %d\n", diagnosis.Name, diagnosis.Count)
		}
	}
	writeFailureGroups(writer, summary.Failures, failureRetryHints(paths, summary.Run.ID))
	if len(summary.Origins) > 0 {
		fmt.Fprintln(writer, cyan("Origins:"))
		for _, origin := range summary.Origins {
			label := firstNonEmpty(origin.RunID, "new")
			fmt.Fprintf(writer, "  %s %d\n", label, origin.Count)
		}
	}
}

func encodeJSON(value any, description string) int {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		printErrorf("failed to encode %s: %v", description, err)
		return 1
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// sourceChangeText describes how a repository's code differs between two
// runs: "changed git 89281f8c3a1b -> 9c0ffee12345 in /repo", "unchanged ...",
// or "unknown ..." when a side is missing, unreadable, or had uncommitted
// changes.
func sourceChangeText(change runlineage.SourceChange) string {
	side := func(revision *model.SourceRevision) string {
		if revision == nil {
			return "not recorded"
		}
		return model.FormatSourceRevision(*revision)
	}
	if change.Change == runlineage.SourceUnchanged {
		return change.Change + " " + side(change.From) + " in " + change.Root
	}
	return change.Change + " " + side(change.From) + " -> " + side(change.To) + " in " + change.Root
}

// lineageNoteWidth caps the NOTE column of the run history.
const lineageNoteWidth = 60

// lineageNote is the first note on the run itself, on one line, shortened
// for the run history; "-" when there is none.
func lineageNote(notes []model.RunNote) string {
	runNotes := model.RunNotesFor(notes, "")
	if len(runNotes) == 0 {
		return "-"
	}
	text := joblist.ShortenText(strings.Join(strings.Fields(runNotes[0].Text), " "), lineageNoteWidth)
	if more := len(runNotes) - 1; more > 0 {
		text = fmt.Sprintf("%s (+%d more)", text, more)
	}
	return text
}
