package server

import (
	"encoding/json"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestRequestJSONRoundTrip(t *testing.T) {
	original := Request{
		Op: "run", QueueName: "demo", RunName: "nightly", LocalConcurrency: 2,
		Executor: "slurm", ExecutorOptions: []string{"--partition short"},
		JobIDs: []string{"job-1"}, SourceRunID: "run-0", PartialArray: true,
		Array: &model.ArraySpec{First: 1, Last: 3},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Op != original.Op || decoded.QueueName != original.QueueName || decoded.RunName != original.RunName || decoded.Executor != original.Executor || decoded.Array == nil || decoded.Array.First != 1 {
		t.Fatalf("decoded request = %#v", decoded)
	}
}

func TestResponseJSONRoundTrip(t *testing.T) {
	original := Response{OK: true, Progress: true, JobID: "job-1", Completed: 2, Total: 4, Succeeded: 1, Failed: 1}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Response
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != original {
		t.Fatalf("decoded response = %#v, want %#v", decoded, original)
	}
}
