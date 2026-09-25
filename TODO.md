# TODO

- Consider `rotari show --stage NAME` to filter the selected queue or run to
  jobs in that stage. Use the stage name directly rather than introducing a
  generated `--stage-id`.

- Consider extending `cancel`/`suspend`/`resume` selectors to also accept a
  `run_name` and/or a bare `project_name`, alongside the existing job_id,
  `att_` attempt_id, and bare run_id support. Unlike run_id (fixed generated
  format) and attempt_id (`att_` prefix), `run_name` is a free-form label with
  no reserved shape, so mixing it into the positional/`--job-id` list risks
  colliding with a real job_id. `project_name` is safer to detect (an existing
  `projects/<name>` directory), but still not fully unambiguous. If this is
  implemented, prefer resolving `run_name` only through a dedicated
  `--run-name` flag (mirroring `run --run-name`) rather than the mixed
  positional list, and reuse `wait`'s active-run scanning
  (`resolveRunNameTargets` in [cmd/rotari/wait.go](cmd/rotari/wait.go)) for
  lookup semantics and ambiguity errors.
