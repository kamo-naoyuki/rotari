# Python API

The client methods return `CommandResult` unless otherwise noted. Command
options are generated from the Rotari CLI schema.

## `Rotari.add`

```python
Rotari.add(
    command: Sequence[str],
    **options: object,
) -> CommandResult
```

Add an executable argument list to the current project queue. `command` is
passed as an argument vector, so shell quoting and shell expansion are not
performed by the Python client.

Supported `options` are:

| Option | Value | Description |
| --- | --- | --- |
| `executor` | `str` | Job executor. |
| `executor_options` | `Sequence[str]` | Scheduler options; may be repeated. |
| `output` | `Sequence[str]` | Stdout destination; may be repeated. |
| `error` | `Sequence[str]` | Stderr destination; may be repeated. |
| `working_directory` | `str` | Working directory for the job. |
| `env` | `Sequence[str]` | Job environment entries such as `KEY=VALUE`. |
| `job_name` | `str` | Job name label. |
| `stage` | `str` | Stage containing the job. |
| `depends_on` | `Sequence[str]` | Prerequisite job or stage; may be repeated. |
| `depends_on_finished` | `Sequence[str]` | Prerequisite that must finish; may be repeated. |
| `timeout` | `str` | Job timeout, such as `90m` or `2h`. |
| `retry` | `int` | Per-job retry limit. |
| `retry_delay` | `str` | Delay before the first retry. |
| `retry_backoff` | `float` | Multiplier for each retry delay. |
| `retry_max_delay` | `str` | Upper limit for retry delay. |
| `array` | `str` | Array range or selected tasks. |
| `matrix` | `Sequence[str]` | Matrix dimensions; may be repeated. |
| `quiet` | `bool` | Suppress success output. |

The full option descriptions and accepted values are maintained in the CLI
schema used by `rotari schema --json`.

::: rotari
    options:
      members:
        - Rotari
        - CommandResult
        - RotariError
