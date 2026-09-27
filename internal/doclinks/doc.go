// Package doclinks checks relative links in the repository's Markdown
// documentation.
//
// It holds only tests. The contracts and ARCHITECTURE.md link to code and
// tests on purpose, so a move or rename that leaves a link dangling fails
// `go test` instead of going unnoticed.
package doclinks
