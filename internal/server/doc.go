// Package server implements the background server's transport and lifetime.
//
// It owns operation names, request and response wire shapes, peer-credential
// checks, the server lease and socket, request dispatch, active-run tracking,
// idle shutdown, the synchronous-run detach and disconnect protocol, and the
// client side of that protocol. The project work behind each request is
// supplied through Operations; this package should not own persisted project
// state, run orchestration, or CLI formatting.
package server
