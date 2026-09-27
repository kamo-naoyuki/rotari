// Package conformance checks invariants from contracts/README.md against the
// built rotari binary and its Web API, from outside the code.
//
// It holds only tests and imports only the standard library, so it keeps
// passing unchanged while packages move: a failure here means user-visible
// behavior changed, not that code was reorganized. TestMain builds
// cmd/rotari once; each test gets its own base directory, master directory,
// home, and config directories, with no ROTARI_* variable inherited.
package conformance
