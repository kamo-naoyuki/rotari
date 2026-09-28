# CLI reference

## Commands at a glance

| Command | Purpose |
| --- | --- |
| `add` | Add a command to the current queue. |
| `run` | Run the queued jobs. |
| `reset` | Discard the current queue. |
| `show` | Show queue, run, job, or log details. |
| `jobs` | List running and recently finished jobs across projects. |
| `diff` | Compare two runs: fixed, still failing, and newly failing jobs, and changed definitions. |
| `web` | Start the local web status UI. |
| `retry` | Rerun failed or unfinished jobs. |
| `cancel` | Cancel running jobs. |
| `wait` | Wait for an asynchronous run to finish. |
| `config` | Create or inspect configuration files. |
| `suspend` / `resume` | Suspend or resume running jobs. |
| `copy` | Copy jobs from a saved run into the queue. |
| `change` | Change a queued or restored job. |
| `export` | Export a queue, saved runs, or a starter workflow manifest. |
| `import` | Validate a workflow manifest and replace the current queue. |
| `remove` | Remove jobs from the queue. |
| `delete` | Delete saved run history. |
| `check` | Check whether a project is ready to run. |
| `diagnose` | Diagnose a failed job. |
| `env` | Show CLI and job environment variables. |
| `completion` | Generate shell completion scripts. |
| `gc` | Find and remove orphaned run registry entries. |
| `unlock` | Recover a confirmed stale run lock. |
| `server` | Inspect or control the project server. |
| `schema` | Print the machine-readable CLI schema. |
| `guide` | Print a usage guide for coding agents. |

## Common options

Frequently used options have short forms:

| Long option | Short option |
| --- | --- |
| `--project-name` | `-p` |
| `--basedir` | `-b` |
| `--run-id` | `-r` |
| `--job-id` | `-j` |
| `--executor` | `-e` |

For the complete command and option schema, run:

```sh
rotari schema --json
```
