package executor

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/state"
)

func TestRegistryNamesAndLookup(t *testing.T) {
	registry := NewRegistry(state.NewStore(0o700, 0o600), nil)
	if got := registry.Names(); !reflect.DeepEqual(got, []string{"local", "lsf", "pbs", "slurm", "ssh"}) {
		t.Fatalf("Names() = %#v", got)
	}
	if !registry.Known("slurm") || registry.Known("kubernetes") {
		t.Fatal("Known() mismatch")
	}
	if executor, ok := registry.Lookup("pbs"); !ok || executor.Name() != "pbs" {
		t.Fatalf("Lookup(pbs) = %v, %v", executor, ok)
	}
}

func TestRegistryOwner(t *testing.T) {
	store := state.NewStore(0o700, 0o600)
	registry := NewRegistry(store, nil)
	jobDir := t.TempDir()
	if _, err := registry.Owner(store, jobDir); !errors.Is(err, ErrJobNotRunning) {
		t.Fatalf("Owner() of an unsubmitted job = %v, want ErrJobNotRunning", err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "pid"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if owner, err := registry.Owner(store, jobDir); err != nil || owner.Name() != "local" {
		t.Fatalf("Owner() with pid = %v, %v, want local", owner, err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "job.json"), []byte(`{"executor":"lsf"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if owner, err := registry.Owner(store, jobDir); err != nil || owner.Name() != "lsf" {
		t.Fatalf("Owner() with job.json = %v, %v, want lsf", owner, err)
	}
}

func TestLocalHostMismatch(t *testing.T) {
	store := state.NewStore(0o700, 0o600)
	registry := NewRegistry(store, nil)
	runDir := t.TempDir()
	local, _ := registry.Lookup("local")
	slurm, _ := registry.Lookup("slurm")
	if err := os.WriteFile(filepath.Join(runDir, "context.json"), []byte(`{"hostname":"elsewhere.invalid"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if host, mismatch := LocalHostMismatch(store, local, runDir); !mismatch || host != "elsewhere.invalid" {
		t.Fatalf("local job on another host = %q, %v", host, mismatch)
	}
	if _, mismatch := LocalHostMismatch(store, slurm, runDir); mismatch {
		t.Fatal("scheduler jobs are controllable from any host")
	}
}
