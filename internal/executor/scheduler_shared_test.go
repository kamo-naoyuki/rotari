package executor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func TestSplitShellWords(t *testing.T) {
	got, err := splitShellWords(`-p "short queue" --constraint='fast\ node' --exclusive`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-p", "short queue", "--constraint=fast\\ node", "--exclusive"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("word %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitShellWordsHandlesQuotingStyles(t *testing.T) {
	words, err := splitShellWords(`--partition "gpu queue" --constraint='a b' escaped\ value`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--partition", "gpu queue", "--constraint=a b", "escaped value"}
	if len(words) != len(want) {
		t.Fatalf("got %#v, want %#v", words, want)
	}
	for i := range want {
		if words[i] != want[i] {
			t.Errorf("word %d: got %q, want %q", i, words[i], want[i])
		}
	}
}

func TestSplitShellWordsRejectsUnterminatedInput(t *testing.T) {
	for _, input := range []string{`"unterminated`, `trailing\`, `unterminated'`} {
		if _, err := splitShellWords(input); err == nil {
			t.Errorf("splitShellWords(%q) returned nil error", input)
		}
	}
}

func TestExpandShellOptions(t *testing.T) {
	expanded, err := ExpandShellOptions([]string{"-p gpu", "--cpus-per-task=2"})
	want := []string{"-p", "gpu", "--cpus-per-task=2"}
	if err != nil || len(expanded) != len(want) {
		t.Fatalf("ExpandShellOptions = %#v, %v", expanded, err)
	}
	for i := range want {
		if expanded[i] != want[i] {
			t.Fatalf("ExpandShellOptions = %#v, want %#v", expanded, want)
		}
	}
}

func TestSlurmArrayWrapperWritesFinishedTaskStatus(t *testing.T) {
	store := testStore()
	for _, testCase := range []struct {
		name         string
		taskVariable string
	}{
		{name: "slurm", taskVariable: "SLURM_ARRAY_TASK_ID"},
		{name: "pbs", taskVariable: "PBS_ARRAY_INDEX"},
		{name: "lsf", taskVariable: "LSB_JOBINDEX"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runDir := t.TempDir()
			task := 1
			jobDir := filepath.Join(runDir, "array-1")
			job := model.JobSpec{
				ID: "array-1", ArrayGroup: "array", ArrayTaskID: &task, ArrayFirst: 1, ArrayLast: 1,
				Command: []string{"sh", "-c", "printf task-output; exit 0"},
				Environment: []string{
					"ROTARI_ARRAY_TASK_ID=1",
					model.EnvJobDir + "=" + jobDir,
				},
			}
			wrapper := filepath.Join(runDir, "wrapper.sh")
			if err := os.WriteFile(wrapper, []byte(schedulerArrayWrapperScript([]model.JobSpec{job}, testCase.taskVariable)), 0o755); err != nil {
				t.Fatal(err)
			}
			command := exec.Command("sh", wrapper)
			command.Env = append(os.Environ(), testCase.taskVariable+"=1")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("wrapper failed: %v, output=%s", err, output)
			}
			status, ok := LoadWrapperStatus(store, filepath.Join(jobDir, "status.json"))
			if !ok || status.Phase != "finished" || status.ExitCode != 0 {
				t.Fatalf("status = %#v, ok=%v", status, ok)
			}
			output, err := os.ReadFile(filepath.Join(jobDir, "output"))
			if err != nil || string(output) != "task-output" {
				t.Fatalf("output = %q, err=%v; want task output in job directory", output, err)
			}
		})
	}
}
