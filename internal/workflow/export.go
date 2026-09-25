package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"gopkg.in/yaml.v3"
)

func EquivalentCommand(left, right model.QueuedCommand) bool {
	return reflect.DeepEqual(left.Command, right.Command) &&
		left.WorkingDirectory == right.WorkingDirectory &&
		left.Executor == right.Executor &&
		reflect.DeepEqual(left.ExecutorOptions, right.ExecutorOptions) &&
		reflect.DeepEqual(left.Environment, right.Environment) &&
		left.Name == right.Name && left.Stage == right.Stage &&
		reflect.DeepEqual(left.DependsOn, right.DependsOn) &&
		sameNames(left.DependsOnFinished, right.DependsOnFinished) && left.Timeout == right.Timeout && reflect.DeepEqual(left.Retry, right.Retry) &&
		reflect.DeepEqual(left.Array, right.Array) && equivalentMatrix(left.Matrix, right.Matrix)
}

// sameNames compares name lists, treating nil and empty as equal.
func sameNames(left, right []string) bool {
	return len(left) == len(right) && (len(left) == 0 || reflect.DeepEqual(left, right))
}

func equivalentMatrix(left, right *model.MatrixSpec) bool {
	if left == nil || right == nil {
		return left == right
	}
	return reflect.DeepEqual(left.Dimensions, right.Dimensions) &&
		reflect.DeepEqual(left.Values, right.Values) && left.BaseName == right.BaseName &&
		reflect.DeepEqual(left.BaseEnvironment, right.BaseEnvironment)
}

func FromQueue(queue model.Queue) (Manifest, error) {
	return fromQueue(queue, nil)
}

func FromRun(queue model.Queue, summary model.RunSummary, source Source) (Manifest, error) {
	manifest, err := fromQueue(queue, model.ResultsByID(summary.Results))
	if err != nil {
		return Manifest{}, err
	}
	manifest.Source = &source
	return manifest, nil
}

func fromQueue(queue model.Queue, results map[string]model.JobResult) (Manifest, error) {
	if len(queue.Commands) == 0 {
		return Manifest{}, fmt.Errorf("queue has no jobs")
	}
	if err := model.ValidateMatrixGroups(queue.Commands); err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{Version: Version}
	seenGroups := make(map[string]bool)
	for _, command := range queue.Commands {
		if command.Matrix == nil {
			job := exportJob(command, false)
			annotateCommand(&job, command, nil, results)
			manifest.Jobs = append(manifest.Jobs, job)
			continue
		}
		groupID := command.Matrix.GroupID
		if seenGroups[groupID] {
			continue
		}
		seenGroups[groupID] = true
		job := exportJob(command, true)
		for _, dimension := range command.Matrix.Dimensions {
			job.Matrix = append(job.Matrix, dimension.Name+"="+strings.Join(dimension.Values, ","))
		}
		for _, member := range queue.Commands {
			if member.Matrix == nil || member.Matrix.GroupID != groupID {
				continue
			}
			annotateCommand(&job, member, member.Matrix.Values, results)
		}
		manifest.Jobs = append(manifest.Jobs, job)
	}
	return manifest, nil
}

func exportJob(command model.QueuedCommand, matrixBase bool) Job {
	name := command.Name
	environment := command.Environment
	if matrixBase {
		name = command.Matrix.BaseName
		environment = command.Matrix.BaseEnvironment
	}
	return Job{
		Name: name, Command: append([]string(nil), command.Command...), Stage: command.Stage,
		DependsOn: append([]string(nil), command.DependsOn...), DependsOnFinished: append([]string(nil), command.DependsOnFinished...),
		Timeout: command.Timeout, Retry: cloneRetry(command.Retry),
		Executor:        command.Executor,
		ExecutorOptions: append([]string(nil), command.ExecutorOptions...), WorkingDirectory: command.WorkingDirectory,
		Environment: append([]string(nil), environment...), Array: formatArray(command.Array),
	}
}

func annotateCommand(job *Job, command model.QueuedCommand, values []model.MatrixValue, results map[string]model.JobResult) {
	if results == nil {
		return
	}
	if command.Array == nil && len(values) == 0 {
		result, ok := results[command.ID]
		job.Status = resultStatus(result, ok)
		if ok {
			job.AttemptID = result.AttemptID
		}
		return
	}
	leafIDs := commandLeafIDs(command)
	for _, leafID := range leafIDs {
		annotateLeaf(job, command, values, leafID, results)
	}
	if job.Status == "" {
		job.Status = "success"
	}
}

func commandLeafIDs(command model.QueuedCommand) []string {
	if command.Array == nil {
		return []string{command.ID}
	}
	leafIDs := make([]string, 0, len(model.ArrayTaskIDs(command.Array)))
	for _, task := range model.ArrayTaskIDs(command.Array) {
		leafIDs = append(leafIDs, fmt.Sprintf("%s-%d", command.ID, task))
	}
	return leafIDs
}

func annotateLeaf(job *Job, command model.QueuedCommand, values []model.MatrixValue, leafID string, results map[string]model.JobResult) {
	result, ok := results[leafID]
	status := resultStatus(result, ok)
	job.Status = mergeStatus(job.Status, status)
	if ok && job.AttemptID == "" {
		job.AttemptID = result.AttemptID
	}
	if status == "success" {
		return
	}
	instance := Instance{Matrix: matrixValuesMap(values), Status: status}
	if command.Array != nil {
		task, _ := strconv.Atoi(strings.TrimPrefix(leafID, command.ID+"-"))
		instance.Task = &task
	}
	if ok {
		instance.AttemptID = result.AttemptID
	}
	job.Instances = append(job.Instances, instance)
}

func mergeStatus(current, next string) string {
	priority := map[string]int{"": 0, "success": 1, "unfinished": 2, "cancelled": 3, "failed": 4}
	if priority[next] > priority[current] {
		return next
	}
	return current
}

func resultStatus(result model.JobResult, ok bool) string {
	if !ok {
		return "unfinished"
	}
	if result.ExitCode == 0 {
		return "success"
	}
	errorText := strings.ToLower(strings.TrimSpace(result.Error))
	if errorText == "cancelled" || errorText == "canceled" || strings.HasPrefix(errorText, "cancelled ") || strings.HasPrefix(errorText, "canceled ") {
		return "cancelled"
	}
	return "failed"
}

func matrixValuesMap(values []model.MatrixValue) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.Name] = value.Value
	}
	return result
}

func formatArray(array *model.ArraySpec) string {
	if array == nil {
		return ""
	}
	if len(array.Tasks) == 0 {
		if array.First == array.Last {
			return strconv.Itoa(array.First)
		}
		return fmt.Sprintf("%d-%d", array.First, array.Last)
	}
	values := make([]string, len(array.Tasks))
	for index, task := range array.Tasks {
		values[index] = strconv.Itoa(task)
	}
	return strings.Join(values, ",")
}

func Encode(manifest Manifest, format string) ([]byte, error) {
	switch strings.ToLower(format) {
	case "yaml", "yml":
		return yaml.Marshal(manifest)
	case "json":
		data, err := json.MarshalIndent(manifest, "", "  ")
		return append(data, '\n'), err
	case "toml":
		var buffer bytes.Buffer
		if err := toml.NewEncoder(&buffer).Encode(manifest); err != nil {
			return nil, err
		}
		return buffer.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported manifest format %q", format)
	}
}
