package artifact

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
)

func values(result Result) []string {
	var found []string
	for _, candidate := range result.Candidates {
		for _, source := range candidate.Sources {
			found = append(found, source.Value)
		}
	}
	return found
}

// TestArgumentCandidates is the interpreter and launcher boundary table:
// each argv gives the literal values ordinary classification accepts. A
// code operand never appears, and arguments outside it stay eligible.
func TestArgumentCandidates(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{name: "script is a candidate", argv: []string{"python", "train.py", "--lr", "0.1"}, want: []string{"train.py"}},
		{name: "bash -c body", argv: []string{"bash", "-c", "python train.py > out/log.txt"}},
		{name: "bash -l -c", argv: []string{"bash", "-l", "-c", "cat a/b.csv"}},
		{name: "bash -o value before -c", argv: []string{"bash", "-o", "pipefail", "-c", "cat a/b.csv"}},
		{name: "bash --rcfile value before -c", argv: []string{"bash", "--rcfile", "rc", "-c", "cat a/b.csv"}},
		{name: "bash -c then -e", argv: []string{"bash", "-c", "-e", "cat a/b.csv"}},
		{name: "bash -ce bundle", argv: []string{"bash", "-ce", "cat a/b.csv"}},
		{name: "bash -lc bundle", argv: []string{"bash", "-lc", "cat a/b.csv"}},
		{name: "bash -xec bundle", argv: []string{"/bin/bash", "-xec", "cat a/b.csv"}},
		{name: "bash -c --", argv: []string{"bash", "-c", "--", "cat a/b.csv"}},
		{name: "bash --login unlisted boolean", argv: []string{"bash", "--login", "-c", "cat a/b.csv"}},
		{name: "bash positional args after code", argv: []string{"bash", "-c", "cat \"$1\"", "name", "data/in.csv"}, want: []string{"data/in.csv"}},
		{name: "bash -O value", argv: []string{"bash", "-O", "extglob", "-c", "cat a/b.csv"}},
		{name: "zsh -c", argv: []string{"zsh", "-o", "x", "-c", "cat a/b.csv"}},
		{name: "upper case shell basename", argv: []string{"/BIN/BASH", "-c", "cat a/b.csv"}},
		{name: "sh -c", argv: []string{"sh", "-c", "cat a/b.csv"}},
		{name: "sh +o value", argv: []string{"dash", "+o", "x", "-c", "cat a/b.csv"}},
		{name: "sh does not take bash -O", argv: []string{"sh", "-O", "a/b.csv", "-c", "x"}, want: []string{"a/b.csv"}},
		{name: "bash script then -c", argv: []string{"bash", "run.sh", "-c", "conf/x.yaml"}, want: []string{"run.sh", "conf/x.yaml"}},
		{name: "bash -c without code", argv: []string{"bash", "-c"}},
		{name: "timeout bash -c", argv: []string{"timeout", "1h", "bash", "-c", "cat a/b.csv"}},
		{name: "timeout options", argv: []string{"timeout", "-s", "TERM", "-k", "5s", "1h", "python", "-c", "open('a/b.csv')"}},
		{name: "env python3.12 -c", argv: []string{"env", "A=1", "python3.12", "-c", "open('out/a.txt')"}},
		{name: "absolute env launcher", argv: []string{"/usr/bin/env", "python3", "train.py"}, want: []string{"train.py"}},
		{name: "env assignment path", argv: []string{"env", "OUT_DIR=results", "python", "train.py"}, want: []string{"results", "train.py"}},
		{name: "python value options", argv: []string{"python", "-W", "ignore", "-X", "dev", "--check-hash-based-pycs", "always", "-c", "open('a/b.csv')"}},
		{name: "python attached values", argv: []string{"python", "-Wignore", "-Xdev", "-c", "open('a/b.csv')"}},
		{name: "python equals value", argv: []string{"python", "--check-hash-based-pycs=always", "-c", "open('a/b.csv')"}},
		{name: "python -u unlisted boolean", argv: []string{"python", "-u", "-c", "open('a/b.csv')"}},
		{name: "python attached code unsupported", argv: []string{"python", "-copen('a/b.csv')"}},
		{name: "python -m stops recognition", argv: []string{"python", "-m", "pkg.train", "--config", "conf/a.yaml"}, want: []string{"conf/a.yaml"}},
		{name: "python -c after script", argv: []string{"python", "train.py", "-c", "config.yaml"}, want: []string{"train.py", "config.yaml"}},
		{name: "python option value named python", argv: []string{"python", "train.py", "--engine", "python", "-c", "config.yaml"}, want: []string{"train.py", "config.yaml"}},
		{name: "pypy3", argv: []string{"pypy3", "-c", "open('a/b.csv')"}},
		{name: "perl value options", argv: []string{"perl", "-I", "./lib", "-M", "Data::Dumper", "-e", "open(F, 'a/b.csv')"}, want: []string{"./lib"}},
		{name: "perl attached values", argv: []string{"perl", "-I./lib", "-MData::Dumper", "-E", "say 'a/b.csv'"}},
		{name: "perl optional attached value", argv: []string{"perl", "-i.bak", "-e", "s/a/b/", "data/x.txt"}, want: []string{"data/x.txt"}},
		{name: "perl bundle is unsupported", argv: []string{"perl", "-lne", "print", "data/x.txt"}, want: []string{"data/x.txt"}},
		{name: "node module options", argv: []string{"node", "--require", "./hook.js", "--import", "./setup.mjs", "-p", "'a/b.csv'"}, want: []string{"./hook.js", "./setup.mjs"}},
		{name: "node equals option", argv: []string{"node", "--experimental-loader=./loader.mjs", "--eval", "'a/b.csv'"}, want: []string{"./loader.mjs"}},
		{name: "node -r attached", argv: []string{"node", "-r./hook.js", "-e", "'a/b.csv'"}},
		{name: "node --inspect unlisted boolean", argv: []string{"node", "--inspect", "-e", "'a/b.csv'"}},
		{name: "node --eval=CODE", argv: []string{"node", "--eval=require('fs').readFileSync('a/b.csv')"}},
		{name: "node --print=CODE", argv: []string{"node", "--print=x('a/b.csv')"}},
		{name: "node unlisted option with value", argv: []string{"node", "--unlisted-option", "VALUE", "--eval", "out/result.csv"}, want: []string{"out/result.csv"}},
		{name: "srun script", argv: []string{"srun", "python", "train.py", "--out", "results/model.pt"}, want: []string{"train.py", "results/model.pt"}},
		{name: "srun option value", argv: []string{"srun", "--ntasks", "2", "python", "-c", "open('a/b.csv')"}},
		{name: "srun protected job name", argv: []string{"srun", "--job-name", "python", "-n", "2", "bash", "-c", "cat a/b.csv"}},
		{name: "srun protected short options", argv: []string{"srun", "-J", "bash", "-p", "python", "sh", "-c", "cat a/b.csv"}},
		{name: "srun equals job name", argv: []string{"srun", "--job-name=python", "bash", "-c", "cat a/b.csv"}},
		{name: "srun stops at first interpreter", argv: []string{"srun", "python", "train.py", "--engine", "bash", "-c", "x.yaml"}, want: []string{"train.py", "x.yaml"}},
		{name: "srun unprotected option false match", argv: []string{"srun", "--comment", "bash", "-c", "out.csv", "python", "train.py"}, want: []string{"train.py"}},
		{name: "nested launchers", argv: []string{"srun", "timeout", "1h", "env", "A=1", "bash", "-c", "cat a/b.csv"}},
		{name: "launcher depth limit", argv: []string{"env", "env", "env", "env", "env", "bash", "-c", "cat a/b.csv"}, want: []string{"cat a/b.csv"}},
		{name: "unsupported launcher", argv: []string{"nice", "bash", "-c", "cat a/b.csv"}, want: []string{"cat a/b.csv"}},
		{name: "cat bash", argv: []string{"cat", "bash"}},
		{name: "echo bash -c", argv: []string{"echo", "bash", "-c", "output.csv"}, want: []string{"output.csv"}},
		{name: "literal redirect is not shell", argv: []string{"python", "train.py", ">", "out.txt"}, want: []string{"train.py", "out.txt"}},
		{name: "script path command word", argv: []string{"./run.sh", "conf/a.yaml"}, want: []string{"./run.sh", "conf/a.yaml"}},
		{name: "equals option", argv: []string{"train", "--config=conf/a.yaml", "--output=png"}, want: []string{"conf/a.yaml"}},
		{name: "path key option value", argv: []string{"train", "--save-dir", "results", "--output", "png"}, want: []string{"results"}},
		{name: "path key option before option", argv: []string{"train", "--save-dir", "--verbose"}},
		{name: "key=value override", argv: []string{"train", "trainer.log_dir=logs", "lr=0.1", "data=imagenet"}, want: []string{"logs"}},
		{name: "hydra prefix override", argv: []string{"train", "+exp.out_dir=runs", "~cache_dir=x"}, want: []string{"runs", "x"}},
		{name: "path containing equals", argv: []string{"cat", "out/a=b.csv"}, want: []string{"out/a=b.csv"}},
		{name: "short option tokens are not values", argv: []string{"tool", "-o", "-"}},
		{name: "duplicate reference", argv: []string{"cp", "a.csv", "./a.csv"}, want: []string{"a.csv", "./a.csv"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := values(FromJob(Job{Command: test.argv}))
			if !slices.Equal(got, test.want) {
				t.Fatalf("FromJob(%q) values = %q, want %q", test.argv, got, test.want)
			}
		})
	}
}

func TestFromJobProvenanceAndResolution(t *testing.T) {
	result := FromJob(Job{
		Command:          []string{"python", "train.py", "--log-dir", "logs", "--config=/etc/a.yaml", "./train.py"},
		Environment:      []string{"OUTPUT_DIR=results", "PATH=/usr/bin:/bin", "PYTHONPATH=src:lib", "LR=0.1", "MODEL_PATH=/m/x.pt"},
		Output:           []string{"out/stdout.log", "/dev/null"},
		Error:            []string{"err.log", "out/stdout.log"},
		WorkingDirectory: "/work/exp/",
	})
	want := []Candidate{
		{Path: "/work/exp/train.py", Basis: BasisWorkingDirectory, Sources: []Source{
			{Kind: KindArgument, Value: "train.py", Rule: RuleExtension, Index: indexOf(1)},
			{Kind: KindArgument, Value: "./train.py", Rule: RuleExplicitPath, Index: indexOf(5)},
		}},
		{Path: "/work/exp/logs", Basis: BasisWorkingDirectory, Sources: []Source{
			{Kind: KindArgument, Value: "logs", Rule: RulePathKey, Index: indexOf(3), Key: "--log-dir"},
		}},
		{Path: "/etc/a.yaml", Basis: BasisAbsolute, Sources: []Source{
			{Kind: KindArgument, Value: "/etc/a.yaml", Rule: RuleExplicitPath, Index: indexOf(4), Key: "--config"},
		}},
		{Path: "/work/exp/results", Basis: BasisWorkingDirectory, Sources: []Source{
			{Kind: KindEnvironment, Value: "results", Rule: RulePathKey, Index: indexOf(0), Key: "OUTPUT_DIR"},
		}},
		{Path: "/m/x.pt", Basis: BasisAbsolute, Sources: []Source{
			{Kind: KindEnvironment, Value: "/m/x.pt", Rule: RuleExplicitPath, Index: indexOf(4), Key: "MODEL_PATH"},
		}},
		{Path: "/work/exp/out/stdout.log", Basis: BasisWorkingDirectory, Sources: []Source{
			{Kind: KindOutput, Value: "out/stdout.log", Rule: RuleLogDestination, Index: indexOf(0), Stream: "stdout"},
			{Kind: KindError, Value: "out/stdout.log", Rule: RuleLogDestination, Index: indexOf(1), Stream: "stderr"},
		}},
		{Path: "/work/exp/err.log", Basis: BasisWorkingDirectory, Sources: []Source{
			{Kind: KindError, Value: "err.log", Rule: RuleLogDestination, Index: indexOf(0), Stream: "stderr"},
		}},
	}
	if !reflect.DeepEqual(result.Candidates, want) {
		t.Fatalf("candidates:\n got %#v\nwant %#v", result.Candidates, want)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestFromJobStderrFollowsOutput(t *testing.T) {
	result := FromJob(Job{Command: []string{"true"}, Output: []string{"run.log"}})
	want := []Candidate{{Path: "run.log", Basis: BasisUnresolved, Sources: []Source{
		{Kind: KindOutput, Value: "run.log", Rule: RuleLogDestination, Index: indexOf(0), Stream: "stdout"},
		{Kind: KindOutput, Value: "run.log", Rule: RuleLogDestination, Index: indexOf(0), Stream: "stderr"},
	}}}
	if !reflect.DeepEqual(result.Candidates, want) {
		t.Fatalf("candidates = %#v, want %#v", result.Candidates, want)
	}
}

func TestFromJobUnresolvedBase(t *testing.T) {
	for _, directory := range []string{"", "sub/dir"} {
		result := FromJob(Job{Command: []string{"cat", "./a.csv", "a.csv", "/abs/b.csv"}, WorkingDirectory: directory})
		got := []string{}
		for _, candidate := range result.Candidates {
			got = append(got, candidate.Basis+":"+candidate.Path)
		}
		want := []string{"unresolved:a.csv", "absolute:/abs/b.csv"}
		if !slices.Equal(got, want) {
			t.Fatalf("WorkingDirectory %q: candidates = %q, want %q", directory, got, want)
		}
	}
}

func TestFromJobCandidateLimit(t *testing.T) {
	command := []string{"cat"}
	for index := 0; index <= MaxCandidates+5; index++ {
		command = append(command, "f/"+strconv.Itoa(index)+".csv")
	}
	command = append(command, "f/0.csv")
	result := FromJob(Job{Command: command})
	if len(result.Candidates) != MaxCandidates {
		t.Fatalf("candidates = %d, want %d", len(result.Candidates), MaxCandidates)
	}
	if len(result.Candidates[0].Sources) != 2 {
		t.Fatalf("an existing candidate still gains sources past the limit: %#v", result.Candidates[0])
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one limit diagnostic", result.Diagnostics)
	}
}
