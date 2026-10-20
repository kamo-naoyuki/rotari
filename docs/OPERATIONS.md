# Operations

Server management, run registry maintenance, shared filesystems, and the security model.

## Quick context

Use `rotari info` for a read-only overview of the resolved master directory,
base directory and project, visible configuration files, running
supervisors, run locks, and active or interrupted runs. Add `--json` for
machine-readable output. When multiple projects exist and none is selected,
the command reports the choices instead of guessing.

An uncreated selected project is marked on the same line as its name. For
unfinished local-executor attempts, `info` best-effort checks whether the
recorded process group exists; other executors and remote hosts are
unverified. This does not prove process identity. The run lock separately
reports coordinator liveness on this host.
For new runs, the run context also retains the coordinator's host, PID,
PID namespace, and process start time independently of that lock. Each active
or interrupted run reports a separate `coordinator` assessment in `info --json`
(`state`, `reason`, `observed_at`, and the recorded `identity`), or a
`coordinator=live|stopped|unknown` line in text output. A matching local
process identity is live; a missing process or a verified reused PID is
stopped. Another host/namespace, insufficient identity, or inaccessible
process evidence is unknown. Legacy runs without the record remain unknown.
Unreadable or malformed context is reported as an error.

The record's `management_finished_at` marks successful completion of run
management, not process death. It never overrides a live-process observation.
This assessment also gates `unlock`, including when the lock is absent.
It does not check whether jobs stopped or automatically recover a run.

Run rows count jobs with a recorded final result and carried results as
`finished`; a failed attempt waiting for retry is not final yet.
Headings and states are colored on a terminal; redirected text and `--json`
remain uncolored.

## Server management

`run` and `retry` start a supervisor for each run, and it stops after the run
finishes, so a project has one only while a run is starting or active. These
commands act on the supervisors of every project in the base directory and
are mainly useful for inspection and cleanup; `server shutdown` leaves an
active run interrupted, so stop runs with `cancel` instead:

```sh
rotari server status
rotari server list
rotari server shutdown
```

## Run registry maintenance

Run IDs do not contain the base directory or project name. Rotari therefore
keeps a master **run registry**, a lookup table that maps each run ID back to
the base directory and project that own it. This lets commands such as
`show --run-id/-r`, `wait --run-id/-r`, and `copy --run-id/-r` work without repeating
`--basedir/-b` and `--project-name/-p`.

The run data itself remains under the project state directory:

```text
<basedir>/projects/<project>/runs/<run-id>/
```

The registry is stored separately, with one JSON file per run:

```text
<masterdir>/runs/<run-id>.json
```

`<masterdir>` is selected from `--masterdir`, `ROTARI_MASTERDIR`,
`$XDG_STATE_HOME/rotari/master`, or `~/.local/state/rotari/master`, in that
order. A registry file contains the run ID, base directory, and project name;
it is only a lookup index, not the source of the run's logs or results.

`gc` is separate from inspection and recovery: it maintains this master
registry after run data was removed outside rotari. To list orphaned registry
entries without removing them:

```sh
rotari gc --dry-run
```

The candidates and their locations are printed. This includes basedir
registry records whose basedir directory no longer exists. Existing basedir
directories, including empty ones, are retained. Malformed or invalid
registry files are listed and left untouched; inspect their run data and
repair or remove them manually. To remove the candidates:

```sh
rotari gc
```

`gc` removes registry entries only. Just before removing each one, it checks
again, and skips a candidate whose registry location changed or whose run
directory reappeared; it never deletes run data or an existing basedir
directory.

## Shared filesystem use

Hosts sharing `--basedir/-b`/`ROTARI_BASEDIR` on NFS can share a queue. Updates
are coordinated through the shared state directory. This requires a consistent
shared filesystem; it cannot fence a host after a network partition or repair
inconsistent mounts. Confirm that jobs on a failed host have stopped before
recovering the project. Separate project names keep their queue and run history
separate, but the base directory and filesystem remain shared.

Queue updates such as `add`, `change`, `remove`, `copy`, `run`, and `delete` are
serialized with an advisory file lock, so NFSv4 servers and clients must be
configured to support file locking. When another update holds the lock, rotari
waits up to 30 seconds and then returns an error. The coordination relies on
the filesystem providing consistent exclusive file creation, atomic rename, and
advisory `flock` behavior; do not use the same project through mounts that can
disagree about the state files.

An active run is recorded in `running.lock` with its run ID, PID, host, and PID
namespace. On the host that started the run, rotari detects when that PID is
gone, removes the lock, and keeps the run as interrupted until you recover it
with `unlock`. A lock created on another host, or in another PID namespace
such as a sandbox or container on the same host, is always treated as active,
because rotari cannot tell whether that PID is still alive.
Using different project names on different hosts is therefore much safer than
sharing one project.

After confirming a failed host's run has stopped, unlock that exact run:

```sh
rotari show -p sweep
rotari unlock -p sweep --run-id RUN_ID --dry-run
rotari unlock -p sweep --run-id RUN_ID
```

`unlock` verifies the run ID, removes a matching lock, and returns the project
to queue collection. Never use it while the run may still be executing, or a
second run could start for the same queue.

### Unlock ownership checks and automation migration

`unlock` now checks the durable coordinator identity even without a run lock:

- `live`: recovery is refused, including with `--assume-stopped`.
- `stopped`: ownership permits recovery without an override.
- `unknown`: recovery is refused unless you independently confirm that the
  coordinator stopped and explicitly pass `--assume-stopped`. This includes
  older runs without identity, other hosts/PID namespaces, missing process
  start times, and inaccessible process evidence. A PID alone is not proof
  that the recorded coordinator is alive.

Refusals include the recorded host and PID namespace when available. Inspect
from the original host **and the same PID namespace** if accessible; changing
hosts alone may not make verification possible. Neither an absent lock nor a
management-completion marker proves the coordinator stopped.

First preview without changing metadata, locks, context, queue, or results:

```sh
rotari unlock -p sweep --run-id RUN_ID --dry-run --json
```

The JSON contains `allowed`, `applied`, `refusal_code`
(`coordinator_live` or `coordinator_unknown`), `coordinator`
(`state`, `reason`, `observed_at`, `identity`), `assume_stopped`, `revision`,
and recorded-job warnings. A successful preview exits zero even when
`allowed` is false; automation must inspect that field. Applying a refused
recovery exits nonzero, retaining the JSON evidence on stdout and diagnostics
on stderr. Applying always reassesses ownership under the state lock; a
preview is not authorization to bypass a later live observation.

**Job termination is not checked in this phase**, including when every job
has a terminal record. Machine-readable output explicitly reports
`execution_checked: false`. Independently confirm that jobs, wrappers and
their managed descendants stopped before recovery or rerunning jobs.
`--assume-stopped` attests to stopped work rather than proving it; it does not
kill jobs, restart monitoring, or resubmit anything.

After that independent confirmation, an unknown-owner recovery is explicit:

```sh
rotari unlock -p sweep --run-id RUN_ID --assume-stopped
```

Existing scripts that used plain `unlock` on older runs or remote locks must
add this operator-confirmation step, not blindly append the flag. It is
command-line-only: no configuration or environment variable silently enables
it. Applied recovery records its authorization time, assessment, and attestation
in the existing run context's `recovery` field, preserving coordinator identity
and result history. The record describes authorization for the recovery attempt;
an I/O failure later in recovery still returns an error.
The queue stays intact, the project returns to collecting, and rerunning
failed/unfinished jobs still requires `retry --run-id RUN_ID`.

Python exposes the same options through
`unlock(run_id=..., dry_run=True, json=True)` and
`unlock(run_id=..., assume_stopped=True)`. MCP uses
`rotari_preview_unlock` followed by `rotari_unlock` at the preview's
`if_revision`, with `assume_stopped` supplied explicitly when needed.
Refused MCP applies are tool errors with structured refusal evidence.
There is no Web UI unlock action.

On NFS every file a command opens, stats, or lists costs a round trip to the
server, so read commands such as `jobs`, `runs`, `show`, `info`, and
`lineage` read each state file once and read independent runs and jobs
concurrently. Their cost still grows with the history they cover: `jobs` and
`runs` without a project read every project of every known state directory.
Name the project (`-p`) and state directory (`-b`) to read less. `jobs` reads
only the summary of a finished run outside its `--since` window, and `show -j`
reads only the selected job's files.

## Security model

Rotari assumes a trusted single-user or HPC/lab environment. The Web UI token
provides HTTP authentication, not encryption.

- **State files:** `--basedir/-b`, `--masterdir`, and their contents (queues,
  metadata, locks, job output and status, wrapper scripts) default to shared
  `0755`/`0644` permissions, because colleagues on the same cluster commonly
  share a job's log path directly. Set `ROTARI_PRIVATE_STATE=true` for
  owner-only `0700`/`0600` to keep job commands, working directories, and
  output private. This affects only newly created paths, which are not
  re-chmodded later, and applies to the whole `--basedir/-b`.
- **Supervisor:** a run's supervisor opens no socket or port. Only the `run`
  command that started it talks to it, over inherited pipes, so `run` works
  where Unix sockets are blocked and with base directories of any length.
  While it runs, it holds `<basedir>/projects/<project>/server.lock` and
  records its PID in `server.pid` next to it; a failed start is recorded in
  `<basedir>/server.log`.
- **Web UI:** without `ROTARI_WEB_AUTH_TOKEN` or `--auth-token`, bind it to
  `127.0.0.1`; with a token, use only a trusted network or HTTPS proxy. Prefer
  the environment variable so the token does not appear in the process list.
  Environment variable values are never exposed over HTTP, but project and run
  pages show raw global/workspace/basedir/project config sources and saved
  merged run file-config snapshots, which may contain secrets. Static exports
  include those contents as well; share them only with trusted readers.
  The notification
  settings UI visually masks a saved webhook URL and does not return it from
  its read API, but this does not encrypt the connection: a replacement URL is
  sent to the Web server when saved. Use HTTPS through a trusted reverse proxy
  whenever the browser-to-server network is not trusted.
- **Artifact files:** `rotari run` reads the configuration files, shell
  scripts, and Python files a job names, as the user running it, to find the
  paths the job uses; it records only those paths, in each attempt's
  `artifacts.json`, never the files' contents. The live Web UI, however,
  serves the contents of recorded files and directories under each job's
  working directory, so anyone who can open the Web UI can read them; with
  job control enabled, they can also add a job that names another file there.
  Paths outside the working directory, and symlinks leading out of it, are not
  served unless `rotari web --artifact-root DIR` allows them; keep such roots
  narrow. SVG files are shown only as images, so their scripts do not run.
  A static export contains only paths, unless it is made with
  `--static-artifact-contents`, which copies the files into the export: anyone
  who can read the export, such as a published site, can then read them.
  The command reports how many files and bytes it copied.

None of this defends against another user with access to your own UID
(e.g. root, or anyone who can read your home directory), only against other
unprivileged users on a shared machine.
