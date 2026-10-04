package artifact

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// scriptSources reads scripts and configuration files from a map of
// absolute paths and records which scripts were read.
func scriptSources(files map[string]string, read *[]string) Sources {
	source := func(path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, errors.New("no such file")
		}
		return []byte(content), nil
	}
	return Sources{
		Script: func(path string) ([]byte, error) {
			*read = append(*read, path)
			return source(path)
		},
		Config: ParseSources(source),
	}
}

func relativeCandidates(result Result) []string {
	var paths []string
	for _, candidate := range result.Candidates {
		paths = append(paths, strings.TrimPrefix(candidate.Path, "/work/"))
	}
	return paths
}

func TestScriptInspection(t *testing.T) {
	files := map[string]string{
		"/work/scripts/run.sh":   "python train.py --config conf/a.yaml > logs/train.log\n",
		"/work/scripts/job":      "cat data/in.csv\n",
		"/work/job.sh":           "#!/bin/sh\ncat data/job.csv\n",
		"/work/tool.sh":          "#!/usr/bin/env python3\nopen('data/py.csv')\n",
		"/work/env.sh":           "#!/usr/bin/env -S bash -e\ncat data/env.csv\n",
		"/work/outer.sh":         "bash inner.sh\ncat data/outer.csv\n",
		"/work/inner.sh":         "cat data/inner.csv\n",
		"/work/a.sh":             "bash b.sh\ncat data/a.csv\n",
		"/work/b.sh":             "bash a.sh\ncat data/b.csv\n",
		"/work/s1.sh":            "bash s2.sh\n",
		"/work/s2.sh":            "bash s3.sh\n",
		"/work/s3.sh":            "bash s4.sh\n",
		"/work/s4.sh":            "cat data/s4.csv\n",
		"/work/e1.sh":            `true > "res/$LR.csv"` + "\n",
		"/work/conf/a.yaml":      "out_dir: results\n",
		"/work/broken.sh":        "if then fi (\n",
		"/work/sub/relative.sh":  "cat data/sub.csv\n",
		"/work/scripts/other.sh": "cat data/other.csv\n",
	}
	tests := []struct {
		name        string
		argv        []string
		env         []string
		want        []string
		read        []string
		diagnostics []string
	}{
		{name: "script operand of a shell", argv: []string{"bash", "scripts/run.sh"},
			want: []string{"scripts/run.sh", "train.py", "conf/a.yaml", "logs/train.log", "results"}, read: []string{"/work/scripts/run.sh", "/work/train.py"},
			diagnostics: []string{"/work/train.py: not inspected: no such file"}},
		{name: "script without extension run by sh", argv: []string{"sh", "scripts/job"},
			want: []string{"scripts/job", "data/in.csv"}, read: []string{"/work/scripts/job"}},
		{name: ".sh candidate behind an unparsed launcher", argv: []string{"srun", "./job.sh"},
			want: []string{"job.sh", "data/job.csv"}, read: []string{"/work/job.sh"}},
		{name: "non-shell shebang", argv: []string{"./tool.sh"},
			want: []string{"tool.sh"}, read: []string{"/work/tool.sh"}, diagnostics: []string{"/work/tool.sh: not inspected: not a shell script"}},
		{name: "env shebang with options", argv: []string{"./env.sh"},
			want: []string{"env.sh", "data/env.csv"}, read: []string{"/work/env.sh"}},
		{name: "script run by a script", argv: []string{"bash", "outer.sh"},
			want: []string{"outer.sh", "inner.sh", "data/outer.csv", "data/inner.csv"}, read: []string{"/work/outer.sh", "/work/inner.sh"}},
		{name: "scripts running each other are read once", argv: []string{"bash", "a.sh"},
			want: []string{"a.sh", "b.sh", "data/a.csv", "data/b.csv"}, read: []string{"/work/a.sh", "/work/b.sh"}},
		{name: "nesting limit", argv: []string{"bash", "s1.sh"},
			want: []string{"s1.sh", "s2.sh", "s3.sh", "s4.sh"}, read: []string{"/work/s1.sh", "/work/s2.sh", "/work/s3.sh"},
			diagnostics: []string{"/work/s4.sh: not inspected: shell source nested deeper than 3"}},
		{name: "missing script", argv: []string{"bash", "missing.sh"},
			want: []string{"missing.sh"}, read: []string{"/work/missing.sh"}, diagnostics: []string{"/work/missing.sh: not inspected: no such file"}},
		{name: "unparsable script", argv: []string{"bash", "broken.sh"},
			want: []string{"broken.sh"}, read: []string{"/work/broken.sh"}, diagnostics: []string{"/work/broken.sh: not inspected: cannot parse shell source"}},
		{name: "job variables expand in a script", argv: []string{"bash", "e1.sh"}, env: []string{"LR=0.1"},
			want: []string{"e1.sh", "res/0.1.csv"}, read: []string{"/work/e1.sh"}},
		{name: "script operand after cd is unknown", argv: []string{"bash", "-c", "cd sub; bash relative.sh; bash /work/scripts/other.sh"},
			want: []string{"scripts/other.sh", "data/other.csv"}, read: []string{"/work/scripts/other.sh"}},
		{name: "script in shell code", argv: []string{"bash", "-c", "bash scripts/job"},
			want: []string{"scripts/job", "data/in.csv"}, read: []string{"/work/scripts/job"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var read []string
			result := Discover(Job{Command: test.argv, Environment: test.env, WorkingDirectory: "/work"}, scriptSources(files, &read))
			if got := relativeCandidates(result); !slices.Equal(got, test.want) {
				t.Fatalf("candidates = %q, want %q", got, test.want)
			}
			if !slices.Equal(read, test.read) {
				t.Fatalf("scripts read = %q, want %q", read, test.read)
			}
			var diagnostics []string
			for _, diagnostic := range result.Diagnostics {
				diagnostics = append(diagnostics, diagnostic.Source+": "+diagnostic.Message)
			}
			if !slices.Equal(diagnostics, test.diagnostics) {
				t.Fatalf("diagnostics = %q, want %q", diagnostics, test.diagnostics)
			}
		})
	}
}

func TestScriptProvenance(t *testing.T) {
	var read []string
	result := Discover(Job{Command: []string{"bash", "run.sh"}, WorkingDirectory: "/work"},
		scriptSources(map[string]string{"/work/run.sh": "true\ncat a/b.csv > out.txt\n"}, &read))
	want := []Source{
		{Kind: KindShell, Value: "a/b.csv", Rule: RuleDirectoryReference, File: "/work/run.sh", Location: "2:5"},
		{Kind: KindShell, Value: "out.txt", Rule: RuleRedirection, File: "/work/run.sh", Location: "2:15", Direction: ">"},
	}
	var got []Source
	for _, candidate := range result.Candidates[1:] {
		got = append(got, candidate.Sources...)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sources = %#v, want %#v", got, want)
	}
}

func TestScriptsNeedAKnownBase(t *testing.T) {
	var read []string
	Discover(Job{Command: []string{"bash", "run.sh", "x.sh"}}, scriptSources(map[string]string{}, &read))
	if len(read) != 0 {
		t.Fatalf("scripts without a known base were read: %q", read)
	}
}
