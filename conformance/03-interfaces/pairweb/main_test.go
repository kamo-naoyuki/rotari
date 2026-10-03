// Package pairweb exercises static Web exports and keeps live-server processes
// outside the generic flag-pair matrix.
package pairweb

import (
	"os"
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

func TestMain(m *testing.M) { os.Exit(support.Run(m)) }

func covers(t *testing.T, _ ...string) { t.Helper() }

type (
	pairFlag    = support.PairFlag
	pairCommand = support.PairCommand
	flagPair    = support.FlagPair
	pairFixture = support.PairFixture
)

var (
	commandFlagPairs = support.CommandFlagPairs
	readPairSchema   = support.ReadPairSchema
	pairInvoke       = support.PairInvoke
	pairHasFlag      = support.PairHasFlag
	newPairFixture   = support.NewPairFixture
)
