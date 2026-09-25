# Operations

Server management, run registry maintenance, shared filesystems, and the security model.

## Server management

The supervisor starts automatically when a run needs it and stops after the
run finishes. These commands are mainly useful for inspection and cleanup:

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
registry after run data was removed outside rotari. First scan for orphaned
registry entries:

```sh
rotari gc
```

The candidates and their locations are printed and cached for ten minutes.
The temporary GC plan is stored at `<masterdir>/gc.json`.
Malformed or invalid registry files are listed and left untouched; inspect
their run data and repair or remove them manually.
After reviewing them, apply that exact plan:

```sh
rotari gc --apply
```

The apply step removes registry entries only. It skips candidates whose
registry location changed or whose run directory reappeared, and never deletes
run data.

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
the lock, and keeps the run as interrupted until you acknowledge it with
`unlock` or `reset --recover`. A lock created on another host is always treated
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
- **Server socket:** `<basedir>/server.sock` accepts job submission and control
  requests, so reaching it means controlling that server. It is always created
  `0600` regardless of `ROTARI_PRIVATE_STATE`, and on Linux the server also
  rejects connections from a different UID.
- **Web UI:** without `ROTARI_WEB_AUTH_TOKEN` or `--auth-token`, bind it to
  `127.0.0.1`; with a token, use only a trusted network or HTTPS proxy. Prefer
  the environment variable so the token does not appear in the process list.
  Environment variable values are never exposed over HTTP, but project and run
  pages show raw config files, which may contain secrets.

None of this defends against another user with access to your own UID
(e.g. root, or anyone who can read your home directory), only against other
unprivileged users on a shared machine.
