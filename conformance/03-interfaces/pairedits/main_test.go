// Package pairedits runs the restored-state flag-pair checks for commands that
// change a project's queue or run history. It is separate from the other
// interface checks so each package stays well within Go's default test time
// limit; see ../flag-pair-coverage.md.
package pairedits

import (
	"os"
	"regexp"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMain(m *testing.M) { os.Exit(support.Run(m)) }

func covers(t *testing.T, _ ...string) { t.Helper() }

type (
	pairFlag           = support.PairFlag
	pairSavedTree      = support.PairSavedTree
	pairMutationResult = support.PairMutationResult
)

var (
	commandFlagPairs  = support.CommandFlagPairs
	readPairSchema    = support.ReadPairSchema
	pairAdapter       = support.PairAdapter
	pairInvoke        = support.PairInvoke
	assertPairOutcome = support.AssertPairOutcome
	pairHasFlag       = support.PairHasFlag
	savePairTree      = support.SavePairTree
	pairIntersect     = support.PairIntersect
)

// pairMutationFixture adds this package's edit adapters to the shared fixture.
type pairMutationFixture struct {
	support.PairMutationFixture
}

func newPairMutationFixture(t *testing.T) pairMutationFixture {
	t.Helper()
	return pairMutationFixture{support.NewPairMutationFixture(t)}
}

func pairMutationRevision(t *testing.T, e *support.Env) string {
	t.Helper()
	return support.PairMutationRevision(t, e)
}

var revisionLine = regexp.MustCompile(`(?m)^revision=([0-9a-f]+)$`)

func revisionOf(t *testing.T, result support.Result) string {
	t.Helper()
	match := revisionLine.FindStringSubmatch(result.Stdout)
	if match == nil {
		t.Fatalf("no revision line: %s", result)
	}
	return match[1]
}
