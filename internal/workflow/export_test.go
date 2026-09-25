package workflow

import (
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestFromQueueRoundTripsArray(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{{ID: "array", Name: "work", Command: []string{"run"}, Environment: []string{"A=1", "A=2"}, Array: &model.ArraySpec{First: 1, Last: 4, Tasks: []int{1, 3, 4}}}}}
	manifest, err := FromQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Jobs) != 1 || manifest.Jobs[0].Array != "1,3,4" || !reflect.DeepEqual(manifest.Jobs[0].Environment, []string{"A=1", "A=2"}) {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestFromRunCompactsMatrixAndListsFailedInstances(t *testing.T) {
	dimensions := []model.MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "one", Name: "train-SEED1", Command: []string{"train"}, Environment: []string{"BASE=yes", "SEED=1"}, Matrix: &model.MatrixSpec{GroupID: "group", Dimensions: dimensions, Values: []model.MatrixValue{{Name: "SEED", Value: "1"}}, BaseName: "train", BaseEnvironment: []string{"BASE=yes"}}},
		{ID: "two", Name: "train-SEED2", Command: []string{"train"}, Environment: []string{"BASE=yes", "SEED=2"}, Matrix: &model.MatrixSpec{GroupID: "group", Dimensions: dimensions, Values: []model.MatrixValue{{Name: "SEED", Value: "2"}}, BaseName: "train", BaseEnvironment: []string{"BASE=yes"}}},
	}}
	summary := model.RunSummary{Results: []model.JobResult{{ID: "one", AttemptID: "att-run-one-0", ExitCode: 0}, {ID: "two", AttemptID: "att-run-two-0", ExitCode: 1}}}
	manifest, err := FromRun(queue, summary, Source{Project: "demo", RunIDs: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Jobs) != 1 || !reflect.DeepEqual(manifest.Jobs[0].Matrix, []string{"SEED=1,2"}) {
		t.Fatalf("manifest = %#v", manifest)
	}
	if manifest.Jobs[0].AttemptID != "att-run-one-0" {
		t.Fatalf("representative attempt = %q", manifest.Jobs[0].AttemptID)
	}
	instances := manifest.Jobs[0].Instances
	if len(instances) != 1 || instances[0].Matrix["SEED"] != "2" || instances[0].Status != "failed" || instances[0].AttemptID != "att-run-two-0" {
		t.Fatalf("instances = %#v", instances)
	}
}

func TestFromRunAggregatesExpandedStatuses(t *testing.T) {
	dimensions := []model.MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "one", Command: []string{"train"}, Environment: []string{"SEED=1"}, Matrix: &model.MatrixSpec{GroupID: "group", Dimensions: dimensions, Values: []model.MatrixValue{{Name: "SEED", Value: "1"}}}},
		{ID: "two", Command: []string{"train"}, Environment: []string{"SEED=2"}, Matrix: &model.MatrixSpec{GroupID: "group", Dimensions: dimensions, Values: []model.MatrixValue{{Name: "SEED", Value: "2"}}}},
	}}
	manifest, err := FromRun(queue, model.RunSummary{}, Source{Project: "demo", RunIDs: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Jobs[0].Status != "unfinished" || len(manifest.Jobs[0].Instances) != 2 {
		t.Fatalf("unfinished manifest = %#v", manifest.Jobs[0])
	}
	manifest, err = FromRun(queue, model.RunSummary{Results: []model.JobResult{
		{ID: "one", ExitCode: 130, Error: "cancelled"}, {ID: "two", ExitCode: 0},
	}}, Source{Project: "demo", RunIDs: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Jobs[0].Status != "cancelled" || len(manifest.Jobs[0].Instances) != 1 || manifest.Jobs[0].Instances[0].Status != "cancelled" {
		t.Fatalf("cancelled manifest = %#v", manifest.Jobs[0])
	}
}

func TestEncodeFormatsCanBeDecoded(t *testing.T) {
	manifest := Manifest{Version: 1, Jobs: []Job{{Name: "job", Command: []string{"echo", "hello"}}}}
	for _, format := range []string{"yaml", "toml", "json"} {
		data, err := Encode(manifest, format)
		if err != nil {
			t.Fatalf("Encode(%s): %v", format, err)
		}
		decoded, err := Decode(strings.NewReader(string(data)), format)
		if err != nil {
			t.Fatalf("Decode(%s): %v\n%s", format, err, data)
		}
		if !reflect.DeepEqual(decoded, manifest) {
			t.Fatalf("round trip %s = %#v, want %#v", format, decoded, manifest)
		}
	}
}

func TestEncodeComplexManifestFormats(t *testing.T) {
	task := 3
	manifest := Manifest{
		Version: 1,
		Source:  &Source{Project: "demo", RunIDs: []string{"run-1", "run-2"}},
		Jobs: []Job{{
			Name: "train", Command: []string{"train"}, Array: "1,3", Matrix: []string{"SEED=1,2"}, Status: "failed", AttemptID: "att_run-1-job-0",
			Instances: []Instance{{Matrix: map[string]string{"SEED": "2"}, Task: &task, Status: "failed", AttemptID: "att_run-2-job-3-0"}},
		}},
	}
	for _, format := range []string{"yaml", "toml", "json"} {
		data, err := Encode(manifest, format)
		if err != nil {
			t.Fatalf("Encode(%s): %v", format, err)
		}
		decoded, err := Decode(strings.NewReader(string(data)), format)
		if err != nil {
			t.Fatalf("Decode(%s): %v\n%s", format, err, data)
		}
		if !reflect.DeepEqual(decoded, manifest) {
			t.Fatalf("round trip %s = %#v, want %#v", format, decoded, manifest)
		}
	}
}
