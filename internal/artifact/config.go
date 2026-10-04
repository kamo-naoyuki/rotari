package artifact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Limits on inspecting one configuration source.
const (
	// MaxSourceBytes is the largest source discovery parses. A larger file
	// is not read at all: a configuration file that size is abnormal, and
	// YAML parses at only a few megabytes per second.
	MaxSourceBytes = 256 << 10
	// MaxSourceDepth bounds nesting of mappings and sequences.
	MaxSourceDepth = 64
	// MaxSourceValues bounds the scalar values inspected in one source.
	MaxSourceValues = 50000
)

// SourceReader returns the contents of a referenced source file, given its
// absolute path. It returns an error when the file cannot be read safely;
// discovery records the error as a diagnostic and continues.
type SourceReader func(path string) ([]byte, error)

// ConfigReader returns the references in the configuration file at an
// absolute path, as ConfigReferences finds them. With an error it may still
// return the references found before a limit was reached. A ConfigReader
// may cache results: they do not depend on the job.
type ConfigReader func(path string) ([]ConfigReference, error)

// ParseSources returns a ConfigReader that reads each file through read and
// parses it with ConfigReferences, without caching.
func ParseSources(read SourceReader) ConfigReader {
	return func(path string) ([]ConfigReference, error) {
		data, err := read(path)
		if err != nil {
			return nil, err
		}
		return ConfigReferences(ConfigFormat(path), data)
	}
}

// Sources reads the files discovery inspects. A nil reader inspects no
// file of its kind.
type Sources struct {
	// Config returns the references of a configuration file.
	Config ConfigReader
	// Script returns the contents of a shell script or Python file.
	Script SourceReader
}

// Discover finds the candidates FromJob finds, then inspects the shell
// scripts the job runs and the Python files it names through
// sources.Script, then each configuration file (.yaml, .yml, .json, .toml)
// referenced by a command argument, an environment value, shell source, or
// Python source and resolved to an absolute path, through sources.Config. References found in a configuration file are not
// inspected in turn.
func Discover(job Job, sources Sources) Result {
	collector := newCollector(job.WorkingDirectory)
	collector.variables = map[string]string{}
	for _, entry := range job.Environment {
		if name, value, found := strings.Cut(entry, "="); found {
			collector.variables[name] = value
		}
	}
	for name, value := range job.Variables {
		collector.variables[name] = value
	}
	collector.arguments(job.Command)
	collector.environment(job.Environment)
	collector.destinations(job.Output, job.Error)
	if sources.Script != nil {
		collector.scripts(sources.Script)
		collector.pythonFiles(sources.Script)
	}
	if sources.Config != nil {
		collector.configs(sources.Config)
	}
	return collector.result
}

func (c *collector) configs(read ConfigReader) {
	var files []string
	for _, candidate := range c.result.Candidates {
		if candidate.Basis == BasisUnresolved || ConfigFormat(candidate.Path) == "" {
			continue
		}
		if slices.ContainsFunc(candidate.Sources, func(source Source) bool {
			return source.Kind == KindArgument || source.Kind == KindEnvironment || source.Kind == KindShell || source.Kind == KindPython
		}) {
			files = append(files, candidate.Path)
		}
	}
	for _, file := range files {
		references, err := read(file)
		if err != nil {
			c.diagnose(file, "not inspected: "+err.Error())
		}
		for _, reference := range references {
			c.add(reference.Value, Source{Kind: KindConfig, Rule: reference.Rule, Key: reference.Key, File: file, Location: reference.Location})
		}
	}
}

// ConfigReference is one path reference found in a configuration source.
type ConfigReference struct {
	Value    string
	Rule     Rule
	Key      string
	Location string
}

// ConfigReferences parses a configuration source of format "yaml",
// "json", or "toml" structurally and classifies each string value, with
// its nearest mapping key as context. Interpolation, custom YAML tags, and
// aliases are not evaluated: a tagged or aliased value is skipped, as is a
// string with unresolved interpolation. The references found before a
// limit was reached are returned with the error.
func ConfigReferences(format string, data []byte) ([]ConfigReference, error) {
	if len(data) > MaxSourceBytes {
		return nil, fmt.Errorf("larger than %d bytes", MaxSourceBytes)
	}
	walker := &configWalker{}
	var err error
	switch format {
	case "yaml":
		err = walker.yaml(data)
	case "json":
		err = walker.json(data)
	case "toml":
		err = walker.toml(data)
	default:
		return nil, fmt.Errorf("unsupported configuration format %q", format)
	}
	return walker.references, err
}

var errSourceLimit = errors.New("inspection limit reached")

type configWalker struct {
	references []ConfigReference
	values     int
}

func (w *configWalker) scalar(value, key, location string) error {
	w.values++
	if w.values > MaxSourceValues {
		return fmt.Errorf("%w: more than %d values", errSourceLimit, MaxSourceValues)
	}
	if rule, ok := Classify(value, key, Interpolated); ok {
		w.references = append(w.references, ConfigReference{Value: value, Rule: rule, Key: key, Location: location})
	}
	return nil
}

func depthError() error {
	return fmt.Errorf("%w: nested deeper than %d", errSourceLimit, MaxSourceDepth)
}

func (w *configWalker) yaml(data []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for document := 0; ; document++ {
		var node yaml.Node
		if err := decoder.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errors.New("cannot parse YAML")
		}
		prefix := ""
		if document > 0 {
			prefix = "$" + strconv.Itoa(document)
		}
		if err := w.yamlNode(&node, "", prefix, 0); err != nil {
			return err
		}
	}
}

func (w *configWalker) yamlNode(node *yaml.Node, key, location string, depth int) error {
	if depth > MaxSourceDepth {
		return depthError()
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := w.yamlNode(child, key, location, depth+1); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			name := node.Content[index]
			if name.Kind != yaml.ScalarNode || name.ShortTag() != "!!str" {
				continue
			}
			if err := w.yamlNode(node.Content[index+1], name.Value, childKey(location, name.Value), depth+1); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for index, child := range node.Content {
			if err := w.yamlNode(child, key, childIndex(location, index), depth+1); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		// Only strings: a custom tag may be evaluated by the application,
		// and other core tags (!!int, !!null, ...) are not strings.
		if node.ShortTag() == "!!str" {
			return w.scalar(node.Value, key, location)
		}
	}
	// Alias nodes are not followed, so aliases cannot expand without bound.
	return nil
}

func (w *configWalker) json(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return errors.New("cannot parse JSON")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("cannot parse JSON")
	}
	return w.value(value, "", "", 0)
}

func (w *configWalker) toml(data []byte) error {
	var value map[string]any
	if _, err := toml.Decode(string(data), &value); err != nil {
		return errors.New("cannot parse TOML")
	}
	return w.value(value, "", "", 0)
}

// value walks a decoded JSON or TOML value. Mapping keys are visited in
// sorted order, so the result is deterministic.
func (w *configWalker) value(value any, key, location string, depth int) error {
	if depth > MaxSourceDepth {
		return depthError()
	}
	switch typed := value.(type) {
	case string:
		return w.scalar(typed, key, location)
	case map[string]any:
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			if err := w.value(typed[name], name, childKey(location, name), depth+1); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := w.value(child, key, childIndex(location, index), depth+1); err != nil {
				return err
			}
		}
	case []map[string]any:
		for index, child := range typed {
			if err := w.value(child, key, childIndex(location, index), depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func childKey(location, name string) string {
	if name == "" || strings.ContainsAny(name, ".[]\"' \t") {
		return location + "[" + strconv.Quote(name) + "]"
	}
	if location == "" {
		return name
	}
	return location + "." + name
}

func childIndex(location string, index int) string {
	return location + "[" + strconv.Itoa(index) + "]"
}
