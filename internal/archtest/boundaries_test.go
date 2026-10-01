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
	// standardOnly forbids imports from outside the standard library.
	standardOnly bool
	// withTests applies the rule to the package's test files too.
	withTests bool
}

// boundaryRules mirrors "Go package boundaries" in
// contracts/00-overview.md and the package table in
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
		pkg:       "internal/jobfilter",
		reason:    "jobfilter evaluates job filters from facts its callers supply, without file access",
		allowed:   []string{"internal/model"},
		forbidden: fileAccess,
	},
	{
		pkg:       "internal/run",
		reason:    "run owns run rules without file access",
		allowed:   []string{"internal/model", "internal/executor", "internal/jobfilter"},
		forbidden: fileAccess,
	},
	{
		pkg:       "internal/runlineage",
		reason:    "runlineage compares loaded runs and never reads state files",
		allowed:   []string{"internal/model"},
		forbidden: fileAccess,
	},
	{
		pkg:          "conformance",
		reason:       "conformance checks the binary and Web API from outside the code",
		allowed:      []string{},
		standardOnly: true,
		withTests:    true,
	},
}

func TestPackageBoundaries(t *testing.T) {
	imports := listImports(t, false)
	withTests := listImports(t, true)

	for _, rule := range boundaryRules {
		listed := imports
		if rule.withTests {
			listed = withTests
		}
		pkgImports, ok := listed[module+"/"+rule.pkg]
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
	for pkg, pkgImports := range listImports(t, false) {
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
	if rule.standardOnly && strings.Contains(strings.Split(imported, "/")[0], ".") {
		return true
	}
	for _, forbidden := range rule.forbidden {
		if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
			return true
		}
	}
	return false
}

// listImports returns the imports of every package in the module, adding
// the imports of its test files when withTests is set.
func listImports(t *testing.T, withTests bool) map[string][]string {
	t.Helper()
	format := `{{.ImportPath}} {{join .Imports " "}}`
	if withTests {
		format += ` {{join .TestImports " "}} {{join .XTestImports " "}}`
	}
	output, err := exec.Command("go", "list", "-f", format, module+"/...").Output()
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
