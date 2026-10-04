package artifact

import (
	"fmt"
	"path"
	"strings"
)

// maxDescribedSources bounds how many sources Describe names before
// counting the rest.
const maxDescribedSources = 3

// Describe returns a one-line account of where a candidate was found, for
// viewers: the option, variable, or key it appeared under, the redirection
// operator, and the file and line:column of configuration, shell, or Python
// source. Identical accounts are named once, and sources beyond the first
// few are counted.
func Describe(sources []Source) string {
	var parts []string
	seen := map[string]bool{}
	for _, source := range sources {
		text := source.describe()
		if !seen[text] {
			seen[text] = true
			parts = append(parts, text)
		}
	}
	if len(parts) > maxDescribedSources {
		return fmt.Sprintf("%s, +%d more", strings.Join(parts[:maxDescribedSources], ", "), len(parts)-maxDescribedSources)
	}
	return strings.Join(parts, ", ")
}

func (source Source) describe() string {
	switch source.Kind {
	case KindArgument:
		if source.Key != "" {
			return source.Key
		}
		return "argument"
	case KindEnvironment:
		return "env " + source.Key
	case KindOutput:
		return "--output"
	case KindError:
		return "--error"
	case KindConfig:
		return path.Base(source.File) + ": " + source.Location
	}
	where := "shell code " + source.Location
	if source.File != "" {
		where = path.Base(source.File) + ":" + source.Location
	}
	switch {
	case source.Direction != "":
		return source.Direction + " (" + where + ")"
	case source.Key != "":
		return source.Key + " (" + where + ")"
	}
	return where
}
