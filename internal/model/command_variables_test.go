package model

import (
	"reflect"
	"testing"
)

func TestUnexpandedVariables(t *testing.T) {
	for _, test := range []struct {
		command []string
		want    []string
	}{
		{[]string{"python3", "train.py", "--lr", "$LR", "--batch-size", "${BS}", "--again", "$LR"}, []string{"$LR", "${BS}"}},
		{[]string{"python3", "train.py", "--tag=run-$SEED"}, []string{"--tag=run-$SEED"}},
		{[]string{"sh", "-c", `python3 train.py --lr "$LR"`}, nil},
		{[]string{"/bin/bash", "-lc", `python3 train.py --lr "$LR"`}, nil},
		{[]string{"bash", "script.sh", "$LR"}, []string{"$LR"}},
		{[]string{"echo", "$5", "price $", "a$"}, nil},
		{[]string{"$HOME/bin/tool"}, nil},
		{nil, nil},
	} {
		if got := UnexpandedVariables(test.command); !reflect.DeepEqual(got, test.want) {
			t.Errorf("UnexpandedVariables(%q) = %q, want %q", test.command, got, test.want)
		}
	}
}
