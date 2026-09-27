package conformance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Contract IDs tie the rules in contracts/ to the tests here. A rule
// gets an ID by starting with "**PREFIX-N**"; contracts/README.md lists every
// ID with a status; a test declares what it checks with covers(t, ID...).
// TestContractStatus keeps the three in agreement.

var (
	contractDefinition = regexp.MustCompile(`(?m)^(?:- )?\*\*([A-Z]+-[0-9]+)\*\* `)
	contractRow        = regexp.MustCompile(`^\| ([A-Z]+-[0-9]+) \| .+ \| ([a-z]+) \| (.+) \|$`)
	contractTestName   = regexp.MustCompile("`(Test[A-Za-z0-9_]+)`")
)

// contractStatuses are the statuses a row may have. "conformance" means the
// listed tests check the whole rule, "partial" that they check part of it,
// "pending" that no conformance test checks it yet, and "deviation" that
// rotari knowingly breaks it, as recorded in ISSUES.md.
var contractStatuses = []string{"conformance", "partial", "pending", "deviation"}

type contractStatusRow struct {
	status string
	tests  []string
}

func TestContractStatus(t *testing.T) {
	defined := contractDefinitions(t)
	rows := contractStatusRows(t)
	coveredBy := coveringTests(t)
	issues := readRepoFile(t, "ISSUES.md")

	for id, file := range defined {
		if _, ok := rows[id]; !ok {
			t.Errorf("%s (in %s) has no row in the contracts/README.md status table", id, file)
		}
	}
	for id, row := range rows {
		if _, ok := defined[id]; !ok {
			t.Errorf("status table lists %s, which no contract in contracts/ defines", id)
			continue
		}
		tests := coveredBy[id]
		if !slices.Equal(row.tests, tests) {
			t.Errorf("%s: status table lists tests %q, but covers() calls name %q", id, row.tests, tests)
		}
		switch row.status {
		case "conformance", "partial":
			if len(tests) == 0 {
				t.Errorf("%s: status %q needs a conformance test that calls covers(t, %q)", id, row.status, id)
			}
		case "pending":
			if len(tests) > 0 {
				t.Errorf("%s: status pending, but %q cover it; mark it partial or conformance", id, tests)
			}
		case "deviation":
			if !strings.Contains(issues, id) {
				t.Errorf("%s: status deviation needs an ISSUES.md entry that names it", id)
			}
		default:
			t.Errorf("%s: unknown status %q; use one of %q", id, row.status, contractStatuses)
		}
	}
	for id, tests := range coveredBy {
		if _, ok := defined[id]; !ok {
			t.Errorf("%q call covers(t, %q), which no contract defines", tests, id)
		}
	}
}

// contractDefinitions returns each contract ID with the file defining it.
func contractDefinitions(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "contracts", "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no contract files found: %v", err)
	}
	defined := map[string]string{}
	for _, file := range files {
		if filepath.Base(file) == "README.md" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range contractDefinition.FindAllStringSubmatch(string(data), -1) {
			id := match[1]
			if previous, ok := defined[id]; ok {
				t.Errorf("%s is defined more than once (%s, %s)", id, previous, filepath.Base(file))
			}
			defined[id] = filepath.Base(file)
		}
	}
	return defined
}

// contractStatusRows reads the status table in contracts/README.md: rows of
// "| ID | rule | status | tests |", with tests as backquoted names or "-".
func contractStatusRows(t *testing.T) map[string]contractStatusRow {
	t.Helper()
	rows := map[string]contractStatusRow{}
	for _, line := range strings.Split(readRepoFile(t, "contracts/README.md"), "\n") {
		match := contractRow.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		id := match[1]
		if _, ok := rows[id]; ok {
			t.Errorf("status table lists %s twice", id)
		}
		var tests []string
		for _, name := range contractTestName.FindAllStringSubmatch(match[3], -1) {
			tests = append(tests, name[1])
		}
		slices.Sort(tests)
		rows[id] = contractStatusRow{status: match[2], tests: tests}
	}
	if len(rows) == 0 {
		t.Fatal("contracts/README.md has no contract status rows")
	}
	return rows
}

// coveringTests returns, for each contract ID, the sorted names of the test
// functions in this package that call covers with it.
func coveringTests(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	coveredBy := map[string][]string{}
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Body == nil {
				continue
			}
			for _, id := range coversCalls(t, fset, fn.Body) {
				if !slices.Contains(coveredBy[id], fn.Name.Name) {
					coveredBy[id] = append(coveredBy[id], fn.Name.Name)
				}
			}
		}
	}
	for id := range coveredBy {
		slices.Sort(coveredBy[id])
	}
	return coveredBy
}

func coversCalls(t *testing.T, fset *token.FileSet, body *ast.BlockStmt) []string {
	t.Helper()
	var ids []string
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); !ok || ident.Name != "covers" {
			return true
		}
		for _, arg := range call.Args[1:] {
			literal, ok := arg.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Errorf("%s: covers takes contract IDs as string literals", fset.Position(arg.Pos()))
				continue
			}
			id, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		return true
	})
	return ids
}

func readRepoFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
