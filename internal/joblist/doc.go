// Package joblist lists recent job attempts across a base directory's
// projects: running jobs of active runs and jobs that finished within a time
// window.
//
// `rotari jobs` and the Web UI's jobs page share it. Row status comes from
// internal/jobstatus; this package decides which attempts are listed and in
// what order, and how their times read. Column selection and table layout
// belong to the callers.
package joblist
