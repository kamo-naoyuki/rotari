// Command rotari provides the CLI, background server, Web adapter, and
// user-facing formatting for Rotari.
//
// This package parses command-line input, resolves concrete project state
// paths, connects internal packages to filesystem and process adapters, and
// formats responses for users. Shared domain, persistence, execution, and Web
// projection behavior should live in internal packages rather than being
// reimplemented here.
package main
