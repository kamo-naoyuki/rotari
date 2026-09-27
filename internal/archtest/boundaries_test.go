package archtest

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const module = "github.com/kamo-naoyuki/rotari"

// fileAccess lists the standard packages that reach the filesystem, other
// processes, or the network. "net" also matches its subpackages.
var fileAccess = []string{"os", "os/exec", "io/fs", "io/ioutil", "path/filepath", "syscall", "net"}

type boundaryRule struct {
	pkg    string
	reason string
	// allowed lists the only rotari packages pkg may import; nil means any.
	allowed []string
	// forbidden lists other imports pkg must not have.
	forbidden []string
}

// boundaryRules mirrors "Go package boundaries" in
// docs/contracts/00-overview.md and the package table in
// docs/ARCHITECTURE.md. Keep them in sync.
var boundaryRules = []boundaryRule{
	{
		pkg:       "internal/model",
		reason:    "model holds domain types and pure rules with no I/O",
		allowed:   []string{},
		forbidden: fileAccess,
	},
	{
		pkg:     "internal/state",
		reason:  "state owns file layout and makes no execution-policy decisions",
		allowed: []string{"internal/model"},
	},
	{
		pkg:     "internal/executor",
		reason:  "executors implement job execution only, with no run semantics",
		allowed: []string{"internal/model", "internal/state"},
	},
	{
		pkg:       "internal/run",
		reason:    "run owns run rules without file access",
		allowed:   []string{"internal/model", "internal/executor"},
		forbidden: fileAccess,
	},
	{
		pkg:       "internal/rundiff",
		reason:    "rundiff compares loaded runs and never reads state files",
		allowed:   []string{"internal/model"},
		forbidden: fileAccess,
	},
}

func TestPackageBoundaries(t *testing.T) {
	imports := listImports(t)

	for _, rule := range boundaryRules {
		pkgImports, ok := imports[module+"/"+rule.pkg]
		if !ok {
			t.Errorf("%s: package not found; update the rule if it moved", rule.pkg)
			continue
		}
		for _, imported := range pkgImports {
			if violatesRule(rule, imported) {
				t.Errorf("%s imports %s, but %s", rule.pkg, imported, rule.reason)
			}
		}
	}
}

func TestInternalPackagesDoNotImportCommands(t *testing.T) {
	for pkg, pkgImports := range listImports(t) {
		if !strings.HasPrefix(pkg, module+"/internal/") {
			continue
		}
		for _, imported := range pkgImports {
			if strings.HasPrefix(imported, module+"/cmd/") {
				t.Errorf("%s imports %s; shared behavior moves down into internal packages", pkg, imported)
			}
		}
	}
}

func violatesRule(rule boundaryRule, imported string) bool {
	if local, ok := strings.CutPrefix(imported, module+"/"); ok {
		return rule.allowed != nil && !slices.Contains(rule.allowed, local)
	}
	for _, forbidden := range rule.forbidden {
		if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
			return true
		}
	}
	return false
}

// listImports returns the non-test imports of every package in the module.
func listImports(t *testing.T) map[string][]string {
	t.Helper()
	output, err := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", module+"/...").Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list: %v\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	imports := make(map[string][]string)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			imports[fields[0]] = fields[1:]
		}
	}
	return imports
}
