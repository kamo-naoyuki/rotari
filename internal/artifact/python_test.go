package artifact

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// pythonFindings lists the python sources of a result as key=value rule.
func pythonFindings(result Result) []string {
	var found []string
	for _, candidate := range result.Candidates {
		for _, source := range candidate.Sources {
			if source.Kind == KindPython {
				found = append(found, source.Key+"="+source.Value+" "+string(source.Rule))
			}
		}
	}
	return found
}

func TestPythonInspection(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{name: "argparse defaults take the option name", source: `
import argparse
parser = argparse.ArgumentParser()
parser.add_argument("--output-dir", type=str, default="results")
parser.add_argument("-c", "--config", default="conf/train.yaml", help="config file")
parser.add_argument("--lr", type=float, default=0.1)
parser.add_argument("--format", default="png")
parser.add_argument("-o", dest="out_dir", default="runs")
parser.add_argument("--name", default="baseline")
parser.add_argument("--choices", choices=["a/b", "c"], default=("x", "y"))
`, want: []string{"--output-dir=results PATH-R5", "--config=conf/train.yaml PATH-R3", "out_dir=runs PATH-R5", "=a/b PATH-R3"}},
		{name: "string literals with key context", source: `
SAVE_DIR = "checkpoints"
model.save("model.pt")
train(log_dir="logs", lr=0.1, name="baseline")
config = {"output_path": "out", "seed": 1, "plot": "plot.png"}
open('data/train.csv')
`, want: []string{"SAVE_DIR=checkpoints PATH-R5", "=model.pt PATH-R4", "log_dir=logs PATH-R5", "output_path=out PATH-R5", "plot=plot.png PATH-R4", "=data/train.csv PATH-R3"}},
		{name: "dynamic strings are skipped", source: `
open(f"results/{epoch}.pt")
open("results/{}.pt".format(epoch))
open("results/%d.pt" % epoch)
open("results/%(epoch)s.pt" % values)
glob.glob("data/*.csv")
os.path.expanduser("~/data")
open(b"data/raw.bin")
url = "https://example.org/model.pt"
`},
		{name: "comments and docstrings", source: `
# open("commented/out.csv")
def main():
    """Train the model and write results/model.pt.

    Long description.
    """
    'one line docstring with a/b.csv'
`, want: []string{"=one line docstring with a/b.csv PATH-R3"}},
		{name: "string forms", source: `
a = r"raw\path/x.csv"
b = '''triple
quoted'''
c = "esc\"aped/q.csv"
d = u"unicode/u.csv"
e = R'upper/raw.csv'
`, want: []string{`a=raw\path/x.csv PATH-R3`, `c=esc"aped/q.csv PATH-R3`, "d=unicode/u.csv PATH-R3", "e=upper/raw.csv PATH-R3"}},
		{name: "comparison is not assignment", source: `
if mode == "eval/x.csv": pass
out_dir: str = "typed"
`, want: []string{"=eval/x.csv PATH-R3"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var read []string
			result := Discover(Job{Command: []string{"python", "train.py"}, WorkingDirectory: "/work"},
				scriptSources(map[string]string{"/work/train.py": test.source}, &read))
			if got := pythonFindings(result); !slices.Equal(got, test.want) {
				t.Fatalf("python findings = %q, want %q", got, test.want)
			}
			for _, diagnostic := range result.Diagnostics {
				// A configuration file the code names is read, and this test
				// provides none; train.py itself must parse.
				if diagnostic.Source == "/work/train.py" {
					t.Fatalf("diagnostics = %+v", result.Diagnostics)
				}
			}
		})
	}
}

func TestPythonFilesAndProvenance(t *testing.T) {
	files := map[string]string{
		"/work/train.py":    "\nsave('a/b.csv')\nyaml.safe_load(open('conf/a.yaml'))\n",
		"/work/conf/a.yaml": "out_dir: results\n",
		"/work/run.sh":      "python train.py\n",
		"/work/bad.py":      "x = 'unterminated\n",
	}
	var read []string
	result := Discover(Job{Command: []string{"python", "train.py"}, WorkingDirectory: "/work"}, scriptSources(files, &read))
	want := Source{Kind: KindPython, Value: "a/b.csv", Rule: RuleDirectoryReference, File: "/work/train.py", Location: "2:6"}
	if got := result.Candidates[1].Sources[0]; !reflect.DeepEqual(got, want) {
		t.Fatalf("source = %#v, want %#v", got, want)
	}
	if got, want := relativeCandidates(result), []string{"train.py", "a/b.csv", "conf/a.yaml", "results"}; !slices.Equal(got, want) {
		t.Fatalf("candidates = %q, want %q (a configuration file named in Python is read)", got, want)
	}

	read = nil
	viaShell := Discover(Job{Command: []string{"bash", "run.sh"}, WorkingDirectory: "/work"}, scriptSources(files, &read))
	if got, want := relativeCandidates(viaShell), []string{"run.sh", "train.py", "a/b.csv", "conf/a.yaml", "results"}; !slices.Equal(got, want) {
		t.Fatalf("candidates through a shell script = %q, want %q", got, want)
	}

	read = nil
	module := Discover(Job{Command: []string{"python", "-m", "train"}, WorkingDirectory: "/work"}, scriptSources(files, &read))
	if len(read) != 0 || len(module.Candidates) != 0 {
		t.Fatalf("python -m read %q and found %+v", read, module.Candidates)
	}

	bad := Discover(Job{Command: []string{"python", "bad.py"}, WorkingDirectory: "/work"}, scriptSources(files, &read))
	if len(bad.Diagnostics) != 1 || !strings.Contains(bad.Diagnostics[0].Message, "unterminated string") {
		t.Fatalf("bad.py diagnostics = %+v", bad.Diagnostics)
	}
}
