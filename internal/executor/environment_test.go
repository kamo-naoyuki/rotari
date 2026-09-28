package executor

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestLSFExplicitEnvironmentVariables(t *testing.T) {
	for _, name := range []string{"HOME", "USER", "PWD", "TERM", "TERMCAP", "LSB_JOBID", "LSB_SUB_USER_DEFINED"} {
		if !LSFExplicitEnvironmentVariable(name) {
			t.Errorf("LSFExplicitEnvironmentVariable(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"PATH", "CUDA_VISIBLE_DEVICES", "ROTARI_JOB_ID"} {
		if LSFExplicitEnvironmentVariable(name) {
			t.Errorf("LSFExplicitEnvironmentVariable(%q) = true, want false", name)
		}
	}
}

func TestSchedulerJobCommandLineClearsOnlyForNone(t *testing.T) {
	command := []string{"sh", "-c", "printf %s \"$ROTARI_JOB_ID\""}
	all := schedulerJobCommandLine(model.JobSpec{EnvMode: model.EnvModeAll, Command: command})
	if strings.HasPrefix(all, "env -i ") {
		t.Fatalf("ALL command was sanitized: %s", all)
	}
	none := schedulerJobCommandLine(model.JobSpec{EnvMode: model.EnvModeNone, Command: command, Environment: []string{"ROTARI_JOB_ID=job-1"}})
	if !strings.HasPrefix(none, "env -i ") || !strings.Contains(none, "ROTARI_JOB_ID=job-1") {
		t.Fatalf("NONE command = %s; want clean env with explicit Rotari metadata", none)
	}
}
