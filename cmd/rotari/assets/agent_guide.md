# rotari guide for coding agents

rotari queues shell commands per project, runs them as a batch (a run), and
keeps each run's results so failed jobs can be fixed and rerun while
successful results are carried forward.

## Rules

- Start runs with `rotari run --async`, then block on `rotari wait`. A
  synchronous `rotari run` requests cancellation when its client is killed, so
  a tool-call timeout that kills the command cancels the whole run. `wait`
  can be killed and repeated safely; it exits with the run's exit code.
- Verify before changing state instead of guessing:
  - `rotari check PROJECT` exits 0 only when the queued run can start.
  - `rotari import --dry-run FILE` previews which jobs a manifest executes,
    reuses, or accepts without writing the queue.
  - `rotari schema --json` lists every command and flag.
- Prefer machine-readable output: `show --json`, `check --json`,
  `wait --json`, and `import --dry-run --json`.
- Do not rely on prompts. `copy` into a non-empty queue needs `--append` or
  `--overwrite`, and `run --run-id` into a non-empty queue needs
  `--overwrite`.
- Pass `--project-name/-p` (or set `ROTARI_PROJECT_NAME`) so each command
  targets the intended project.
- Use `--no-pager` with log views such as `show --logs`.

## Typical loop

```sh
rotari add -p sweep --job-name train --matrix LR=0.1,0.01 -- python train.py
rotari check sweep
rotari run -p sweep --async
rotari wait sweep
rotari show -p sweep --json
rotari show -p sweep --failed-logs --no-pager
# Fix the cause, then rerun only failed and unfinished jobs:
rotari retry -p sweep --async
rotari wait sweep
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

For one failed job, `rotari show ATTEMPT_ID --report` prints a Markdown
report with the command, status, and recent output.
