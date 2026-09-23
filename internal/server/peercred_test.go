package server

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVerifyPeerCredentialAcceptsSameUIDConnection(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "rotari-peercred-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseDir)
	socketPath := filepath.Join(baseDir, "test.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	client, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	var serverConn net.Conn
	select {
	case serverConn = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("server did not accept connection")
	}
	defer serverConn.Close()

	if err := VerifyPeerCredential(serverConn); err != nil {
		t.Fatalf("VerifyPeerCredential rejected same-process connection: %v", err)
	}
}
