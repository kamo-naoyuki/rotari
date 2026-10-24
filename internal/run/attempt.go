package run

import (
	"github.com/rotari-dev/rotari/internal/executor"
)

func configuredExecutor(jobExecutor executor.JobExecutor, settings executor.RunSettings) executor.JobExecutor {
	if configurer, ok := jobExecutor.(executor.RunSettingsConfigurer); ok {
		return configurer.WithRunSettings(settings)
	}
	return jobExecutor
}
