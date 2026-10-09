package model

// ProgressEvent is a run-start, job-start, or job-result event. Its fields
// preserve the attached supervisor's progress stream, without transport-only
// responses (handshakes, control messages, or run completion).
type ProgressEvent struct {
	OK        bool   `json:"ok"`
	RunID     string `json:"run_id,omitempty"`
	Notice    string `json:"notice,omitempty"`
	Message   string `json:"message,omitempty"`
	Progress  bool   `json:"progress,omitempty"`
	JobID     string `json:"job_id,omitempty"`
	Completed int    `json:"completed,omitempty"`
	Total     int    `json:"total,omitempty"`
	Succeeded int    `json:"succeeded,omitempty"`
	Failed    int    `json:"failed,omitempty"`
}
