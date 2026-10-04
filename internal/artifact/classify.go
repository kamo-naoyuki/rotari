// Package artifact discovers artifact candidates: file and directory
// references that can be found statically in a job's definition. A
// candidate is not a proven output of the job; inputs and outputs are not
// distinguished, and discovery is best-effort. The package owns the one
// classifier every discovery source uses. It never opens the paths it finds
// and reads no files itself; callers supply the job definition and the
// contents of referenced sources.
package artifact

import (
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// Rule identifies the classifier rule that accepted a reference, so a
// candidate stays explainable. The IDs match the discovery plan in
// development/2026-10-03-artifact-discovery/plan.md.
type Rule string

const (
	// RuleRedirection accepts a literal file-opening shell redirection
	// target. It is reserved for shell inspection.
	RuleRedirection Rule = "PATH-R1"
	// RuleExplicitPath accepts explicit absolute or relative path notation:
	// /work/results, ./results, ../results.
	RuleExplicitPath Rule = "PATH-R2"
	// RuleDirectoryReference accepts a relative reference containing a
	// directory separator: results/metrics, results/.
	RuleDirectoryReference Rule = "PATH-R3"
	// RuleExtension accepts a basename with a recognized filename extension.
	RuleExtension Rule = "PATH-R4"
	// RulePathKey accepts a literal value under a narrowly recognized path
	// key or long option: output_dir, --file-path, checkpoint_file.
	RulePathKey Rule = "PATH-R5"
	// RuleLogDestination accepts a job's own stdout or stderr destination,
	// which rotari writes itself, without classification.
	RuleLogDestination Rule = "PATH-D1"
)

// Syntax says how a value is written, which decides whether unexpanded
// interpolation and glob characters are literal.
type Syntax int

const (
	// Literal values reach the program as written: argv and environment
	// values, which the executors pass quoted and no shell expands.
	Literal Syntax = iota
	// Interpolated values come from configuration or shell source, where an
	// application or shell may expand $NAME, ${...}, globs, or ~. They are
	// skipped when they contain such syntax (PATH-X2).
	Interpolated
)

// recognizedExtensions is the PATH-R4 list. It is a classifier list, not a
// list of supported viewers or parsers; change it only with accepted and
// rejected fixtures in classify_test.go.
var recognizedExtensions = []string{
	".yaml", ".yml", ".json", ".toml",
	".csv", ".tsv", ".jsonl", ".txt",
	".png", ".jpg", ".jpeg", ".svg", ".pdf",
	".npy", ".npz", ".h5", ".hdf5", ".pt", ".pth",
	".sh", ".py",
}

// specialSinks are device and descriptor paths that are never artifacts
// (PATH-X3).
var specialSinks = []string{
	"/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom",
	"/dev/stdin", "/dev/stdout", "/dev/stderr", "/dev/tty",
}

var (
	uriScheme    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://`)
	opaqueScheme = regexp.MustCompile(`^(?i:mailto|data|urn):`)
	numericRatio = regexp.MustCompile(`^[+-]?[0-9]+(\.[0-9]+)?/[0-9]+(\.[0-9]+)?$`)
	descriptor   = regexp.MustCompile(`^/(dev/fd|proc/self/fd)/[0-9]+$`)
	// regexMarkers are fragments a filename practically never contains but a
	// regular expression usually does.
	regexMarkers = []string{".*", ".+", `\.`, `\d`, `\w`, `\s`, "(?", "[^"}
	// interpolation is $NAME, ${...}, $(...), backquotes, Jinja {{ }}, and
	// Python %(name)s formatting.
	interpolation = regexp.MustCompile("\\$[A-Za-z_{(]|`|\\{\\{|%\\([A-Za-z_]")
	globPattern   = regexp.MustCompile(`[*?]|\[[^\]]*\]`)
)

// Classify decides whether value is a path reference. key is the context
// the value appears under: a long option name, a key=value key, an
// environment variable name, or a configuration key; empty when none. It
// returns the accepting rule, or false when the value is skipped.
func Classify(value, key string, syntax Syntax) (Rule, bool) {
	if excluded(value, syntax) {
		return "", false
	}
	switch {
	case strings.HasPrefix(value, "/"), strings.HasPrefix(value, "./"), strings.HasPrefix(value, "../"):
		return RuleExplicitPath, true
	case strings.Contains(value, "/"):
		return RuleDirectoryReference, true
	case hasRecognizedExtension(value):
		return RuleExtension, true
	case IsPathKey(key):
		return RulePathKey, true
	}
	return "", false
}

// excluded applies PATH-X1 to PATH-X3, which take precedence over every
// positive rule.
func excluded(value string, syntax Syntax) bool {
	if strings.TrimSpace(value) == "" || containsControl(value) {
		return true
	}
	if uriScheme.MatchString(value) || opaqueScheme.MatchString(value) {
		return true
	}
	if isNumeric(value) || numericRatio.MatchString(value) {
		return true
	}
	for _, marker := range regexMarkers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	if strings.HasPrefix(value, "^") {
		return true
	}
	if syntax == Interpolated && (interpolation.MatchString(value) || globPattern.MatchString(value) || strings.HasPrefix(value, "~")) {
		return true
	}
	return IsSpecialSink(value)
}

// IsSpecialSink reports whether value names a device or descriptor that is
// never an artifact, such as /dev/null (PATH-X3).
func IsSpecialSink(value string) bool {
	cleaned := path.Clean(value)
	return slices.Contains(specialSinks, cleaned) || descriptor.MatchString(cleaned)
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func isNumeric(value string) bool {
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func hasRecognizedExtension(value string) bool {
	base := path.Base(value)
	extension := strings.ToLower(path.Ext(base))
	if extension == "" || len(extension) == len(base) {
		return false
	}
	return slices.Contains(recognizedExtensions, extension)
}

// IsPathKey reports whether key narrowly names a path for PATH-R5: path,
// file, or dir, or a name ending in _path, _file, or _dir, compared
// case-insensitively with hyphens read as underscores. Only the leaf of a
// dotted key counts, and leading option dashes and Hydra override prefixes
// (+, ++, ~) are ignored.
func IsPathKey(key string) bool {
	key = strings.TrimLeft(key, "-+~")
	if index := strings.LastIndex(key, "."); index >= 0 {
		key = key[index+1:]
	}
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	switch key {
	case "path", "file", "dir":
		return true
	}
	return strings.HasSuffix(key, "_path") || strings.HasSuffix(key, "_file") || strings.HasSuffix(key, "_dir")
}

// ConfigFormat returns "yaml", "json", or "toml" for a configuration file
// discovery inspects, or "" for any other value.
func ConfigFormat(value string) string {
	switch strings.ToLower(path.Ext(value)) {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".toml":
		return "toml"
	}
	return ""
}
