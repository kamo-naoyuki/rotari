// Package diagnose provides rule-based and provider-backed job failure
// diagnosis.
//
// It matches logs and job state against local diagnosis rules and can request
// explanations from supported LLM providers. It should return diagnosis data to
// callers without owning run orchestration, persistence layout, or user-facing
// command formatting.
package diagnose
