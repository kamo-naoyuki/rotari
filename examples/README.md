# Examples

Each script shows one use of rotari. Install `rotari` on your `PATH`, or build
the checkout with `go build -o rotari ./cmd/rotari` and add the repository root
to `PATH`. Run the scripts from any directory; they accept no arguments.

All examples use the same `.example-state` directory under the current working
directory, with a separate project for each example. Each unlocks any
interrupted run and resets its project's next queue before adding jobs, so
re-running it starts with a clean queue; previous run history remains. From
the repository root, the state
directory is ignored by Git. The scripts leave it in place so you can inspect
results with `rotari show --basedir PATH --project-name NAME` afterward.
Remove the directory when finished.

| Goal | Run | Requirement |
| --- | --- | --- |
| Dependencies, a local run, and the files a job names | `./examples/basic.sh` | rotari |
| Local array tasks | `./examples/array.sh` | rotari |
| Matrix of independent jobs | `./examples/matrix.sh` | rotari |
| Retry only failed work | `./examples/retry.sh` | rotari |
| Compare generations after fixing a job | `./examples/lineage.sh` | rotari |
| Start an async run and wait | `./examples/async.sh` | rotari |
| Slurm array | `./examples/slurm.sh` | Configured Slurm cluster |
| Import a stage and matrix manifest, then export | `./examples/workflow.sh` | rotari |
| Fix and accept results from an exported run | `./examples/workflow-reconcile.sh` | rotari |
| Diagnose a failure with local rules | `./examples/diagnose-rules.sh` | Python 3 |

The [workflow manifest](workflow.yaml) can be edited before importing it. The
[reconciliation manifest](workflow-reconcile.yaml) intentionally fails two
jobs; its example edits one command and accepts the other's result, so the
next run reuses completed work.
