package model

import (
	"reflect"
	"strings"
	"testing"
)

func testMatrixGroup(groupID, baseName string, values ...string) []QueuedCommand {
	dimensions := []MatrixDimension{{Name: "SEED", Values: values}}
	commands := make([]QueuedCommand, 0, len(values))
	for _, value := range values {
		combination := []MatrixValue{{Name: "SEED", Value: value}}
		commands = append(commands, QueuedCommand{
			ID: groupID + "-" + value, Name: MatrixJobName(baseName, combination), Command: []string{"train"},
			Environment: MatrixEnvironment([]string{"BASE=yes"}, combination),
			Matrix: &MatrixSpec{
				GroupID: groupID, Dimensions: dimensions, Values: combination,
				BaseName: baseName, BaseEnvironment: []string{"BASE=yes"},
			},
		})
	}
	return commands
}

func TestMatrixJobNameAndEnvironment(t *testing.T) {
	values := []MatrixValue{{Name: "PY", Value: "3.10"}, {Name: "MODEL", Value: "a/b c"}}
	if got := MatrixJobName("train", values); got != "train-PY3.10-MODELa_b_c" {
		t.Fatalf("MatrixJobName = %q", got)
	}
	if got := MatrixJobName("", values); got != "" {
		t.Fatalf("MatrixJobName for unnamed job = %q, want empty", got)
	}
	base := []string{"A=1", "A=2"}
	got := MatrixEnvironment(base, values)
	if !reflect.DeepEqual(got, []string{"A=1", "A=2", "PY=3.10", "MODEL=a/b c"}) {
		t.Fatalf("MatrixEnvironment = %#v", got)
	}
	got[0] = "mutated"
	if base[0] != "A=1" {
		t.Fatal("MatrixEnvironment aliased the base environment")
	}
}

func TestValidateMatrixGroupsAcceptsCompleteGroups(t *testing.T) {
	commands := append(testMatrixGroup("group-a", "train", "1", "2"), testMatrixGroup("group-b", "", "1", "2")...)
	commands = append(commands, QueuedCommand{ID: "plain", Name: "plain", Command: []string{"true"}})
	if err := ValidateMatrixGroups(commands); err != nil {
		t.Fatalf("ValidateMatrixGroups: %v", err)
	}
}

func TestValidateMatrixGroupsRejectsMalformedGroups(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]QueuedCommand) []QueuedCommand
		want   string
	}{
		{name: "missing group ID", want: "invalid matrix provenance", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[0].Matrix.GroupID = ""
			return commands
		}},
		{name: "missing dimensions", want: "invalid matrix provenance", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[0].Matrix.Dimensions = nil
			return commands
		}},
		{name: "incomplete", want: "incomplete", mutate: func(commands []QueuedCommand) []QueuedCommand {
			return commands[:1]
		}},
		{name: "duplicate combination", want: "", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[1].Matrix.Values = commands[0].Matrix.Values
			commands[1].Name = commands[0].Name
			commands[1].Environment = commands[0].Environment
			return commands
		}},
		{name: "unknown combination", want: "", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[1].Matrix.Values = []MatrixValue{{Name: "SEED", Value: "9"}}
			commands[1].Name = "train-SEED9"
			commands[1].Environment = []string{"BASE=yes", "SEED=9"}
			return commands
		}},
		{name: "inconsistent base environment", want: "inconsistent provenance", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[1].Matrix.BaseEnvironment = []string{"BASE=no"}
			commands[1].Environment = []string{"BASE=no", "SEED=2"}
			return commands
		}},
		{name: "inconsistent executor", want: "inconsistent provenance", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[1].Executor = "slurm"
			return commands
		}},
		{name: "inconsistent array", want: "inconsistent provenance", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[1].Array = &ArraySpec{First: 1, Last: 2}
			return commands
		}},
		{name: "renamed member", want: "inconsistent expanded job", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[1].Name = "renamed"
			return commands
		}},
		{name: "edited member environment", want: "inconsistent expanded job", mutate: func(commands []QueuedCommand) []QueuedCommand {
			commands[1].Environment = []string{"BASE=yes", "SEED=2", "EXTRA=1"}
			return commands
		}},
		{name: "same base name in two groups", want: "duplicate matrix name", mutate: func(commands []QueuedCommand) []QueuedCommand {
			return append(commands, testMatrixGroup("other-group", "train", "3")...)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands := test.mutate(testMatrixGroup("group", "train", "1", "2"))
			err := ValidateMatrixGroups(commands)
			if err == nil {
				t.Fatal("ValidateMatrixGroups accepted a malformed group")
			}
			if test.want != "" && !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestClearMatrixGroupOnlyClearsSelectedGroup(t *testing.T) {
	commands := append(testMatrixGroup("group-a", "a", "1", "2"), testMatrixGroup("group-b", "b", "1", "2")...)
	ClearMatrixGroup(commands, "group-a")
	if commands[0].Matrix != nil || commands[1].Matrix != nil {
		t.Fatalf("group-a provenance was not cleared: %#v", commands[:2])
	}
	if commands[2].Matrix == nil || commands[3].Matrix == nil {
		t.Fatalf("group-b provenance was cleared: %#v", commands[2:])
	}
	ClearMatrixGroup(commands, "")
	if commands[2].Matrix == nil {
		t.Fatal("empty group ID cleared provenance")
	}
}

func TestClearIncompleteMatrixGroupsKeepsCompleteGroups(t *testing.T) {
	complete := testMatrixGroup("complete", "a", "1", "2")
	incomplete := testMatrixGroup("incomplete", "b", "1", "2", "3")[:2]
	commands := append(complete, incomplete...)
	ClearIncompleteMatrixGroups(commands)
	if commands[0].Matrix == nil || commands[1].Matrix == nil {
		t.Fatalf("complete group provenance was cleared: %#v", commands[:2])
	}
	if commands[2].Matrix != nil || commands[3].Matrix != nil {
		t.Fatalf("incomplete group provenance was kept: %#v", commands[2:])
	}
	if err := ValidateMatrixGroups(commands); err != nil {
		t.Fatalf("ValidateMatrixGroups after clearing: %v", err)
	}
}

func TestValidateQueueDependenciesResolvesMatrixBaseName(t *testing.T) {
	commands := append(testMatrixGroup("group", "train", "1", "2"),
		QueuedCommand{ID: "evaluate", Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}})
	if err := ValidateQueueDependencies(commands); err != nil {
		t.Fatalf("ValidateQueueDependencies: %v", err)
	}
	jobs := QueueToJobs(commands)
	if got := jobs[2].DependsOn; !reflect.DeepEqual(got, []string{"train-SEED1", "train-SEED2"}) {
		t.Fatalf("expanded dependencies = %#v", got)
	}
}

func TestValidateQueueDependenciesRejectsMatrixNamespaceConflicts(t *testing.T) {
	tests := []struct {
		name  string
		extra QueuedCommand
		want  string
	}{
		{name: "job name", extra: QueuedCommand{ID: "other", Name: "train", Command: []string{"true"}}, want: "job name conflicts with matrix name"},
		{name: "stage name", extra: QueuedCommand{ID: "other", Name: "other", Stage: "train", Command: []string{"true"}}, want: "stage name conflicts with matrix name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands := append(testMatrixGroup("group", "train", "1", "2"), test.extra)
			err := ValidateQueueDependencies(commands)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateQueueDependencies error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestClearMatrixGroupRewritesBaseNameDependencies(t *testing.T) {
	commands := append(testMatrixGroup("group", "train", "1", "2"),
		QueuedCommand{ID: "evaluate", Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"prepare", "train", "train-SEED2"}},
		QueuedCommand{ID: "prepare", Name: "prepare", Command: []string{"prepare"}})
	ClearMatrixGroup(commands, "group")
	if got := commands[2].DependsOn; !reflect.DeepEqual(got, []string{"prepare", "train-SEED1", "train-SEED2"}) {
		t.Fatalf("rewritten dependencies = %#v", got)
	}
	if err := ValidateQueueDependencies(commands); err != nil {
		t.Fatalf("ValidateQueueDependencies after clearing: %v", err)
	}
}

func TestClearIncompleteMatrixGroupsRewritesToRemainingMembers(t *testing.T) {
	commands := append(testMatrixGroup("group", "train", "1", "2", "3")[:2],
		QueuedCommand{ID: "evaluate", Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}})
	ClearIncompleteMatrixGroups(commands)
	if got := commands[2].DependsOn; !reflect.DeepEqual(got, []string{"train-SEED1", "train-SEED2"}) {
		t.Fatalf("rewritten dependencies = %#v", got)
	}
	if err := ValidateQueueDependencies(commands); err != nil {
		t.Fatalf("ValidateQueueDependencies after clearing: %v", err)
	}
}

func TestClearMatrixGroupKeepsUnnamedGroupDependencies(t *testing.T) {
	commands := append(testMatrixGroup("group", "", "1", "2"),
		QueuedCommand{ID: "evaluate", Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"prepare"}})
	ClearMatrixGroup(commands, "group")
	if got := commands[2].DependsOn; !reflect.DeepEqual(got, []string{"prepare"}) {
		t.Fatalf("dependencies = %#v", got)
	}
}
