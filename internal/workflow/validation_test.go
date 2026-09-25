package workflow

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
)

func sequentialIDs() func() string {
	next := 0
	return func() string {
		next++
		return fmt.Sprintf("job-%d", next)
	}
}

func TestEquivalentCommandComparesExecutionDefinition(t *testing.T) {
	base := model.QueuedCommand{
		ID: "left", Name: "train", Command: []string{"train", "--fast"}, Stage: "compute", DependsOn: []string{"prepare"},
		Executor: "slurm", ExecutorOptions: []string{"--partition=gpu"}, WorkingDirectory: "work",
		Environment: []string{"A=1", "A=2"}, Array: &model.ArraySpec{First: 1, Last: 3, Tasks: []int{1, 3}},
	}
	clone := func() model.QueuedCommand {
		command := base
		command.Command = append([]string(nil), base.Command...)
		command.DependsOn = append([]string(nil), base.DependsOn...)
		command.ExecutorOptions = append([]string(nil), base.ExecutorOptions...)
		command.Environment = append([]string(nil), base.Environment...)
		array := *base.Array
		array.Tasks = append([]int(nil), base.Array.Tasks...)
		command.Array = &array
		return command
	}
	same := clone()
	same.ID = "right"
	same.Origin = &model.JobOrigin{RunID: "run"}
	same.Force = true
	if !EquivalentCommand(base, same) {
		t.Fatal("commands differing only in runtime state are not equivalent")
	}
	tests := map[string]func(*model.QueuedCommand){
		"command":             func(command *model.QueuedCommand) { command.Command[1] = "--slow" },
		"name":                func(command *model.QueuedCommand) { command.Name = "other" },
		"stage":               func(command *model.QueuedCommand) { command.Stage = "" },
		"dependencies":        func(command *model.QueuedCommand) { command.DependsOn = nil },
		"executor":            func(command *model.QueuedCommand) { command.Executor = "local" },
		"executor options":    func(command *model.QueuedCommand) { command.ExecutorOptions = []string{"--partition=cpu"} },
		"working directory":   func(command *model.QueuedCommand) { command.WorkingDirectory = "other" },
		"environment order":   func(command *model.QueuedCommand) { command.Environment = []string{"A=2", "A=1"} },
		"environment removed": func(command *model.QueuedCommand) { command.Environment = command.Environment[:1] },
		"array tasks":         func(command *model.QueuedCommand) { command.Array.Tasks = []int{1, 2} },
		"array removed":       func(command *model.QueuedCommand) { command.Array = nil },
		"matrix added": func(command *model.QueuedCommand) {
			command.Matrix = &model.MatrixSpec{GroupID: "group", Dimensions: []model.MatrixDimension{{Name: "SEED", Values: []string{"1"}}}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := clone()
			mutate(&changed)
			if EquivalentCommand(base, changed) || EquivalentCommand(changed, base) {
				t.Fatalf("changed %s is considered equivalent", name)
			}
		})
	}
}

func TestEquivalentCommandComparesMatrixDefinitionButNotGroupID(t *testing.T) {
	matrix := func(groupID string, values []string, seed string) *model.MatrixSpec {
		return &model.MatrixSpec{
			GroupID: groupID, Dimensions: []model.MatrixDimension{{Name: "SEED", Values: values}},
			Values: []model.MatrixValue{{Name: "SEED", Value: seed}}, BaseName: "train", BaseEnvironment: []string{"A=1"},
		}
	}
	left := model.QueuedCommand{Command: []string{"train"}, Matrix: matrix("source", []string{"1", "2"}, "1")}
	right := model.QueuedCommand{Command: []string{"train"}, Matrix: matrix("destination", []string{"1", "2"}, "1")}
	if !EquivalentCommand(left, right) {
		t.Fatal("matrix commands differing only in group ID are not equivalent")
	}
	right.Matrix = matrix("destination", []string{"1", "2", "3"}, "1")
	if EquivalentCommand(left, right) {
		t.Fatal("changed matrix dimensions are considered equivalent")
	}
	right.Matrix = matrix("destination", []string{"1", "2"}, "2")
	if EquivalentCommand(left, right) {
		t.Fatal("different matrix combinations are considered equivalent")
	}
}

func TestDecodeRejectsTrailingDocumentsAndUnsupportedFormat(t *testing.T) {
	tests := []struct {
		format string
		input  string
	}{
		{format: "json", input: `{"version":1,"jobs":[{"command":["true"]}]} {"version":1}`},
		{format: "yaml", input: "version: 1\njobs:\n  - command: [true]\n---\nversion: 1\n"},
		{format: "yaml", input: "version: 1\njobs:\n  - command: [true]\n    unknown: 1\n"},
		{format: "toml", input: "version = 1\n[[jobs]]\ncommand = [\"true\"]\nunknown = 1\n"},
		{format: "xml", input: "<manifest/>"},
	}
	for _, test := range tests {
		if _, err := Decode(strings.NewReader(test.input), test.format); err == nil {
			t.Errorf("Decode(%s) accepted %q", test.format, test.input)
		}
	}
}

func TestValidateRejectsInconsistentSourceState(t *testing.T) {
	source := &Source{Project: "demo", RunIDs: []string{"run"}}
	tests := map[string]Manifest{
		"source without project":    {Version: 1, Source: &Source{RunIDs: []string{"run"}}, Jobs: []Job{{Command: []string{"true"}}}},
		"source without runs":       {Version: 1, Source: &Source{Project: "demo"}, Jobs: []Job{{Command: []string{"true"}}}},
		"status without source":     {Version: 1, Jobs: []Job{{Command: []string{"true"}, Status: "success"}}},
		"attempt without source":    {Version: 1, Jobs: []Job{{Command: []string{"true"}, AttemptID: "att_x"}}},
		"instances without source":  {Version: 1, Jobs: []Job{{Command: []string{"true"}, Array: "1", Instances: []Instance{{Status: "failed"}}}}},
		"instances on scalar job":   {Version: 1, Source: source, Jobs: []Job{{Command: []string{"true"}, Instances: []Instance{{Status: "failed"}}}}},
		"instance without status":   {Version: 1, Source: source, Jobs: []Job{{Command: []string{"true"}, Array: "1", Instances: []Instance{{}}}}},
		"instance invalid status":   {Version: 1, Source: source, Jobs: []Job{{Command: []string{"true"}, Array: "1", Instances: []Instance{{Status: "running"}}}}},
		"empty command executable":  {Version: 1, Jobs: []Job{{Command: []string{""}}}},
		"empty matrix values":       {Version: 1, Jobs: []Job{{Command: []string{"true"}, Matrix: []string{"SEED="}}}},
		"malformed matrix":          {Version: 1, Jobs: []Job{{Command: []string{"true"}, Matrix: []string{"SEED"}}}},
		"malformed array":           {Version: 1, Jobs: []Job{{Command: []string{"true"}, Array: "one"}}},
		"invalid environment entry": {Version: 1, Jobs: []Job{{Command: []string{"true"}, Environment: []string{"=value"}}}},
	}
	for name, manifest := range tests {
		t.Run(name, func(t *testing.T) {
			if err := Validate(manifest); err == nil {
				t.Fatal("Validate accepted an invalid manifest")
			}
		})
	}
	valid := Manifest{Version: 1, Source: source, Jobs: []Job{{Command: []string{"true"}, Array: "1-2", Status: "failed", Instances: []Instance{{Task: intPointer(2), Status: "success"}}}}}
	if err := Validate(valid); err != nil {
		t.Fatalf("Validate rejected a valid source manifest: %v", err)
	}
}

func intPointer(value int) *int {
	return &value
}

func TestCompileRejectsInvalidExpandedGraph(t *testing.T) {
	tests := map[string]Manifest{
		"unknown dependency": {Version: 1, Jobs: []Job{{Name: "a", Command: []string{"true"}, DependsOn: []string{"missing"}}}},
		"cycle": {Version: 1, Jobs: []Job{
			{Name: "a", Command: []string{"true"}, DependsOn: []string{"b"}},
			{Name: "b", Command: []string{"true"}, DependsOn: []string{"a"}},
		}},
		"self dependency": {Version: 1, Jobs: []Job{{Name: "a", Command: []string{"true"}, DependsOn: []string{"a"}}}},
		"expanded name collides with job": {Version: 1, Jobs: []Job{
			{Name: "train", Command: []string{"true"}, Matrix: []string{"SEED=1"}},
			{Name: "train-SEED1", Command: []string{"true"}},
		}},
		"sanitized matrix names collide": {Version: 1, Jobs: []Job{{Name: "train", Command: []string{"true"}, Matrix: []string{"MODEL=a/b,a_b"}}}},
		"job name collides with stage": {Version: 1, Jobs: []Job{
			{Name: "prepare", Command: []string{"true"}},
			{Name: "other", Stage: "prepare", Command: []string{"true"}},
		}},
	}
	for name, manifest := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(manifest, sequentialIDs()); err == nil {
				t.Fatal("Compile accepted an invalid expanded graph")
			}
		})
	}
	if _, err := Compile(Manifest{Version: 1, Jobs: []Job{{Command: []string{"true"}}}}, nil); err == nil {
		t.Fatal("Compile accepted a nil ID generator")
	}
}

func TestCompileExpandsMatrixWithArrayAndDependencies(t *testing.T) {
	manifest := Manifest{Version: 1, Jobs: []Job{
		{Name: "prepare", Stage: "inputs", Command: []string{"prepare"}},
		{Name: "train", Command: []string{"train"}, DependsOn: []string{"inputs"}, Environment: []string{"BASE=1"},
			Matrix: []string{"SEED=1,2", "MODEL=small,large"}, Array: "1-2"},
		{Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}},
	}}
	queue, err := Compile(manifest, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.Commands) != 6 {
		t.Fatalf("commands = %d, want 6", len(queue.Commands))
	}
	wantNames := []string{"prepare", "train-SEED1-MODELsmall", "train-SEED1-MODELlarge", "train-SEED2-MODELsmall", "train-SEED2-MODELlarge", "evaluate"}
	for index, want := range wantNames {
		if queue.Commands[index].Name != want {
			t.Fatalf("command %d name = %q, want %q", index, queue.Commands[index].Name, want)
		}
	}
	member := queue.Commands[2]
	if !reflect.DeepEqual(member.Environment, []string{"BASE=1", "SEED=1", "MODEL=large"}) || member.Array == nil || member.Matrix.BaseName != "train" {
		t.Fatalf("matrix member = %#v", member)
	}
	if err := model.ValidateMatrixGroups(queue.Commands); err != nil {
		t.Fatalf("compiled matrix group is invalid: %v", err)
	}
	jobs := model.QueueToJobs(queue.Commands)
	evaluate := jobs[len(jobs)-1]
	if len(evaluate.DependsOn) != 8 {
		t.Fatalf("evaluate dependencies = %#v, want all 8 matrix/array tasks", evaluate.DependsOn)
	}
}

func TestCompiledMatrixExportsBackToSameManifest(t *testing.T) {
	manifest := Manifest{Version: 1, Jobs: []Job{
		{Name: "train", Command: []string{"train"}, Environment: []string{"BASE=1"}, Matrix: []string{"SEED=1,2", "MODEL=small,large"}, Array: "1,3"},
		{Command: []string{"unnamed"}, Matrix: []string{"X=a,b"}},
		{Name: "evaluate", Command: []string{"evaluate"}, DependsOn: []string{"train"}, Executor: "local", ExecutorOptions: []string{"--x"}, WorkingDirectory: "work"},
	}}
	queue, err := Compile(manifest, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	exported, err := FromQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(exported, manifest) {
		t.Fatalf("exported manifest = %#v, want %#v", exported, manifest)
	}
}

func TestFromQueueKeepsLegacyExpandedMatrixAsSeparateJobs(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "one", Name: "train-SEED1", Command: []string{"train"}, Environment: []string{"SEED=1"}},
		{ID: "two", Name: "train-SEED2", Command: []string{"train"}, Environment: []string{"SEED=2"}},
	}}
	manifest, err := FromQueue(queue)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Jobs) != 2 || manifest.Jobs[0].Name != "train-SEED1" || len(manifest.Jobs[0].Matrix) != 0 {
		t.Fatalf("legacy manifest = %#v", manifest)
	}
}

func TestFromQueueRejectsMalformedMatrixProvenance(t *testing.T) {
	dimensions := []model.MatrixDimension{{Name: "SEED", Values: []string{"1", "2"}}}
	queue := model.Queue{Commands: []model.QueuedCommand{{
		ID: "one", Name: "train-SEED1", Command: []string{"train"}, Environment: []string{"SEED=1"},
		Matrix: &model.MatrixSpec{GroupID: "group", Dimensions: dimensions, Values: []model.MatrixValue{{Name: "SEED", Value: "1"}}, BaseName: "train"},
	}}}
	if _, err := FromQueue(queue); err == nil {
		t.Fatal("FromQueue accepted an incomplete matrix group")
	}
	if _, err := FromQueue(model.Queue{}); err == nil {
		t.Fatal("FromQueue accepted an empty queue")
	}
}

func TestFromRunAnnotatesScalarStatuses(t *testing.T) {
	queue := model.Queue{Commands: []model.QueuedCommand{
		{ID: "success", Command: []string{"true"}},
		{ID: "failed", Command: []string{"false"}},
		{ID: "cancelled", Command: []string{"sleep"}},
		{ID: "unfinished", Command: []string{"wait"}},
	}}
	summary := model.RunSummary{Results: []model.JobResult{
		{ID: "success", AttemptID: "att-success", ExitCode: 0},
		{ID: "failed", AttemptID: "att-failed", ExitCode: 2},
		{ID: "cancelled", AttemptID: "att-cancelled", ExitCode: 130, Error: "Cancelled by user"},
	}}
	manifest, err := FromRun(queue, summary, Source{Project: "demo", RunIDs: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ status, attempt string }{{"success", "att-success"}, {"failed", "att-failed"}, {"cancelled", "att-cancelled"}, {"unfinished", ""}}
	for index, expected := range want {
		job := manifest.Jobs[index]
		if job.Status != expected.status || job.AttemptID != expected.attempt || len(job.Instances) != 0 {
			t.Fatalf("job %d = %#v, want status %q attempt %q", index, job, expected.status, expected.attempt)
		}
	}
}

func TestFromRunListsFailedArrayTasksInMatrixGroup(t *testing.T) {
	queue, err := Compile(Manifest{Version: 1, Jobs: []Job{{Name: "train", Command: []string{"train"}, Matrix: []string{"SEED=1,2"}, Array: "1-2"}}}, sequentialIDs())
	if err != nil {
		t.Fatal(err)
	}
	first, second := queue.Commands[0].ID, queue.Commands[1].ID
	summary := model.RunSummary{Results: []model.JobResult{
		{ID: first + "-1", AttemptID: "a1", ExitCode: 0},
		{ID: first + "-2", AttemptID: "a2", ExitCode: 0},
		{ID: second + "-1", AttemptID: "b1", ExitCode: 0},
		{ID: second + "-2", AttemptID: "b2", ExitCode: 1},
	}}
	manifest, err := FromRun(queue, summary, Source{Project: "demo", RunIDs: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	job := manifest.Jobs[0]
	if len(manifest.Jobs) != 1 || job.Status != "failed" || len(job.Instances) != 1 {
		t.Fatalf("manifest = %#v", manifest)
	}
	instance := job.Instances[0]
	if instance.Task == nil || *instance.Task != 2 || instance.Matrix["SEED"] != "2" || instance.AttemptID != "b2" || instance.Status != "failed" {
		t.Fatalf("instance = %#v", instance)
	}
	if err := Validate(manifest); err != nil {
		t.Fatalf("exported manifest is invalid: %v", err)
	}
}

func TestEncodeRejectsUnsupportedFormat(t *testing.T) {
	if _, err := Encode(Manifest{Version: 1}, "xml"); err == nil {
		t.Fatal("Encode accepted an unsupported format")
	}
}
