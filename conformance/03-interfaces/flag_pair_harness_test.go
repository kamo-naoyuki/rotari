package interfaces

import (
	"testing"

	"github.com/kamo-naoyuki/rotari/conformance/support"
)

// The flag-pair harness lives in conformance/support so the long pair suites
// can run in their own packages. These names keep this package's read-only,
// file-output, and projection checks independent of that split.
type (
	pairFlag    = support.PairFlag
	pairCommand = support.PairCommand
	flagPair    = support.FlagPair
)

var (
	pairAdapter       = support.PairAdapter
	commandFlagPairs  = support.CommandFlagPairs
	readPairSchema    = support.ReadPairSchema
	pairInvoke        = support.PairInvoke
	assertPairOutcome = support.AssertPairOutcome
	pairHasFlag       = support.PairHasFlag
)

// pairFixture exposes the shared finished-run fixture under the field names
// this package's checks use.
type pairFixture struct {
	support.PairFixture
	e                                *support.Env
	project, run, bad, array, config string
	jobs                             []string
}

func newPairFixture(t *testing.T) pairFixture {
	t.Helper()
	f := support.NewPairFixture(t)
	return pairFixture{PairFixture: f, e: f.E, project: f.Project, run: f.Run, bad: f.Bad, array: f.Array, config: f.Config, jobs: f.Jobs}
}

func (f pairFixture) sample(t *testing.T, flag pairFlag) []string {
	t.Helper()
	return f.Sample(t, flag)
}
