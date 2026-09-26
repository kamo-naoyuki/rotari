// Package report formats a run's or a job's evidence for AI-assisted
// diagnosis: status, commands, rule diagnoses, and bounded log tails, with
// paths and hostnames redacted.
//
// `show --report` and the Web UI's report endpoints share it, so both
// produce the same text. It reads results through internal/web's job
// projection, which follows the internal/jobstatus fallback chain, and must
// not resolve job status on its own.
package report
