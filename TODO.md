# TODO

- Consider `rotari show --stage NAME` to filter the selected queue or run to
  jobs in that stage. Use the stage name directly rather than introducing a
  generated `--stage-id`.

- Revisit a declarative workflow manifest after collecting cases that remain
  awkward with shell scripts and stages. Prefer a constrained manifest compiled
  into the existing queue over a standalone DSL; keep shell commands as the
  execution language and do not add automatic input/output freshness checks.
  Candidate formats are YAML, TOML, and JSON, using one shared schema. The
  initial schema could cover named jobs, stages, dependencies, command argv,
  environment, working directory, executor/options, and array or matrix
  expansion. Consider `rotari workflow plan FILE` for static validation and
  expanded-DAG preview, `rotari workflow import FILE` to replace or append to a
  queue, and `rotari workflow export` to write the current queue as a manifest.
  Define whether export preserves queue job IDs and how import reports generated
  or colliding IDs before implementing it.

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
