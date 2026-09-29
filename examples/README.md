# Examples

Each script shows one use of rotari. Install `rotari` on your `PATH`, or build
the checkout with `go build -o rotari ./cmd/rotari` and add the repository root
to `PATH`. Run the scripts from any directory; they accept no arguments.

All examples use the same `.example-state` directory under the current working
directory, with a separate project for each example. Each resets its project's
queue with `--recover` before adding jobs, so re-running it starts with a clean
queue; previous run history remains. From the repository root, the state
directory is ignored by Git. The scripts leave it in place so you can inspect
results with `rotari show --basedir PATH --project-name NAME` afterward.
Remove the directory when finished.

| Goal | Run | Requirement |
| --- | --- | --- |
| Dependencies and a local run | `./examples/basic.sh` | rotari |
| Local array tasks | `./examples/array.sh` | rotari |
| Matrix of independent jobs | `./examples/matrix.sh` | rotari |
| Retry only failed work | `./examples/retry.sh` | rotari |
| Start an async run and wait | `./examples/async.sh` | rotari |
| Slurm array | `./examples/slurm.sh` | Configured Slurm cluster |
| Import a stage and matrix manifest, then export | `./examples/workflow.sh` | rotari |
| Fix and accept results from an exported run | `./examples/workflow-reconcile.sh` | rotari |
| Diagnose a failure with local rules | `./examples/diagnose-rules.sh` | Python 3 |
| Diagnose a failure with an LLM | `./examples/diagnose-llm.sh` | Python 3, `ROTARI_LLM_API_KEY` and `ROTARI_LLM_MODEL` |

The [workflow manifest](workflow.yaml) can be edited before importing it. The
[reconciliation manifest](workflow-reconcile.yaml) intentionally fails two
jobs; its example edits one command and accepts the other's result, so the
next run reuses completed work.
The LLM example sends the failure log to the configured provider; see the
[LLM diagnosis guide](../docs/LLM_DIAGNOSIS.md) before running it.

## Inspect a fix-and-rerun cycle

Suppose the first run has one failed training job and one successful
evaluation job. After fixing the training command and rerunning the batch,
compare the two generations directly:

```sh
rotari lineage -p sweep 20260930-101500-a1b2c3d4 20260930-104200-e5f6a7b8
```

The comparison is intentionally focused on what changed:

```text
Runs: first (20260930-101500-a1b2c3d4) -> fixed (20260930-104200-e5f6a7b8)
Summary: fixed 1, still failing 0, newly failing 0, added 0, removed 0, changed 1, carried 1

JOB       FROM     TO       RESULT          CHANGES
train     failed   success  fixed           command (carried)
eval      success  success  unchanged       - (carried)
```

Use one run ID to inspect what remains in that generation:

```sh
rotari lineage -p sweep 20260930-104200-e5f6a7b8
```

For automation, request the complete comparison as JSON:

```sh
rotari lineage -p sweep --json 20260930-101500-a1b2c3d4 20260930-104200-e5f6a7b8
```
