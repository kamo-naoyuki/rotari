# MCP server (experimental)

rotari includes an experimental MCP server, `rotari mcp`, for agents that
cannot run shell commands. Agents that can should use the `rotari` CLI, as
`rotari guide` describes; each tool below names the CLI command that returns
the same information. The tools below only read; importing a manifest and
starting a run are described in [Changing a project](#changing-a-project).

| Tool | Input | Returns | CLI equivalent |
| --- | --- | --- | --- |
| `rotari_list_projects` | none | every project of every registered state directory, with its state, queue size, run count, and last run's ID, status, and failed and total job counts | `rotari show` |
| `rotari_run_summary` | `run_id` | the run's `state` (`running`, `finished`, `interrupted`, or `ended` without a summary), job counts, and failed and blocked jobs grouped by cause, each with its jobs, exit codes, an example evidence line, the attempt to inspect, and a suggested fix | `rotari lineage RUN_ID` |
| `rotari_wait_run` | `run_id`, optional `timeout_seconds` (1 to 300, default 30) and `until_failure` | waits until the run is no longer running, or with `until_failure` until a job has failed with no retry left, then returns what `rotari_run_summary` does and `reason`: `settled`, `failure`, or `timeout` (call again to keep waiting) | `rotari wait RUN_ID --timeout` (`--until-failure`) |
| `rotari_get_job_info` | `run_id`, `job_id` | a report on one job: status, result, diagnosis, and the log lines around the diagnosis evidence | `rotari show -j ATTEMPT_ID --report` |
| `rotari_check_project` | `basedir_ref`, `project` | whether the project's queued run can start: its state (`ready`, `empty`, `running`, `locked`, or `interrupted`), queued job count, and lock, after validating the queue's jobs, dependencies, and executors | `rotari check PROJECT` (without `--deep`) |
| `rotari_export_run` | `run_id`, optional `format` (`yaml`, `json`, or `toml`) | the finished run as a workflow manifest for reading, with environment values, executor options, and paths redacted; the MCP import tools refuse it until the placeholders are replaced, and `rotari export` gives the full manifest | `rotari export RUN_ID` |
| `rotari_compare_runs` | `run_id`, optional `previous_run_id` and `all_jobs` | which jobs were fixed, still fail, or newly fail, each run's failure cause and whether it changed, and which job definitions changed; `previous_run_id` defaults to the run that started just before `run_id`, and jobs that did not change are only counted in `hidden_unchanged`, as `rotari lineage` hides them, unless `all_jobs` is set | `rotari lineage PREVIOUS_RUN_ID RUN_ID` |

A typical session lists projects, summarizes the run with failures, inspects
one job from a failure group's attempt, and compares a later run with it.
Tools that act on a project rather than a run take the `basedir_ref` and
`project` that `rotari_list_projects` returns.

## Changing a project

The tools that change a project come in pairs: a read-only preview and a
write that applies only at the revision the preview returned. An MCP client
can let previews run freely and ask its user before each write; the
read-only tools are annotated as such.

| Preview | Write | Input | CLI equivalent |
| --- | --- | --- | --- |
| `rotari_preview_import` | `rotari_import` | `basedir_ref`, `project`, `manifest` (text), optional `format` (`yaml`, `json`, or `toml`), `overwrite`, and `detail`; the write also `if_revision` | `rotari import --dry-run` / `--if-revision` |
| `rotari_preview_run` | `rotari_start_run` | `basedir_ref`, `project`, optional `retry`; the write also `if_revision` and optional `run_name` | `rotari run` or `retry`, with `--dry-run` / `--async --if-revision` |

An import tool returns the plan's summary: each job with its status, an
array's tasks counted by status, the statuses counted over the plan, the
source jobs it drops, and `revision`; with `detail`, also the full plan,
with each command and source attempt, as `rotari import --dry-run --json`
gives it. A preview returns the plan and `revision`; pass that revision as
`if_revision` to the write. If anything wrote the project in between, the
write fails with `project changed since the planned revision` and changes
nothing; preview again. `rotari_preview_run` lists the jobs the run would
execute, planned as the run itself is, and `rotari_start_run` returns the
`run_id` of the run it started in the background; follow it with
`rotari_wait_run`, calling it again while it returns `timeout`. Right after
the start, the run may have no jobs yet.
With `retry`, the run executes only the failed and unfinished jobs and
carries the other results, as `rotari retry` does: those of the queue, such
as one just imported, or of the project's last run, which it copies into an
empty queue first.

A started run uses `rotari run`'s defaults, and its jobs run in the working
directory and with the environment of the `rotari mcp` process, which is
usually the MCP client's.

The server finds a run's state directory and project through the run
registry of its master directory, the same registry the `rotari` CLI uses to
resolve `--run-id`. It serves only runs and state directories registered
there: it does not fall back to a default state directory or walk other
directories. Results name a state directory by `basedir_ref`, a stable
reference that does not reveal its path, and by `basedir_name`, the last
element of its path; they contain no absolute paths, and errors write
`BASEDIR` for a registered state directory's path. Reports and evidence
lines redact paths and hostnames where detected; redaction is not guaranteed
to catch every secret. Queue edits other than import, job control,
and deleting history are not exposed, and the server does not remove stale
locks or migrate registries.

## Start the server

The server is part of the `rotari` binary:

```sh
rotari mcp
```

It speaks MCP on stdin and stdout, so an MCP client starts it as a
subprocess. It resolves its master directory when it starts: `--masterdir`,
then the same order as the CLI, `ROTARI_MASTERDIR`, then
`$XDG_STATE_HOME/rotari/master`, then `~/.local/state/rotari/master`. It exits
with a non-zero status if the master directory cannot be resolved or the
server fails.

## Configure VS Code

Add an MCP server entry to the workspace's `.vscode/mcp.json`, giving the
absolute path of the `rotari` binary when it is not on the client's `PATH`:

```json
{
  "servers": {
    "rotari": {
      "type": "stdio",
      "command": "rotari",
      "args": ["mcp"]
    }
  }
}
```

Set `ROTARI_MASTERDIR` in the entry's `env` when your runs are registered
under a non-default master directory.

A run ID that is not registered in the master directory, or whose run
directory is gone, is reported as a tool error, as is a comparison of runs
from different projects. Run and job IDs must not contain `/` or `\`.
