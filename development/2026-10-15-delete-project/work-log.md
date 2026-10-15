# Work Log: Delete a Whole Project

See [plan.md](plan.md).

## Plan

- **Commit:** `42a6da59` (2026-10-15 13:24:11 +0900)
- **Change:** Added the plan.
- **Reason:** The user asked for a Delete button for each project on the Web
  UI's All projects page, with a matching CLI command and a plan before the
  work started.
- **Plan impact:** Decisions D1–D8 recorded.
- **Validation:** `go test ./internal/doclinks` passed.
- **Remaining:** Phases 1–4.

## Phase 1: Shared operation

- **Commit:** `d97325f3` (2026-10-15 13:35:03 +0900)
- **Change:** `queueops.Editor.DeleteProject` and `moveProjectToTrash` in
  `internal/queueops/delete.go`, with tests in `delete_test.go`.
- **Reason:** D1, D3, D4, D6.
- **Plan impact:** D4 changed: the runs are unregistered after the lock is
  released and the directory is removed, not under the lock.
- **Validation:** `go test ./internal/queueops` passed, including the five
  new `TestDeleteProject*` tests. The active-run fixture first failed because
  it had no `context.json`; it was fixed in the test.
- **Remaining:** None.

## Phase 2: CLI

- **Commit:** `98398cba` (2026-10-15 13:43:35 +0900)
- **Change:** `delete --project` (`cmd/rotari/delete.go`, `cli_spec.go`),
  CLI tests, regenerated `docs/CLI_REFERENCE.md` and
  `python/rotari/generated_cli.py`, golden schema, flag-pair inventory and
  mutation adapter, SEL-10 rows and contract table, and the flag-pair
  coverage document (whose pair total was already stale, now 7,410).
- **Reason:** D2, D8.
- **Plan impact:** None.
- **Validation:** `go test ./cmd/rotari -run 'TestCmdDelete|TestDeleteAll'`,
  `TestCLIFlagPairInventory`, `TestCLIFlagPairMutations` (13 new
  `delete/*project*` pairs ran), and `TestPositionalArguments` passed;
  generated-doc checks passed. `go test ./cmd/rotari` failed only in
  `TestWebRunViewDrawsMatrixGrid` and `TestWebRunViewClampsLongCells` with
  the system Node 12; both passed with Node 24 later.
- **Remaining:** None.

## Phase 3: Web

- **Commit:** `e963c343` (2026-10-15 13:50:55 +0900)
- **Change:** `/api/delete-project` in `internal/webui/webui.go`;
  `deleteProjectButton` and `deleteProject` in the Web assets;
  `TestWebDeleteProjectEndpoint`, `TestWebAllProjectsDeleteProject`, the
  read-only route lists, and the route in
  `TestWebAPIRejectsUnsafePathElements`.
- **Reason:** D7.
- **Plan impact:** None.
- **Validation:** With the VS Code server's Node 24 first on `PATH`,
  `go test ./internal/webui ./cmd/rotari` passed, as did
  `tsc -p internal/webui/tsconfig.json` and `prettier --check` on the changed
  assets. `TestWebAPIRejectsUnsafePathElements` passed.
- **Remaining:** None.

## Phase 4: Contracts and documentation

- **Commit:** `99f597c3` (2026-10-15 14:06:57 +0900)
- **Change:** SAFE-12 and WEB-11, SAFE-2/SAFE-3 wording, status table rows;
  `TestDeleteProjectRemovesOnlyThatProject` in conformance;
  `delete --project` in `support.GuardedCommands` and the SAFE-2 test;
  WEB-9's expected overview row actions now include Delete; RUNNING, FAQ,
  and ARCHITECTURE.
- **Reason:** Phase 4 of the plan.
- **Plan impact:** Phases 1–4 marked done.
- **Validation:** `go vet ./...` passed. `go test -short ./...` with Node 24
  failed in `TestStaticExportPagesShowWhatLivePagesShow` (the expected row
  actions lacked Delete; fixed, then `go test ./conformance/05-web` passed)
  and `TestShowReportsElapsedAndQuietTime` (reports "quiet 14m" from the NFS
  clock skew already recorded for this host; unrelated). The contract status
  test and `internal/doclinks` passed.
- **Remaining:** None.
