package artifact

import "testing"

// TestClassify is the accepted/rejected fixture table for the shared
// classifier. A change to a rule must keep every row's decision or change
// the row deliberately.
func TestClassify(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		key    string
		syntax Syntax
		want   Rule // "" means skipped
	}{
		// Positive rules.
		{name: "absolute path", value: "/work/results", want: RuleExplicitPath},
		{name: "dot relative", value: "./results", want: RuleExplicitPath},
		{name: "parent relative", value: "../results", want: RuleExplicitPath},
		{name: "directory reference", value: "results/metrics", want: RuleDirectoryReference},
		{name: "trailing separator", value: "results/", want: RuleDirectoryReference},
		{name: "nonexistent csv", value: "metrics.csv", want: RuleExtension},
		{name: "upper case extension", value: "PLOT.PNG", want: RuleExtension},
		{name: "script", value: "train.py", want: RuleExtension},
		{name: "shell script", value: "run.sh", want: RuleExtension},
		{name: "config", value: "config.yaml", want: RuleExtension},
		{name: "space and unicode", value: "結果 table.csv", want: RuleExtension},
		{name: "space in directory", value: "my results/", want: RuleDirectoryReference},
		{name: "directory key", value: "results", key: "output_dir", want: RulePathKey},
		{name: "hyphenated long option", value: "results", key: "--file-path", want: RulePathKey},
		{name: "file suffix key", value: "latest", key: "checkpoint_file", want: RulePathKey},
		{name: "exact path key", value: "results", key: "PATH", want: RulePathKey},
		{name: "environment directory key", value: "results", key: "OUTPUT_DIR", want: RulePathKey},
		{name: "dotted key leaf", value: "results", key: "train.log_dir", want: RulePathKey},
		{name: "hydra override key", value: "results", key: "+trainer.save_dir", want: RulePathKey},
		{name: "extension wins over key", value: "run.yaml", key: "config_file", want: RuleExtension},
		{name: "raw argv dollar is literal", value: "out/$X", want: RuleDirectoryReference},
		{name: "raw argv star is literal", value: "out/*.csv", want: RuleDirectoryReference},
		{name: "raw argv tilde is literal", value: "~/data", want: RuleDirectoryReference},

		// Known false positives that PATH-R3 accepts by design.
		{name: "hugging face model id", value: "meta-llama/Llama-3-8B", want: RuleDirectoryReference},
		{name: "git ref", value: "origin/main", want: RuleDirectoryReference},
		{name: "date", value: "2026/10/03", want: RuleDirectoryReference},

		// Skipped values.
		{name: "bare name", value: "results"},
		{name: "format under generic output", value: "png", key: "output"},
		{name: "generic input key", value: "data", key: "input"},
		{name: "version", value: "v1.2.3"},
		{name: "unknown suffix", value: "model.ckpt"},
		{name: "extension alone", value: ".json"},
		{name: "empty", value: ""},
		{name: "blank under path key", value: " ", key: "output_dir"},
		{name: "url", value: "https://example.org/plot.png"},
		{name: "s3 uri", value: "s3://bucket/results"},
		{name: "mailto", value: "mailto:me@example.org"},
		{name: "decimal", value: "0.001"},
		{name: "scientific", value: "1e-3"},
		{name: "number under path key", value: "10", key: "output_dir"},
		{name: "ratio", value: "1/2"},
		{name: "decimal ratio", value: "0.5/1.5"},
		{name: "regex anchored", value: "^results"},
		{name: "regex wildcard", value: ".*/metrics.csv"},
		{name: "regex escaped dot", value: `metrics\.csv`},
		{name: "newline code", value: "import os\nopen('a.csv')"},
		{name: "dev null", value: "/dev/null"},
		{name: "dev null unclean", value: "/dev//null"},
		{name: "dev stderr", value: "/dev/stderr"},
		{name: "descriptor", value: "/dev/fd/3"},
		{name: "proc descriptor", value: "/proc/self/fd/1"},
		{name: "literal redirect word", value: ">"},
		{name: "yaml interpolation", value: "${output_dir}/plot.png", syntax: Interpolated},
		{name: "env reference", value: "$HOME/out.csv", syntax: Interpolated},
		{name: "command substitution", value: "$(pwd)/out.csv", syntax: Interpolated},
		{name: "backquotes", value: "`pwd`/out.csv", syntax: Interpolated},
		{name: "jinja", value: "{{ dir }}/out.csv", syntax: Interpolated},
		{name: "python format", value: "%(dir)s/out.csv", syntax: Interpolated},
		{name: "glob star", value: "out/*.csv", syntax: Interpolated},
		{name: "glob question", value: "out/run?.csv", syntax: Interpolated},
		{name: "glob class", value: "out/run[0-9].csv", syntax: Interpolated},
		{name: "tilde", value: "~/data", syntax: Interpolated},
		{name: "interpolated plain path still accepted", value: "results/a.csv", syntax: Interpolated, want: RuleDirectoryReference},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule, ok := Classify(test.value, test.key, test.syntax)
			if test.want == "" {
				if ok {
					t.Fatalf("Classify(%q, %q) = %s, want skipped", test.value, test.key, rule)
				}
				return
			}
			if !ok || rule != test.want {
				t.Fatalf("Classify(%q, %q) = %q, %v, want %s", test.value, test.key, rule, ok, test.want)
			}
		})
	}
}

func TestIsPathKey(t *testing.T) {
	for key, want := range map[string]bool{
		"path": true, "file": true, "dir": true, "DIR": true,
		"output_dir": true, "data-path": true, "--log-file": true, "a.b.cache_dir": true,
		"output": false, "input": false, "format": false, "dirname": false,
		"filename": false, "profile": false, "": false, "paths": false,
	} {
		if got := IsPathKey(key); got != want {
			t.Errorf("IsPathKey(%q) = %v, want %v", key, got, want)
		}
	}
}
