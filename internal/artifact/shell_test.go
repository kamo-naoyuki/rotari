package artifact

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// shellFindings lists the shell sources of a result as value, rule, and
// redirection operator, with "+E1" when PATH-E1 expanded the value.
func shellFindings(result Result) []string {
	var found []string
	for _, candidate := range result.Candidates {
		for _, source := range candidate.Sources {
			if source.Kind != KindShell {
				continue
			}
			finding := source.Value + " " + string(source.Rule)
			if source.Direction != "" {
				finding += " " + source.Direction
			}
			if source.Expanded {
				finding += " +E1"
			}
			found = append(found, finding)
		}
	}
	return found
}

func TestShellInspection(t *testing.T) {
	variables := map[string]string{"ROTARI_ARRAY_TASK_ID": "3", "ROTARI_JOB_DIR": "/runs/r/j/attempts/a"}
	tests := []struct {
		name string
		argv []string
		env  []string
		want []string
	}{
		{name: "redirections", argv: []string{"bash", "-c", "a > results; b >> log; c < input; d 2> errors; e <> rw; f >| clob"},
			want: []string{"results PATH-R1 >", "log PATH-R1 >>", "input PATH-R1 <", "errors PATH-R1 >", "rw PATH-R1 <>", "clob PATH-R1 >|"}},
		{name: "bash all-output redirections", argv: []string{"bash", "-c", "a &> all; b &>> app"},
			want: []string{"all PATH-R1 &>", "app PATH-R1 &>>"}},
		{name: "quoted space and numeric target", argv: []string{"sh", "-c", `a > "result table"; b > 123`},
			want: []string{"result table PATH-R1 >", "123 PATH-R1 >"}},
		{name: "descriptor duplication and closing", argv: []string{"bash", "-c", "a 2>&1; b <&0; c >&-"}},
		{name: "here-document and here-string data", argv: []string{"bash", "-c", "cat <<EOF\nresults.csv\nEOF\ncat <<< \"a/b.csv\""}},
		{name: "unresolved expansion targets", argv: []string{"bash", "-c", `a > "$OUTPUT"; b > "$(choose_path)"; c > $(date).log`}},
		{name: "process substitution", argv: []string{"bash", "-c", "a > >(consumer)"}},
		{name: "special sink", argv: []string{"bash", "-c", "a > /dev/null 2>/dev/stderr"}},
		{name: "command arguments", argv: []string{"bash", "-c", "python train.py --out results/x --lr 0.1"},
			want: []string{"train.py PATH-R4", "results/x PATH-R3"}},
		{name: "echo arguments are text", argv: []string{"bash", "-c", "echo a/b.csv > log.txt"},
			want: []string{"log.txt PATH-R1 >"}},
		{name: "python code inside shell", argv: []string{"bash", "-c", `python -c "open('a/b.csv')" > out.txt`},
			want: []string{"out.txt PATH-R1 >"}},
		{name: "nested shell", argv: []string{"bash", "-c", `bash -c 'cat a/b.csv'`},
			want: []string{"a/b.csv PATH-R3"}},
		{name: "shell heredoc with quoted delimiter", argv: []string{"bash", "-c", "bash <<'SH'\npython train.py > result.csv\nSH"},
			want: []string{"train.py PATH-R4", "result.csv PATH-R1 >"}},
		{name: "double-quoted and escaped delimiters", argv: []string{"bash", "-c", "sh <<\"SH\"\ncat a/1.csv\nSH\nsh <<\\SH\ncat a/2.csv\nSH"},
			want: []string{"a/1.csv PATH-R3", "a/2.csv PATH-R3"}},
		{name: "shell heredoc with unquoted delimiter", argv: []string{"bash", "-c", "bash <<SH\ncat a/b.csv\nSH"}},
		{name: "python heredoc", argv: []string{"bash", "-c", "python <<'PY'\nopen('result.csv')\nPY"}},
		{name: "heredoc to a shell with -c", argv: []string{"bash", "-c", "bash -c 'true' <<'SH'\ncat a/b.csv\nSH"}},
		{name: "cd makes later relative references unknown", argv: []string{"bash", "-c", "cat a.csv; cd sub; cat b.csv > /abs/out.txt c.txt"},
			want: []string{"a.csv PATH-R4", "/abs/out.txt PATH-R1 >"}},
		{name: "glob brace and tilde", argv: []string{"bash", "-c", "cat *.csv a/[ab].csv ~/x.csv a/{x,y}.csv a/{1..3}.csv"}},
		{name: "escaped glob is literal", argv: []string{"bash", "-c", `cat a/\*.csv`},
			want: []string{"a/*.csv PATH-R3"}},
		{name: "prefix assignment", argv: []string{"bash", "-c", "OUT_DIR=results python t.py"},
			want: []string{"results PATH-R5", "t.py PATH-R4"}},
		{name: "array task variable", argv: []string{"bash", "-c", "python t.py > out/$ROTARI_ARRAY_TASK_ID.log"},
			want: []string{"t.py PATH-R4", "out/3.log PATH-R1 > +E1"}},
		{name: "braced job directory variable", argv: []string{"bash", "-c", `cp model.pt "${ROTARI_JOB_DIR}/model.pt"`},
			want: []string{"model.pt PATH-R4", "/runs/r/j/attempts/a/model.pt PATH-R2 +E1"}},
		{name: "job environment variable", argv: []string{"bash", "-c", `train --out "results/$LR"`}, env: []string{"LR=0.1"},
			want: []string{"results/0.1 PATH-R3 +E1"}},
		{name: "unexpanded variable forms", argv: []string{"bash", "-c", `a > "${LR:-x}.csv"; b > "$HOME/x.csv"; c > "${#LR}/x"; d > "$1/x"`}, env: []string{"LR=0.1"}},
		{name: "variable the source assigns", argv: []string{"bash", "-c", `LR=5; a > "out/$LR.csv"; for T in 1 2; do b > "out/$T.csv"; done`}, env: []string{"LR=0.1"}},
		{name: "unquoted value with spaces", argv: []string{"bash", "-c", "a > out/$NAME"}, env: []string{"NAME=a b"}},
		{name: "zsh source", argv: []string{"zsh", "-c", "cat a/b.csv > out.txt"},
			want: []string{"a/b.csv PATH-R3", "out.txt PATH-R1 >"}},
		{name: "launcher before the shell", argv: []string{"timeout", "1h", "bash", "-lc", "cat a/b.csv"},
			want: []string{"a/b.csv PATH-R3"}},
		{name: "shell script operand is not code", argv: []string{"bash", "run.sh", "a/b.csv"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := shellFindings(Discover(Job{Command: test.argv, Environment: test.env, Variables: variables}, Sources{}))
			if !slices.Equal(got, test.want) {
				t.Fatalf("shell findings = %q, want %q", got, test.want)
			}
		})
	}
}

func TestShellProvenanceAndDiagnostics(t *testing.T) {
	result := Discover(Job{Command: []string{"bash", "-c", "true\nbash -c 'cat > out.txt'"}, WorkingDirectory: "/work"}, Sources{})
	want := []Candidate{{Path: "/work/out.txt", Basis: BasisWorkingDirectory, Sources: []Source{
		{Kind: KindShell, Value: "out.txt", Rule: RuleRedirection, Index: indexOf(2), Location: "2:9/1:7", Direction: ">"},
	}}}
	if !reflect.DeepEqual(result.Candidates, want) {
		t.Fatalf("candidates = %#v, want %#v", result.Candidates, want)
	}

	broken := Discover(Job{Command: []string{"bash", "-c", "if then fi ("}}, Sources{})
	if len(broken.Candidates) != 0 || len(broken.Diagnostics) != 1 || broken.Diagnostics[0].Source != "argument 2" ||
		!strings.Contains(broken.Diagnostics[0].Message, "cannot parse shell source") {
		t.Fatalf("broken shell source = %+v", broken)
	}

	deep := Discover(Job{Command: []string{"bash", "-c", `bash -c "bash -c 'bash -c \"cat a/b.csv\"'"`}}, Sources{})
	if len(deep.Candidates) != 0 || len(deep.Diagnostics) != 1 || !strings.Contains(deep.Diagnostics[0].Message, "nested deeper than 3") {
		t.Fatalf("deeply nested shell source = %+v", deep)
	}
}
