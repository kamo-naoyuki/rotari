package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestSelectCommands(t *testing.T) {
	commands := []QueuedCommand{
		{ID: "a", Name: "prep", Stage: "setup"},
		{ID: "b", Name: "train-SEED1", Matrix: &MatrixSpec{GroupID: "g", BaseName: "train"}},
		{ID: "c", Name: "train-SEED2", Matrix: &MatrixSpec{GroupID: "g", BaseName: "train"}},
		{ID: "d", Name: "eval", Command: []string{"eval"}, Array: &ArraySpec{First: 1, Last: 2}},
	}
	for _, test := range []struct {
		name     string
		selector CommandSelector
		want     []int
		err      string
	}{
		{"ids", CommandSelector{IDs: []string{"c", "a"}}, []int{0, 2}, ""},
		{"name", CommandSelector{Name: "eval"}, []int{3}, ""},
		{"stage", CommandSelector{Stage: "setup"}, []int{0}, ""},
		{"matrix", CommandSelector{Matrix: "train"}, []int{1, 2}, ""},
		{"all", CommandSelector{All: true}, []int{0, 1, 2, 3}, ""},
		{"one id missing", CommandSelector{IDs: []string{"a", "x"}}, nil, "one or more jobs not found: x"},
		{"no id found", CommandSelector{IDs: []string{"x"}}, nil, "job not found"},
		{"array task id", CommandSelector{IDs: []string{"d-2"}}, nil, "d-2 is a task of array job d"},
		{"array task name", CommandSelector{Name: "eval[1]"}, nil, "eval[1] is a task of array job d"},
		{"unknown stage", CommandSelector{Stage: "train"}, nil, `no jobs in stage "train"`},
		{"unknown matrix", CommandSelector{Matrix: "setup"}, nil, `no matrix named "setup"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := SelectCommands(commands, test.selector)
			if test.err != "" {
				if err == nil || !strings.Contains(err.Error(), test.err) {
					t.Fatalf("SelectCommands() error = %v, want %q", err, test.err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("SelectCommands() = %v, %v; want %v", got, err, test.want)
			}
		})
	}
}
