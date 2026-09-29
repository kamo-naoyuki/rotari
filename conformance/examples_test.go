package conformance

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type exampleTestCase struct {
	name      string
	want      []string
	artifacts []string
}

func TestExamplesRun(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate conformance test source")
	}
	examplesDir := filepath.Join(filepath.Dir(source), "..", "examples")
	pathEnv := filepath.Dir(rotariBin) + string(os.PathListSeparator) + os.Getenv("PATH")

	// The Slurm and LLM examples need external infrastructure or credentials.
	tests := []exampleTestCase{
		{name: "basic.sh", want: []string{"Success: 2"}},
		{name: "array.sh", want: []string{"Success: 3"}},
		{name: "matrix.sh", want: []string{"Success: 4"}},
		{name: "retry.sh", want: []string{"Success: 1"}},
		{name: "async.sh", want: []string{"Success: 1"}},
		{name: "workflow.sh", want: []string{"Exported workflow:"}, artifacts: []string{"exported.yaml"}},
		{name: "workflow-reconcile.sh", want: []string{"Example state:"}, artifacts: []string{"exported.yaml", "reconciled.yaml"}},
		{name: "diagnose-rules.sh", want: []string{"Python import or module missing", "Example state:"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runExample(t, examplesDir, pathEnv, tt)
		})
	}
}

func runExample(t *testing.T, examplesDir, pathEnv string, tt exampleTestCase) {
	t.Helper()
	e := newEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", filepath.Join(examplesDir, tt.name))
	cmd.Dir = e.root
	cmd.Env = e.withVar("PATH", pathEnv).vars
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("example failed: %v\noutput:\n%s", err, output)
	}
	for _, want := range tt.want {
		if !strings.Contains(string(output), want) {
			t.Errorf("example output does not contain %q\noutput:\n%s", want, output)
		}
	}
	for _, artifact := range tt.artifacts {
		artifactPath := filepath.Join(e.root, ".example-state", artifact)
		if info, err := os.Stat(artifactPath); err != nil || info.Size() == 0 {
			t.Errorf("example did not produce non-empty %s: %v", artifact, err)
		}
	}
}
