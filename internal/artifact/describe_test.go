package artifact

import "testing"

func TestDescribe(t *testing.T) {
	tests := []struct {
		name    string
		sources []Source
		want    string
	}{
		{name: "argument", sources: []Source{{Kind: KindArgument}}, want: "argument"},
		{name: "option", sources: []Source{{Kind: KindArgument, Key: "--config"}}, want: "--config"},
		{name: "environment", sources: []Source{{Kind: KindEnvironment, Key: "OUT_DIR"}}, want: "env OUT_DIR"},
		{name: "both streams", sources: []Source{{Kind: KindOutput, Stream: "stdout"}, {Kind: KindOutput, Stream: "stderr"}}, want: "--output"},
		{name: "config", sources: []Source{{Kind: KindConfig, File: "/work/conf/a.yaml", Location: "train.out_dir"}}, want: "a.yaml: train.out_dir"},
		{name: "redirection in shell code", sources: []Source{{Kind: KindShell, Location: "1:20", Direction: ">"}}, want: "> (shell code 1:20)"},
		{name: "redirection in a script", sources: []Source{{Kind: KindShell, File: "/work/run.sh", Location: "2:15", Direction: ">>"}}, want: ">> (run.sh:2:15)"},
		{name: "shell argument", sources: []Source{{Kind: KindShell, File: "/work/run.sh", Location: "1:8"}}, want: "run.sh:1:8"},
		{name: "python default", sources: []Source{{Kind: KindPython, File: "/work/train.py", Location: "3:41", Key: "--output-dir"}}, want: "--output-dir (train.py:3:41)"},
		{name: "several", sources: []Source{{Kind: KindArgument}, {Kind: KindEnvironment, Key: "A"}, {Kind: KindArgument}, {Kind: KindOutput}, {Kind: KindError}}, want: "argument, env A, --output, +1 more"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Describe(test.sources); got != test.want {
				t.Fatalf("Describe = %q, want %q", got, test.want)
			}
		})
	}
}
