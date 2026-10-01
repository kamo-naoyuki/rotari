package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kamo-naoyuki/rotari/internal/executor"
)

type schedulerContainerTestConfig struct {
	executor  string
	container string
	user      string
	setup     string
}

func TestSchedulerContainerSGEAccounting(t *testing.T) {
	config := requireSchedulerContainerTest(t)
	if config.executor != "sge" {
		t.Skip("SGE container test")
	}
	project := newContainerProject(t)
	project.add("--job-name", "accounted", "--", "true")
	if code := project.run(); code != 0 {
		t.Fatalf("run exit = %d, want success", code)
	}
	jobID := project.jobIDs()["accounted"]
	metadata := project.attemptMetadata(jobID)
	schedulerJobID, ok := metadata["sge_job_id"].(string)
	if !ok || schedulerJobID == "" {
		t.Fatalf("job metadata = %#v, want an SGE job ID", metadata)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		output, code := project.shell(30*time.Second, "qacct -j "+executor.ShellQuote(schedulerJobID))
		if code == 0 && accountingField(output, "exit_status") != "" && accountingField(output, "failed") != "" {
			if accountingField(output, "exit_status") != "0" || accountingField(output, "failed") != "0" {
				t.Fatalf("qacct -j %s returned unexpected accounting:\n%s", schedulerJobID, output)
			}
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("qacct did not publish accounting for job %s before timeout", schedulerJobID)
}

func accountingField(output, name string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == name {
			return fields[1]
		}
	}
	return ""
}

func requireSchedulerContainerTest(t *testing.T) schedulerContainerTestConfig {
	t.Helper()
	if os.Getenv("ROTARI_SCHEDULER_CONTAINER_TEST") != "1" {
		t.Skip("set ROTARI_SCHEDULER_CONTAINER_TEST=1 to run scheduler container tests")
	}
	config := schedulerContainerTestConfig{
		executor:  os.Getenv("ROTARI_SCHEDULER_EXECUTOR"),
		container: os.Getenv("SCHEDULER_CONTAINER"),
		user:      os.Getenv("SCHEDULER_USER"),
		setup:     os.Getenv("SCHEDULER_SETUP"),
	}
	if config.executor == "" || config.container == "" {
		t.Fatalf("ROTARI_SCHEDULER_EXECUTOR and SCHEDULER_CONTAINER must be set")
	}
	return config

}

func TestSchedulerContainerSlurmArrayTaskVariable(t *testing.T) {
	config := requireSchedulerContainerTest(t)
	if config.executor != "slurm" {
		t.Skip("Slurm container test")
	}

	marker := fmt.Sprintf("/state/rotari-slurm-array-%d.out", time.Now().UnixNano())
	wrapped := fmt.Sprintf("printf '%%s\\n' \"$SLURM_ARRAY_TASK_ID\" >> %s", executor.ShellQuote(marker))
	runSchedulerContainerCommand(t, config, fmt.Sprintf("rm -f %s; sbatch --parsable --array=1-2 --wrap %s", executor.ShellQuote(marker), executor.ShellQuote(wrapped)))

	assertSchedulerContainerFileLines(t, config, marker, []string{"1", "2"})
}

func TestSchedulerContainerPBSArrayTaskVariable(t *testing.T) {
	config := requireSchedulerContainerTest(t)
	if config.executor != "pbs" {
		t.Skip("PBS container test")
	}

	marker := fmt.Sprintf("/state/rotari-pbs-array-%d.out", time.Now().UnixNano())
	scriptPath := fmt.Sprintf("/state/rotari-pbs-array-%d.sh", time.Now().UnixNano())
	runSchedulerContainerCommand(t, config, fmt.Sprintf(`cat > %s <<'ROTARI_SCRIPT'
#!/bin/sh
printf '%%s\n' "$PBS_ARRAY_INDEX" >> %s
ROTARI_SCRIPT
chmod +x %s
rm -f %s
qsub -j oe -J 1-2 %s`, executor.ShellQuote(scriptPath), executor.ShellQuote(marker), executor.ShellQuote(scriptPath), executor.ShellQuote(marker), executor.ShellQuote(scriptPath)))

	assertSchedulerContainerFileLines(t, config, marker, []string{"1", "2"})
}

func assertSchedulerContainerFileLines(t *testing.T, config schedulerContainerTestConfig, path string, want []string) {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for {
		output := runSchedulerContainerCommand(t, config, fmt.Sprintf("cat %s 2>/dev/null || true", executor.ShellQuote(path)))
		got := nonEmptySortedLines(output)
		if reflect.DeepEqual(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("container file %s lines = %#v, want %#v", path, got, want)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func nonEmptySortedLines(output string) []string {
	lines := strings.Split(output, "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			values = append(values, line)
		}
	}
	sort.Strings(values)
	return values
}

func runSchedulerContainerCommand(t *testing.T, config schedulerContainerTestConfig, script string) string {
	t.Helper()
	args := []string{"exec"}
	if config.user != "" {
		args = append(args, "--user", config.user)
	}
	if config.setup != "" {
		script = config.setup + "\n" + script
	}
	args = append(args, config.container, "sh", "-lc", script)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("docker exec timed out: %s", script)
	}
	if err != nil {
		t.Fatalf("docker exec failed: %v\nscript: %s\noutput: %s", err, script, output)
	}
	return string(output)
}
