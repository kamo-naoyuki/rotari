package model

import (
	"fmt"
	"strings"
)

// ValidateQueueDependencies checks queue-level names before validating the
// expanded job dependency graph used by the runner.
func ValidateQueueDependencies(commands []QueuedCommand) error {
	if err := ValidateMatrixGroups(commands); err != nil {
		return err
	}
	if err := ValidateStageNames(commands); err != nil {
		return err
	}
	return ValidateDependencies(QueueToJobs(commands))
}

func ValidateStageNames(commands []QueuedCommand) error {
	stages, matrixGroups := queueNamespaces(commands)
	for _, command := range commands {
		if err := validateCommandNamespace(command, stages, matrixGroups); err != nil {
			return err
		}
	}
	return nil
}

func queueNamespaces(commands []QueuedCommand) (map[string]bool, map[string]bool) {
	stages := make(map[string]bool)
	matrixGroups := make(map[string]bool)
	for _, command := range commands {
		if command.Stage != "" {
			stages[command.Stage] = true
		}
		if command.Matrix != nil && command.Matrix.BaseName != "" {
			matrixGroups[command.Matrix.BaseName] = true
		}
	}
	return stages, matrixGroups
}

func validateCommandNamespace(command QueuedCommand, stages, matrixGroups map[string]bool) error {
	if command.Name != "" && stages[command.Name] {
		return fmt.Errorf("job name conflicts with stage name: %s", command.Name)
	}
	if command.Name != "" && matrixGroups[command.Name] && (command.Matrix == nil || command.Name != command.Matrix.BaseName) {
		return fmt.Errorf("job name conflicts with matrix name: %s", command.Name)
	}
	if command.Stage != "" && matrixGroups[command.Stage] {
		return fmt.Errorf("stage name conflicts with matrix name: %s", command.Stage)
	}
	for _, name := range command.DependsOnFinished {
		if containsString(command.DependsOn, name) {
			return fmt.Errorf("job %q lists %q in both depends_on and depends_on_finished", commandLabel(command), name)
		}
	}
	return nil
}

func commandLabel(command QueuedCommand) string {
	if command.Name != "" {
		return command.Name
	}
	return command.ID
}

func ValidateMatrixGroups(commands []QueuedCommand) error {
	groups := make(map[string][]QueuedCommand)
	baseNames := make(map[string]string)
	for _, command := range commands {
		if command.Matrix == nil {
			continue
		}
		if command.Matrix.GroupID == "" || len(command.Matrix.Dimensions) == 0 {
			return fmt.Errorf("job %q has invalid matrix provenance", command.ID)
		}
		if command.Matrix.BaseName != "" {
			if otherGroup, exists := baseNames[command.Matrix.BaseName]; exists && otherGroup != command.Matrix.GroupID {
				return fmt.Errorf("duplicate matrix name: %s", command.Matrix.BaseName)
			}
			baseNames[command.Matrix.BaseName] = command.Matrix.GroupID
		}
		groups[command.Matrix.GroupID] = append(groups[command.Matrix.GroupID], command)
	}
	for groupID, members := range groups {
		if err := validateMatrixGroup(groupID, members); err != nil {
			return err
		}
	}
	return nil
}

func validateMatrixGroup(groupID string, members []QueuedCommand) error {
	first := members[0].Matrix
	expected := ExpandMatrix(first.Dimensions)
	if len(members) != len(expected) {
		return fmt.Errorf("matrix group %q is incomplete: got %d combinations, want %d", groupID, len(members), len(expected))
	}
	seen := make(map[string]bool, len(members))
	for _, member := range members {
		if err := validateMatrixMember(groupID, member, members[0]); err != nil {
			return err
		}
		key := matrixValuesKey(member.Matrix.Values)
		if seen[key] {
			return fmt.Errorf("matrix group %q has duplicate combination", groupID)
		}
		seen[key] = true
	}
	for _, combination := range expected {
		if !seen[matrixValuesKey(combination)] {
			return fmt.Errorf("matrix group %q is missing a combination", groupID)
		}
	}
	return nil
}

func validateMatrixMember(groupID string, member, base QueuedCommand) error {
	matrix := member.Matrix
	first := base.Matrix
	if !equalMatrixDimensions(matrix.Dimensions, first.Dimensions) || matrix.BaseName != first.BaseName || !equalStrings(matrix.BaseEnvironment, first.BaseEnvironment) || !equalMatrixCommandBase(member, base) {
		return fmt.Errorf("matrix group %q has inconsistent provenance", groupID)
	}
	if member.Name != MatrixJobName(matrix.BaseName, matrix.Values) || !equalStrings(member.Environment, MatrixEnvironment(matrix.BaseEnvironment, matrix.Values)) {
		return fmt.Errorf("matrix group %q has an inconsistent expanded job %q", groupID, member.ID)
	}
	return nil
}

func equalMatrixCommandBase(left, right QueuedCommand) bool {
	return equalStrings(left.Command, right.Command) && left.WorkingDirectory == right.WorkingDirectory &&
		left.Executor == right.Executor && equalStrings(left.ExecutorOptions, right.ExecutorOptions) &&
		left.Stage == right.Stage && equalStrings(left.DependsOn, right.DependsOn) &&
		equalStrings(left.DependsOnFinished, right.DependsOnFinished) && left.Timeout == right.Timeout && equalArraySpec(left.Array, right.Array)
}

func equalArraySpec(left, right *ArraySpec) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.First == right.First && left.Last == right.Last && equalInts(left.Tasks, right.Tasks)
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// ClearMatrixGroup drops matrix provenance from every member of a group. The
// group's base name stops resolving as a dependency target once provenance is
// gone, so dependencies on it are rewritten to the members' own names.
func ClearMatrixGroup(commands []QueuedCommand, groupID string) {
	if groupID == "" {
		return
	}
	baseName := ""
	var memberNames []string
	for index := range commands {
		if commands[index].Matrix != nil && commands[index].Matrix.GroupID == groupID {
			baseName = commands[index].Matrix.BaseName
			if commands[index].Name != "" {
				memberNames = append(memberNames, commands[index].Name)
			}
			commands[index].Matrix = nil
		}
	}
	if baseName != "" {
		replaceDependency(commands, baseName, memberNames)
	}
}

func replaceDependency(commands []QueuedCommand, target string, replacements []string) {
	for index := range commands {
		commands[index].DependsOn = replaceName(commands[index].DependsOn, target, replacements)
		commands[index].DependsOnFinished = replaceName(commands[index].DependsOnFinished, target, replacements)
	}
}

func replaceName(names []string, target string, replacements []string) []string {
	if !containsString(names, target) {
		return names
	}
	rewritten := make([]string, 0, len(names)+len(replacements))
	for _, name := range names {
		if name != target {
			rewritten = appendUnique(rewritten, name)
			continue
		}
		for _, replacement := range replacements {
			rewritten = appendUnique(rewritten, replacement)
		}
	}
	return rewritten
}

func appendUnique(values []string, value string) []string {
	if containsString(values, value) {
		return values
	}
	return append(values, value)
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func ClearIncompleteMatrixGroups(commands []QueuedCommand) {
	counts := make(map[string]int)
	wants := make(map[string]int)
	for _, command := range commands {
		if command.Matrix == nil || command.Matrix.GroupID == "" {
			continue
		}
		counts[command.Matrix.GroupID]++
		want := 1
		for _, dimension := range command.Matrix.Dimensions {
			want *= len(dimension.Values)
		}
		wants[command.Matrix.GroupID] = want
	}
	for groupID, count := range counts {
		if count != wants[groupID] {
			ClearMatrixGroup(commands, groupID)
		}
	}
}

func matrixValuesKey(values []MatrixValue) string {
	var builder strings.Builder
	for _, value := range values {
		builder.WriteString(value.Name)
		builder.WriteByte('=')
		builder.WriteString(value.Value)
		builder.WriteByte(0)
	}
	return builder.String()
}

func equalMatrixDimensions(left, right []MatrixDimension) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Name != right[index].Name || !equalStrings(left[index].Values, right[index].Values) {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func ValidateDependencies(jobs []JobSpec) error {
	byName := make(map[string]JobSpec, len(jobs))
	for _, job := range jobs {
		if job.Name == "" {
			continue
		}
		if _, exists := byName[job.Name]; exists {
			return fmt.Errorf("duplicate job name: %s", job.Name)
		}
		byName[job.Name] = job
	}
	for _, job := range jobs {
		for _, dependency := range job.AllDependencies() {
			if _, exists := byName[dependency]; !exists {
				return fmt.Errorf("job %q depends on unknown job %q", job.Name, dependency)
			}
			if dependency == job.Name {
				return fmt.Errorf("job %q depends on itself", job.Name)
			}
		}
	}
	visiting := make(map[string]bool)
	visited := make(map[string]bool)
	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return fmt.Errorf("dependency cycle detected at job %q", name)
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		for _, dependency := range byName[name].AllDependencies() {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		delete(visiting, name)
		visited[name] = true
		return nil
	}
	for name := range byName {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

func DependenciesReady(job JobSpec, results map[string]JobResult, jobsByName map[string]JobSpec) (bool, string) {
	for _, dependency := range job.DependsOn {
		dependencyJob := jobsByName[dependency]
		result, done := results[dependencyJob.ID]
		if !done {
			return false, ""
		}
		if result.ExitCode != 0 {
			return false, dependency
		}
	}
	return true, ""
}
