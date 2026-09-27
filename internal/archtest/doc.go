// Package archtest checks the Go package boundary rules in
// docs/contracts/00-overview.md against the import graph.
//
// It holds only tests. A rule names a package and the imports it must not
// have, so a change that crosses a documented boundary fails `go test`
// instead of relying on review.
package archtest
