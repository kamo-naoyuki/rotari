package queueops

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

// TestAddAndChangeWarnAboutUnexpandedVariables adds a matrix whose command
// passes $LR and $BS as arguments, which rotari does not expand, then the
// same command through a shell, which does; and changes a job's command to
// one with a variable. Only the commands without a shell are warned about,
// once each, naming every such argument.
func TestAddAndChangeWarnAboutUnexpandedVariables(t *testing.T) {
	baseDir := t.TempDir()
	var warnings []string
	editor := testEditor()
	editor.Warn = func(message string) { warnings = append(warnings, message) }
	matrix := func(command ...string) []model.QueuedCommand {
		var commands []model.QueuedCommand
		for _, lr := range []string{"0.1", "0.01"} {
			commands = append(commands, model.QueuedCommand{Name: "train-" + lr, Command: command, Environment: []string{"LR=" + lr, "BS=32"}})
		}
		return commands
	}

	if _, err := editor.Add(baseDir, "demo", matrix("python3", "train.py", "--lr", "$LR", "--batch-size", "${BS}"), nil); err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"$LR", "${BS}" reach the command as written`) {
		t.Fatalf("add warnings = %q, want one naming $LR and ${BS}", warnings)
	}

	warnings = nil
	shell := matrix("sh", "-c", `python3 train.py --lr "$LR"`)
	shell[0].Name, shell[1].Name = "shell-0.1", "shell-0.01"
	if _, err := editor.Add(baseDir, "demo", shell, nil); err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("a command run through a shell was warned about: %q", warnings)
	}

	if _, err := editor.Change(baseDir, "demo", "", model.CommandSelector{Name: "shell-0.1"}, Mutation{Command: []string{"python3", "eval.py", "--seed=$SEED"}}); err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `"--seed=$SEED"`) {
		t.Fatalf("change warnings = %q, want one naming --seed=$SEED", warnings)
	}
}
