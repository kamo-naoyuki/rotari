package workflow

import (
	"errors"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestDecodeReportsReadAndSyntaxErrors(t *testing.T) {
	if _, err := Decode(failingReader{}, "yaml"); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("Decode read error = %v", err)
	}
	tests := []struct {
		format string
		input  string
	}{
		{format: "yaml", input: "version: [\n"},
		{format: "yaml", input: "version: 1\nversion: 2\njobs:\n  - command: [true]\n"},
		{format: "yaml", input: "version: 1\njobs:\n  - command: [true]\n---\nversion: [\n"},
		{format: "toml", input: "version = \n"},
		{format: "json", input: `{"version":1,`},
		{format: "json", input: `{"version":1,"jobs":[{"command":["true"]}]} garbage`},
	}
	for _, test := range tests {
		if _, err := Decode(strings.NewReader(test.input), test.format); err == nil {
			t.Errorf("Decode(%s) accepted %q", test.format, test.input)
		}
	}
}

func TestDecodeAcceptsYMLAndCaseInsensitiveFormat(t *testing.T) {
	for _, format := range []string{"yml", "YAML"} {
		if _, err := Decode(strings.NewReader("version: 1\njobs:\n  - command: [true]\n"), format); err != nil {
			t.Fatalf("Decode(%s): %v", format, err)
		}
	}
}

func TestFromRunRejectsEmptyQueue(t *testing.T) {
	if _, err := FromRun(model.Queue{}, model.RunSummary{}, Source{Project: "demo", RunIDs: []string{"run"}}); err == nil {
		t.Fatal("FromRun accepted an empty queue")
	}
}

func TestFromRunMarksSuccessfulArrayWithoutInstances(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "array", Command: []string{"work"}, Array: &model.ArraySpec{First: 2, Last: 2}}}}
	manifest, err := FromRun(queue, model.RunSummary{Results: []model.JobResult{{ID: "array-2", AttemptID: "att", ExitCode: 0}}}, Source{Project: "demo", RunIDs: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	job := manifest.Jobs[0]
	if job.Array != "2" || job.Status != "success" || job.AttemptID != "att" || len(job.Instances) != 0 {
		t.Fatalf("job = %#v", job)
	}
}
