package web

import "github.com/kamo-naoyuki/rotari/internal/model"

type OriginTimestampSource func(runID, jobID string) (submittedAt, finishedAt string)

func ResolveOriginTimestamps(submittedAt, finishedAt string, origin *model.JobOrigin, source OriginTimestampSource) (string, string) {
	if origin == nil || (submittedAt != "" && finishedAt != "") {
		return submittedAt, finishedAt
	}
	if submittedAt == "" {
		submittedAt = origin.SubmittedAt
	}
	if finishedAt == "" {
		finishedAt = origin.FinishedAt
	}
	if source != nil && (submittedAt == "" || finishedAt == "") {
		sourceSubmitted, sourceFinished := source(origin.RunID, origin.JobID)
		if submittedAt == "" {
			submittedAt = sourceSubmitted
		}
		if finishedAt == "" {
			finishedAt = sourceFinished
		}
	}
	return submittedAt, finishedAt
}
