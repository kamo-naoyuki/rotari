# MCP server (experimental)

rotari includes an experimental read-only MCP server for agents that cannot
run shell commands. Agents that can should use the `rotari` CLI, as
`rotari guide` describes; each tool below names the CLI command that returns
the same information.

| Tool | Input | Returns | CLI equivalent |
| --- | --- | --- | --- |
| `rotari_list_projects` | none | every project of every registered state directory, with its state, queue size, run count, and last run's ID, status, and failed and total job counts | `rotari show` |
| `rotari_run_summary` | `run_id` | job counts, and failed and blocked jobs grouped by cause, each with its jobs, exit codes, an example evidence line, the attempt to inspect, and a suggested fix | `rotari lineage RUN_ID` |
| `rotari_get_job_info` | `run_id`, `job_id` | a report on one job: status, result, diagnosis, and the log lines around the diagnosis evidence | `rotari show -j ATTEMPT_ID --report` |
| `rotari_check_project` | `basedir_ref`, `project` | whether the project's queued run can start: its state (`ready`, `empty`, `running`, `locked`, or `interrupted`), queued job count, and lock, after validating the queue's jobs, dependencies, and executors | `rotari check PROJECT` (without `--deep`) |
| `rotari_compare_runs` | `run_id`, optional `previous_run_id` | which jobs were fixed, still fail, or newly fail, each run's failure cause and whether it changed, and which job definitions changed; `previous_run_id` defaults to the run that started just before `run_id` | `rotari lineage PREVIOUS_RUN_ID RUN_ID` |

A typical session lists projects, summarizes the run with failures, inspects
one job from a failure group's attempt, and compares a later run with it.
Tools that act on a project rather than a run take the `basedir_ref` and
`project` that `rotari_list_projects` returns.

The server finds a run's state directory and project through the run
registry of its master directory, the same registry the `rotari` CLI uses to
resolve `--run-id`. It serves only runs and state directories registered
there: it does not fall back to a default state directory or walk other
directories. Results name a state directory by `basedir_ref`, a stable
reference that does not reveal its path, and by `basedir_name`, the last
element of its path; they contain no absolute paths. Reports and evidence
lines redact paths and hostnames where detected; redaction is not guaranteed
to catch every secret. No queue edits, job execution, or job control are
exposed, and the server does not remove stale locks or migrate registries.

## Build

Build the MCP stdio server from the repository:

```sh
mkdir -p ./bin
go build -o ./bin/rotari-mcp ./cmd/mcp/server
```

The MCP server uses the official Go MCP SDK, which currently requires Go 1.23
or newer.

The server resolves its master directory when it starts, in the same order as
the CLI: `ROTARI_MASTERDIR`, then `$XDG_STATE_HOME/rotari/master`, then
`~/.local/state/rotari/master`. It exits with a non-zero status if the master
directory cannot be resolved or the server fails.

## Configure VS Code

Add an MCP server entry to the workspace's `.vscode/mcp.json`, changing the
command to the absolute path of the built binary:

```json
{
  "servers": {
    "rotari": {
      "type": "stdio",
      "command": "/absolute/path/to/rotari/bin/rotari-mcp",
      "args": []
    }
  }
}
```

Set `ROTARI_MASTERDIR` in the entry's `env` when your runs are registered
under a non-default master directory.

A run ID that is not registered in the master directory, or whose run
directory is gone, is reported as a tool error, as is a comparison of runs
from different projects. Run and job IDs must not contain `/` or `\`.
