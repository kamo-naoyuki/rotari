package model

import (
	"path"
	"regexp"
	"strings"
)

var variableReference = regexp.MustCompile(`\$(\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)`)

// shellNames are the shells whose -c script expands variables.
var shellNames = map[string]bool{"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true}

// UnexpandedVariables returns the arguments of command, in order and each
// once, that refer to a variable such as $LR or ${LR}. rotari starts a
// command without a shell, so such an argument reaches the command as
// written. A command a shell runs with -c expands its script itself and has
// none.
func UnexpandedVariables(command []string) []string {
	if len(command) == 0 || runsShellScript(command) {
		return nil
	}
	var found []string
	seen := map[string]bool{}
	for _, argument := range command[1:] {
		if variableReference.MatchString(argument) && !seen[argument] {
			seen[argument] = true
			found = append(found, argument)
		}
	}
	return found
}

// runsShellScript reports a command such as `sh -c SCRIPT` or `bash -lc
// SCRIPT`, whose shell expands the script's variables.
func runsShellScript(command []string) bool {
	if !shellNames[path.Base(command[0])] {
		return false
	}
	for _, argument := range command[1:] {
		if !strings.HasPrefix(argument, "-") || strings.HasPrefix(argument, "--") {
			return false
		}
		if strings.Contains(argument, "c") {
			return true
		}
	}
	return false
}
