package run

import (
	"strconv"
)

func WorkerArgs(options Options, baseDirExplicit bool, settingNames []string) []string {
	args := []string{"__worker-run"}
	if baseDirExplicit {
		args = append(args, "--basedir", options.BaseDir)
	}
	if options.Executor != "" {
		args = append(args, "--executor", options.Executor)
	}
	for _, option := range options.ExecutorOptions {
		args = append(args, "--executor-option", option)
	}
	for _, name := range settingNames {
		setting := options.ExecutorSettings[name]
		if setting.Concurrency > 0 {
			args = append(args, "--"+name+"-concurrency", strconv.Itoa(setting.Concurrency))
		}
		for _, option := range setting.Options {
			args = append(args, "--"+name+"-options", option)
		}
		if setting.SubmitInterval > 0 {
			args = append(args, "--"+name+"-submit-interval", setting.SubmitInterval.String())
		}
		if setting.SubmitRetryLimit > 0 {
			args = append(args, "--"+name+"-submit-retry-limit", strconv.Itoa(setting.SubmitRetryLimit))
		}
	}
	if options.Selection != "" {
		args = append(args, "--selection", options.Selection)
	}
	for _, jobID := range options.JobIDs {
		args = append(args, "--job-id", jobID)
	}
	if options.SourceRunID != "" {
		args = append(args, "--source-run-id", options.SourceRunID)
	}
	args = append(args, "--partial-array", strconv.FormatBool(options.PartialArray))
	args = append(args, options.QueueName, options.RunID, options.RunName, strconv.Itoa(options.LocalConcurrency), strconv.Itoa(options.BatchMaxActive), strconv.Itoa(options.Retry), options.CWD)
	return args
}
