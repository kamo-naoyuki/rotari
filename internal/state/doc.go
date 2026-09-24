// Package state owns Rotari's filesystem-backed project state.
//
// It reads and writes model values as durable JSON files, validates path
// elements, lists run and attempt directories, and manages locks and other
// persistence details. It should not decide run orchestration, retry policy,
// dependency scheduling, or executor behavior.
package state
