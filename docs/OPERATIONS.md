# Operations

Server management, run registry maintenance, shared filesystems, and the security model.

## Quick context

Use `rotari info` for a read-only overview of the resolved master directory,
base directory and project, visible and loaded configuration files, running
supervisors, run locks, and active or interrupted runs. Add `--json` for
machine-readable output. When multiple projects exist and none is selected,
the command reports the choices instead of guessing.

The run lock reports whether its coordinator PID is alive on this host. It
does not verify individual job processes: job statuses are recorded results,
not proof that a process is currently running. Use `jobs` or `show` for job
details. Run-lock inspection does not remove stale locks.

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

An active run is recorded in `running.lock` with its run ID, PID, and host. On
the host that started the run, rotari detects when that PID is gone, removes
the lock, and keeps the run as interrupted until you recover it with
`unlock`. A lock created on another host is always treated
as active, because rotari cannot tell whether a remote PID is still alive.
Using different project names on different hosts is therefore much safer than
sharing one project.

After confirming a failed host's run has stopped, unlock that exact run:

```sh
rotari show -p sweep
rotari unlock -p sweep RUN_ID
```

`unlock` verifies the run ID, removes a matching lock, and returns the project
to queue collection. Never use it while the run may still be executing, or a
second run could start for the same queue.

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
