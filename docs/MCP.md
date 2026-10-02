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

## Build both entry points

Build the MCP stdio server and the terminal command from the repository:

```sh
mkdir -p ./bin
go build -o ./bin/mcp ./cmd/rotari/mcp
go build -o ./bin/mcp-cmd ./cmd/rotari/mcp-cmd
```

Both commands call `internal/mcp.GetJobInfo`; the MCP server exposes it as
`rotari_get_job_info`, while `mcp-cmd` prints the same report in a terminal.
The MCP server uses the official Go MCP SDK, which currently requires Go 1.23
or newer.

Run the terminal form with the same four identifiers:

```sh
./bin/mcp-cmd --basedir /path/to/rotari-state --project experiment \
  --run-id 20261002-120000-12345678 --job-id abc123def
```

## Configure VS Code

Add an MCP server entry to the workspace's `.vscode/mcp.json`, changing the
command to the absolute path of the built binary:

```json
{
  "servers": {
    "rotari": {
      "type": "stdio",
      "command": "/absolute/path/to/rotari/bin/mcp",
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
does not walk arbitrary directories or infer a basedir from the job ID. When
MCP is unavailable, use `mcp-cmd` with the same `basedir`, `project`, `run-id`,
and `job-id` values; both entry points call the same lookup and report code.
