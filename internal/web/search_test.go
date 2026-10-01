package web

import (
	"testing"
)

func TestSearchHistoryCombinesAncestorAndJobConditions(t *testing.T) {
	records := []HistorySearchRecord{
		{BaseDirID: "base-a", BaseDirPath: "/a", ProjectName: "alpha", RunID: "run-1", RunName: "nightly", RunStatus: "failed", JobID: "job-1", JobName: "build", JobStatus: "failed", Command: "make build", Timestamp: "2026-10-01T10:00:00Z"},
		{BaseDirID: "base-a", BaseDirPath: "/a", ProjectName: "alpha", RunID: "run-1", RunName: "nightly", RunStatus: "failed", JobID: "job-2", JobName: "test", JobStatus: "success", Command: "go test ./...", Timestamp: "2026-10-01T11:00:00Z"},
		{BaseDirID: "base-b", BaseDirPath: "/b", ProjectName: "beta", RunID: "run-2", RunName: "nightly", RunStatus: "success", JobID: "job-3", JobName: "build", JobStatus: "success", Command: "make build", Timestamp: "2026-10-02T10:00:00Z"},
	}
	request := HistorySearchRequest{Filters: []HistorySearchFilter{
		{Target: HistorySearchProject, Field: "project_name", Word: "ALPHA"},
		{Target: HistorySearchRun, Field: "run_name", Word: "nightly", Join: "and"},
		{Target: HistorySearchJob, Field: "command", Word: "go test", Join: "and"},
	}}
	got, err := SearchHistory(records, request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Rows[0].Target != HistorySearchJob || got.Rows[0].BaseDirID != "base-a" || got.Rows[0].JobID != "job-2" {
		t.Fatalf("SearchHistory() = %#v, want alpha/run-1/job-2", got)
	}
}

func TestSearchHistoryUsesSingleResultTargetWithAncestorAttributes(t *testing.T) {
	records := []HistorySearchRecord{
		{BaseDirID: "base", ProjectName: "alpha", RunID: "run-a", RunName: "nightly", RunStatus: "failed", JobID: "job-a", JobName: "build", JobStatus: "failed", Command: "make build", Timestamp: "2026-10-01T10:00:00Z"},
		{BaseDirID: "base", ProjectName: "beta", RunID: "run-b", RunName: "nightly", RunStatus: "success", JobID: "job-b", JobName: "build", JobStatus: "success", Command: "make build", Timestamp: "2026-10-01T11:00:00Z"},
	}
	request := HistorySearchRequest{
		Target: HistorySearchJob,
		Filters: []HistorySearchFilter{
			{Target: HistorySearchProject, Field: "project_name", Word: "alpha"},
			{Target: HistorySearchRun, Field: "run_name", Word: "nightly", Join: "and"},
			{Target: HistorySearchJob, Field: "status", Word: "failed", Join: "and"},
		},
	}
	got, err := SearchHistory(records, request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Rows[0].Target != HistorySearchJob || got.Rows[0].JobID != "job-a" {
		t.Fatalf("SearchHistory() = %#v, want the failed job under alpha/nightly", got)
	}
}

func TestSearchHistoryUsesPerConditionOrAndTimeWindow(t *testing.T) {
	records := []HistorySearchRecord{
		{BaseDirID: "base", ProjectName: "alpha", RunID: "run-1", RunStatus: "failed", JobID: "job-1", JobStatus: "failed", Command: "make build", Timestamp: "2026-10-01T10:00:00Z"},
		{BaseDirID: "base", ProjectName: "alpha", RunID: "run-1", RunStatus: "failed", JobID: "job-2", JobStatus: "success", Command: "go test ./...", Timestamp: "2026-10-02T10:00:00Z"},
	}
	request := HistorySearchRequest{
		Filters: []HistorySearchFilter{
			{Target: HistorySearchJob, Field: "command", Word: "make"},
			{Target: HistorySearchJob, Field: "status", Word: "success", Join: "or"},
		},
		From: "2026-10-02T00:00:00Z", To: "2026-10-02T23:59:59Z",
	}
	got, err := SearchHistory(records, request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Rows[0].JobID != "job-2" {
		t.Fatalf("SearchHistory() = %#v, want only in-range successful job", got)
	}
}

func TestSearchHistoryDeduplicatesRunsAndPaginatesDeterministically(t *testing.T) {
	records := []HistorySearchRecord{
		{BaseDirID: "base", ProjectName: "alpha", RunID: "run-1", RunName: "nightly", RunStatus: "success", RunFinished: "2026-10-02T10:00:00Z", Timestamp: "2026-10-02T10:00:00Z"},
		{BaseDirID: "base", ProjectName: "alpha", RunID: "run-1", RunName: "nightly", RunStatus: "success", RunFinished: "2026-10-02T10:00:00Z", JobID: "job-1", JobName: "build", Timestamp: "2026-10-02T10:00:00Z"},
		{BaseDirID: "base", ProjectName: "alpha", RunID: "run-0", RunName: "nightly", RunStatus: "success", RunFinished: "2026-10-01T10:00:00Z", Timestamp: "2026-10-01T10:00:00Z"},
	}
	request := HistorySearchRequest{Filters: []HistorySearchFilter{{Target: HistorySearchRun, Field: "run_name", Word: "nightly"}}, Limit: 1, Offset: 1}
	got, err := SearchHistory(records, request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.Limit != 1 || len(got.Rows) != 1 || got.Rows[0].RunID != "run-0" {
		t.Fatalf("SearchHistory() = %#v, want second unique run", got)
	}
}

func TestValidateHistorySearchRequestRejectsUnsupportedFields(t *testing.T) {
	request := HistorySearchRequest{Filters: []HistorySearchFilter{{Target: HistorySearchJob, Field: "project_name", Word: "alpha"}}}
	if err := ValidateHistorySearchRequest(request); err == nil {
		t.Fatal("ValidateHistorySearchRequest() accepted an unsupported job field")
	}
}

func TestValidateHistorySearchRequestRejectsConditionMoreSpecificThanTarget(t *testing.T) {
	request := HistorySearchRequest{
		Target:  HistorySearchRun,
		Filters: []HistorySearchFilter{{Target: HistorySearchJob, Field: "command", Word: "make"}},
	}
	if err := ValidateHistorySearchRequest(request); err == nil {
		t.Fatal("ValidateHistorySearchRequest() accepted a job condition for run results")
	}
}
