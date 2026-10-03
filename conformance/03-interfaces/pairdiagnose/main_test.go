// Package pairdiagnose runs schema-driven diagnose flag checks against local
// rules or an in-process fake LLM endpoint.
package pairdiagnose

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
	pairFixture = support.PairFixture
)

var (
	commandFlagPairs  = support.CommandFlagPairs
	readPairSchema    = support.ReadPairSchema
	pairInvoke        = support.PairInvoke
	assertPairOutcome = support.AssertPairOutcome
	newPairFixture    = support.NewPairFixture
	savePairTree      = support.SavePairTree
)

// These tests exercise diagnostics, not mutations or live provider access.
// Every provider request is routed through the suite's local fake endpoint.
// The package split keeps the 55-pair matrix under Go's package test timeout.
// The suite executes the full matrix in normal and race modes.
