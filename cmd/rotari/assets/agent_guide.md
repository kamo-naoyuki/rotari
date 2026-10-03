# rotari guide for coding agents

rotari queues shell commands per project, runs them as a batch (a run), and
keeps each run's results so failed jobs can be fixed and rerun while
successful results are carried forward.

## Rules

- Start runs with `rotari run --async`, then block on `rotari wait`. A
  synchronous `rotari run` requests cancellation when its client is killed, so
  a tool-call timeout that kills the command cancels the whole run. `wait`
  can be killed and repeated safely; it exits with the run's exit code.
  `wait --until-failure` also returns as soon as a job fails with no retry
  left, so a long run's first failure can be fixed early.
- Read summaries before logs. `rotari lineage RUN_ID` prints a run's counts
  and its failures grouped by cause, each with the jobs, an example line, the
  command that shows one job, and a suggested fix. It stays short however
  many jobs fail; `show` tables and `--failed-logs` grow with the run.
- Verify before changing state instead of guessing:
  - `rotari check PROJECT` exits 0 only when the queued run can start.
  - `rotari import --dry-run FILE` previews which jobs a manifest executes,
    reuses, or accepts without writing the queue.
  - `rotari schema --json` lists every command and flag.
- Prefer machine-readable output: `show --json`, `check --json`,
  `wait --json`, and `import --dry-run --json`.
- Preview before changing state, then apply exactly what you previewed:
  `add`, `change`, `copy`, `delete`, `import`, `remove`, `reset`, `run`, and
  `retry` take `--dry-run`,
  which prints the change and `revision=REVISION` without writing, and
  `--if-revision REVISION`, which applies only if nothing changed the project
  since. `rotari check --json` also reports the revision.
- Do not rely on prompts. `copy` into a non-empty queue needs `--append` or
  `--overwrite`, and `run --run-id` into a non-empty queue needs
  `--overwrite`.
- Pass `--project-name/-p` (or set `ROTARI_PROJECT_NAME`) so each command
  targets the intended project.
- Use `--no-pager` with log views such as `show --logs`.
- In `add` and `change`, put rotari options before the job command and
  separate them with `--`: every argument after the command's first word goes
  to the job, so `add python train.py --timeout 30` passes `--timeout 30` to
  the script. Other commands accept options anywhere.

## Typical loop

```sh
rotari add -p sweep --job-name train --matrix LR=0.1,0.01 -- python train.py
rotari check sweep
rotari run -p sweep --async
rotari wait sweep --json
# Failures grouped by cause; take RUN_ID from the wait output:
rotari lineage RUN_ID
# One job's evidence, taking ATTEMPT_ID from the cause's "show:" line:
rotari show -j ATTEMPT_ID --report
# Fix the cause, then rerun only failed and unfinished jobs:
rotari retry -p sweep --async
rotari wait sweep --json
# What the retry fixed, what still fails, and whether the cause changed:
rotari lineage RUN_ID NEW_RUN_ID
```

To edit many jobs at once, export a run as a manifest, edit it, preview the
import, then import and run it. Take `RUN_ID` from `wait --json` or
`show --json`:

```sh
rotari export -p sweep -r RUN_ID > experiment.yaml
rotari import --dry-run experiment.yaml sweep
rotari import experiment.yaml sweep
rotari run -p sweep --async
```

For one failed job, `rotari show -j ATTEMPT_ID --report` prints a Markdown
report with the command, status, diagnosis, and the log lines around the
diagnosis evidence.
