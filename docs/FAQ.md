# Rotari FAQ

Answers to specific "what happens if...?" questions about rotari's behavior.
For feature walkthroughs, see [README.md](../README.md); for the underlying
contracts, see [INTERNALS.md](INTERNALS.md).

## Projects, queues, runs, and registry

For the complete base-directory and project-name precedence rules, see
[State and project resolution](../README.md#state-and-project-resolution).

### What's the difference between a project, a queue, and a run?
A project is a named container (`--project-name`) that holds one current
queue and its saved run history. The queue (`queue.json`) is the batch of
commands waiting to execute. A run is an immutable snapshot of a queue at the
moment `run` started, kept under `runs/<run-id>/` with its own logs and
results. See [Projects, queues, runs, and state](../README.md#projects-queues-runs-and-state).

### I didn't pass `--project-name` — which project does rotari use?
`--project-name`, then `ROTARI_PROJECT_NAME`, then the only project in the
resolved state directory. If no project exists yet, it defaults to `default`.
If multiple projects exist and none of the above narrows it down, commands
other than a bare `rotari show` ask you to pick one explicitly. A bare
`rotari show` lists projects across known basedirs.

### How do I list projects in a state directory?
Run `rotari show`. It lists each project's basedir, queued-job count, run
state, and latest run ID across known basedirs. Add `--basedir` to limit the
list to a specific state directory.
The output also suggests `rotari show -p PROJECT` to inspect a project and
`rotari show RUN_ID` to inspect jobs in a specific run.

### I don't know which basedir contains my jobs. How do I find it?
Run `rotari show --basedirs`. It prints the master directory and basedirs
known from saved-run and live-server registry records. Then inspect one with
`rotari show --basedir DIR`. A basedir with neither a registered
run nor a running server cannot be discovered this way. Add `--masterdir DIR`
to choose a registry explicitly.

### Where can I put option defaults?
Put `config.yaml`, `config.toml`, or `config.json` in
`$XDG_CONFIG_HOME/rotari` (or `~/.config/rotari`) for home-wide defaults, in
the resolved basedir for experiment-wide defaults, or under
`projects/<project>/` for project defaults. Later locations override earlier
ones. Common options such as `project-name` are root keys; command-specific
options are nested under the command name, such as `run.batch-concurrency`.
CLI options and environment variables override config values. If multiple
config formats exist in one directory, rotari errors rather than selecting a
format implicitly. When configs exist at multiple locations, project config has
the highest priority, followed by basedir and then global config; `show` and
the web UI display only the highest-priority path.

### How do I see every configurable option?
Run `rotari config` to print a complete template, or pass `--output FILE` to
create one. YAML and JSON use `null` for unset options; those entries are
ignored when loaded. TOML represents unset options as comments because TOML
does not define a null value.

`rotari show` also prints the resolved config path list in its header so you
can confirm which home, basedir, and project config files were active for the
current target. When `--output` is omitted, rotari interactively offers the
home, basedir, and existing project config paths, plus stdout and an `other
path` choice. Selecting stdout prints the template without creating a file;
`other path` prompts for an arbitrary file path. Add `--project-name NAME` to
limit the project candidate. Supplying `--output FILE` skips the prompt.

Options with a finite choice list, including `--executor`, `--format`, and
`--provider`, reject an explicit CLI value outside that list while parsing.
The accepted values come from the same CLI metadata used by help, shell
completion, and `rotari schema --json`.

### How can I notify another service when a run finishes?

Set `webhook.url` in a config file or `ROTARI_WEBHOOK_URL` to a service endpoint
that accepts JSON `POST` requests. Use `webhook.on` or `ROTARI_WEBHOOK_ON` with
`success` or `failure` to filter events; the default is `always`. The payload
includes run status, failed job IDs, and, for failed runs, a copy-pasteable
command to display their logs. A webhook error is only a warning and does not
alter the run result. Set `webhook.format` to `slack`, `teams`, or `discord`, or set
`ROTARI_WEBHOOK_FORMAT` to one of those values, to send a service-specific
payload directly. The default format is the generic rotari JSON payload. See
[Webhook integrations](WEBHOOK_INTEGRATIONS.md) for examples.

### How are concurrency and executor options selected?
`--local-concurrency` applies to local jobs. `--batch-concurrency` is the
common dispatch default for non-local executors, while `--ssh-concurrency`,
`--slurm-concurrency`, `--pbs-concurrency`, and `--lsf-concurrency` provide
independent limits. Similarly, `--executor-option` is the common option list,
and the executor-specific `--ssh-options`, `--slurm-options`, `--pbs-options`,
and `--lsf-options` override it. Job-specific options have the highest
priority.

### `rotari show` displayed my queue, not the run I expected — why?
With no project, run, or job selector, `show` lists projects across known basedirs.
Use `--basedir/-b` to limit that list to one basedir. With
`--project-name/-p`, `show` lists the project's runs and includes its current
queue when non-empty. Pass `--run-id` to target a specific run regardless of
current queue state; `--run-id latest` selects the latest saved run.

### How do I clean up run registry entries left by manual deletion?
Run `rotari gc` to scan for registry entries whose run directories no longer
exist. It caches the candidates for ten minutes and does not delete anything
by itself. Malformed or invalid registry files are reported and left
untouched; inspect their run data and repair or remove them manually. Review
the result, then run `rotari gc --apply`; it removes only unchanged candidates
and skips any run directory that has reappeared.

## Retries, copying, arrays, and dependencies

### How do I add a command and run it asynchronously?
Add the command first, then start the queue with `rotari run --async`.

### What exit status does `rotari run` return when a job fails?
For a synchronous run, `rotari run` returns `0` when every job succeeds and
`1` when any job fails; it does not propagate an individual job's exit code.
`rotari run --async` returns `0` once the background run starts successfully,
regardless of its eventual result. Use `rotari wait PROJECT` or
`rotari wait --run-id RUN_ID` to wait
for an asynchronous run and obtain its recorded run exit status.

### Can an array run only selected task IDs?
Yes. Use `--array 1,3,4` for a sparse task list (ranges such as `1-10` are
also supported). Sparse lists run as independent scheduler submissions, so
they do not require native sparse-array support from PBS, LSF, or Slurm.

### I ran `rotari retry` — which jobs actually rerun?
`retry` is shorthand for `run --failed --unfinished`: jobs that failed or
never finished are re-executed; jobs that already succeeded are carried
forward into the new run with their previous result, not re-run. A job
explicitly cancelled during a run is not automatically retried by that run's
`--retry=N` loop, but it can be selected by a later `rotari retry`. Use
`--success` to force-rerun jobs that already succeeded.

### Does retrying an array job rerun every task?
No, by default (`--partial-array=true`) only the tasks matching the filter
(e.g. the failed ones) are re-executed; the rest carry forward their own
previous result. Pass `--partial-array=false` to rerun the entire array
whenever any one task matches, matching pre-partial-array behavior.

### Why did `copy` reuse the same job ID instead of generating a new one?
`copy` only assigns a new job ID when the original one would collide with a
job already in the destination queue. Otherwise the original ID — and any
dependency relationships between copied jobs — is preserved.

### Can I copy a job without its prerequisite?
Yes. `copy` removes an omitted prerequisite from the copied job. If that
prerequisite did not succeed in the source run, `copy` rejects the selection;
include the prerequisite in the copy selection before retrying.

### If a prerequisite job (`--depends-on`) fails, what happens to the jobs that depend on it?
They are recorded as `blocked` and are never executed for that run. A retry
reruns only the failed prerequisite (and any other failed/unfinished jobs);
once it succeeds, the previously blocked dependents run on the next
`rotari run`/`retry` that includes them.

## LLM diagnosis

### Can I diagnose common failures without sending logs to an LLM?
Yes. For finalized failed jobs, rotari runs local rule-based diagnosis
automatically and saves the result in that run's `summary.json`. The saved
analysis is informational only and does not affect job status, retries,
dependencies, or scheduler control. Every finalized failed job records a
recognized diagnosis, an explicit no-match result, or an analysis-unavailable
result when output cannot be read.

Use `rotari show --run-id RUN_ID --job-id JOB_ID` to display saved diagnoses
in the CLI. The Web UI always shows a `Diagnosis` button beside each job's log
control; it is enabled for finalized failed jobs with saved analysis and opens
the evidence and suggested next steps.

You can also run `rotari diagnose --run-id RUN_ID --job-id JOB_ID --rules` to
check a saved job manually. The `diagnose` command/API is experimental. With
`--rules`, it is a local, read-only signature check: it sends nothing over the
network and needs neither an API key nor a model. It recognizes only the
documented error patterns, shows the matching log or scheduler-error line, and
reports no diagnosis when none matches. See [local diagnosis
rules](LOCAL_DIAGNOSIS.md) for the exact patterns and suggested actions.

### What does `rotari diagnose` send to an LLM?
The `diagnose` command/API is experimental; its options, provider behavior,
prompt, and response format may change in future releases.

It sends one selected job's command, recorded exit code/error, and no more
than the final 12,000 characters of that job's output log. It sends nothing
until `ROTARI_LLM_API_KEY` and `--model` (or `ROTARI_LLM_MODEL`) are supplied.
The key and returned diagnosis are not saved in Rotari state or made available
to job processes. Inspect the log first if it may contain sensitive data.

### How do I choose the diagnosis response language?
Pass `--language` with a BCP 47 tag, for example `--language ja` or
`--language en-US`. Set `ROTARI_LLM_LANGUAGE` to make that tag the default.
Without either, Rotari does not choose a language and the model decides.

## Interrupted runs and locking

### How can I check whether a project is ready to run without changing it?
Run `rotari check --project-name PROJECT`. It exits with status 0 when the
project is idle and has at least one queued job with valid dependencies, and
status 1 otherwise. The output distinguishes an active local runner, a stale
local lock, a lock from another host, an interrupted run, and an empty queue.
For an idle project, the same preflight used by `run` validates executor names,
option quoting, required SSH targets, and scheduler array options reserved by
rotari. It also validates array ranges, selected task ordering and bounds, and
job ID uniqueness after array expansion. Empty commands, invalid job IDs,
malformed environment assignments, and process-incompatible working-directory
values are rejected as well. It does not connect to external hosts or
schedulers, and does not test whether a working directory exists.
For an active or interrupted project, an unreadable queue is shown as
`queued=unknown` so the runner state remains visible.
The command is read-only: it does not remove even a stale lock.
Use `--json` for machine-readable output. Its `queued` field is a number when
the queue can be read and `null` when an active or interrupted project's queue
cannot be read.

If the run lock, project metadata, or saved run files disagree, `check` reports
an error instead of declaring the project ready. `run` performs the same safety
check before starting, and `reset` performs it before recovering an interrupted
run.

`check --deep` additionally verifies that the required local, SSH, or scheduler
client executable is available on the current host. For local jobs it checks
the working directory and resolves the command using the job's effective
`PATH`. It does not connect to SSH hosts or schedulers, and it does not check a
remote working directory locally.

### What happens if runners on multiple hosts use the same project?
This is supported when every host sees the same `basedir` through a shared
filesystem whose `O_EXCL`, atomic rename, and advisory `flock` operations work
correctly (for example, an NFSv4 mount configured for file locking). Rotari
uses the state lock to serialize queue updates and the run lock to prevent two
runners from starting the same project at once. This is still file-based
coordination, not a distributed lock service: it cannot fence a host after a
network partition, verify a remote PID, or repair inconsistent/stale mounts.
If the hosts use different project names, their queue, metadata, and run locks
are separate, so direct state-file interference is much less likely; the
shared `basedir` server/registry and filesystem still remain common
infrastructure.
If a host fails during a run, confirm independently that its jobs have
stopped before using `rotari unlock --run-id RUN_ID` from another host. Do not
run the same project through mounts that do not share a consistent view of the
state files.

### A runner or supervisor process died mid-run — what do I do?
Rotari does not run a watchdog that automatically restarts a crashed detached
supervisor. If the supervisor is killed, runs out of memory, or the host is
rebooted, the jobs themselves may keep running; local jobs write their own
`status.json` through a wrapper. The run lock records the supervisor PID and
host, so `rotari show --run-id RUN_ID` is the first place to inspect whether
jobs kept running or reported completion after the supervisor disappeared.
Once you've independently confirmed that the jobs really stopped, run
`rotari unlock --run-id RUN_ID` (the exact command is shown by `show`) to
acknowledge the stopped run and keep its retained queue. To discard the
retained queue instead, run `rotari reset` — interactively it asks for the same
confirmation, or supply it up front with `rotari reset --recover`. `add`,
`copy`, and `run` stay blocked until one of these is run.

### A remote host's lock looks stuck even though the job actually stopped — why won't `unlock` go away automatically?
Rotari only auto-clears a run lock by checking whether the PID that created
it is still alive, and it can only do that on the same host. A lock created
on another host is always treated as active. Confirm independently that the
run has really stopped, then run `rotari unlock --run-id RUN_ID` explicitly;
running it while the job might still be alive can let a second run start
against the same queue.

## Client control and job cancellation

### Is it safe to Ctrl-C a synchronous `rotari run`?
Yes, but the terminal returns immediately, before jobs actually stop. Ctrl-C
prints "Cancellation requested..." and exits with code 130 right away — it
doesn't wait for cleanup. The background supervisor keeps working after your
shell prompt is back: it cancels the still-running jobs and only then does
normal finalization (summary, queue clearing, lock removal). If you run
`add`/`copy`/`run` against the same project right after Ctrl-C, expect it to
be rejected as "not allowed" until finalization catches up a moment later —
that's normal and needs no `unlock`, unless the process is killed outright
(see the next question).

### Can I detach a synchronous run without cancelling it?
Yes. Press Ctrl-D while `rotari run` is waiting for progress. The client exits
cleanly and the run continues in the background; this is equivalent to
starting the run with `--async` after it has begun. This is the key difference
from Ctrl-Z: Ctrl-D is rotari's detach command, while Ctrl-Z is the shell's
job-control suspend command.

### What happens if I press Ctrl-Z during a synchronous run?
The terminal suspends the foreground `rotari` client, but the run continues in
the background because it is supervised by the separate server process. Use
`fg` to resume the client and keep watching progress. Do not use Ctrl-Z as a
way to detach: the stopped client is still a shell job, and its terminal
connection is not cleanly handed off. If you close the terminal while it is
stopped, the client is killed and the server sees a disconnect, which requests
cancellation of the run. Use Ctrl-D for a one-way detach, or start with
`rotari run --async` when you already know you do not need the interactive
progress view.

### Can `wait` find the run ID for me?
Yes. With no selector, `rotari wait` scans the resolved basedir and waits when
exactly one project is running. If multiple projects are running, it prints
their project and run IDs so you can choose one. A positional selector is
resolved as project name, active run name, then run ID, in that priority order.

### How does rotari actually stop a running job on Ctrl-C or `rotari cancel`?
It depends on the executor. `local` runs the job command through its own
self-reporting wrapper script in its own process group, and sends `SIGTERM` to
that whole process group, so both the wrapper and the command it launched are
signaled — but a command that itself spawns further children of its own still
has to forward the signal to those if you want them killed too. For `ssh`,
rotari starts the remote command in its own process group and records its PID
and Linux `/proc` start time in a private runtime directory. Cancellation
reconnects over SSH, verifies both values to avoid signaling a reused PID, and
signals that remote process group. Older SSH jobs without this metadata retain
the previous local-client cancellation behavior. `slurm`/`pbs`/`lsf` call the
scheduler's native cancel command (`scancel`/`qdel`/`bkill`) against the job's
cluster ID instead of signaling a local process. In every case rotari only asks
the job to stop (`SIGTERM` or the scheduler equivalent); it never escalates to
`SIGKILL` for you, so a job that ignores the signal keeps running until it exits
on its own or you intervene manually.

### I ran the runner on one host and `rotari web`/CLI on another over a shared base directory — why do `cancel`/`suspend`/`resume` say the job isn't running even though it clearly is?
For a `local`-executor job, those commands signal the job by PID, and a PID
is only meaningful on the host that actually spawned the process. Run the
command from the same host as the runner instead. rotari detects this
mismatch by comparing the current host against the run's recorded hostname
and reports which host the job actually runs on, rather than a bare "not
running".

### What about plain `rotari cancel` with no `--job-id`, cancelling the whole run at once — does that have the same cross-host problem?
Yes, and it used to be worse: without a specific job, cancel signals the
runner's whole process group by the PID recorded in `running.lock`. From the
wrong host that PID doesn't exist, and the old code treated "no such
process" as "already stopped" and reported success — silently doing nothing
while the real runner, on another host, kept going. rotari now checks the
lock's recorded host first and errors instead of guessing, the same way
per-job `cancel`/`suspend`/`resume` do.

### Same setup, but the job runs through Slurm/PBS/LSF instead of `local` — why do I still get an error from `rotari web`/CLI on another host?
Unlike `local`, these executors don't use PID signaling, so there's no host
mismatch check for them. But `cancel`/`suspend`/`resume` still shell out to
that scheduler's own client command (`scancel`/`scontrol`, `qdel`/`qsig`,
`bkill`/`bstop`/`bresume`), which must be installed and configured on
whichever host runs it. A web/CLI host outside the cluster that only shares
the state directory over NFS typically doesn't have those binaries, so the
command fails with a "command not found" error naming the missing binary
instead of a scheduler response. Run `rotari web`/CLI from a host that has
that scheduler's client tools installed (e.g. a login node), or use `ssh` as
the executor, whose control commands only need local SSH access to the
remote host.

### The run/job status column says "running", but for a Slurm/PBS/LSF job that can also mean it's actually still queued (`PENDING`) in the scheduler, not executing yet — what happens if I `suspend`/`cancel` it then?
rotari's own "running" here only means "submitted and not yet finished", not
"currently executing on a compute node" — it doesn't poll the scheduler
before every UI render. `cancel` works either way (`scancel`/`qdel`/`bkill`
also dequeue a still-pending job). `suspend`, however, is rejected by the
scheduler itself for a job that hasn't started running yet; rotari surfaces
that scheduler's own explanation (e.g. Slurm's `slurm_suspend error: Job is
not running`) instead of a bare "exit status 1", so the error tells you the
job is pending rather than actually running. `resume` on a job that was
never suspended is normally a harmless no-op for the same reason.

## Python interface

### Is there a Python API?
Yes. The optional `python/` package is a thin subprocess wrapper around the
`rotari` executable. It does not reimplement queue or execution behavior.
Install it with
`python3 -m pip install --no-deps ./python`; `wait` and `show` consume the
CLI's JSON output, while CLI errors remain exceptions. It accepts executable
argument lists, not Python functions or closures to serialize and submit. This
is deliberately different from function-oriented frameworks such as
[Submitit](https://github.com/facebookincubator/submitit). Build the generated
HTML API reference with the command in the
[Python client documentation](../python/README.md#api-documentation).

## Web UI

### Does closing/stopping `rotari web` stop my jobs?
No. The web UI is a separate, optional process you start explicitly
(`rotari web`) and never starts or stops the runner itself — closing it has
no effect on any run. It is unrelated to the background supervisor described
below.

### Can I start a run from the Web UI?
No. Even with `--allow-control` enabled, the web UI has no endpoint that
launches a run; its control operations (cancel/suspend/resume/change/remove/
copy/clear) only ever act on an already-running run or edit the persisted
queue. Starting execution is deliberately left to the CLI (`rotari run`) or
your own scheduler/automation, so that no HTTP request can, by itself,
trigger command execution on the host.

### What do Create and Append do on a run page?
Select one or more jobs in the jobs table, then use `Create` to replace the
current queue with those jobs or `Append` to add them to the current queue.
The web UI only updates the persisted queue; a separate runner must execute
the queued jobs.

### How do I view an older job attempt in the Web UI?
Use the arrow beside a job's attempt ID and choose an attempt. The row switches
to that attempt's status, timestamps, result, and log. The selection is per
job, so multiple rows can show different attempts at the same time.

### Does the Web UI send run details to an AI service?
No. A run or job's `AI` button prepares a Markdown report locally in the
browser. `Copy` only writes it to the clipboard. The `Open ChatGPT`, `Open
Gemini`, and `Open Claude` buttons also open that service in a new tab, but
they do not paste or submit anything; review the report and paste it yourself.
Failed-job reports include at most the last 100 log lines and 12,000 characters
per log. The same report is available in a terminal with `rotari show
--run-id RUN_ID --report`, optionally with `--job-id JOB_ID` or `--failed`.
Reports redact known hostnames and paths and apply heuristic redaction to common
path and hostname patterns in logs. This is not a guarantee that every secret
has been removed, so review the report before pasting it into an AI service.

### What does the project page's “Project runtime” panel show?
It shows the persisted runner-lock record for that project, when present, and
whether the local coordinator's socket and PID records exist. It is a
troubleshooting view, not a liveness probe: in particular, the short-lived
advisory state lock is intentionally not inspected.

### Is `rotari web` safe to expose beyond `127.0.0.1`?
Set `ROTARI_WEB_AUTH_TOKEN` (or `--auth-token TOKEN`) and send the token as
`Authorization: Bearer TOKEN` or `X-Rotari-Token: TOKEN`; the browser UI can
use Basic authentication with username `rotari` and the token as the password.
Prefer the environment variable so it does not appear in the process list.
This protects the HTTP routes from unauthenticated requests, but it does not
encrypt traffic; use HTTPS or a trusted/private network. Without a token, treat
the UI as an unauthenticated admin surface and keep it on loopback. The
`copy`/`change`/`remove`/`cancel`/`clear-run` APIs are enabled by default;
pass `--allow-control=false` for a read-only UI that rejects them with `403`.
Environment variable *values* are never returned by `/api/state` or
`/environment/` (only whether each is set), so secrets in your shell
environment are not exposed.

## Background server (supervisor)

### Do I need to start a server manually?
No. Unlike `rotari web`, this supervisor process is never started by hand —
`run`/`add`/`copy` launch it automatically when needed, it is not shown
anywhere in their own output, and it stops on its own once the run finishes
(or once idle, for a server kept up for other reasons). `rotari server
status`/`list`/`shutdown` are for inspection and manual cleanup only — useful
if you want to confirm the supervisor is still finishing a Ctrl-C'd run, or
to force it down.

### Who can control my jobs through the server's Unix socket?
Anyone who can connect to `<basedir>/server.sock` can `submit`/`cancel`/
`suspend`/`resume`/`run`/`shutdown` — there's no separate authentication.
The socket itself is always created `0600` regardless of other settings, and
on Linux the server additionally checks the connecting peer's UID
(`SO_PEERCRED`) against its own before accepting. This protects against
other unprivileged users on a shared machine, not against root or anyone
who already has your UID's access.

### Can other people on the cluster read my job logs/commands, or is everything locked down now?
By default, yes — the state directory (`queue.json`, job `output`, working
directories, etc.) keeps rotari's traditional `0755`/`0644` permissions, so
you can still hand a colleague a log path directly, which is common on
shared HPC/lab filesystems. If you'd rather keep that private, set
`ROTARI_PRIVATE_STATE=true` to switch newly created state to owner-only
`0700`/`0600`; it doesn't retroactively change existing directories, and the
server's control socket is always `0600` either way (see above).

## Timestamps and environment

### What timezone are the times shown by `show`/`web` in?
Everything is stored in UTC. For display, rotari uses a valid IANA timezone
from `TZ` if set, otherwise Go's local location (the system timezone via
`/etc/localtime` on Linux).

### A CLI option and its matching `ROTARI_*` environment variable are both set — which wins?
The explicit command-line option always takes precedence over the
environment variable. When an option has an environment-variable default, its
name appears in that command's `--help` output.
