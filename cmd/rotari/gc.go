package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/runregistry"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

// cmdGC removes stale run and basedir registry entries from the master
// registry; --dry-run lists them without removing anything.
func cmdGC(args []string) int {
	fs := flag.NewFlagSet("gc", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	masterdir := cliString(fs, "masterdir", "")
	dryRun := cliBool(fs, "dry-run", false)
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
	return runRegistryGC(masterDir, *dryRun)
}

// runRegistryGC finds run registry entries whose run directory is gone and
// basedir registry records whose basedir is gone, and removes them unless
// dryRun is set. Each removal checks again that the entry is unchanged and
// its directory still absent, so an entry that reappeared since the scan is
// kept. Malformed run registry files are listed and left alone.
func runRegistryGC(masterDir string, dryRun bool) int {
	entries, skipped, err := runregistry.Open(masterDir).Orphans()
	if err != nil {
		printErrorf("failed to scan run registry: %v", err)
		return 1
	}
	missingBasedirs, err := basedirregistry.Open(masterDir).Missing()
	if err != nil {
		printErrorf("failed to scan basedir registry: %v", err)
		return 1
	}
	if dryRun {
		fmt.Printf("dry run: found %d orphan run registry entr%s\n", len(entries), pluralSuffix(len(entries)))
		for _, entry := range entries {
			fmt.Printf("  %s -> %s/projects/%s/runs/%s\n", entry.RunID, entry.BaseDir, entry.ProjectName, entry.RunID)
		}
		fmt.Printf("dry run: found %d missing basedir registr%s\n", len(missingBasedirs), pluralSuffix(len(missingBasedirs)))
		for _, baseDir := range missingBasedirs {
			fmt.Printf("  %s\n", baseDir)
		}
	} else {
		registry := runregistry.Open(masterDir)
		removed := 0
		for _, planned := range entries {
			ok, err := registry.RemoveOrphan(planned)
			if err != nil {
				printError(err)
				return 1
			}
			if ok {
				removed++
				fmt.Printf("  removed %s -> %s/projects/%s/runs/%s\n", planned.RunID, planned.BaseDir, planned.ProjectName, planned.RunID)
			}
		}
		basedirRegistry := basedirregistry.Open(masterDir)
		removedBasedirs := 0
		for _, baseDir := range missingBasedirs {
			if _, err := os.Stat(baseDir); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				printError(err)
				return 1
			}
			ok, err := basedirRegistry.Remove(baseDir)
			if err != nil {
				printError(err)
				return 1
			}
			if ok {
				removedBasedirs++
				fmt.Printf("  removed %s\n", baseDir)
			}
		}
		fmt.Printf("removed %d orphan run registry entr%s\n", removed, pluralSuffix(removed))
		fmt.Printf("removed %d missing basedir registr%s\n", removedBasedirs, pluralSuffix(removedBasedirs))
	}
	if len(skipped) > 0 {
		fmt.Printf("skipped %d invalid run registry entr%s\n", len(skipped), pluralSuffix(len(skipped)))
		for _, path := range skipped {
			fmt.Printf("  %s\n", path)
		}
		fmt.Println("Inspect these files and repair or remove them manually only after confirming their run data is safe.")
	}
	return 0
}

func pluralSuffix(count int) string {
	if count == 1 {
		return "y"
	}
	return "ies"
}
