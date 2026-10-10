package queueops

import (
	"fmt"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// unexpandedVariableWarning warns, once for all commands, about arguments
// that refer to a variable the command will receive as written, because
// rotari starts commands without a shell (model.UnexpandedVariables).
func unexpandedVariableWarning(commands []model.QueuedCommand) []string {
	var found []string
	seen := map[string]bool{}
	for _, command := range commands {
		for _, argument := range model.UnexpandedVariables(command.Command) {
			if !seen[argument] {
				seen[argument] = true
				found = append(found, fmt.Sprintf("%q", argument))
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("warning: rotari starts commands without a shell, so %s reach the command as written, not expanded. "+
		"Read the variable in the program, such as os.environ[\"LR\"], or run the command through a shell: "+
		"-- sh -c 'python3 train.py --lr \"$LR\"'", strings.Join(found, ", "))}
}
