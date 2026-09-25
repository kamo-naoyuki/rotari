package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"gopkg.in/yaml.v3"
)

const Version = 1

type Manifest struct {
	Version int     `json:"version" yaml:"version" toml:"version"`
	Source  *Source `json:"source,omitempty" yaml:"source,omitempty" toml:"source,omitempty"`
	Jobs    []Job   `json:"jobs" yaml:"jobs" toml:"jobs"`
}

type Source struct {
	Project string   `json:"project" yaml:"project" toml:"project"`
	RunIDs  []string `json:"run_ids" yaml:"run_ids" toml:"run_ids"`
}

type Job struct {
	Name      string   `json:"name,omitempty" yaml:"name,omitempty" toml:"name,omitempty"`
	Command   []string `json:"command" yaml:"command" toml:"command"`
	Stage     string   `json:"stage,omitempty" yaml:"stage,omitempty" toml:"stage,omitempty"`
	DependsOn []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty" toml:"depends_on,omitempty"`
	// DependsOnFinished names prerequisites that only need to finish.
	DependsOnFinished []string `json:"depends_on_finished,omitempty" yaml:"depends_on_finished,omitempty" toml:"depends_on_finished,omitempty"`
	// Timeout matches add --timeout, such as "2h".
	Timeout string `json:"timeout,omitempty" yaml:"timeout,omitempty" toml:"timeout,omitempty"`
	// Retry matches add --retry.
	Retry            *int       `json:"retry,omitempty" yaml:"retry,omitempty" toml:"retry,omitempty"`
	Executor         string     `json:"executor,omitempty" yaml:"executor,omitempty" toml:"executor,omitempty"`
	ExecutorOptions  []string   `json:"executor_options,omitempty" yaml:"executor_options,omitempty" toml:"executor_options,omitempty"`
	WorkingDirectory string     `json:"working_directory,omitempty" yaml:"working_directory,omitempty" toml:"working_directory,omitempty"`
	Environment      []string   `json:"environment,omitempty" yaml:"environment,omitempty" toml:"environment,omitempty"`
	Array            string     `json:"array,omitempty" yaml:"array,omitempty" toml:"array,omitempty"`
	Matrix           []string   `json:"matrix,omitempty" yaml:"matrix,omitempty" toml:"matrix,omitempty"`
	Status           string     `json:"status,omitempty" yaml:"status,omitempty" toml:"status,omitempty"`
	AttemptID        string     `json:"attempt_id,omitempty" yaml:"attempt_id,omitempty" toml:"attempt_id,omitempty"`
	Instances        []Instance `json:"instances,omitempty" yaml:"instances,omitempty" toml:"instances,omitempty"`
}

type Instance struct {
	Matrix    map[string]string `json:"matrix,omitempty" yaml:"matrix,omitempty" toml:"matrix,omitempty"`
	Task      *int              `json:"task,omitempty" yaml:"task,omitempty" toml:"task,omitempty"`
	Status    string            `json:"status" yaml:"status" toml:"status"`
	AttemptID string            `json:"attempt_id,omitempty" yaml:"attempt_id,omitempty" toml:"attempt_id,omitempty"`
}

func Decode(reader io.Reader, format string) (Manifest, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	switch strings.ToLower(format) {
	case "json":
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&manifest); err != nil {
			return Manifest{}, fmt.Errorf("decode JSON manifest: %w", err)
		}
		if err := requireEOF(decoder); err != nil {
			return Manifest{}, err
		}
	case "yaml", "yml":
		var root yaml.Node
		if err := yaml.Unmarshal(data, &root); err != nil {
			return Manifest{}, fmt.Errorf("decode YAML manifest: %w", err)
		}
		if err := rejectYAMLFeatures(&root); err != nil {
			return Manifest{}, err
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&manifest); err != nil {
			return Manifest{}, fmt.Errorf("decode YAML manifest: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err == nil {
				return Manifest{}, errors.New("decode YAML manifest: multiple documents are not allowed")
			}
			return Manifest{}, fmt.Errorf("decode YAML manifest: %w", err)
		}
	case "toml":
		metadata, err := toml.Decode(string(data), &manifest)
		if err != nil {
			return Manifest{}, fmt.Errorf("decode TOML manifest: %w", err)
		}
		if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
			return Manifest{}, fmt.Errorf("decode TOML manifest: unknown field %s", undecoded[0])
		}
	default:
		return Manifest{}, fmt.Errorf("unsupported manifest format %q", format)
	}
	if err := Validate(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("decode JSON manifest: multiple values are not allowed")
		}
		return fmt.Errorf("decode JSON manifest: %w", err)
	}
	return nil
}

func rejectYAMLFeatures(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return errors.New("decode YAML manifest: aliases are not allowed")
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index < len(node.Content); index += 2 {
			if node.Content[index].Value == "<<" {
				return errors.New("decode YAML manifest: merge keys are not allowed")
			}
		}
	}
	for _, child := range node.Content {
		if err := rejectYAMLFeatures(child); err != nil {
			return err
		}
	}
	return nil
}

func Validate(manifest Manifest) error {
	if manifest.Version != Version {
		return fmt.Errorf("unsupported workflow manifest version %d", manifest.Version)
	}
	if len(manifest.Jobs) == 0 {
		return errors.New("workflow manifest has no jobs")
	}
	if err := validateSource(manifest.Source); err != nil {
		return err
	}
	seenNames := make(map[string]bool)
	for index, job := range manifest.Jobs {
		if err := validateJob(job, index, manifest.Source != nil, seenNames); err != nil {
			return err
		}
	}
	return nil
}

func validateSource(source *Source) error {
	if source != nil && (source.Project == "" || len(source.RunIDs) == 0) {
		return errors.New("workflow source requires project and run_ids")
	}
	return nil
}

func validateJob(job Job, index int, hasSource bool, seenNames map[string]bool) error {
	label, err := validateJobName(job.Name, index, seenNames)
	if err != nil {
		return err
	}
	if len(job.Command) == 0 || job.Command[0] == "" {
		return fmt.Errorf("%s has an empty command", label)
	}
	if err := model.ValidateEnvironment(job.Environment); err != nil {
		return fmt.Errorf("%s has invalid environment: %w", label, err)
	}
	if _, _, err := parseExpansion(job); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if !validStatus(job.Status) {
		return fmt.Errorf("%s has invalid status %q", label, job.Status)
	}
	if !hasSource && (job.Status != "" || job.AttemptID != "" || len(job.Instances) > 0) {
		return fmt.Errorf("%s has source state without a workflow source", label)
	}
	if job.Array == "" && len(job.Matrix) == 0 && len(job.Instances) > 0 {
		return fmt.Errorf("%s has instances but is not an array or matrix job", label)
	}
	for _, instance := range job.Instances {
		if !validStatus(instance.Status) || instance.Status == "" {
			return fmt.Errorf("%s has an instance with invalid status %q", label, instance.Status)
		}
	}
	return nil
}

func validateJobName(name string, index int, seenNames map[string]bool) (string, error) {
	if name == "" {
		return fmt.Sprintf("job %d", index+1), nil
	}
	if seenNames[name] {
		return "", fmt.Errorf("duplicate job name: %s", name)
	}
	seenNames[name] = true
	return fmt.Sprintf("job %q", name), nil
}

func validStatus(status string) bool {
	switch status {
	case "", "success", "failed", "cancelled", "unfinished":
		return true
	default:
		return false
	}
}

func Compile(manifest Manifest, nextID func() string) (model.Queue, error) {
	if err := Validate(manifest); err != nil {
		return model.Queue{}, err
	}
	if nextID == nil {
		return model.Queue{}, errors.New("job ID generator is required")
	}
	queue := model.Queue{}
	for _, job := range manifest.Jobs {
		array, dimensions, err := parseExpansion(job)
		if err != nil {
			return model.Queue{}, err
		}
		combinations := model.ExpandMatrix(dimensions)
		if len(combinations) == 0 {
			combinations = [][]model.MatrixValue{{}}
		}
		matrixGroupID := ""
		if len(dimensions) > 0 {
			matrixGroupID = nextID()
		}
		for _, combination := range combinations {
			name := model.MatrixJobName(job.Name, combination)
			environment := model.MatrixEnvironment(job.Environment, combination)
			command := model.QueuedCommand{
				ID: nextID(), Command: append([]string(nil), job.Command...), Name: name,
				Stage: job.Stage, DependsOn: append([]string(nil), job.DependsOn...),
				DependsOnFinished: append([]string(nil), job.DependsOnFinished...), Timeout: job.Timeout, Retry: cloneRetry(job.Retry),
				Executor: job.Executor, ExecutorOptions: append([]string(nil), job.ExecutorOptions...),
				WorkingDirectory: job.WorkingDirectory, Environment: environment, Array: cloneArray(array),
			}
			if matrixGroupID != "" {
				command.Matrix = &model.MatrixSpec{
					GroupID: matrixGroupID, Dimensions: cloneDimensions(dimensions), Values: append([]model.MatrixValue(nil), combination...),
					BaseName: job.Name, BaseEnvironment: append([]string(nil), job.Environment...),
				}
			}
			queue.Commands = append(queue.Commands, command)
		}
	}
	if err := model.ValidateQueueDependencies(queue.Commands); err != nil {
		return model.Queue{}, fmt.Errorf("invalid dependencies: %w", err)
	}
	return queue, nil
}

func cloneDimensions(dimensions []model.MatrixDimension) []model.MatrixDimension {
	cloned := make([]model.MatrixDimension, len(dimensions))
	for index, dimension := range dimensions {
		cloned[index] = model.MatrixDimension{Name: dimension.Name, Values: append([]string(nil), dimension.Values...)}
	}
	return cloned
}

func parseExpansion(job Job) (*model.ArraySpec, []model.MatrixDimension, error) {
	var array *model.ArraySpec
	if job.Array != "" {
		parsed, err := model.ParseArrayRange(job.Array)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid array: %w", err)
		}
		array = &parsed
	}
	dimensions := make([]model.MatrixDimension, 0, len(job.Matrix))
	seen := make(map[string]bool, len(job.Matrix))
	for _, value := range job.Matrix {
		dimension, err := model.ParseMatrixDimension(value)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid matrix: %w", err)
		}
		if seen[dimension.Name] {
			return nil, nil, fmt.Errorf("invalid matrix: duplicate key %q", dimension.Name)
		}
		seen[dimension.Name] = true
		dimensions = append(dimensions, dimension)
	}
	return array, dimensions, nil
}

func cloneArray(array *model.ArraySpec) *model.ArraySpec {
	if array == nil {
		return nil
	}
	cloned := *array
	cloned.Tasks = append([]int(nil), array.Tasks...)
	return &cloned
}

func cloneRetry(retry *int) *int {
	if retry == nil {
		return nil
	}
	value := *retry
	return &value
}
