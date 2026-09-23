package run

import (
	"fmt"
	"strings"

	"github.com/kamo-naoyuki/rotari/internal/executor"
	"github.com/kamo-naoyuki/rotari/internal/model"
)

type EnvironmentNames struct {
	BaseDir, ProjectName, RunID, JobID, Executor, Bin, RunDir, JobDir, CWD string
	JobName, ArrayTaskID, ArrayFirst, ArrayLast, ArraySize                 string
	RunName, LocalConcurrency, BatchConcurrency, Retry, ExecutorOptions    string
}

type EnvironmentConfig struct {
	Names            EnvironmentNames
	BaseDir          string
	ProjectName      string
	RunID            string
	RunDir           string
	RunName          string
	Bin              string
	CWD              string
	LocalConcurrency int
	BatchConcurrency int
	Retry            int
	ExecutorOptions  []string
	Inherited        map[string]string
	JobDir           func(runDir string, job model.JobSpec) (string, error)
}

func PrepareJobEnvironments(jobs []model.JobSpec, config EnvironmentConfig) {
	for index := range jobs {
		job := &jobs[index]
		jobDir, err := config.JobDir(config.RunDir, *job)
		if err != nil {
			continue
		}
		environment := []string{
			config.Names.BaseDir + "=" + config.BaseDir,
			config.Names.ProjectName + "=" + config.ProjectName,
			config.Names.RunID + "=" + config.RunID,
			config.Names.JobID + "=" + job.ID,
			config.Names.Executor + "=" + job.Executor,
			config.Names.Bin + "=" + config.Bin,
			config.Names.RunDir + "=" + config.RunDir,
			config.Names.JobDir + "=" + jobDir,
			config.Names.CWD + "=" + config.CWD,
		}
		if job.Name != "" {
			environment = append(environment, config.Names.JobName+"="+job.Name)
		}
		if job.ArrayTaskID != nil {
			environment = append(environment,
				fmt.Sprintf("%s=%d", config.Names.ArrayTaskID, *job.ArrayTaskID),
				fmt.Sprintf("%s=%d", config.Names.ArrayFirst, job.ArrayFirst),
				fmt.Sprintf("%s=%d", config.Names.ArrayLast, job.ArrayLast),
				fmt.Sprintf("%s=%d", config.Names.ArraySize, job.ArraySize),
			)
		}
		if config.RunName != "" {
			environment = append(environment, config.Names.RunName+"="+config.RunName)
		}
		environment = append(environment,
			fmt.Sprintf("%s=%d", config.Names.LocalConcurrency, config.LocalConcurrency),
			fmt.Sprintf("%s=%d", config.Names.BatchConcurrency, config.BatchConcurrency),
			fmt.Sprintf("%s=%d", config.Names.Retry, config.Retry),
		)
		if len(config.ExecutorOptions) > 0 {
			environment = append(environment, config.Names.ExecutorOptions+"="+strings.Join(config.ExecutorOptions, " "))
		}
		for name, value := range config.Inherited {
			if _, exists := EnvironmentEntry(environment, name); !exists {
				environment = append(environment, name+"="+value)
			}
		}
		job.Environment = executor.MergeEnvironment(job.Environment, environment)
	}
}
