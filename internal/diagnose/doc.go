// Package diagnose provides rule-based job failure diagnosis.
//
// It matches logs and job state against local diagnosis rules. It should
// return diagnosis data to callers without owning run orchestration,
// persistence layout, or user-facing command formatting.
package diagnose
