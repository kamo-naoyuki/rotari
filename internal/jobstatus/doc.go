// Package jobstatus resolves job outcomes from persisted run state.
//
// It owns the read-side fallback chain shared by the CLI and Web projections:
// an attempt's own `status` file, then its wrapper `status.json` once terminal,
// then the persisted `scheduler_status.json`, and finally the run's recorded
// result: its `summary.json` result, or before the summary the carried result
// it recorded when it started (RecordedResults). It also resolves displayed job timestamps, including
// the fallback to a carried job's origin. Callers render a resolution; they
// should not re-implement any step of the chain or change run state.
package jobstatus
