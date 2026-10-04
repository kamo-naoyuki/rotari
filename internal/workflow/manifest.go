package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"gopkg.in/yaml.v3"
)

const Version = 1

const yamlNullTag = "!!null"

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
	Retry *int `json:"retry,omitempty" yaml:"retry,omitempty" toml:"retry,omitempty"`
	// RetryDelay, RetryBackoff, and RetryMaxDelay match add --retry-delay,
	// --retry-backoff, and --retry-max-delay.
	RetryDelay      string   `json:"retry_delay,omitempty" yaml:"retry_delay,omitempty" toml:"retry_delay,omitempty"`
	RetryBackoff    float64  `json:"retry_backoff,omitempty" yaml:"retry_backoff,omitempty" toml:"retry_backoff,omitempty"`
	RetryMaxDelay   string   `json:"retry_max_delay,omitempty" yaml:"retry_max_delay,omitempty" toml:"retry_max_delay,omitempty"`
	Executor        string   `json:"executor,omitempty" yaml:"executor,omitempty" toml:"executor,omitempty"`
	ExecutorOptions []string `json:"executor_options,omitempty" yaml:"executor_options,omitempty" toml:"executor_options,omitempty"`
	Output          []string `json:"output,omitempty" yaml:"output,omitempty" toml:"output,omitempty"`
	Error           []string `json:"error,omitempty" yaml:"error,omitempty" toml:"error,omitempty"`
	// Artifacts matches add --artifact.
	Artifacts        []string            `json:"artifacts,omitempty" yaml:"artifacts,omitempty" toml:"artifacts,omitempty"`
	LogMode          string              `json:"log_mode,omitempty" yaml:"log_mode,omitempty" toml:"log_mode,omitempty"`
	OpenMode         string              `json:"open_mode,omitempty" yaml:"open_mode,omitempty" toml:"open_mode,omitempty"`
	WorkingDirectory string              `json:"working_directory,omitempty" yaml:"working_directory,omitempty" toml:"working_directory,omitempty"`
	Environment      []string            `json:"environment,omitempty" yaml:"env,omitempty" toml:"environment,omitempty"`
	Array            string              `json:"array,omitempty" yaml:"array,omitempty" toml:"array,omitempty"`
	Matrix           []string            `json:"matrix,omitempty" yaml:"matrix,omitempty" toml:"matrix,omitempty"`
	MatrixExclude    []map[string]string `json:"matrix_exclude,omitempty" yaml:"matrix_exclude,omitempty" toml:"matrix_exclude,omitempty"`
	Status           string              `json:"status,omitempty" yaml:"status,omitempty" toml:"status,omitempty"`
	AttemptID        string              `json:"attempt_id,omitempty" yaml:"attempt_id,omitempty" toml:"attempt_id,omitempty"`
	Instances        []Instance          `json:"instances,omitempty" yaml:"instances,omitempty" toml:"instances,omitempty"`
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
		if err := rejectJSONRepeatedExclusionDimensions(data); err != nil {
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
		if err := rejectYAMLDocuments(data); err != nil {
			return Manifest{}, err
		}
		if err := normalizeYAMLJobMappings(&root); err != nil {
			return Manifest{}, err
		}
		normalized, err := yaml.Marshal(&root)
		if err != nil {
			return Manifest{}, fmt.Errorf("decode YAML manifest: %w", err)
		}
		decoder := yaml.NewDecoder(bytes.NewReader(normalized))
		decoder.KnownFields(true)
		if err := decoder.Decode(&manifest); err != nil {
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

func rejectYAMLDocuments(data []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("decode YAML manifest: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("decode YAML manifest: multiple documents are not allowed")
		}
		return fmt.Errorf("decode YAML manifest: %w", err)
	}
	return nil
}

// rejectJSONRepeatedExclusionDimensions rejects a matrix_exclude rule that
// names a dimension twice. encoding/json keeps the last value, which would
// silently change the rule; YAML and TOML decoding reject the repeat.
func rejectJSONRepeatedExclusionDimensions(data []byte) error {
	var raw struct {
		Jobs []struct {
			MatrixExclude []json.RawMessage `json:"matrix_exclude"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode JSON manifest: %w", err)
	}
	for _, job := range raw.Jobs {
		for _, rule := range job.MatrixExclude {
			decoder := json.NewDecoder(bytes.NewReader(rule))
			if _, err := decoder.Token(); err != nil {
				return fmt.Errorf("decode JSON manifest: %w", err)
			}
			seen := make(map[string]bool)
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return fmt.Errorf("decode JSON manifest: %w", err)
				}
				key, _ := token.(string)
				if seen[key] {
					return fmt.Errorf("decode JSON manifest: matrix_exclude repeats dimension %q", key)
				}
				seen[key] = true
				var value json.RawMessage
				if err := decoder.Decode(&value); err != nil {
					return fmt.Errorf("decode JSON manifest: %w", err)
				}
			}
		}
	}
	return nil
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

func normalizeYAMLJobMappings(root *yaml.Node) error {
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 {
		return nil
	}
	jobs := yamlMappingValue(root.Content[0], "jobs")
	if jobs == nil || jobs.Kind != yaml.SequenceNode {
		return nil
	}
	for _, job := range jobs.Content {
		if err := normalizeYAMLJob(job); err != nil {
			return err
		}
	}
	return nil
}

func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func normalizeYAMLJob(job *yaml.Node) error {
	if job.Kind != yaml.MappingNode {
		return nil
	}
	if err := normalizeYAMLEnvironmentField(job); err != nil {
		return err
	}
	for index := 0; index+1 < len(job.Content); index += 2 {
		key := job.Content[index].Value
		var err error
		switch key {
		case "matrix":
			err = normalizeYAMLMappingField(job, index, normalizeYAMLMatrixMapping)
		case "matrix_exclude":
			err = normalizeYAMLSequenceField(job, index, normalizeYAMLMatrixExclusions)
		case "array":
			err = normalizeYAMLSequenceField(job, index, normalizeYAMLArraySequence)
		case "executor_options":
			err = normalizeYAMLMappingField(job, index, normalizeYAMLExecutorOptionsMapping)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func normalizeYAMLEnvironmentField(job *yaml.Node) error {
	environmentSeen := false
	for index := 0; index+1 < len(job.Content); index += 2 {
		if !isYAMLEnvironmentKey(job.Content[index].Value) {
			continue
		}
		if environmentSeen {
			return errors.New("decode YAML manifest: env and environment cannot both be set")
		}
		environmentSeen = true
		if err := normalizeYAMLMappingField(job, index, normalizeYAMLEnvironmentMapping); err != nil {
			return err
		}
		job.Content[index].Value = "env"
	}
	return nil
}

func isYAMLEnvironmentKey(key string) bool {
	return key == "env" || key == "environment"
}

func normalizeYAMLMappingField(job *yaml.Node, index int, normalize func(*yaml.Node) (*yaml.Node, error)) error {
	return normalizeYAMLField(job, index, yaml.MappingNode, normalize)
}

func normalizeYAMLSequenceField(job *yaml.Node, index int, normalize func(*yaml.Node) (*yaml.Node, error)) error {
	return normalizeYAMLField(job, index, yaml.SequenceNode, normalize)
}

func normalizeYAMLField(job *yaml.Node, index int, kind yaml.Kind, normalize func(*yaml.Node) (*yaml.Node, error)) error {
	value := job.Content[index+1]
	if value.Kind != kind {
		return nil
	}
	normalized, err := normalize(value)
	if err != nil {
		return err
	}
	job.Content[index+1] = normalized
	return nil
}

func normalizeYAMLArraySequence(array *yaml.Node) (*yaml.Node, error) {
	values := make([]string, 0, len(array.Content))
	for _, value := range array.Content {
		if value.Kind != yaml.ScalarNode || value.Tag == yamlNullTag {
			return nil, errors.New("decode YAML manifest: array must be a sequence of non-null scalar task indices")
		}
		values = append(values, value.Value)
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: strings.Join(values, ",")}, nil
}

func normalizeYAMLExecutorOptionsMapping(options *yaml.Node) (*yaml.Node, error) {
	entries := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	seen := make(map[string]bool, len(options.Content)/2)
	for index := 0; index+1 < len(options.Content); index += 2 {
		key, value := options.Content[index], options.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag == yamlNullTag || value.Kind != yaml.ScalarNode || value.Tag == yamlNullTag {
			return nil, errors.New("decode YAML manifest: executor_options must map option names to non-null scalar values")
		}
		if seen[key.Value] {
			return nil, fmt.Errorf("decode YAML manifest: duplicate executor option %q", key.Value)
		}
		seen[key.Value] = true
		entries.Content = append(entries.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.Value + "=" + value.Value})
	}
	return entries, nil
}

func normalizeYAMLEnvironmentMapping(environment *yaml.Node) (*yaml.Node, error) {
	entries := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	seen := make(map[string]bool, len(environment.Content)/2)
	for index := 0; index+1 < len(environment.Content); index += 2 {
		key, value := environment.Content[index], environment.Content[index+1]
		if key.Kind != yaml.ScalarNode || key.Tag == yamlNullTag || value.Kind != yaml.ScalarNode || value.Tag == yamlNullTag {
			return nil, errors.New("decode YAML manifest: environment must map names to non-null scalar values")
		}
		if seen[key.Value] {
			return nil, fmt.Errorf("decode YAML manifest: duplicate environment name %q", key.Value)
		}
		seen[key.Value] = true
		entries.Content = append(entries.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.Value + "=" + value.Value})
	}
	return entries, nil
}

func normalizeYAMLMatrixMapping(matrix *yaml.Node) (*yaml.Node, error) {
	dimensions := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for index := 0; index+1 < len(matrix.Content); index += 2 {
		dimension, err := normalizeYAMLMatrixDimension(matrix.Content[index], matrix.Content[index+1])
		if err != nil {
			return nil, err
		}
		dimensions.Content = append(dimensions.Content, dimension)
	}
	return dimensions, nil
}

func normalizeYAMLMatrixDimension(key, values *yaml.Node) (*yaml.Node, error) {
	if key.Kind != yaml.ScalarNode || values.Kind != yaml.SequenceNode {
		return nil, errors.New("decode YAML manifest: matrix dimensions must map names to value sequences")
	}
	items := make([]string, 0, len(values.Content))
	for _, value := range values.Content {
		item, err := normalizeYAMLMatrixValue(value)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.Value + "=" + strings.Join(items, ",")}, nil
}

func normalizeYAMLMatrixValue(value *yaml.Node) (string, error) {
	if value.Kind != yaml.ScalarNode || value.Tag == "!!null" {
		return "", errors.New("decode YAML manifest: matrix values must be non-null scalars")
	}
	if strings.Contains(value.Value, ",") {
		return "", fmt.Errorf("decode YAML manifest: matrix value %q cannot contain a comma", value.Value)
	}
	return value.Value, nil
}

func normalizeYAMLMatrixExclusions(exclusions *yaml.Node) (*yaml.Node, error) {
	normalized := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, exclusion := range exclusions.Content {
		if exclusion.Kind != yaml.MappingNode {
			return nil, errors.New("decode YAML manifest: matrix_exclude entries must be mappings")
		}
		mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		seen := make(map[string]bool, len(exclusion.Content)/2)
		for index := 0; index+1 < len(exclusion.Content); index += 2 {
			key, value := exclusion.Content[index], exclusion.Content[index+1]
			if key.Kind != yaml.ScalarNode || key.Tag == yamlNullTag || value.Kind != yaml.ScalarNode || value.Tag == yamlNullTag {
				return nil, errors.New("decode YAML manifest: matrix_exclude must map dimension names to non-null scalar values")
			}
			if seen[key.Value] {
				return nil, fmt.Errorf("decode YAML manifest: matrix_exclude repeats dimension %q", key.Value)
			}
			seen[key.Value] = true
			mapping.Content = append(mapping.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key.Value},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value.Value},
			)
		}
		normalized.Content = append(normalized.Content, mapping)
	}
	return normalized, nil
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
	for _, destinations := range [][]string{job.Output, job.Error} {
		seen := make(map[string]bool, len(destinations))
		for _, destination := range destinations {
			if destination == "" || strings.ContainsRune(destination, '\x00') {
				return fmt.Errorf("%s has an invalid output destination", label)
			}
			cleaned := filepath.Clean(destination)
			if seen[cleaned] {
				return fmt.Errorf("%s repeats output destination %q", label, destination)
			}
			seen[cleaned] = true
		}
	}
	if err := model.ValidateArtifacts(job.Artifacts); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if job.LogMode != "" && job.LogMode != model.LogModeMerge && job.LogMode != model.LogModeSeparate {
		return fmt.Errorf("%s has invalid log mode %q", label, job.LogMode)
	}
	if job.OpenMode != "" && job.OpenMode != model.OpenModeAppend && job.OpenMode != model.OpenModeTruncate {
		return fmt.Errorf("%s has invalid output open mode %q", label, job.OpenMode)
	}
	if _, err := parseExpansion(job); err != nil {
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
		expansion, err := parseExpansion(job)
		if err != nil {
			return model.Queue{}, err
		}
		array, dimensions, exclusions, combinations := expansion.array, expansion.dimensions, expansion.exclusions, expansion.combinations
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
				RetryDelay: job.RetryDelay, RetryBackoff: job.RetryBackoff, RetryMaxDelay: job.RetryMaxDelay,
				Executor: job.Executor, ExecutorOptions: append([]string(nil), job.ExecutorOptions...),
				Output: append([]string(nil), job.Output...), Error: append([]string(nil), job.Error...), Artifacts: append([]string(nil), job.Artifacts...),
				LogMode: job.LogMode, OpenMode: job.OpenMode,
				WorkingDirectory: job.WorkingDirectory, Environment: environment, Array: cloneArray(array),
			}
			if matrixGroupID != "" {
				command.Matrix = &model.MatrixSpec{
					GroupID: matrixGroupID, Dimensions: cloneDimensions(dimensions), Values: append([]model.MatrixValue(nil), combination...), Exclusions: model.CloneMatrixExclusions(exclusions),
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

type jobExpansion struct {
	array        *model.ArraySpec
	dimensions   []model.MatrixDimension
	exclusions   []model.MatrixExclusion
	combinations [][]model.MatrixValue
}

func parseExpansion(job Job) (jobExpansion, error) {
	var expansion jobExpansion
	if job.Array != "" {
		parsed, err := model.ParseArrayRange(job.Array)
		if err != nil {
			return jobExpansion{}, fmt.Errorf("invalid array: %w", err)
		}
		expansion.array = &parsed
	}
	dimensions := make([]model.MatrixDimension, 0, len(job.Matrix))
	seen := make(map[string]bool, len(job.Matrix))
	for _, value := range job.Matrix {
		dimension, err := model.ParseMatrixDimension(value)
		if err != nil {
			return jobExpansion{}, fmt.Errorf("invalid matrix: %w", err)
		}
		if seen[dimension.Name] {
			return jobExpansion{}, fmt.Errorf("invalid matrix: duplicate key %q", dimension.Name)
		}
		seen[dimension.Name] = true
		dimensions = append(dimensions, dimension)
	}
	exclusions := make([]model.MatrixExclusion, 0, len(job.MatrixExclude))
	for _, raw := range job.MatrixExclude {
		exclusion := model.MatrixExclusion{Values: make([]model.MatrixValue, 0, len(raw))}
		for name, value := range raw {
			exclusion.Values = append(exclusion.Values, model.MatrixValue{Name: name, Value: value})
		}
		exclusions = append(exclusions, exclusion)
	}
	combinations, normalizedExclusions, err := model.ExpandMatrixWithExclusions(dimensions, exclusions)
	if err != nil {
		return jobExpansion{}, fmt.Errorf("invalid matrix_exclude: %w", err)
	}
	expansion.dimensions = dimensions
	expansion.exclusions = normalizedExclusions
	expansion.combinations = combinations
	return expansion, nil
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
