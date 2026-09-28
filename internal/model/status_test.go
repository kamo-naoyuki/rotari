package model

import "testing"

func TestMarkResult(t *testing.T) {
	failed := JobResult{ID: "job", ExitCode: 2, Error: "boom", AttemptID: "att", DiagnosisStatus: DiagnosisNoMatch}
	success := JobResult{ID: "job", AttemptID: "att"}
	tests := []struct {
		name     string
		result   JobResult
		finished bool
		status   string
		want     string
		accepted bool
	}{
		{name: "unmarked", result: failed, finished: true, want: StatusFailed},
		{name: "same status", result: failed, finished: true, status: StatusFailed, want: StatusFailed},
		{name: "accept", result: failed, finished: true, status: StatusSuccess, want: StatusSuccess, accepted: true},
		{name: "drop", result: success, finished: true, status: StatusUnfinished, want: StatusUnfinished},
		{name: "fail success", result: success, finished: true, status: StatusFailed, want: StatusFailed},
		{name: "cancel failure", result: failed, finished: true, status: StatusCancelled, want: StatusCancelled},
		{name: "unfinished stays", status: StatusUnfinished, want: StatusUnfinished},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, finished, err := MarkResult("job", test.result, test.finished, test.status)
			if err != nil {
				t.Fatal(err)
			}
			if got := ResultStatus(result, finished); got != test.want || result.Accepted != test.accepted {
				t.Fatalf("MarkResult() = %#v (%s), want %s accepted=%v", result, got, test.want, test.accepted)
			}
			if finished && result.AttemptID != "att" {
				t.Fatalf("MarkResult() dropped the attempt: %#v", result)
			}
			if test.status != "" && test.status != ResultStatus(test.result, test.finished) && result.DiagnosisStatus != "" {
				t.Fatalf("MarkResult() kept the diagnosis of the recorded result: %#v", result)
			}
		})
	}
	if _, _, err := MarkResult("job", JobResult{}, false, StatusSuccess); err == nil {
		t.Fatal("MarkResult() accepted a job without a result")
	}
}

func TestQueuedStatusText(t *testing.T) {
	failed := &JobOrigin{Status: StatusFailed}
	for _, test := range []struct {
		origin *JobOrigin
		marked string
		want   string
	}{
		{nil, "", "-"},
		{failed, "", "failed"},
		{failed, StatusFailed, "failed"},
		{failed, StatusSuccess, "success (accepted)"},
		{failed, StatusUnfinished, "unfinished (marked)"},
		{nil, StatusCancelled, "cancelled (marked)"},
	} {
		if got := QueuedStatusText(test.origin, test.marked); got != test.want {
			t.Fatalf("QueuedStatusText(%#v, %q) = %q, want %q", test.origin, test.marked, got, test.want)
		}
	}
}
