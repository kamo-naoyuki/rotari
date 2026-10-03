package mcp

import (
	"strings"
	"testing"

	"github.com/kamo-naoyuki/rotari/internal/basedirregistry"
	"github.com/kamo-naoyuki/rotari/internal/model"
	"github.com/kamo-naoyuki/rotari/internal/project"
	"github.com/kamo-naoyuki/rotari/internal/server"
	"github.com/kamo-naoyuki/rotari/internal/state"
)

func testWriteTools(f toolFixture, started *[]server.Request) writeTools {
	next := 0
	return writeTools{masterDir: f.masterDir, options: Options{
		NewJobID: func() string { next++; return "new" + string(rune('a'+next)) },
		StartRun: func(_ state.ProjectPaths, request server.Request) (server.Response, error) {
			*started = append(*started, request)
			return server.Response{OK: true, RunID: "20260101-000001-dddddddd"}, nil
		},
	}}
}

func TestImportToolsPreviewThenApplyAtTheRevision(t *testing.T) {
	f := newToolFixture(t)
	tools := testWriteTools(f, &[]server.Request{})
	input := ImportInput{BaseDirRef: basedirregistry.Ref(f.secondBaseDir), Project: "fresh", Manifest: "version: 1\njobs:\n  - name: train\n    command: [\"true\"]\n"}
	paths, _ := state.ResolveProjectPaths(f.secondBaseDir, "fresh")

	preview, err := tools.importManifest(input, project.Guard{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Jobs) != 1 || preview.Revision == "" || preview.Counts["unfinished"] != 1 || preview.Plan != nil {
		t.Fatalf("preview = %+v", preview)
	}
	detailed := input
	detailed.Detail = true
	if full, err := tools.importManifest(detailed, project.Guard{DryRun: true}); err != nil || full.Plan == nil || len(full.Plan.Jobs[0].Command) == 0 {
		t.Fatalf("detailed preview = %+v, %v", full, err)
	}
	if queue, _ := state.LoadQueue(paths.QueueFile); len(queue.Commands) != 0 {
		t.Fatal("the preview wrote the queue")
	}

	if _, err := tools.importManifest(input, project.Guard{IfRevision: "0000000000000000"}); err == nil || !strings.Contains(err.Error(), "project changed") {
		t.Fatalf("stale import error = %v", err)
	}
	applied, err := tools.importManifest(input, project.Guard{IfRevision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if queue, _ := state.LoadQueue(paths.QueueFile); len(queue.Commands) != 1 || applied.Revision == preview.Revision {
		t.Fatalf("applied import: queue %+v, revision %s", queue, applied.Revision)
	}

	input.Manifest = "not: [a manifest"
	if _, err := tools.importManifest(input, project.Guard{DryRun: true}); err == nil {
		t.Fatal("an invalid manifest was accepted")
	}
}

func TestRunToolsPreviewAndStartTheRetry(t *testing.T) {
	f := newToolFixture(t)
	var started []server.Request
	tools := testWriteTools(f, &started)
	input := RunInput{BaseDirRef: basedirregistry.Ref(f.firstBaseDir), Project: "exp", Retry: true}
	paths, _ := state.ResolveProjectPaths(f.firstBaseDir, "exp")

	// The first basedir's last run failed tasks 2 and 3, and its queue is
	// empty, so a retry copies that run and executes those tasks.
	preview, err := tools.previewRun(input)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, job := range preview.Execute {
		ids = append(ids, job.ID)
	}
	if strings.Join(ids, ",") != "tr-2,tr-3" || preview.Jobs != 3 || preview.Carried != 1 {
		t.Fatalf("retry preview = %+v", preview)
	}
	if queue, _ := state.LoadQueue(paths.QueueFile); len(queue.Commands) != 0 {
		t.Fatal("the preview copied the run into the queue")
	}

	if _, err := tools.startRun(StartRunInput{RunInput: input, IfRevision: "0000000000000000"}); err == nil || !strings.Contains(err.Error(), "project changed") || len(started) != 0 {
		t.Fatalf("stale start: %v, requests %d", err, len(started))
	}
	output, err := tools.startRun(StartRunInput{RunInput: input, IfRevision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if output.RunID != "20260101-000001-dddddddd" || len(started) != 1 {
		t.Fatalf("start = %+v, requests %d", output, len(started))
	}
	request := started[0]
	now, _ := project.Revision(paths)
	if !request.Async || request.Selection != model.ResultSelection(true, true, false) || request.SourceRunID != f.secondRun || request.IfRevision != now || now == preview.Revision {
		t.Fatalf("run request = %+v; want an async retry of %s at the revision after the copy (%s)", request, f.secondRun, now)
	}
	if queue, _ := state.LoadQueue(paths.QueueFile); len(queue.Commands) != 1 {
		t.Fatal("the start did not copy the last run into the queue")
	}
}

func TestStartRunNeedsARevisionAndAStarter(t *testing.T) {
	f := newToolFixture(t)
	input := StartRunInput{RunInput: RunInput{BaseDirRef: basedirregistry.Ref(f.firstBaseDir), Project: "exp"}}
	if _, err := testWriteTools(f, &[]server.Request{}).startRun(input); err == nil || !strings.Contains(err.Error(), "if_revision is required") {
		t.Fatalf("start without a revision: %v", err)
	}
	input.IfRevision = "x"
	if _, err := (writeTools{masterDir: f.masterDir}).startRun(input); err == nil || !strings.Contains(err.Error(), "cannot start runs") {
		t.Fatalf("start without a starter: %v", err)
	}
}

func TestExportRunRedactsAndImportRefusesTheRedactedView(t *testing.T) {
	f := newToolFixture(t)
	paths, _ := state.ResolveProjectPaths(f.secondBaseDir, "exp")
	commands := model.Queue{Commands: []model.QueuedCommand{{
		ID: "tr", Name: "train", Command: []string{"python", "/home/alice/train.py"}, Array: &model.ArraySpec{First: 1, Last: 3},
		Environment: []string{"API_TOKEN=s3cret"}, WorkingDirectory: "/data/alice/exp", ExecutorOptions: []string{"--account=alice-lab"},
	}}}
	if err := state.WriteJSON(paths.RunsDir+"/"+f.healthyRun+"/commands.json", commands); err != nil {
		t.Fatal(err)
	}
	output, err := exportRun(f.masterDir, ExportRunInput{RunID: f.healthyRun})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"s3cret", "/home/alice", "/data/alice", "alice-lab", f.secondBaseDir} {
		if strings.Contains(output.Manifest, secret) {
			t.Errorf("exported manifest reveals %q:\n%s", secret, output.Manifest)
		}
	}
	for _, kept := range []string{"API_TOKEN", "train", "python"} {
		if !strings.Contains(output.Manifest, kept) {
			t.Errorf("exported manifest lost %q:\n%s", kept, output.Manifest)
		}
	}

	tools := testWriteTools(f, &[]server.Request{})
	_, err = tools.importManifest(ImportInput{BaseDirRef: basedirregistry.Ref(f.secondBaseDir), Project: "exp", Manifest: output.Manifest, Overwrite: true}, project.Guard{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "redacted placeholders") {
		t.Fatalf("import of the redacted view: %v", err)
	}
}

func TestExportRunRefusesAnActiveRun(t *testing.T) {
	f := newToolFixture(t)
	paths, _ := state.ResolveProjectPaths(f.firstBaseDir, "exp")
	if err := state.WriteJSON(paths.MetaFile, model.Meta{Phase: "running", LastRunID: f.secondRun}); err != nil {
		t.Fatal(err)
	}
	if _, err := exportRun(f.masterDir, ExportRunInput{RunID: f.secondRun}); err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("export of an unsettled run: %v", err)
	}
}
