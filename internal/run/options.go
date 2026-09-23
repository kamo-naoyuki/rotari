package run

import "github.com/kamo-naoyuki/rotari/internal/executor"

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
	SourceRunID      string
	PartialArray     bool
	CWD              string
	OnDone           func()
	ExecutorSettings map[string]executor.RunSettings
}
