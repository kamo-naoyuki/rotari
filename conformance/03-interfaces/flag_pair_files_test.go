package interfaces

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

type pairFileResult struct {
	process  support.Result
	contents string
	exists   bool
	mode     fs.FileMode
}

// The file adapter owns a single isolated output path. It restores the output
// directory before each invocation, so config's no-overwrite behavior does not
// contaminate the second order. It never changes fixture state to fix a failure.
func pairFileInvoke(t *testing.T, f pairFixture, command string, flags []pairFlag) pairFileResult {
	t.Helper()
	outputDir := filepath.Join(f.e.Root, "pair-output")
	output := filepath.Join(outputDir, "nested", "result.toml")
	if err := os.RemoveAll(outputDir); err != nil {
		t.Fatal(err)
	}
	args := []string{command}
	hasOutput, listing := false, false
	for _, flag := range flags {
		switch flag.Name {
		case "output":
			hasOutput = true
			args = append(args, "--output", output)
		case "format":
			args = append(args, "--format", "json")
		default:
			args = append(args, f.sample(t, flag)...)
		}
		listing = listing || flag.Name == "list"
	}
	// Config otherwise prompts; do not insert output if it is under test or
	// list is selected, which forbids output. Export defaults to stdout.
	if command == "config" && !hasOutput && !listing {
		args = append(args, "--output", output)
	}
	r := pairFileResult{process: pairInvoke(t, f.e, args...)}
	data, err := os.ReadFile(output)
	if errors.Is(err, os.ErrNotExist) {
		return r
	}
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	r.exists, r.contents, r.mode = true, string(data), info.Mode().Perm()
	return r
}

func assertPairFileOutcome(t *testing.T, command string, flags []pairFlag, r pairFileResult) {
	t.Helper()
	if r.process.Code != 0 && (r.exists || r.contents != "") {
		t.Fatalf("rejected command wrote output: %+v", r)
	}
	// These rejections belong only to the file adapters. Do not weaken the
	// read-only classifier by treating every usage/exit 1 as expected.
	if r.process.Code == 1 && command == "config" && pairHasFlag(flags, "notifications") && pairHasFlag(flags, "format") && strings.TrimSpace(r.process.Stderr) == "notification configuration only supports TOML" {
		return
	}
	if r.process.Code == 1 && command == "export" && pairHasFlag(flags, "template") && pairHasExportSource(flags) && strings.HasPrefix(r.process.Stderr, "usage: rotari export ") {
		return
	}
	assertPairOutcome(t, r.process)
}

func pairHasExportSource(flags []pairFlag) bool {
	return pairHasFlag(flags, "run-id") || pairHasFlag(flags, "basedir") || pairHasFlag(flags, "project-name")
}

// Compare paths, contents, and modes, excluding only directories' timestamps.
// Output goes outside the basedir; no queue/history/config may change.
func pairStateSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[relative] = fmt.Sprintf("%o:%x", info.Mode().Perm(), sha256.Sum256(data))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestCLIFlagPairFiles(t *testing.T) {
	start := time.Now()
	f := newPairFixture(t)
	state := pairStateSnapshot(t, f.e.Base)
	outcomes := [2]int{}
	for _, command := range readPairSchema(t, f.e) {
		if pairAdapter(command.Name) != "file" {
			continue
		}
		t.Run(command.Name, func(t *testing.T) {
			for _, pair := range commandFlagPairs(command) {
				t.Run(pair.A.Name+"+"+pair.B.Name, func(t *testing.T) {
					outcomes[assertPairFileOrder(t, f, command.Name, pair, state)]++
				})
			}
		})
	}
	t.Logf("file pairs accepted=%d explicitly rejected=%d invocations=%d elapsed=%s", outcomes[0], outcomes[1], 2*(outcomes[0]+outcomes[1]), time.Since(start))
}

func assertPairFileOrder(t *testing.T, f pairFixture, command string, pair flagPair, state map[string]string) int {
	t.Helper()
	ab := pairFileInvoke(t, f, command, []pairFlag{pair.A, pair.B})
	assertPairFileOutcome(t, command, []pairFlag{pair.A, pair.B}, ab)
	assertPairStateUnchanged(t, f.e.Base, state, ab.process)
	ba := pairFileInvoke(t, f, command, []pairFlag{pair.B, pair.A})
	assertPairFileOutcome(t, command, []pairFlag{pair.B, pair.A}, ba)
	assertPairStateUnchanged(t, f.e.Base, state, ba.process)
	// Args differ by design; compare all observations, including absence of a
	// file, contents, and permissions.
	ab.process.Args, ba.process.Args = nil, nil
	if !reflect.DeepEqual(ab, ba) {
		t.Fatalf("order-dependent file/output:\n%+v\n%+v", ab, ba)
	}
	return ab.process.Code
}

func assertPairStateUnchanged(t *testing.T, root string, before map[string]string, r support.Result) {
	t.Helper()
	if !reflect.DeepEqual(before, pairStateSnapshot(t, root)) {
		t.Fatalf("command changed fixture state: %s", r)
	}
}

func TestCLIFlagPairFileSamples(t *testing.T) {
	f := newPairFixture(t)
	for _, command := range readPairSchema(t, f.e) {
		if pairAdapter(command.Name) != "file" {
			continue
		}
		for _, flag := range command.Flags {
			t.Run(command.Name+"/"+flag.Name, func(t *testing.T) {
				assertPairFileOutcome(t, command.Name, []pairFlag{flag}, pairFileInvoke(t, f, command.Name, []pairFlag{flag}))
			})
		}
	}
}

func TestCLIFlagPairFileObservability(t *testing.T) {
	f := newPairFixture(t)
	for _, command := range []string{"config", "export"} {
		for _, format := range []string{"yaml", "toml", "json"} {
			t.Run(command+"/output+format/"+format, func(t *testing.T) {
				assertPairFileFormat(t, f, command, format)
			})
		}
	}
}

func assertPairFileFormat(t *testing.T, f pairFixture, command, format string) {
	t.Helper()
	args := []string{command, "--format", format}
	if command == "config" {
		args = append(args, "--output", "-")
	}
	printed := pairInvoke(t, f.e, args...)
	if printed.Code != 0 || printed.Stdout == "" {
		t.Fatalf("missing stdout format witness: %s", printed)
	}
	file := filepath.Join(t.TempDir(), "nested", "output.data")
	written := pairInvoke(t, f.e, command, "--output", file, "--format", format)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if written.Code != 0 || written.Stdout != "" || written.Stderr != printed.Stderr || string(data) != printed.Stdout {
		t.Fatalf("output/format mismatch:\n%s\n%s\nfile: %s", printed, written, data)
	}
	if format == "json" && !json.Valid(data) {
		t.Fatalf("--format json was ignored: %s", data)
	}
	if format == "toml" && !strings.Contains(string(data), "=") {
		t.Fatalf("no TOML format witness: %s", data)
	}
	if format == "yaml" && !strings.Contains(string(data), ":") {
		t.Fatalf("no YAML format witness: %s", data)
	}
}
