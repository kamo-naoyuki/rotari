package workflowstate

import (
	"os"
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/state"
	"github.com/kamo-naoyuki/rotari/internal/workflow"
)

func TestImportPreviewsThenWritesUnderTheRevision(t *testing.T) {
	baseDir := t.TempDir()
	paths, err := state.ResolveProjectPaths(baseDir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	var registered []string
	next := 0
	request := Import{
		Store: state.NewStore(0o700, 0o600), BaseDir: baseDir, Project: "demo",
		Manifest: workflow.Manifest{Version: workflow.Version, Jobs: []workflow.Job{{Name: "train", Command: []string{"true"}}}},
		NewJobID: func() string { next++; return "job-" + string(rune('0'+next)) },
		Validate: func(model.Queue) error { return nil },
		Register: func(baseDir string) error { registered = append(registered, baseDir); return nil },
	}

	request.Guard = project.Guard{DryRun: true}
	preview, err := request.Apply()
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Jobs) != 1 || preview.Jobs[0].Name != "train" || preview.Revision == "" {
		t.Fatalf("preview plan = %+v", preview)
	}
	if _, err := os.Stat(paths.ProjectDir); !os.IsNotExist(err) || len(registered) != 0 {
		t.Fatalf("a preview of a new project created it (%v) or registered it (%v)", err, registered)
	}

	request.Guard = project.Guard{IfRevision: preview.Revision}
	applied, err := request.Apply()
	if err != nil {
		t.Fatal(err)
	}
	queue, err := state.LoadQueue(paths.QueueFile)
	if err != nil || len(queue.Commands) != 1 || queue.Commands[0].Name != "train" {
		t.Fatalf("imported queue = %+v, %v", queue, err)
	}
	if applied.Revision == preview.Revision || len(registered) != 1 || registered[0] != baseDir {
		t.Fatalf("applied revision %s (preview %s), registered %v", applied.Revision, preview.Revision, registered)
	}

	// The queue now has a job, so a second import needs Overwrite.
	request.Guard = project.Guard{}
	if _, err := request.Apply(); err == nil || !strings.Contains(err.Error(), "use --overwrite") {
		t.Fatalf("import into a non-empty queue: %v", err)
	}
}
