package model

import "testing"

func TestFormatDependenciesJoinsArrayTasksIntoRanges(t *testing.T) {
	tasks := func(name string, ids ...string) []string {
		out := make([]string, len(ids))
		for index, id := range ids {
			out[index] = name + "[" + id + "]"
		}
		return out
	}
	for _, test := range []struct {
		name                string
		dependsOn, finished []string
		want                string
	}{
		{"whole array", nil, tasks("train", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"), "finished:train[1-12]"},
		{"gaps", tasks("train", "1", "2", "4", "6", "7", "8"), nil, "train[1-2,4,6-8]"},
		{"mixed", append([]string{"prep"}, tasks("train", "1", "2")...), []string{"setup"}, "prep,train[1-2],finished:setup"},
		{"no array", []string{"a", "b"}, nil, "a,b"},
	} {
		if got := FormatDependencies(test.dependsOn, test.finished, ","); got != test.want {
			t.Errorf("%s: FormatDependencies = %q, want %q", test.name, got, test.want)
		}
	}
}
