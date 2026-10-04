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

## Design of content preview

**Commit:** `34d2f09` — design recorded in plan.md (no code).

**Change:** Recorded the step 3 design agreed with the user: entries named by
index only, the job's working directory plus `--artifact-root` as allowed
roots, `os.Root` against symlink escapes, a warning (not a refusal) on a
non-loopback host without a token, no file-size limits, `.log`/`.txt` from the
end and other text from the start, immediate directory children 200 per page
reading at most 100,000 names, unlimited streamed downloads, and no file
contents in static exports.

**Validation:** `./internal/doclinks` passed.

## Content preview (step 3)

**Commit:** `5e68129` — 2026-10-04T19:58:25+09:00

**Change:** Added `internal/webui/artifact_files.go`:
`/api/artifact-file` (inline images, everything else or `download=1` as a
streamed attachment), `/api/artifact-text` (64 KiB pages on line boundaries,
forward or backward, NUL means binary), and `/api/artifact-directory`
(immediate children, directories first, 200 per page, at most 100,000 names
read). All three resolve an entry index of the recorded listing and an
optional clean child path, require an allowed root, open through `os.Root`,
and set `no-store`, `nosniff`, and a sandbox CSP. `/api/artifacts` adds
`previewable`. `rotari web --artifact-root DIR` (repeatable, command line
only, must be a directory, rejected with `--static-dir`) adds roots, and the
non-loopback warning names artifact files. The UI moved to a new
`web_app_artifacts.js`: a listing table with open buttons, image, table, text,
and directory previews with paging, and downloads. Handler arguments are
written with `JSON.stringify` before HTML escaping. Added WEB-6, regenerated
goldens, the CLI reference, and Python CLI metadata, and updated the `web`
flag-pair inventory (9 flags, 36 pairs), the static-export server-only list,
the coverage totals (also correcting the read-only count missed in step 1),
the asset layout in the contract, docs/INSPECT.md, and docs/ARCHITECTURE.md.

**Reason:** Step 3 of the plan, as designed.

**Plan impact:** All three steps are done.

**Validation:** `TestArtifactPreviewableFlags`, `TestArtifactFileServing` (11
refusals including outside roots, symlink escapes of a file and of a child,
`..`, absolute and unclean child paths, missing, unresolved, a directory as a
file, bad entries; `--artifact-root`; POST), `TestArtifactTextPages`, and
`TestArtifactDirectoryPages` passed on their first run. The jsdom test
`TestWebArtifactPreviewInBrowser`, which drives the real page against the real
handler (listing, image fetch, log tail and an earlier page, CSV table,
directory pages, a child named with a quote, a file without a preview), first
failed on test errors only (module resolution, two escaping mistakes, and a
wrong expected byte count) and then passed. `TestWebPreviewsArtifactsUnderAllowedRoots`
(binary, live server) and the updated `TestWebShowsArtifactCandidates`
passed, as did `pairweb`, `TestCLIFlagPairInventory`, `./internal/webui`,
`./cmd/rotari`, goldens, contract status, doclinks, prettier, the Python
tests, ruff, and the generator checks. A full `scripts/check.sh` (vet, test,
race) started after this commit passed with "all checks passed".

**Remaining:** None in this plan.
