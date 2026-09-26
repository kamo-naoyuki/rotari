package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const projectNameEnv = "ROTARI_PROJECT_NAME"

// DefaultProjectName is the project used when none is given and the base
// directory does not have exactly one.
const DefaultProjectName = "default"

// ProjectNameGiven reports whether a project was chosen explicitly, by
// cliProjectName or ROTARI_PROJECT_NAME, rather than inferred from the base
// directory.
func ProjectNameGiven(cliProjectName string) bool {
	return cliProjectName != "" || os.Getenv(projectNameEnv) != ""
}

func ResolveProjectName(baseDir, cliProjectName string) (string, error) {
	if cliProjectName != "" {
		if !IsValidPathElement(cliProjectName) {
			return "", fmt.Errorf("invalid project name %q", cliProjectName)
		}
		return cliProjectName, nil
	}
	if value := os.Getenv(projectNameEnv); value != "" {
		if !IsValidPathElement(value) {
			return "", fmt.Errorf("invalid project name %q", value)
		}
		return value, nil
	}
	projectsDir := filepath.Join(baseDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err == nil {
		available := make([]string, 0)
		for _, entry := range entries {
			if entry.IsDir() {
				available = append(available, entry.Name())
			}
		}
		if len(available) == 1 {
			return available[0], nil
		}
		if len(available) > 1 {
			sort.Strings(available)
			list := make([]string, 0, len(available))
			for _, project := range available {
				list = append(list, "  - "+project)
			}
			return "", fmt.Errorf("multiple projects exist in state directory %q; please specify one with --project-name or ROTARI_PROJECT_NAME:\n%s", baseDir, joinLines(list))
		}
	}
	return DefaultProjectName, nil
}

func joinLines(lines []string) string {
	result := ""
	for index, line := range lines {
		if index > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}
