# Artifact candidate viewing work log

## CLI listing (step 1)

**Commit:** `47a17cf` — 2026-10-04T18:34:26+09:00

**Change:** Added `jobstatus.ListArtifacts`, which reads an attempt's record
through the carried-origin chain and observes each path now (file, directory,
other, missing, unknown), and `artifact.Describe`, the one-line provenance
text. `rotari show -j JOB` prints the listing after the command, at most 20
entries, with a pointer to `show -j ATTEMPT --artifacts`. The new
`--artifacts` flag prints every entry and the discovery notes instead of the
logs, requires one job, rejects an array command's ID, and cannot be combined
with log, follow, stream, JSON, report, queue, list, or result-filter options.
`show -j JOB --json` carries the listing per job. Records now keep the
effective working directory, so text lists paths relative to it. Added
CLI-16, `TestShowListsArtifactCandidates`, unit tests, and the regenerated
help/schema goldens, CLI reference, Python CLI metadata, and Python API
docs. Updated the flag-pair inventory (show: 40 flags, 780 pairs) and coverage
counts, docs/INSPECT.md, docs/ARCHITECTURE.md, and the agent guide.

**Reason:** The user agreed to start viewing with the CLI: list artifacts in
`show -j`, by default, with a limit and `--artifacts` for all of them.

**Plan impact:** Step 1 done.

**Validation:** `TestDescribe`, `TestListArtifactsObservesEachPath`,
`TestWriteArtifactListing`, and `TestShowListsArtifactCandidates` (text,
limit, `--artifacts`, notes, `--json`, carried rerun, rejections) passed;
the conformance test first failed on two wrong expectations in the test
(the redirection target's column, and counting `data/` on the command line),
fixed in the test. `TestCLIFlagPairs` ran 822 pairs (1,644 invocations, 484
accepted, 338 explicitly rejected) and passed, as did `TestCLIFlagPairSamples`,
`TestCLIFlagPairObservability`, and `TestCLIFlagPairInventory`.
`go test ./cmd/rotari`, the Python tests (28 passed), `ruff check`/`format`,
`generate_cli_reference.py --check`, `generate_python_api_docs.py --check`,
`TestGoldenOutputs`, `TestContractStatus`, and doclinks passed.

**Remaining:** Step 2 (Web).

## Web UI and API (step 2)

**Commit:** `e316f0a` — 2026-10-04T18:39:56+09:00

**Change:** Added `GET /api/artifacts` (`internal/webui/artifacts.go`),
returning the same listing JSON as `show -j --json` for a job's latest or
given attempt, rejecting an unknown job or another job's attempt. A job row's
Artifacts button opens the listing of the attempt its Output button shows,
laid out by `formatArtifactListing` as the CLI prints it. The static export
embeds the listing of each job's latest attempt and of each attempt.
`display_path` moved into the shared listing so both interfaces only lay out
fields; the CLI prints a diagnostic without a source as its message alone.
Added WEB-5, `TestWebShowsArtifactCandidates`, `TestWebArtifactsAPI`, and a
jsdom test of the Web layout against the CLI's.

**Reason:** Step 2 of the plan.

**Plan impact:** Step 2 done. The listing is fetched on demand, not with the
run, so loading a run observes no paths.

**Validation:** `TestWebArtifactsAPI` (latest and selected attempts, six
rejections, POST) and `TestWebArtifactsFormatMatchesCLI` passed; the latter
first failed because loading the asset replaced the test's modal stubs,
fixed in the test. `TestWebShowsArtifactCandidates` compared `/api/artifacts`
and the static export's embedded listing with `show -j --json` and passed.
`./internal/webui`, `./internal/doclinks`, `./conformance/05-web`, `pairweb`,
`TestContractStatus`, `TestConformanceLayout`, `TestGoldenOutputs`, and
`prettier --check` on the assets passed. A full `scripts/check.sh` (vet, test,
race) started after this commit passed with "all checks passed".

**Remaining:** Step 3, content preview, after a safety design.
