# Artifact candidate viewing

Created: 2026-10-04

Status: steps 1 (CLI-16) and 2 (WEB-5) done; step 3 (content preview)
pending its safety design.

## Purpose

Show the artifact candidates that each attempt records
([artifact discovery](../2026-10-03-artifact-discovery/plan.md), contract
RUN-9) in the CLI and the Web UI, so a user can see which files a job named
without reading `artifacts.json`.

## Scope

1. **Shared listing and CLI.** One listing, built in `internal/jobstatus`,
   reads an attempt's record (following carried origins) and observes each
   candidate on the viewing host now: file, directory, other, missing, or
   unknown for a path without a known base. `rotari show -j JOB` prints it
   before the logs, at most 20 entries with a pointer to the rest;
   `show -j JOB --artifacts` prints the whole listing and its diagnostics
   instead of the logs; `show -j JOB --json` carries it per job.
2. **Web UI.** The job detail shows the same listing, and the Web API job
   detail carries it.
3. **Content preview.** Image, table, and text previews and downloads in the
   Web UI, after a separate safety design: allowed roots, symlink escapes,
   authentication, and size limits. Detection is not permission to serve.

## Decisions

- The listing, its observation, and the one-line provenance text are built
  once (`jobstatus.ListArtifacts`, `artifact.Describe`) and only formatted by
  the CLI, JSON, and Web, so the interfaces cannot disagree.
- Observation follows symlinks and never opens a file. "missing" means not
  found on the viewing host: an SSH job's files may exist only on its host.
- An attempt without a record shows "not recorded", which differs from a
  record with no candidates ("none found").
- `--artifacts` requires `--job-id` and cannot be combined with log,
  report, JSON, or queue views; it never silently ignores another option.
- `show -j JOB --artifacts` for a queued job looks past the queue to the
  latest run, like the other run-only options; an array command's ID is
  rejected, since one attempt is listed.
- The record also keeps the effective working directory, so text views list
  paths relative to it; JSON keeps absolute paths.

- The Web UI fetches the listing only when a job's Artifacts button is
  pressed (`/api/artifacts`), so loading a run does not observe every
  candidate of every job. A static export embeds the listing of each job's
  latest attempt and of each attempt, observed at export time.
- `display_path` is computed with the listing, so the CLI and the Web UI only
  lay out fields; a jsdom test checks the Web layout against the CLI's.

## Validation

- Unit tests for the listing (observation types, carried origin, older
  attempt, no record) and the provenance text.
- CLI tests for the section, the limit, `--artifacts`, and `--json`.
- Binary conformance for the CLI and Web API views, flag-pair inventory and
  outcomes, help and schema goldens, generated CLI reference, and Python CLI
  metadata.
