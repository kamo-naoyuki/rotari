# Outputs and artifacts

A run's outputs are the files its jobs write: checkpoints, plots, audio,
tables, logs for experiment trackers. Rotari records where each job's outputs
are, so the Web UI can list them with the job, preview them, open the
experiment directory around the state directory in its **Files** view, and
launch viewers such as TensorBoard or Jupyter Lab on them. This page covers:

- [Artifacts](#artifacts): which paths rotari records for each job, and how
  to list and preview them.
- [Using rotari with experiment trackers](#using-rotari-with-experiment-trackers):
  MLflow and Aim stores that jobs write without naming them.
- [Launching commands on artifacts](#launching-commands-on-artifacts):
  TensorBoard, Jupyter Lab, and your own commands on a job's outputs or in
  [the run's Files view](#the-runs-files-view) of the experiment directory.
- [Viewing tables](#viewing-tables): the built-in viewer for CSV and similar
  tables, including the audio, image, and video files they name.

For a run's status and logs, see [Inspecting runs and jobs](INSPECT.md).

## Artifacts

Rotari records candidate files and directories referenced by a job definition.
Discovery is best-effort and static: it does not run the job, verify that a
path exists, or determine whether it is an input or output. In particular, it
may not find paths that the job constructs at runtime. Recording candidates
does not affect job execution. Declare runtime-generated paths explicitly with
repeatable `rotari add --artifact PATH`; paths can use
`$ROTARI_ARRAY_TASK_ID`, `$ROTARI_JOB_DIR`, and the job's `--env` and matrix
variables, so each array task or matrix member can have its own artifact:

```sh
rotari add --array 0-9 --artifact 'results/$ROTARI_ARRAY_TASK_ID/plot.png' python train.py
```

Automatic discovery looks for likely paths in command arguments, environment
and matrix values, output destinations, and referenced configuration or script
files. A local `file:` or `sqlite:` URI counts as the path it names, so an
experiment tracker's store set as `MLFLOW_TRACKING_URI=sqlite:///runs/mlflow.db`
or `file:./mlruns` is found; other URLs and URIs are not paths. Paths in configuration and scripts are inspected without running the
job; dynamically generated paths and unreadable files may be missed. For SSH
and scheduler jobs, discovery reads files available on the host running
`rotari run`.

`rotari show -j JOB` lists the first 20 of them after the command, each with
what is at the path now on the host running `show` (`file`, `directory`,
`other`, `missing`, or `unknown` for a relative path whose directory was
never known), the path relative to the job's working directory, and where it
was found:

```text
Artifacts: relative to /work/exp
  file       train.py  (argument)
  file       conf/a.yaml  (--config)
  directory  results  (a.yaml: out_dir)
  missing    out/3.log  (> (run.sh:2:15))
```

`missing` means only that the path is absent on the host running `show`; an
SSH or scheduler job's files may be on another host. Use
`rotari show -j ATTEMPT_ID --artifacts` to list all recorded candidates and
discovery notes instead of logs; `show -j JOB --json` includes them as
`artifacts`. Older runs from before artifact recording show `(not recorded)`.
A carried job shows candidates from the attempt that produced its result.

In the live Web UI, a job row's Artifacts button opens the listing for the
attempt shown by Log, also while the job is still running (disabled until it
has started), so a tracker store or TensorBoard log directory can be opened
live; reopen the listing to see new files. Files under the job's working directory, or under the
directory holding its basedir (for example the experiment directory around
`.rotari-state`), can be previewed in supported formats (including images,
audio/video, NumPy arrays, CSV/TSV, and text) or downloaded; directories show
their immediate contents. The listing's first row, `.`, is the job's working
directory itself (it is not a candidate). It holds what a job writes without
naming it, such as the default `mlflow.db` or Aim repository, so you can
browse it and launch a viewer on it; it is served under the same roots as the
entries. Other paths are listed but not opened unless allowed
by `rotari web --artifact-root DIR`. When another user can write the project's
state (a group-writable shared basedir, or one you do not own), that user could
have chosen the job's working directory and paths, so only `--artifact-root`
directories are served for its jobs and the Artifacts view says why.
A static export records the listing as of export time. It includes no file
contents by default; `--static-artifact-contents` copies previewable files
(up to 10 MiB each and 100 MiB total) so they can be previewed there. Anyone
who can access the Web UI or an export with copied contents may be able to read
those files; see
[Web UI security](SECURITY.md#artifact-files-and-static-exports) before sharing.

## Using rotari with experiment trackers

Rotari runs the job and keeps its command, status, attempts, and logs. An
experiment tracker records and compares what the job measured; rotari does
not read tracker metrics or replace the tracker's UI.

With the default local settings, MLflow 3.17 creates `mlflow.db` in the job's
working directory and uses a SQLite tracking URI. Aim 3 creates its repository
in `.aim` under the working directory. These paths may be created by the job
at runtime and therefore need not appear among the discovered artifact
candidates. The Artifacts view includes the working directory as the `.` row,
even when the job definition named no paths:

- Choose **Aim** in the row of `.` (or use the job's **Launch** button on
  the run page) to open the repository beneath that directory.
- Open `mlflow.db` inside `.` and choose **MLflow (SQLite)**. This file
  appears after MLflow creates it.
- When the jobs share one store, such as an Aim repository or a TensorBoard
  log directory in the experiment directory around `.rotari-state`, open
  **Files** in the run page's toolbar and choose **Aim** or **TensorBoard**
  in the row of that directory, or of the directory inside it that holds
  the store, to open it for the whole run.
- A file store such as `mlruns` can be opened with **MLflow** on that
  directory. If the job names a local `file:` or `sqlite:` tracking URI,
  discovery also records its local path; remote tracking URIs are not local
  artifacts.

These built-in services are available from the live Web UI when it is started
with `rotari web --allow-exec`; see
[Built-in services](CONFIGURATION.md#built-in-services) and
[Launching commands on artifacts](#launching-commands-on-artifacts). The
basedir must be trusted or the path must be under an explicitly allowed
`--artifact-root`. The Web server must also be able to access the same
working directory and tracker store. Static exports cannot launch services.

Use the metadata rotari supplies to link each tracker run back to the exact
job and attempt. For example, an MLflow job can set tags when it starts its
run:

```python
import os
import mlflow

with mlflow.start_run():
    mlflow.set_tags({
        "rotari_run_id": os.environ["ROTARI_RUN_ID"],
        "rotari_job_id": os.environ["ROTARI_JOB_ID"],
        "rotari_attempt_id": os.environ["ROTARI_ATTEMPT_ID"],
    })
    # Log parameters, metrics, and artifacts here.
```

The same IDs can be added as tags or metadata in other trackers. A job ID
identifies the logical job; the attempt ID distinguishes retries, and the run
ID identifies the rotari batch. Tracker links make it easier to move between
the tracker's measurements and rotari's logs and job history.

Choose storage for the expected concurrency and where the jobs run. A single
SQLite `mlflow.db` written by many parallel jobs on an NFS mount can encounter
locking failures or corruption because SQLite relies on filesystem locking.
For that workload, consider an MLflow file store, separate stores per job, or
a tracking server; test the chosen store and filesystem under the expected
write load. Aim repositories also need care with concurrent writers on
network filesystems. A tracking server is a better fit when jobs on different
hosts need one centrally managed store, or when access control and shared
visibility matter. Rotari does not start or manage that server; its built-in
launchers open local stores only.

## Launching commands on artifacts

The live Web UI can run commands you registered yourself against an artifact,
for example a plotting script on a file or a viewer on a directory. It is off
by default. To turn it on:

1. TensorBoard, Jupyter Lab, MLflow, Aim, and Streamlit are built in (see
   [Built-in services](CONFIGURATION.md#built-in-services)). Register other
   commands in the global launcher file; `rotari config --launchers` writes
   a commented template (see
   [Launcher commands](CONFIGURATION.md#launcher-commands)).
2. Start `rotari web --allow-exec`. Launching requires a token on every
   listener, including `127.0.0.1`, because other users on a shared host can
   connect to loopback ports. The server always prints the sign-in user name,
   `rotari`. Unless `ROTARI_WEB_AUTH_TOKEN` (or `--auth-token`) is set, it
   generates a token and prints it. Enter the user name and token in the Web
   login form; plain HTTP works for a browser on the same host opening
   `http://127.0.0.1:PORT` or `http://localhost:PORT`, and other access needs
   HTTPS (see [Security](SECURITY.md#web-ui-authentication-and-transport)).
   Set `ROTARI_WEB_AUTH_TOKEN` to keep the same token across restarts.

Opening a service establishes an HttpOnly, same-site authentication cookie
scoped to `/_launch/`, valid for 12 hours or until the Web server restarts.
This lets JupyterLab send its own API authorization without replacing the
Rotari Web login. You do not need to enter a Jupyter token. After expiration,
reload the service page to authenticate again. The cookie is not forwarded
to the service and does not authenticate other Rotari Web APIs.

In a job's Artifacts, each row of an entry (including the working
directory row `.`), and of a file or directory inside an opened directory,
has a button per registered command that accepts it, such as
**TensorBoard**, **Jupyter Lab**, and **Aim**; an opened file or directory
has them in its title line too. A row offers only the commands that accept
a file or a directory, whichever it is, and whose name patterns match it.
On a run page, a started job's **Launch** button (shown when the server
allows launching) opens its Artifacts at the working directory's row.

Each row also has **Download** for a file. When nothing can be launched,
the rows keep only the Web UI's own viewers and the view says why and how
to change that (launching disabled, read-only server, no registered
commands, or a static export). A command whose program is not installed on
the Web server's host is shown disabled, with the reason. Choosing a command shows what will run before
anything does: the
command text, the absolute target, the working directory, the user and host
it runs as, and the job and attempt it came from. **Launch** starts it. The
browser never sends command text: it names a registered command and an
artifact, which the server resolves as it does for previews, so only
recorded candidates and their children under the allowed roots can be
targets, and they must exist on the Web server's host. The working directory
row is the only other target.

A service (such as TensorBoard) shows **Open** once it answers; it opens in a
new tab through the Web UI's own port. Open links in both Artifacts and
Launches use the server-wide service route, even when viewing another basedir.
Starting the same command on the same
target again while it runs reuses that launch and offers **Start another
instance**. **Launches** in the top bar
lists this server's launches, newest first, with their state, exit status,
and output (the server keeps at least the most recent 8 MiB of each, and the
view shows the last 1 MiB), and stops a running one: SIGTERM
to its whole process group, then SIGKILL after its `stop_grace`. Closing the
page does not stop a launch; stopping or killing `rotari web` does, even with
SIGKILL. A command that moves itself out of its process group (for example by
daemonizing) is not stopped. At most 16 launches run at once, and the 50 most
recent finished ones are listed. Launches are not runs: they do not appear in
a project's history and do not lock it.

### The run's Files view

The run's toolbar has one **Files** button, beside **View config snapshot**
with the run's other actions. It opens the directory holding
the basedir (for example the experiment directory around `.rotari-state`),
where a run's jobs can share one tracker store or TensorBoard log
directory, and you can open the directories inside it (**Up** goes back);
every row has the same
launch buttons as an Artifacts row. That directory is served under the same
roots as the run's artifacts, so only for a trusted basedir or under
`--artifact-root`, and the view cannot go above it.

## Viewing tables

The row of a table file in a job's Artifacts (`.csv`, `.tsv`,
`.tab`, `.psv`, `.txt`, `.scp`, `.lst`, `.list`, `.dat`) also has
**Table viewer**, which opens the file in a new tab. It is part of the Web
UI and runs no command, so it is offered on every live server, including
one without `--allow-exec` or a read-only one; it reads the same files a
preview may, under the same roots.

The viewer pages through the file on the server, so a file with millions of
rows opens as quickly as a small one after the server has indexed it once:

- **Separator**: comma, tab, semicolon, pipe, space, or any other single
  character. The file's extension picks the first choice (tab for `.tsv`,
  space for `.scp`). Cells in double quotes may hold the separator, line
  breaks, and doubled quotes, as in CSV; blank lines are skipped.
- **First row is a header**: whether the first record names the columns.
- **Media cells**: a cell holding the path of an image, audio, or video file
  (the types a preview shows) is a link to the file with one line of its
  properties under it, read from the file's header: format, duration,
  width × height, sample rate, channels, bitrate, and size, for example
  `WAV PCM 16-bit · 0:03.12 · 16 kHz · 1 ch · 256 kbps · 97.5 KiB`. A file
  that is missing or outside the roots a preview may read says so instead.
  Absolute paths are used as they are; **Resolve media paths from** chooses
  what a relative path is relative to: the basedir (the default), the job's
  working directory, or the table's own directory.
- **Panes**: clicking a media path opens the file above the table, in a tab
  of the active pane (Ctrl- or Cmd-click opens it in a browser tab
  instead). An image or video shows as it is; audio has a player, its
  **Waveform**, and its **Spectrogram**, which the browser computes from
  the decoded samples (the first 10 minutes of a file up to 512 MiB).
  Click a plot to seek, drag across it to zoom to that range, and scroll
  the wheel to move along; **−**, **+**, and **Fit** zoom the active pane.
  **Split right** and **Split down** open a pane beside one with its file,
  and make the new pane active, so the next file clicked opens beside the
  first: one pane is always active, outlined and marked **Files open
  here**, and clicking in a pane makes it active. Drag the borders between
  panes, or the bar under them, to resize. Drag a tab to another pane's
  tabs or its middle to move it there, or near a pane's edge to split
  that pane on that side with the tab in the new half. **Link time &
  zoom** shows the same time range and playback line in every pane, in
  seconds: zooming or scrolling one moves them all, and playing one pane
  pauses the others and starts from the shared time, to compare
  recordings of different lengths. In **Group by stem**, a row's
  **Open row** opens its files side by side, one pane each, linked.
- **Filter**: choose a column, an operator, and a value, then **+ Add**.
  `=` and `≠` compare numbers as numbers (`0.80` equals `0.8`) and other
  cells as text; `<`, `≤`, `>`, and `≥` need a number and skip cells that
  are not one; **contains** and **does not contain** match text
  case-sensitively; **is empty** and **is not empty** test for blank cells.
  A row must match every condition. Remove one with its ×, or all with
  **Clear all**.
- **Sort**: click a column name to sort ascending, again for descending,
  and a third time for the file's order. Numbers sort by value, before
  other cells.
- **Describe** adds rows under the column names summarizing each column
  over the matching rows, as pandas' `describe()` does: count of non-blank
  cells, distinct values (shown as `100,000+` past that), and the mean,
  standard deviation, minimum, maximum, and sum of the cells that are
  numbers.
- The pager moves by page, to a page number, or to the first or last page,
  with 50 to 500 rows per page. The `#` column numbers rows as they are in
  the file, also when filtered or sorted.

Filters, sorting, and Describe cover the whole file, not just the page
shown. A directory opens in the viewer too, from **Table viewer** in its row
(including the working directory `.`). Only its immediate children are read
at first; it does not recursively scan the directory:

- **Expand directories**: click a directory's arrow or path in **Files**
  to read its immediate children and insert them beneath it. Click again
  to collapse it and its descendants. Unopened directories are not scanned.
  Expanded directories are kept in the URL, so reloading restores them.

- **Pattern** chooses files by their path inside the directory: `*` matches
  within a name, `**` any number of directories, and `{wav,png}` either
  alternative, so `**/*.{wav,png}` is every WAV and PNG file. The default,
  `**/*`, matches every file in the opened scope. Patterns do not expand
  directories: directory rows stay available regardless of the pattern so
  you can open them. Directory symlinks are not followed, and at most
  100,000 entries are read across the opened scope. A listing is reused for
  up to 10 seconds.
- **Files** has one row per file or directory with its path, format, duration, sample
  rate, channels, bitrate, width, height, size, and modification time. A
  filter on `duration` takes seconds or `m:ss`, and one on `size` takes
  values such as `10M` or `1.5G`; Describe on `duration` gives the total
  and mean length of the audio. Directories have no media properties or
  file size. Filters, sorting, and Describe apply only to this opened scope,
  including directory rows in **Files**; a sort may place rows outside their
  tree order, with their full relative paths still shown.
- **Group by stem** uses files in the same opened scope (not directory
  rows); expand directories in **Files** first to include their contents.
  It puts files that share a name in one row, with a column
  per group, to compare outputs side by side and find missing ones:
  `ref/u1.wav`, `hyp/u1.wav`, and `plots/u1.png` become the row `u1` under
  `ref/`, `hyp/`, and `plots/`. **Groups from** chooses how a path splits:
  the first-level directory, the parent directory, the file extension (for
  `a.wav` and `a.png` in one directory), or the suffix after a separator
  (for `a_ref.wav` and `a_hyp.wav`); automatic uses the extension when no
  file is in a subdirectory and the first-level directory otherwise. A
  group without the stem's file shows **— missing**, and **Only stems
  missing in some group** lists just those.

The URL keeps these settings after `#`, so a reload or a shared link
opens the same view; choosing another separator clears the filters and the
sort, since the columns change. Cells longer than 4 KiB are shortened; **Download** gets the
whole file.

`rotari tableview PATH` opens the same viewer for any CSV or TSV file or
directory, outside any project, and prints its address:

```sh
rotari tableview metadata.csv
rotari tableview eval/ --port 8090
```

It reads only under that directory, or the file's, and resolves relative
media paths from there. It listens on `127.0.0.1` (port 8788, or the next
free one); on a remote host, forward the port with `ssh -L 8788:localhost:8788
HOST`. Set `ROTARI_WEB_AUTH_TOKEN` or `--auth-token` to require sign-in, as
for `rotari web`; use `rotari` as the user name and the token as the password.
