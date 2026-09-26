package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

const runRegistryGCCacheTTL = 10 * time.Minute

type runRegistryGCCache struct {
	CreatedAt string                 `json:"created_at"`
	Entries   []runregistry.Location `json:"entries"`
}

// cmdGC removes stale run registry entries from the master registry.
func cmdGC(args []string) int {
	fs := flag.NewFlagSet("gc", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	masterdir := cliString(fs, "masterdir", "")
	apply := fs.Bool("apply", false, "remove the cached orphan entries")
	if err := cliParse(fs, args); err != nil || len(fs.Args()) > 1 || (len(fs.Args()) == 1 && cliOptionSet(fs, "masterdir")) {
		printError("usage: " + cliUsage("gc"))
		return 1
	}
	if len(fs.Args()) == 1 {
		*masterdir = fs.Args()[0]
	}

	masterDir, err := state.ResolveMasterDir(*masterdir)
	if err != nil {
		printError(err)
		return 1
	}
	if *apply {
		return applyRunRegistryGC(masterDir)
	}
	return scanRunRegistryGC(masterDir)
}

func scanRunRegistryGC(masterDir string) int {
	entries, skipped, err := runregistry.Open(masterDir).Orphans()
	if err != nil {
		printErrorf("failed to scan run registry: %v", err)
		return 1
	}
	cache := runRegistryGCCache{CreatedAt: nowRFC3339(), Entries: entries}
	cachePath := filepath.Join(masterDir, "gc.json")
	if err := os.MkdirAll(masterDir, state.DirectoryMode()); err != nil {
		printErrorf("failed to create master directory: %v", err)
		return 1
	}
	if err := state.WriteJSON(cachePath, cache); err != nil {
		printErrorf("failed to save GC plan: %v", err)
		return 1
	}
	fmt.Printf("found %d orphan run registry entr%s\n", len(entries), pluralSuffix(len(entries)))
	for _, entry := range entries {
		fmt.Printf("  %s -> %s/projects/%s/runs/%s\n", entry.RunID, entry.BaseDir, entry.ProjectName, entry.RunID)
	}
	if len(skipped) > 0 {
		fmt.Printf("skipped %d invalid run registry entr%s; no automatic changes made\n", len(skipped), pluralSuffix(len(skipped)))
		for _, path := range skipped {
			fmt.Printf("  %s\n", path)
		}
		fmt.Println("Inspect these files and repair or remove them manually only after confirming their run data is safe.")
	}
	fmt.Printf("GC plan cached at %s (expires in %s)\n", cachePath, runRegistryGCCacheTTL)
	fmt.Printf("review the plan, then run: rotari gc --apply --masterdir %s\n", executor.ShellQuote(masterDir))
	return 0
}

func pluralSuffix(count int) string {
	if count == 1 {
		return "y"
	}
	return "ies"
}

func applyRunRegistryGC(masterDir string) int {
	cachePath := filepath.Join(masterDir, "gc.json")
	data, err := os.ReadFile(cachePath)
	if os.IsNotExist(err) {
		printError("no cached GC plan; run 'rotari gc' first")
		return 1
	}
	if err != nil {
		printErrorf("failed to read GC plan: %v", err)
		return 1
	}
	var cache runRegistryGCCache
	if err := json.Unmarshal(data, &cache); err != nil {
		printErrorf("invalid GC plan: %v", err)
		return 1
	}
	createdAt, err := time.Parse(time.RFC3339, cache.CreatedAt)
	age := time.Since(createdAt)
	if err != nil || age < 0 || age > runRegistryGCCacheTTL {
		printError("cached GC plan has expired; run 'rotari gc' again")
		return 1
	}

	registry := runregistry.Open(masterDir)
	removed := 0
	for _, planned := range cache.Entries {
		ok, err := registry.RemoveOrphan(planned)
		if err != nil {
			printError(err)
			return 1
		}
		if ok {
			removed++
		}
	}
	_ = os.Remove(cachePath)
	fmt.Printf("removed %d orphan run registry entr%s\n", removed, pluralSuffix(removed))
	return 0
}
