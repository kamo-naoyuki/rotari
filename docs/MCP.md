# MCP server (experimental)

rotari includes an experimental read-only MCP server for asking an agent to
inspect one job. It exposes the `rotari_get_job_info` tool, which takes an
exact run ID and an exact job ID. It returns an AI-oriented report with the
job's status, result, diagnosis, and a bounded tail of recent log output.
Known paths and hostnames are redacted where detected; redaction is not
guaranteed to catch every secret.

The server finds the run's state directory and project through the run
registry of its master directory, the same registry the `rotari` CLI uses to
resolve `--run-id`. It serves only runs registered there: it does not fall
back to a default state directory, walk other directories, list projects, or
discover a job from its ID alone. No queue edits, job execution, or job
control are exposed.

Agents that can run shell commands can use the `rotari` CLI directly; for
example, `rotari show -r RUN_ID --report` prints the same report, and
`rotari show -r RUN_ID --json` returns structured run data.

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

Then ask the agent to call `rotari_get_job_info` with the run ID and job ID,
for example:

```json
{
  "run_id": "20261002-120000-12345678",
  "job_id": "abc123def"
}
```

A run ID that is not registered in the master directory, or whose run
directory is gone, is reported as a tool error. Run and job IDs must not
contain `/` or `\`.
