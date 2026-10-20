# Plan: Use rotari with experiment trackers

Created: 2026-10-21
Status: Proposed; no implementation changes

## Goal

Make jobs that record to an experiment tracker, MLflow or Aim in
particular, easy to open from rotari's Web UI, and document how rotari and
these tools divide the work. Rotari runs and tracks jobs; trackers record and
compare what a job measured. They are complementary, but today the step
from a rotari job to its tracker data is often missing.

## Background

The [artifact launcher](../2026-10-20-artifact-launcher/plan.md) added
built-in services for MLflow (a directory named `mlruns`, or a file named
`mlflow.db`) and Aim (a directory holding an Aim repository). They open only
from an artifact candidate of a job, and the trackers' data is rarely a
candidate:

- **Default locations are implicit.** With nothing configured, MLflow 3.17
  records to `sqlite:///<working directory>/mlflow.db`, and Aim uses the
  working directory as its repository (`<working directory>/.aim`); both were
  checked on 2026-10-21 against the installed versions. The training code
  creates them, and no job definition names them, so static discovery
  ([artifact discovery](../2026-10-03-artifact-discovery/plan.md)) finds
  nothing.
- **URIs are skipped on purpose.** `MLFLOW_TRACKING_URI=sqlite:///runs/mlflow.db`
  or `file:./mlruns` in a job's environment is a URI, and PATH-X1 skips every
  URI scheme so that `https://…` and `s3://…` are never taken for files. A
  local `file:` or `sqlite:` URI is a file reference, though. A plain path
  (`MLFLOW_TRACKING_URI=./mlruns`, `AIM_REPO=/work/exp`) is already found by
  PATH-R2.
- **The working directory is not an entry.** The Artifacts view lists
  candidates and their children; the job's working directory, which holds the
  default stores, cannot be browsed or launched unless something names it.
- **Stores are shared, artifacts are per job.** A tracking store collects
  many jobs' records. Opening it from any of those jobs shows the same whole
  store, and the launcher's reuse key (target and working directory) already
  makes them share one service.

Other observations that belong in the user guide rather than in code:

- Jobs receive `ROTARI_RUN_ID`, `ROTARI_JOB_ID`, and `ROTARI_ATTEMPT_ID`.
  Logging them as tracker tags (for example `mlflow.set_tag("rotari_job",
  os.environ["ROTARI_JOB_ID"])`) links a tracker run back to the rotari job
  and attempt.
- Many parallel jobs writing one SQLite `mlflow.db` on a network file system
  risk lock errors and corruption, since SQLite's locking is unreliable over
  NFS. A file store (`mlruns`), a per-job store, or a tracking server is safer
  at that scale. Aim's repository also needs care with concurrent writers on
  a network file system.

## Scope and non-goals

Include:

1. Recognize local `file:` and `sqlite:` URIs as artifact references.
2. Offer each job's working directory as a browsable, launchable entry.
3. A user guide section on using rotari with experiment trackers.

Exclude:

- Reading tracker data, such as metrics or run IDs, into rotari's own views.
  Rotari stays unaware of tracker formats; the trackers' own UIs show them.
- Starting or managing tracking servers outside the launcher.
- Remote tracking URIs (`http://`, `databricks`, `s3://`), which stay skipped.
- Changing where trackers write, or adding tracker-specific code to jobs.

## Design

### 1. Local file URIs (new rule PATH-R6)

Add a classifier rule, in [internal/artifact](../../internal/artifact/), that
accepts a literal `file:` URI (`file:///abs/path`, `file:/abs/path`,
`file:./rel`, `file:rel`) or a `sqlite:` URI (`sqlite:///rel/path.db`,
`sqlite:////abs/path.db`) and records the filesystem path it names. It runs
before PATH-X1, which keeps skipping every other scheme. Relative paths
resolve against the job's working directory as other relative candidates do.
Percent-encoding is decoded; a URI with a host (`file://server/…`) or with
query parameters that select something other than a file is skipped.

The rule applies wherever literal values are classified (argv, environment,
configuration values, shell and Python literals), so
`MLFLOW_TRACKING_URI=sqlite:///runs/mlflow.db` in `rotari add --env` and
`mlflow.set_tracking_uri("file:./mlruns")` in a script are both found. The
candidate's source says it came from a URI.

Update the discovery rule table, the accepted and rejected fixtures in
`classify_test.go`, and the artifact contract and guide.

### 2. The working directory as an entry

Give every listing whose working directory is known a working-directory
entry, separate from the recorded candidates (it is not a guess about
outputs), shown first in the Artifacts view with type `directory`. It can be
browsed like a listed directory and offered to Launch with, so the default
`mlflow.db` (a child) and Aim repository (the directory) open with the
built-in services. It follows WEB-6's roots: it exists only when the working
directory is served, which for an untrusted basedir it is not.

Decide during implementation whether this is a recorded candidate kind
(PATH-D3, written to `artifacts.json` by `rotari run`, which old runs lack) or
derived by the Web server from the listing's working directory (works for old
runs, no state change). Prefer the derived form unless the CLI listing
(`show --artifacts`) must show it too, since CLI-16 requires the Web listing to
match the CLI's.

### 3. User guide

Add "Using rotari with experiment trackers" to the guides: where MLflow and Aim
write by default, how to open them from Artifacts, linking tracker runs to
rotari jobs with the `ROTARI_*` variables, choosing a store for many parallel
jobs on shared storage, and when a tracking server is better. Link it from the
tool comparison.

## Phases

1. **URI rule.** Classifier, fixtures, rule table, contract, guide. A unit
   table test gives each discovery source (argv, environment, YAML/TOML/JSON
   value, shell, Python) the same URIs and expects the same candidates.
2. **Working-directory entry.** Listing, Web view, Launch with, static export,
   WEB-5/WEB-6 contract text and conformance rows. Check the CLI listing
   decision above first.
3. **Guide and comparison.** User guide section; cross-links from
   `docs/TOOL_COMPARISON.md`.
4. **End-to-end check.** With MLflow and Aim installed in a scratch
   environment, a job that writes to each default location, and one that sets
   `MLFLOW_TRACKING_URI` to a `sqlite:` URI, can be opened from Artifacts
   through the built-in services.

## Acceptance

- `file:` and `sqlite:` URIs naming local paths become candidates from every
  discovery source; `http(s)://`, `s3://`, `databricks`, and `file://host/…`
  do not.
- A job with no path in its definition still offers its working directory in
  Artifacts on a trusted basedir, not on an untrusted one, and Launch with on
  it lists Aim, and on its `mlflow.db` child lists MLflow (SQLite).
- The guide's examples run as written against the versions tested.
- Contracts, the status table, and conformance tests agree, and
  `scripts/check.sh` passes.
