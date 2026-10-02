# MCP server (experimental)

rotari includes an experimental read-only MCP server for asking an agent to
inspect one job. It exposes the `rotari_get_job_info` tool, which takes the
state directory (`basedir`), project name, exact run ID, and exact job ID. It
returns an AI-oriented report with the job's status, result, diagnosis, and a
bounded tail of recent log output. Known paths and hostnames are redacted where
detected; redaction is not guaranteed to catch every secret.

The tool currently requires the caller to supply all four identifiers. It does
not search the basedir registry, list projects, or discover a job from its ID
alone. No queue edits, job execution, or job control are exposed.

## Build

Build the standalone stdio server from the repository:

```sh
go build -o ./bin/rotari-mcp ./cmd/rotari-mcp
```

The MCP server uses the official Go MCP SDK, which currently requires Go 1.23
or newer.

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

Then ask the agent to call `rotari_get_job_info` with all four values. For
example, it needs a request equivalent to:

```json
{
  "basedir": "/path/to/rotari-state",
  "project": "experiment",
  "run_id": "20261002-120000-12345678",
  "job_id": "abc123def"
}
```

The `basedir` is an explicit tool argument in this first prototype. The server
does not walk arbitrary directories or infer a basedir from the job ID.
