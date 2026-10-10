// Package pairruns runs the flag-pair checks for run, retry, unlock, wait,
// and note. Run and retry use dry-run previews; unlock and wait use synthetic
// locks over a finished fixture, and note adds notes to it, so no job or
// scheduler is started. It is separate from
// the other interface checks so each package stays well within Go's default
// test time limit; see ../flag-pair-coverage.md.
package pairruns

import (
	"os"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMain(m *testing.M) { os.Exit(support.Run(m)) }

func covers(t *testing.T, _ ...string) { t.Helper() }

type (
	pairFlag            = support.PairFlag
	pairCommand         = support.PairCommand
	flagPair            = support.FlagPair
	pairFixture         = support.PairFixture
	pairSavedTree       = support.PairSavedTree
	pairMutationFixture = support.PairMutationFixture
	pairMutationResult  = support.PairMutationResult
)

var (
	commandFlagPairs       = support.CommandFlagPairs
	readPairSchema         = support.ReadPairSchema
	pairInvoke             = support.PairInvoke
	assertPairOutcome      = support.AssertPairOutcome
	pairHasFlag            = support.PairHasFlag
	savePairTree           = support.SavePairTree
	pairIntersect          = support.PairIntersect
	newPairFixture         = support.NewPairFixture
	newPairMutationFixture = support.NewPairMutationFixture
)
