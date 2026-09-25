package workflow

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestDecodeFormatsCompileEquivalentQueues(t *testing.T) {
	inputs := map[string]string{
		"yaml": "version: 1\njobs:\n  - name: train\n    command: [echo, hello]\n    environment: [BASE=yes]\n    array: \"1,3\"\n    matrix: [\"SEED=1,2\"]\n",
		"json": `{"version":1,"jobs":[{"name":"train","command":["echo","hello"],"environment":["BASE=yes"],"array":"1,3","matrix":["SEED=1,2"]}]}`,
		"toml": "version = 1\n[[jobs]]\nname = \"train\"\ncommand = [\"echo\", \"hello\"]\nenvironment = [\"BASE=yes\"]\narray = \"1,3\"\nmatrix = [\"SEED=1,2\"]\n",
	}
	var want *model.Queue
	for format, input := range inputs {
		queue := compileFormatFixture(t, format, input)
		if want == nil {
			want = &queue
		} else if !reflect.DeepEqual(queue, *want) {
			t.Fatalf("Compile(%s) = %#v, want %#v", format, queue, *want)
		}
		if len(queue.Commands) != 2 || queue.Commands[0].Name != "train-SEED1" || queue.Commands[1].Name != "train-SEED2" {
			t.Fatalf("Compile(%s) commands = %#v", format, queue.Commands)
		}
		if queue.Commands[0].Matrix == nil || queue.Commands[0].Matrix.GroupID != queue.Commands[1].Matrix.GroupID {
			t.Fatalf("Compile(%s) matrix provenance = %#v", format, queue.Commands)
		}
		if queue.Commands[0].Array == nil || !reflect.DeepEqual(queue.Commands[0].Array.Tasks, []int{1, 3}) {
			t.Fatalf("Compile(%s) array = %#v", format, queue.Commands[0].Array)
		}
	}
}

func compileFormatFixture(t *testing.T, format, input string) model.Queue {
	t.Helper()
	manifest, err := Decode(strings.NewReader(input), format)
	if err != nil {
		t.Fatalf("Decode(%s): %v", format, err)
	}
	next := 0
	queue, err := Compile(manifest, func() string {
		next++
		return fmt.Sprintf("job-%d", next)
	})
	if err != nil {
		t.Fatalf("Compile(%s): %v", format, err)
	}
	return queue
}

func TestCompileExpandsDependencyOnMatrixName(t *testing.T) {
	manifest := Manifest{Version: 1, Jobs: []Job{
		{Name: "train", Command: []string{"train"}, Matrix: []string{"SEED=1,2"}},
		{Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}},
	}}
	next := 0
	queue, err := Compile(manifest, func() string {
		next++
		return fmt.Sprintf("job-%d", next)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 3 {
		t.Fatalf("commands = %d, want 3", len(queue.Commands))
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	inputs := map[string]string{
		"yaml": "version: 1\nunknown: true\njobs:\n  - command: [true]\n",
		"json": `{"version":1,"unknown":true,"jobs":[{"command":["true"]}]}`,
		"toml": "version = 1\nunknown = true\n[[jobs]]\ncommand = [\"true\"]\n",
	}
	for format, input := range inputs {
		if _, err := Decode(strings.NewReader(input), format); err == nil {
			t.Errorf("Decode(%s) accepted an unknown field", format)
		}
	}
}

func TestDecodeRejectsYAMLAliasesAndMergeKeys(t *testing.T) {
	for _, input := range []string{
		"version: 1\njobs:\n  - &job\n    command: [true]\n  - *job\n",
		"version: 1\nbase: &base\n  command: [true]\njobs:\n  - <<: *base\n",
	} {
		if _, err := Decode(strings.NewReader(input), "yaml"); err == nil {
			t.Fatal("Decode accepted a YAML alias or merge key")
		}
	}
}

func TestCompileRejectsInvalidManifestValues(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
	}{
		{name: "version", manifest: Manifest{Version: 2, Jobs: []Job{{Command: []string{"true"}}}}},
		{name: "empty jobs", manifest: Manifest{Version: 1}},
		{name: "empty command", manifest: Manifest{Version: 1, Jobs: []Job{{Name: "job"}}}},
		{name: "duplicate names", manifest: Manifest{Version: 1, Jobs: []Job{{Name: "job", Command: []string{"true"}}, {Name: "job", Command: []string{"true"}}}}},
		{name: "array", manifest: Manifest{Version: 1, Jobs: []Job{{Command: []string{"true"}, Array: "3-1"}}}},
		{name: "matrix", manifest: Manifest{Version: 1, Jobs: []Job{{Command: []string{"true"}, Matrix: []string{"SEED=1", "SEED=2"}}}}},
		{name: "environment", manifest: Manifest{Version: 1, Jobs: []Job{{Command: []string{"true"}, Environment: []string{"INVALID"}}}}},
		{name: "status", manifest: Manifest{Version: 1, Jobs: []Job{{Command: []string{"true"}, Status: "running"}}}},
		{name: "status without source", manifest: Manifest{Version: 1, Jobs: []Job{{Command: []string{"true"}, Status: "success"}}}},
		{name: "instances on scalar", manifest: Manifest{Version: 1, Source: &Source{Project: "demo", RunIDs: []string{"run"}}, Jobs: []Job{{Command: []string{"true"}, Instances: []Instance{{Status: "failed"}}}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Compile(test.manifest, func() string { return "job" }); err == nil {
				t.Fatal("Compile accepted invalid manifest")
			}
		})
	}
}
