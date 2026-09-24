// Package server defines the background server protocol shared by clients and
// the server implementation.
//
// It owns operation names, request and response wire shapes, peer-credential
// helpers, and client protocol helpers. It should describe communication
// contracts without owning persisted project state, run orchestration, or CLI
// formatting.
package server
