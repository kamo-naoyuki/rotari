package artifact

import (
	"path"
	"regexp"
	"strings"
)

// Source kinds say where a reference was found.
const (
	KindArgument    = "argument"
	KindEnvironment = "environment"
	KindOutput      = "output"
	KindError       = "error"
	KindConfig      = "config"
)

// Resolution bases say how a candidate's Path was obtained.
const (
	// BasisAbsolute: the reference was written as an absolute path.
	BasisAbsolute = "absolute"
	// BasisWorkingDirectory: a relative reference joined to the job's
	// effective working directory.
	BasisWorkingDirectory = "working_directory"
	// BasisUnresolved: a relative reference whose working directory is not
	// known yet, such as a queued job without an absolute directory.
	BasisUnresolved = "unresolved"
)

// MaxCandidates bounds the candidates one job records. References beyond it
// are dropped with a diagnostic.
const MaxCandidates = 1000

// Job is the part of a job definition discovery reads.
type Job struct {
	Command     []string
	Environment []string
	Output      []string
	Error       []string
	// WorkingDirectory is the effective directory the job runs in. When it
	// is empty or relative, relative references stay unresolved.
	WorkingDirectory string
}

// Candidate is one discovered file or directory reference. It says nothing
// about whether the path exists, its type, or whether the job created it.
type Candidate struct {
	// Path is the cleaned reference: absolute unless Basis is unresolved.
	Path    string   `json:"path"`
	Basis   string   `json:"basis"`
	Sources []Source `json:"sources"`
}

// Source records where one reference to a candidate was found and the
// evidence that hints at its role, without deciding the role.
type Source struct {
	Kind string `json:"kind"`
	// Value is the reference as written.
	Value string `json:"value"`
	Rule  Rule   `json:"rule"`
	// Index is the position in the command, environment, or destination list.
	Index *int `json:"index,omitempty"`
	// Key is the option, key=value key, environment variable, or
	// configuration key the value appeared under.
	Key string `json:"key,omitempty"`
	// Stream is stdout or stderr for a log destination.
	Stream string `json:"stream,omitempty"`
	// File is the configuration source the reference was found in.
	File string `json:"file,omitempty"`
	// Location is the key or index path within File, such as train.paths[1].
	Location string `json:"location,omitempty"`
}

// Diagnostic records a discovery problem. It never changes the job's
// execution or result.
type Diagnostic struct {
	Source  string `json:"source,omitempty"`
	Message string `json:"message"`
}

// Result is a job's discovered candidates, in order of first reference.
type Result struct {
	Candidates  []Candidate  `json:"candidates,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// overrideKey is the key of a key=value argument: an identifier, possibly
// dotted, optionally with a Hydra override prefix (+, ++, ~). Anything else
// containing = is read as one standalone value.
var overrideKey = regexp.MustCompile(`^(\+\+|\+|~)?[A-Za-z_][A-Za-z0-9_.\-]*$`)

// FromJob discovers the candidates in a job's command arguments,
// environment values, and log destinations. Referenced configuration files
// are not read; see Discover.
func FromJob(job Job) Result {
	collector := newCollector(job.WorkingDirectory)
	collector.arguments(job.Command)
	collector.environment(job.Environment)
	collector.destinations(job.Output, job.Error)
	return collector.result
}

type collector struct {
	base    string
	result  Result
	byPath  map[string]int
	limited bool
}

func newCollector(workingDirectory string) *collector {
	base := ""
	if path.IsAbs(workingDirectory) {
		base = path.Clean(workingDirectory)
	}
	return &collector{base: base, byPath: map[string]int{}}
}

// resolve returns the candidate path and basis for a reference.
func (c *collector) resolve(value string) (string, string) {
	switch {
	case path.IsAbs(value):
		return path.Clean(value), BasisAbsolute
	case c.base != "":
		return path.Join(c.base, value), BasisWorkingDirectory
	}
	return path.Clean(value), BasisUnresolved
}

// add records source for value, merging it into an existing candidate with
// the same path. A new candidate beyond MaxCandidates is dropped.
func (c *collector) add(value string, source Source) {
	resolved, basis := c.resolve(value)
	source.Value = value
	if index, ok := c.byPath[resolved]; ok {
		c.result.Candidates[index].Sources = append(c.result.Candidates[index].Sources, source)
		return
	}
	if len(c.result.Candidates) >= MaxCandidates {
		if !c.limited {
			c.limited = true
			c.diagnose(source.Kind, "candidate limit reached; later references are not recorded")
		}
		return
	}
	c.byPath[resolved] = len(c.result.Candidates)
	c.result.Candidates = append(c.result.Candidates, Candidate{Path: resolved, Basis: basis, Sources: []Source{source}})
}

func (c *collector) diagnose(source, message string) {
	for _, existing := range c.result.Diagnostics {
		if existing.Source == source && existing.Message == message {
			return
		}
	}
	c.result.Diagnostics = append(c.result.Diagnostics, Diagnostic{Source: source, Message: message})
}

func indexOf(value int) *int {
	return &value
}

// arguments classifies the literal argv words. The argv is never joined
// and reinterpreted as shell code, so a word containing > or $ is literal.
func (c *collector) arguments(command []string) {
	shape := recognizeCommand(command)
	skip := map[int]bool{}
	for _, index := range shape.commandWords {
		skip[index] = true
	}
	if shape.code >= 0 {
		skip[shape.code] = true
	}
	previousOption := ""
	for index, word := range command {
		option := previousOption
		previousOption = ""
		if skip[index] {
			continue
		}
		value, key := word, option
		switch {
		case strings.HasPrefix(word, "--"):
			name, optionValue, found := strings.Cut(word, "=")
			if !found {
				previousOption = word
				continue
			}
			value, key = optionValue, name
		case strings.HasPrefix(word, "-"):
			continue
		default:
			if name, keyValue, found := strings.Cut(word, "="); found && overrideKey.MatchString(name) {
				value, key = keyValue, name
			}
		}
		if rule, ok := Classify(value, key, Literal); ok {
			c.add(value, Source{Kind: KindArgument, Rule: rule, Index: indexOf(index), Key: key})
		}
	}
}

// environment classifies KEY=value entries with the variable name as key
// context. A search-path list such as PATH=/usr/bin:/bin is not one path
// and is skipped.
func (c *collector) environment(entries []string) {
	for index, entry := range entries {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if strings.HasSuffix(strings.ToUpper(name), "PATH") && strings.Contains(value, ":") {
			continue
		}
		if rule, ok := Classify(value, name, Literal); ok {
			c.add(value, Source{Kind: KindEnvironment, Rule: rule, Index: indexOf(index), Key: name})
		}
	}
}

// destinations records the job's own log destinations (PATH-D1). rotari
// opens them itself, so they skip classification apart from the special
// sink exclusion. stderr follows the stdout destinations when the job has
// no error destination.
func (c *collector) destinations(outputs, errors []string) {
	for index, value := range outputs {
		c.destination(KindOutput, index, value, "stdout")
		if len(errors) == 0 {
			c.destination(KindOutput, index, value, "stderr")
		}
	}
	for index, value := range errors {
		c.destination(KindError, index, value, "stderr")
	}
}

func (c *collector) destination(kind string, index int, value, stream string) {
	if value == "" || IsSpecialSink(value) {
		return
	}
	c.add(value, Source{Kind: kind, Rule: RuleLogDestination, Index: indexOf(index), Stream: stream})
}
