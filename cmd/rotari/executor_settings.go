package main

import (
	"flag"
)

// ExecutorRunSettings controls the dispatch defaults for one executor.
// Job-specific options remain higher priority than these values.
type ExecutorRunSettings struct {
	Concurrency int      `json:"concurrency,omitempty"`
	Options     []string `json:"options,omitempty"`
}

type executorRunSettingsMap map[string]ExecutorRunSettings

var executorRunSettingNames = []string{"ssh", "slurm", "pbs", "lsf"}

func cliExecutorRunSettings(fs *flag.FlagSet) executorRunSettingsMap {
	settings := make(executorRunSettingsMap)
	for _, name := range executorRunSettingNames {
		concurrency := cliInt(fs, name+"-concurrency", 0)
		var options stringSliceFlag
		cliValue(fs, &options, name+"-options")
		settings[name] = ExecutorRunSettings{Concurrency: *concurrency, Options: options}
	}
	return settings
}

func executorSettingsFor(settings executorRunSettingsMap, name string) ExecutorRunSettings {
	if settings == nil {
		return ExecutorRunSettings{}
	}
	return settings[name]
}

func effectiveExecutorConcurrency(settings executorRunSettingsMap, name string, fallback int) int {
	if concurrency := executorSettingsFor(settings, name).Concurrency; concurrency > 0 {
		return concurrency
	}
	return fallback
}

func effectiveExecutorOptions(settings executorRunSettingsMap, name string, fallback []string) []string {
	if options := executorSettingsFor(settings, name).Options; len(options) > 0 {
		return options
	}
	return fallback
}
