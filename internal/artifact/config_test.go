package artifact

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func locations(references []ConfigReference) []string {
	var found []string
	for _, reference := range references {
		found = append(found, reference.Location+"="+reference.Value)
	}
	return found
}

func TestConfigReferences(t *testing.T) {
	tests := []struct {
		name   string
		format string
		source string
		want   []string
	}{
		{
			name:   "yaml nested mappings and sequences",
			format: "yaml",
			source: `
train:
  output_dir: results
  data: [data/train.csv, data/valid.csv, 3, true]
  lr: 0.001
  name: baseline
  format: png
  output: png
"odd.key": {log_file: latest}
`,
			want: []string{"train.output_dir=results", "train.data[0]=data/train.csv", "train.data[1]=data/valid.csv", `["odd.key"].log_file=latest`},
		},
		{
			name:   "yaml sequence items keep the sequence key",
			format: "yaml",
			source: "cache_dirs: [a]\ncache_dir: [b, c]\n",
			want:   []string{"cache_dir[0]=b", "cache_dir[1]=c"},
		},
		{
			name:   "yaml interpolation and separate keys are not combined",
			format: "yaml",
			source: "output_dir: out\nfilename: plot.png\nplot: ${output_dir}/plot.png\nenv: ${oc.env:HOME}/x.csv\nglob: out/*.csv\nhome: ~/x.csv\n",
			want:   []string{"output_dir=out", "filename=plot.png"},
		},
		{
			name:   "yaml custom tags are not evaluated",
			format: "yaml",
			source: "a: !include other.yaml\nb: !!python/name:os.system run.sh\nc: !!str kept.csv\nd: !!binary aGVsbG8=\n",
			want:   []string{"c=kept.csv"},
		},
		{
			name:   "yaml aliases are not followed",
			format: "yaml",
			source: "base: &base {log_dir: logs}\nrun: *base\nmerged: {<<: *base, x: y}\n",
			want:   []string{"base.log_dir=logs"},
		},
		{
			name:   "yaml multiple documents",
			format: "yaml",
			source: "a: one.csv\n---\nb: two.csv\n",
			want:   []string{"a=one.csv", "$1.b=two.csv"},
		},
		{
			name:   "yaml non-string keys are skipped",
			format: "yaml",
			source: "1: a.csv\nnull: b.csv\nok: c.csv\n",
			want:   []string{"ok=c.csv"},
		},
		{
			name:   "yaml empty",
			format: "yaml",
			source: "",
		},
		{
			name:   "json sorted keys",
			format: "json",
			source: `{"z": "z.csv", "a": {"save_dir": "ckpt", "n": 1, "list": ["x/y", null]}, "url": "https://e.org/a.png"}`,
			want:   []string{"a.list[0]=x/y", "a.save_dir=ckpt", "z=z.csv"},
		},
		{
			name:   "json top-level string",
			format: "json",
			source: `"results/a.csv"`,
			want:   []string{"=results/a.csv"},
		},
		{
			name:   "toml tables and arrays of tables",
			format: "toml",
			source: "log_dir = \"logs\"\n[paths]\nmodel_path = \"m.pt\"\n[[runs]]\nout = \"runs/1\"\n[[runs]]\nout = \"runs/2\"\nwhen = 1979-05-27\n",
			want:   []string{"log_dir=logs", "paths.model_path=m.pt", "runs[0].out=runs/1", "runs[1].out=runs/2"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			references, err := ConfigReferences(test.format, []byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			if got := locations(references); !slices.Equal(got, test.want) {
				t.Fatalf("references = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConfigReferencesKeepKeyAndRule(t *testing.T) {
	references, err := ConfigReferences("yaml", []byte("trainer:\n  save_dir: ckpt\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []ConfigReference{{Value: "ckpt", Rule: RulePathKey, Key: "save_dir", Location: "trainer.save_dir"}}
	if !reflect.DeepEqual(references, want) {
		t.Fatalf("references = %#v, want %#v", references, want)
	}
}

func TestConfigReferencesMalformed(t *testing.T) {
	for format, source := range map[string]string{
		"yaml": "a: [unclosed\n",
		"json": `{"a": "secret-value.csv"`,
		"toml": "a = \n",
	} {
		_, err := ConfigReferences(format, []byte(source))
		if err == nil {
			t.Errorf("%s: no error for malformed source", format)
			continue
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: error exposes source contents: %v", format, err)
		}
	}
	if _, err := ConfigReferences("json", []byte(`{"a": 1} {"b": 2}`)); err == nil {
		t.Error("json: trailing value accepted")
	}
	if _, err := ConfigReferences("ini", nil); err == nil {
		t.Error("unsupported format accepted")
	}
}

func TestConfigReferencesLimits(t *testing.T) {
	deep := strings.Repeat("[", MaxSourceDepth+2) + strings.Repeat("]", MaxSourceDepth+2)
	if _, err := ConfigReferences("json", []byte(deep)); !errors.Is(err, errSourceLimit) {
		t.Fatalf("deep json: err = %v, want inspection limit", err)
	}
	deepYAML := strings.Repeat("- ", MaxSourceDepth+2) + "a.csv\n"
	if _, err := ConfigReferences("yaml", []byte(deepYAML)); !errors.Is(err, errSourceLimit) {
		t.Fatalf("deep yaml: err = %v, want inspection limit", err)
	}
	if _, err := ConfigReferences("yaml", make([]byte, MaxSourceBytes+1)); err == nil {
		t.Fatal("oversized source accepted")
	}
	many := "[" + strings.Repeat("a/b,", MaxSourceValues) + "b/c]"
	if len(many) > MaxSourceBytes {
		t.Fatalf("value-limit fixture is %d bytes, over the byte limit", len(many))
	}
	references, err := ConfigReferences("yaml", []byte(many))
	if !errors.Is(err, errSourceLimit) {
		t.Fatalf("many values: err = %v, want inspection limit", err)
	}
	if len(references) != MaxSourceValues {
		t.Fatalf("references before the limit = %d, want %d", len(references), MaxSourceValues)
	}
}

func TestDiscoverInspectsReferencedConfigs(t *testing.T) {
	sources := map[string]string{
		"/work/conf/train.yaml": "output_dir: results\nmodel: ckpt/m.pt\nnested: other.yaml\n",
		"/etc/env.json":         `{"log_file": "/var/log/a.txt"}`,
		"/work/other.yaml":      "never: read.csv\n",
		"/work/out.toml":        "x = \"y/z\"\n",
	}
	var read []string
	reader := func(path string) ([]byte, error) {
		read = append(read, path)
		source, ok := sources[path]
		if !ok {
			return nil, errors.New("no such file")
		}
		return []byte(source), nil
	}
	result := Discover(Job{
		Command:          []string{"python", "train.py", "--config", "conf/train.yaml", "missing.json"},
		Environment:      []string{"SETTINGS=/etc/env.json"},
		Output:           []string{"out.toml"},
		WorkingDirectory: "/work",
	}, Sources{Config: ParseSources(reader)})
	if want := []string{"/work/conf/train.yaml", "/work/missing.json", "/etc/env.json"}; !slices.Equal(read, want) {
		t.Fatalf("read = %q, want %q (log destinations and nested configs are not inspected)", read, want)
	}
	var got []string
	for _, candidate := range result.Candidates {
		source := candidate.Sources[len(candidate.Sources)-1]
		if source.Kind == KindConfig {
			got = append(got, candidate.Path+" <- "+source.File+":"+source.Location)
		}
	}
	want := []string{
		"/work/results <- /work/conf/train.yaml:output_dir",
		"/work/ckpt/m.pt <- /work/conf/train.yaml:model",
		"/work/other.yaml <- /work/conf/train.yaml:nested",
		"/var/log/a.txt <- /etc/env.json:log_file",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("config candidates = %q, want %q", got, want)
	}
	wantDiagnostics := []Diagnostic{{Source: "/work/missing.json", Message: "not inspected: no such file"}}
	if !reflect.DeepEqual(result.Diagnostics, wantDiagnostics) {
		t.Fatalf("diagnostics = %#v, want %#v", result.Diagnostics, wantDiagnostics)
	}
}

func TestDiscoverSkipsUnresolvedConfigs(t *testing.T) {
	called := false
	Discover(Job{Command: []string{"train", "conf/a.yaml"}}, Sources{Config: func(string) ([]ConfigReference, error) {
		called = true
		return nil, nil
	}})
	if called {
		t.Fatal("a configuration file without a known base was read")
	}
}

func TestDiscoverReadsConfigsNamedInShellSource(t *testing.T) {
	reader := func(path string) ([]byte, error) {
		if path == "/work/conf/a.yaml" {
			return []byte("out_dir: results\n"), nil
		}
		return nil, errors.New("no such file")
	}
	result := Discover(Job{Command: []string{"bash", "-c", "python train.py --config conf/a.yaml"}, WorkingDirectory: "/work"}, Sources{Config: ParseSources(reader)})
	var got []string
	for _, candidate := range result.Candidates {
		got = append(got, candidate.Path)
	}
	if want := []string{"/work/train.py", "/work/conf/a.yaml", "/work/results"}; !slices.Equal(got, want) {
		t.Fatalf("candidates = %q, want %q", got, want)
	}
}
