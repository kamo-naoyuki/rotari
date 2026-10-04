package artifactsource

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/artifact"
)

func TestRead(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "a.yaml")
	if err := os.WriteFile(regular, []byte("out: x.csv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := Read(regular)
	if err != nil || string(data) != "out: x.csv\n" {
		t.Fatalf("Read(regular) = %q, %v", data, err)
	}

	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	if data, err := Read(link); err != nil || string(data) != "out: x.csv\n" {
		t.Fatalf("Read(symlink) = %q, %v; a link to a regular file is followed", data, err)
	}

	if _, err := Read(dir); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("Read(directory) error = %v, want ErrNotRegular", err)
	}
	dirLink := filepath.Join(dir, "dir.yaml")
	if err := os.Symlink(dir, dirLink); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dirLink); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("Read(link to directory) error = %v, want ErrNotRegular", err)
	}
	if _, err := Read(filepath.Join(dir, "missing.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Read(missing) error = %v", err)
	}

	large := filepath.Join(dir, "large.json")
	if err := os.WriteFile(large, make([]byte, artifact.MaxSourceBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(large); err == nil {
		t.Fatal("Read(oversized) succeeded")
	}
}

func TestReadDoesNotBlockOnFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "pipe.yaml")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Read(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("Read(fifo) error = %v, want ErrNotRegular", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read blocked on a FIFO")
	}
}

func TestReadUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of mode")
	}
	path := filepath.Join(t.TempDir(), "secret.yaml")
	if err := os.WriteFile(path, []byte("a: b.csv\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Read(unreadable) error = %v, want permission error", err)
	}
}
