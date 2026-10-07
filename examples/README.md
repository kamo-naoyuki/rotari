# Examples

Each script shows one use of rotari. Install `rotari` on your `PATH`, or build
the checkout with `go build -o rotari ./cmd/rotari` and add the repository root
to `PATH`. Run the scripts from any directory; they accept no arguments.

All examples use rotari's normally resolved state directory and select a
separate project, rather than creating another basedir. Each resets its
project's next queue before adding jobs, so re-running it starts with a clean
queue; previous run history remains. Run `rotari show -p PROJECT` to inspect
an example later. Work files and exported manifests are kept separately in
`.rotari-example-work` under the current working directory; remove that
directory when finished. From the repository root, it is ignored by Git.

The scripts do not automatically recover interrupted runs. If a run was
interrupted, inspect it and confirm its jobs have stopped before following
the recovery instructions from `rotari show` or the [inspection guide](../docs/INSPECT.md).

| Goal | Run | Requirement |
| --- | --- | --- |
| Direct commands and `sh -c` | `./examples/basic.sh` | rotari |
| Jobs with dependencies | `./examples/dependencies.sh` | rotari |
| Working directories and artifact candidates | `./examples/artifacts.sh` | rotari |
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
