package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useShortSocketRoot points the short socket directory at a fresh temporary
// root so tests do not touch the real /tmp/rotari-<uid>.
func useShortSocketRoot(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "rs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	old := shortSocketRoot
	shortSocketRoot = root
	t.Cleanup(func() { shortSocketRoot = old })
	return root
}

// longBaseDir returns an existing directory whose server.sock path exceeds
// maxSocketPathLength.
func longBaseDir(t *testing.T) string {
	t.Helper()
	baseDir := filepath.Join(t.TempDir(), strings.Repeat("d", 60), strings.Repeat("e", 60))
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return baseDir
}

func TestSocketPathKeepsShortBaseDirSocket(t *testing.T) {
	useShortSocketRoot(t)
	baseDir := "/state"
	if got := SocketPath(baseDir); got != "/state/server.sock" {
		t.Fatalf("SocketPath(%q) = %q, want the base directory socket", baseDir, got)
	}
}

func TestSocketPathShortensLongBaseDirStably(t *testing.T) {
	root := useShortSocketRoot(t)
	baseDir := longBaseDir(t)
	got := SocketPath(baseDir)
	if len(got) > maxSocketPathLength || !strings.HasPrefix(got, filepath.Join(root, "rotari-")) {
		t.Fatalf("SocketPath(long) = %q (%d bytes), want a short path under %s", got, len(got), root)
	}
	if again := SocketPath(baseDir); again != got {
		t.Fatalf("SocketPath is not stable: %q then %q", got, again)
	}
	link := filepath.Join(t.TempDir(), strings.Repeat("l", 100))
	if err := os.Symlink(baseDir, link); err != nil {
		t.Fatal(err)
	}
	if viaLink := SocketPath(link); viaLink != got {
		t.Fatalf("SocketPath via symlink = %q, want %q", viaLink, got)
	}
	if other := SocketPath(longBaseDir(t)); other == got {
		t.Fatalf("different base directories share socket %q", got)
	}
}

func TestListenServesLongBaseDir(t *testing.T) {
	useShortSocketRoot(t)
	baseDir := longBaseDir(t)
	listener, release, err := Listen(baseDir, 0o600)
	if err != nil {
		t.Fatalf("Listen(long base directory) returned error: %v", err)
	}
	defer release()
	server := New(listener, &fakeOperations{}, nil)
	go server.Serve()
	defer server.Stop()
	response, err := SendRequest(baseDir, Request{Op: OpPing})
	if err != nil || !response.OK {
		t.Fatalf("ping over short socket = %+v, %v", response, err)
	}
	info, err := os.Stat(filepath.Dir(SocketPath(baseDir)))
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("short socket directory = %v, %v, want mode 0700", info, err)
	}
}

func TestListenRejectsUnsafeShortSocketDir(t *testing.T) {
	useShortSocketRoot(t)
	baseDir := longBaseDir(t)
	dir := filepath.Dir(SocketPath(baseDir))
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Listen(baseDir, 0o600); err == nil || !strings.Contains(err.Error(), "mode 0700") {
		t.Fatalf("Listen with a group-readable socket directory error = %v, want refusal", err)
	}
}
