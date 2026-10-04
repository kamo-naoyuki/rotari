# Artifact candidate viewing

Created: 2026-10-04

Status: steps 1 (CLI-16), 2 (WEB-5), and 3 (WEB-6) done.

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

## Step 3 design: content preview

Agreed 2026-10-04.

### What may be served

A file or directory is served only when all three hold:

1. **It is a recorded candidate.** Requests name a project, run, job,
   attempt, and entry index in that attempt's listing; there is no path
   parameter, so a path that is not in a record cannot be requested. A child
   of a listed directory is named by the entry index plus a relative child
   path, which must not contain `..` or a separator-led component.
2. **It is under an allowed root.** By default the only root is the job's
   recorded working directory. `rotari web --artifact-root DIR`
   (repeatable) adds roots such as a shared data area. A candidate outside
   every root stays in the listing, marked as outside the allowed roots, and
   is not served. This matters because the Web UI can add jobs: a job
   `cat ~/.ssh/id_rsa` would otherwise put that file in a listing.
3. **Symlinks do not escape the root.** Files are opened through
   `os.Root` (Go 1.24+), which refuses any path, including through a symlink,
   that leaves the root, at open time rather than in a separate check.

Serving follows the existing exposure rules: every route is behind
`--auth-token` when one is set. A live server on a non-loopback host without
a token serves previews too; its startup warning mentions that file contents
are now readable through the Web UI.

### How content is shown

There is no file-size limit; the server streams and never reads a whole file
into memory. Only how much the browser renders at once is bounded:

| Type | View |
| --- | --- |
| png, jpg, jpeg, gif | `<img>` |
| svg | `<img>` only, so scripts never run; the file response carries `Content-Security-Policy: sandbox` and `X-Content-Type-Options: nosniff`, so opening it directly is safe too |
| csv, tsv | A table of a first chunk of rows, with more loaded on demand |
| `.log`, `.txt` | Text from the end, with earlier chunks loaded on demand, like job logs |
| json, yaml, yml, toml, py, sh, and other text | Text from the start, with later chunks loaded on demand |
| Anything else (pdf, npy, pt, h5, ...) | Size and modification time, and a download |
| Directory | Its immediate children (name, type, size, modified), directories first and then by name, 200 per page with more on demand; each child openable under the same rules; no recursive listing |

A directory's names are read up to 100,000 entries, so a directory with
millions of children cannot stall the server; beyond that the view says only
the first 100,000 are listed. Size and modification time are read only for
the page shown.

Downloads are allowed without a size limit (`Content-Disposition:
attachment`, streamed). Every content response carries
`Cache-Control: no-store`.

### Out of scope

- Static exports keep the listing only, never file contents.
- Only files visible to the Web server's host are previewed; an SSH job's
  files are not fetched.
- The CLI does not preview; it prints the paths.

## Validation

- Unit tests for the listing (observation types, carried origin, older
  attempt, no record) and the provenance text.
- CLI tests for the section, the limit, `--artifacts`, and `--json`.
- Binary conformance for the CLI and Web API views, flag-pair inventory and
  outcomes, help and schema goldens, generated CLI reference, and Python CLI
  metadata.
