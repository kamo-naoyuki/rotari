package run

import (
	"github.com/kamo-naoyuki/rotari/internal/executor"
)

func configuredExecutor(jobExecutor executor.JobExecutor, settings executor.RunSettings) executor.JobExecutor {
	if configurer, ok := jobExecutor.(executor.RunSettingsConfigurer); ok {
		return configurer.WithRunSettings(settings)
	}
	return jobExecutor
}

func effectiveConcurrency(settings executor.RunSettingsMap, name string, fallback int) int {
	if settings != nil && settings[name].Concurrency > 0 {
		return settings[name].Concurrency
	}
	return fallback
}

func effectiveOptions(settings executor.RunSettingsMap, name string, fallback []string) []string {
	if settings != nil && len(settings[name].Options) > 0 {
		return settings[name].Options
	}
	return fallback
}
