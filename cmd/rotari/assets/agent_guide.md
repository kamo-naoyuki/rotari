# rotari guide for coding agents

rotari queues shell commands per project, runs them as a batch (a run), and
keeps each run's results so failed jobs can be fixed and rerun while
successful results are carried forward.

## Rules

- Start runs with `rotari run --async`, then block on `rotari wait`. To
  preview a run first, give the same command `--dry-run` in place of
  `--async`, which it cannot be combined with, then start it with
  `--async --if-revision REVISION`. A
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
- Read the default text output. `--json` is for programs such as the Python
  client: it prints every job's full result, including passed and carried
  ones, so it grows with the run.
- Preview before changing state, then apply exactly what you previewed:
  `add`, `change`, `copy`, `delete`, `import`, `remove`, `reset`, `run`, and
  `retry` take `--dry-run`,
  which prints the change and `revision=REVISION` without writing, and
  `--if-revision REVISION`, which applies only if nothing changed the project
  since. `rotari check` also prints the revision.
- Do not rely on prompts. `copy` into a non-empty queue needs `--append` or
  `--overwrite`, and `run --run-id` into a non-empty queue needs
  `--overwrite`.
- Pass `--project-name/-p` (or set `ROTARI_PROJECT_NAME`) so each command
  targets the intended project. A project in another state directory is
  named in the error, with how to select it; `rotari show` with no project
  lists the projects of every registered state directory. A command given a
  run ID or an attempt ID, such as the `show -j ATTEMPT_ID` lines that
  `lineage` prints, finds its state directory and project itself, so those
  hints work as printed without `--basedir`.
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
rotari wait sweep
# Failures grouped by cause; take RUN_ID from the wait output:
rotari lineage RUN_ID
# One job's evidence, taking ATTEMPT_ID from the cause's "show:" line:
rotari show -j ATTEMPT_ID --report
# Fix the cause, preview the rerun of failed and unfinished jobs, then
# start exactly that, taking REVISION from the preview's last line. retry
# keeps a queue changed with change -r; it copies the last run only into an
# empty queue:
rotari retry -p sweep --dry-run
rotari retry -p sweep --async --if-revision REVISION
rotari wait sweep
# What the retry fixed, what still fails, and whether the cause changed:
rotari lineage RUN_ID NEW_RUN_ID
```

To edit many jobs at once, export a run as a manifest, edit it, preview the
import, then import and run it. Take `RUN_ID` from the `Run:` line of
`wait`, or from `show -p sweep`:

```sh
rotari export -p sweep -r RUN_ID > experiment.yaml
rotari import --dry-run experiment.yaml sweep
rotari import experiment.yaml sweep
rotari run -p sweep --async
```

For one failed job, `rotari show -j ATTEMPT_ID --report` prints a Markdown
report with the command, status, diagnosis, and the log lines around the
diagnosis evidence.
