// Package report formats a run's or a job's record as Markdown for the people
// and agents who read it: the run's sources and notes, a table of its jobs
// with the environment values that tell them apart and their first and last
// log lines, then each job's notes, command, rule diagnoses, and bounded log
// excerpt. Paths and hostnames are redacted by default; callers may opt out
// of redaction.
//
// `show --report` and the Web UI's report endpoints share it, so both
// produce the same text. It reads results through internal/web's job
// projection, which follows the internal/jobstatus fallback chain, and must
// not resolve job status on its own.
package report
