package run

import (
	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

type Options struct {
	BaseDir          string
	QueueName        string
	RunID            string
	RunName          string
	LocalConcurrency int
	BatchMaxActive   int
	Retry            int
	Executor         string
	ExecutorOptions  []string
	Selection        string
	JobIDs           []string
	// Scope narrows Selection to one stage or matrix; see PlanRerun.
	Scope            model.CommandSelector
	SourceRunID      string
	PartialArray     bool
	CWD              string
	OnDone           func()
	ExecutorSettings map[string]executor.RunSettings
}
