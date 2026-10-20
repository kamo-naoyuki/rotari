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
Recovery also assesses managed execution evidence before finalization or
ordinary release; it never stops jobs or automatically recovers a run.

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

`unlock` verifies the run ID, removes a matching lock, and either repairs
finalization or returns the project to queue collection (see below).
Never use it while the run may still be executing, or a
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
(`coordinator_live`, `coordinator_unknown`, `execution_active`, or `execution_unknown`), `coordinator`
(`state`, `reason`, `observed_at`, `identity`), `assume_stopped`, `revision`,
`action` (`finalize` or `release`), `repair` (completeness, reason, execution
inventory), and recorded-job warnings. A successful preview exits zero even when
`allowed` is false; automation must inspect that field. Applying a refused
recovery exits nonzero, retaining the JSON evidence on stdout and diagnostics
on stderr. Applying always reassesses ownership under the state lock; a
preview is not authorization to bypass a later live observation. Execution
probes run outside the state lock; before applying, rotari reacquires the lock
and checks that the queue, metadata, coordinator, and execution evidence still
match the preview revision.

Both finalization repair and ordinary `release` check local process groups,
scheduler executions, and SSH remote process groups as described below.
`execution_checked` is true only for a nonempty inventory whose units all
have verified `active` or `stopped` evidence within their stated scope: it
does not mean stopped or allowed. Unknown evidence reports false. Active
execution always blocks recovery; unknown evidence requires explicit
`--assume-stopped`. Independently confirm unknown or unchecked jobs, wrappers
and managed descendants stopped before recovery or rerunning.
`--assume-stopped` attests to stopped work rather than proving it; it does not
override verified activity, kill jobs, restart monitoring, or resubmit anything.

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
The queue stays intact. Incomplete runs return to collecting, and rerunning
failed/unfinished jobs still requires `retry --run-id RUN_ID`; complete runs
can repair finalization instead.

### Attested finalization repair

Computation may have finished while management finalization failed. If the
summary names the interrupted run and exactly covers the command snapshot's
expanded jobs, array tasks and matrix members, recovery can restore metadata
to `finished`. It validates carried origins, status marks and attempt
associations, including earlier attempts and incomplete launch records.
Preview reports `action: "finalize"` and `repair.complete: true`.

The summary alone does **not** prove execution termination. Inspect
`repair.executions`, including earlier attempts and the original attempts
behind carried results:

- All executions assessed `stopped`: repair needs no execution attestation.
- Any verified `active` execution: `execution_active`, even with
  `--assume-stopped`. A terminal outcome does not override positive activity;
  the evidence exposes the conflict.
- Any `unknown` execution: `execution_unknown` unless you independently
  confirm stopped work and explicitly attest with `--assume-stopped`.

Coordinator ownership is checked separately and may still need attestation.
Only jobs established never launched (for
example blocked dependencies or pre-dispatch cancellation) need no execution
attestation. A live coordinator always blocks.

Repair preserves the original summary/results, commands, origins, run ID,
and recorded finish time; it completes applicable client status without
replaying completion notifications or hooks. It consumes no queued work and
executes no jobs. A retry after partial cleanup is safe. An idle project
without a lock remains a no-op.

Without a complete trustworthy summary, preview reports `action: "release"`
and explains why it cannot finalize. Apply returns to collecting without
inventing a summary. Retry is a separate command. Malformed/unreadable repair
records are explicit errors, not permission to skip validation. Apply rereads
the summary and execution inventory under the state lock; a preview does not
freeze the evidence.

#### Read-only execution evidence and limits

`rotari info --json` exposes the same assessment at
`active_runs[].execution_assessment`, independently of the recorded job
outcome counts. Text includes each unit's reason. Recovery preview uses
`repair.executions`; each unit includes run/job/attempt identity, backend,
state, evidence source, observation time, native identity when available,
scope, `recorded_terminal` and possibly-live/conflict indicators. A saved
summary can conflict with positive activity even when attempt outcome files
are missing. Neither view changes execution
or outcome files.

New local attempts record the host, PID namespace, process-group PID and
leader start time in the existing attempt `job.json`. Only the same host
and namespace can inspect the group. An absent group is stopped; a group
whose leader start time matches is active. An existing group without a
matching/readable start time, including surviving children after its leader
exits, is possibly live and unknown, not safely stopped or unoverridable
activity. Legacy records without host/namespace identity and permission
failures stay unknown. Descendants that escape the group, for example with
`setsid`, are outside this check; stopped evidence is not a machine-wide
process audit.

Slurm, PBS, SGE and LSF use fresh queue/accounting queries and existing
backend parsers. Queued, held and suspended jobs count as active. A matching,
unique terminal accounting record can establish scheduler-managed
termination only alongside a successful classifiable queue observation
without active evidence. Queue absence alone, expired/missing accounting,
wrong or ambiguous native identities, errors and unrecognized states stay
unknown. PBS/LSF installations that reject queries for removed jobs may
therefore require attestation despite available history; LSF history-only
`bhist` is not used as verified evidence. SGE fetches its whole queue once
per assessment, including pending jobs.

Scheduler probes run against the scheduler selected by the current command
environment, not a stored cluster connection. Use the original scheduler
environment; they cannot audit compute-node descendants or fence a remote
host. Each command has a five-second timeout and 64 KiB output limit; the
batch has a thirty-second budget including the existing query rate gate.
Budget exhaustion is unknown.

New SSH attempts persist a private random token in their attempt metadata.
The remote wrapper stores the process-group identity and a completion marker
in an owner-only directory under the remote user's runtime directory. Recovery
queries that token over SSH and checks `/proc` plus the process-group table;
missing token state or failed/ambiguous remote process queries remain
unknown. Remote state is retained for 30 days and pruned only by a later SSH
wrapper when the matching process group is absent; active or unverifiable
groups are retained. Descendants that leave the recorded group, for example
with `setsid`, are outside the probe's scope. The remote runtime directory may
also be cleared by the host, which safely degrades the evidence to unknown.

No check kills, resubmits or restarts anything, and Web UI unlock remains
unavailable.

#### Why terminal records alone do not prove termination

The current wrapper/executor audit allows no terminal-record-only fast path:

| Path | What a terminal record establishes | What remains unverified |
| --- | --- | --- |
| Normal wrapper completion | The wrapper waited for its direct command | Background managed descendants can outlive that command; wrapper/group and allocation termination are not established |
| Cancellation | A recorded cancellation outcome; grace escalation can write it before the final kill | Wrapper, descendants/process group, and scheduler allocation may still be ending |
| Timeout | A timeout outcome; the watchdog can write it before its group kill | The same process/allocation scope may still exist when the record becomes visible |
| Local/SSH wait or lost-wrapper reconstruction | Local wait observes a direct process; SSH wait observes the SSH client, not a fresh remote scope check | Neither proves all managed local/remote descendants stopped |
| Scheduler wait | A persisted wrapper or accounting outcome; wrapper completion can be returned before another scheduler query | Fresh scheduler identity/allocation termination still needs a probe |
| Earlier retry or incomplete launch | A directory or launch record exists, even without final wrapper status | A later successful attempt says nothing about its predecessors |

See the [wrapper](../internal/executor/wrapper.go),
[SSH wait](../internal/executor/ssh.go),
[scheduler wait](../internal/executor/scheduler_shared.go), and
[recovery tests](../internal/project/repair_test.go).
Read-only process-group, scheduler, and SSH probes add scoped evidence; their
limitations are described above. A terminal record remains distinct from
execution verification.

`projectrun.Finish` still removes its own lock on a finalization failure.
Keeping it would misleadingly retain a running lock after the supervisor
exits; durable coordinator identity already prevents recovery past a live
supervisor without relying on that lock. Interrupted metadata and the
original summary remain available for explicit repair.

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
- **Launching (`rotari web --allow-exec`):** off by default. It lets anyone
  who can sign in run the commands registered in your global
  `launchers.toml`, as you, against the job artifacts the Web UI may serve
  (see [Launching commands on artifacts](INSPECT.md#launching-commands-on-artifacts)).
  The browser and API only name a registered command and an artifact; command
  text, working directories, and modes come only from that file, which rotari
  refuses unless no other user can write or replace it. The listed artifact
  roots limit what can be selected, not what a command does: a command has all
  your file permissions, and a path can change between the check and the
  command opening it. With launching on, `rotari web` always requires a token,
  even on `127.0.0.1`, since other users of a shared host can reach loopback
  ports; accepts only `localhost`, `127.0.0.1`, `[::1]`, a specific `--host`,
  and `--allowed-host` names as the `Host` header, against DNS rebinding;
  and rejects a POST from another origin, or one without `Origin` unless it
  authenticates with `X-Rotari-Token` or a bearer token, which a browser never
  adds by itself. Launched commands get neither the Web token nor stdin.
  Their output stays in a private `0700` directory under `$TMPDIR` until the
  Web server stops; a later `rotari web --allow-exec` removes directories that
  a killed server left behind.

None of this defends against another user with access to your own UID
(e.g. root, or anyone who can read your home directory), only against other
unprivileged users on a shared machine.
