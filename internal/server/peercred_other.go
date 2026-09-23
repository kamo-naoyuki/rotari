//go:build !linux

package server

import "net"

// VerifyPeerCredential is a no-op outside Linux: SO_PEERCRED is Linux-specific.
// The socket's 0600 mode remains the primary access control on other platforms.
func VerifyPeerCredential(conn net.Conn) error {
	return nil
}
