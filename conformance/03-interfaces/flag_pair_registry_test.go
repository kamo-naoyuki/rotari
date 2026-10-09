package interfaces

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// Registry cases never start jobs or supervisors. All paths, including the
// missing targets, are inside the harness-owned temporary root.
type pairRegistryFixture struct {
	e                                         *support.Env
	config                                    string
	initial                                   support.PairSavedTree
	current                                   support.PairSavedTree
	orphan, malformed, staleBase, staleServer string
}

func newPairRegistryFixture(t *testing.T) *pairRegistryFixture {
	t.Helper()
	e := support.NewEnv(t)
	f := &pairRegistryFixture{e: e, config: filepath.Join(e.Root, "registry.yaml")}
	pairRegistryWrite(t, f.config, []byte("{}\n"))
	liveRun := filepath.Join(e.Base, "projects", "live", "runs", "live-run")
	if err := os.MkdirAll(liveRun, 0o755); err != nil {
		t.Fatal(err)
	}
	pairRegistryWrite(t, filepath.Join(liveRun, "sentinel"), []byte("run data must remain\n"))
	for _, id := range []string{"live-run", "missing-run"} {
		path := filepath.Join(e.Master, "runs", id+".json")
		pairRegistryJSON(t, path, map[string]any{"base_dir": e.Base, "project_name": "live", "run_id": id})
		if id == "missing-run" {
			f.orphan = path
		}
	}
	f.malformed = filepath.Join(e.Master, "runs", "invalid.json")
	pairRegistryWrite(t, f.malformed, []byte("{broken\n"))
	for _, base := range []string{e.Base, filepath.Join(e.Root, "missing-base")} {
		sum := sha256.Sum256([]byte(base))
		path := filepath.Join(e.Master, "basedirs", fmt.Sprintf("%x", sum)[:32]+".json")
		pairRegistryJSON(t, path, map[string]any{"base_dir": base})
		if base != e.Base {
			f.staleBase = path
		}
	}
	f.staleServer = filepath.Join(e.Master, "stale-server.json")
	pairRegistryJSON(t, f.staleServer, map[string]any{"base_dir": e.Base, "project": "live", "pid": -1})
	f.initial = support.SavePairTree(t, e.Root)
	f.current = f.initial
	return f
}

func pairRegistryWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func pairRegistryJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	pairRegistryWrite(t, path, data)
}

func (f *pairRegistryFixture) args(t *testing.T, command, subcommand string, flags []pairFlag) []string {
	t.Helper()
	args := []string{command}
	if subcommand != "" {
		args = append(args, subcommand)
	}
	values := map[string]string{"config": f.config, "basedir": f.e.Base, "masterdir": f.e.Master}
	for _, flag := range flags {
		if flag.Name == "dry-run" {
			args = append(args, "--dry-run=true")
			continue
		}
		value, ok := values[flag.Name]
		if !ok {
			t.Fatalf("no registry sample for --%s", flag.Name)
		}
		args = append(args, "--"+flag.Name, value)
	}
	return args
}

func (f *pairRegistryFixture) invoke(t *testing.T, command, subcommand string, flags []pairFlag) support.Result {
	t.Helper()
	f.initial.Restore(t, f.e.Root, f.current)
	r := pairInvoke(t, f.e, f.args(t, command, subcommand, flags)...)
	after := support.SavePairTree(t, f.e.Root)
	f.current = after
	want := f.initial.Raw()
	if command == "gc" {
		if r.Code != 0 || r.Stderr != "" || !strings.Contains(r.Stdout, "1 orphan run registry entry") || !strings.Contains(r.Stdout, "1 missing basedir registry") || !strings.Contains(r.Stdout, "skipped 1 invalid run registry entry") {
			t.Fatalf("unexpected gc outcome: %s", r)
		}
		if !pairHasFlag(flags, "dry-run") {
			f.removeExpected(t, want, f.orphan, f.staleBase)
		}
	} else {
		f.assertServer(t, subcommand, flags, r, want)
	}
	if !reflect.DeepEqual(want, after.Raw()) {
		t.Fatalf("unexpected registry/state mutation: %s", r)
	}
	return r
}

func (f *pairRegistryFixture) removeExpected(t *testing.T, files map[string]string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		relative, err := filepath.Rel(f.e.Root, path)
		if err != nil {
			t.Fatal(err)
		}
		delete(files, relative)
	}
}

func (f *pairRegistryFixture) assertServer(t *testing.T, subcommand string, flags []pairFlag, r support.Result, want map[string]string) {
	t.Helper()
	unsupported := "masterdir"
	if subcommand == "list" {
		unsupported = "basedir"
	}
	if pairHasFlag(flags, unsupported) {
		if r.Code != 1 || r.Stdout != "" || !strings.Contains(r.Stderr, "unknown option --"+unsupported) {
			t.Fatalf("unsupported subcommand option was not diagnosed: %s", r)
		}
		return
	}
	if subcommand == "status" {
		if r.Code != 1 || r.Stdout != "" || strings.TrimSpace(r.Stderr) != "server is not running" {
			t.Fatalf("unexpected stopped-server status: %s", r)
		}
		return
	}
	if r.Code != 0 || r.Stderr != "" || r.Stdout != "master="+f.e.Master+"\nno running servers\n" {
		t.Fatalf("unexpected server list: %s", r)
	}
	f.removeExpected(t, want, f.staleServer)
}

func TestCLIFlagPairRegistries(t *testing.T) {
	f := newPairRegistryFixture(t)
	pairs, invocations := 0, 0
	for _, command := range readPairSchema(t, f.e) {
		if command.Name != "gc" && command.Name != "server" {
			continue
		}
		for _, pair := range commandFlagPairs(command) {
			pairs++
			for _, subcommand := range pairRegistrySubcommands(command.Name) {
				t.Run(command.Name+"/"+subcommand+"/"+pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
					ab := f.invoke(t, command.Name, subcommand, []pairFlag{pair.A, pair.B})
					ba := f.invoke(t, command.Name, subcommand, []pairFlag{pair.B, pair.A})
					invocations += 2
					if ab.Code != ba.Code || ab.Stdout != ba.Stdout || ab.Stderr != ba.Stderr {
						t.Fatalf("order-dependent registry operation:\n%s\n%s", ab, ba)
					}
				})
			}
		}
	}
	t.Logf("registry pairs=%d invocations=%d (server status/list each tested)", pairs, invocations)
}

func TestCLIFlagPairRegistrySamples(t *testing.T) {
	f := newPairRegistryFixture(t)
	for _, command := range readPairSchema(t, f.e) {
		if command.Name != "gc" && command.Name != "server" {
			continue
		}
		for _, subcommand := range pairRegistrySubcommands(command.Name) {
			for _, flag := range command.Flags {
				t.Run(command.Name+"/"+subcommand+"/"+flag.Name, func(t *testing.T) { f.invoke(t, command.Name, subcommand, []pairFlag{flag}) })
			}
		}
	}
}

func pairRegistrySubcommands(command string) []string {
	if command == "server" {
		return []string{"status", "list"}
	}
	return []string{""}
}

func TestCLIFlagPairRegistryLocationEffects(t *testing.T) {
	f := newPairRegistryFixture(t)
	other := filepath.Join(f.e.Root, "other-master")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	pairRegistryWrite(t, f.config, []byte("masterdir: "+fmt.Sprintf("%q", other)+"\n"))
	before := support.SavePairTree(t, f.e.Root)
	// Environment points at the fixture registry, while config points at an
	// empty registry. Removing the env source proves the config takes effect.
	configOnly := pairInvoke(t, f.e.Without("ROTARI_MASTERDIR"), "gc", "--config", f.config, "--dry-run")
	if configOnly.Code != 0 || configOnly.Stderr != "" || !strings.Contains(configOnly.Stdout, "found 0 orphan") {
		t.Fatalf("config did not select the empty registry: %s", configOnly)
	}
	for _, flags := range [][]string{{"--config", f.config, "--masterdir", f.e.Master}, {"--masterdir", f.e.Master, "--config", f.config}} {
		selected := pairInvoke(t, f.e.WithVar("ROTARI_MASTERDIR", other), append([]string{"gc", "--dry-run"}, flags...)...)
		if selected.Code != 0 || selected.Stderr != "" || !strings.Contains(selected.Stdout, "found 1 orphan") || selected.Stdout == configOnly.Stdout {
			t.Fatalf("explicit masterdir did not override config/environment: %s", selected)
		}
	}
	if !reflect.DeepEqual(before.Raw(), support.SavePairTree(t, f.e.Root).Raw()) {
		t.Fatal("registry location previews changed state")
	}
}
