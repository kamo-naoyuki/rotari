// Package pairjobcontrol runs the schema-driven flag-pair checks for cancel,
// suspend, and resume against live local runs of sleeping jobs. Suspend and
// resume share one run whose job states are reset before every invocation;
// each cancel that is expected to act gets a fresh run. Every run is cancelled
// and its processes are reaped when its test ends. It is a separate package so
// each interface package stays within Go's default test time limit; see
// ../flag-pair-coverage.md.
package pairjobcontrol

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
)

var (
	commandFlagPairs = support.CommandFlagPairs
	readPairSchema   = support.ReadPairSchema
	pairInvoke       = support.PairInvoke
)
