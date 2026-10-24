# Promo video pipeline

These scripts make rotari's videos from real runs: rotari, its Web UI, and a
one-node Slurm cluster run in Linux namespaces, and a headless browser
renders the material into frames for ffmpeg. Every command, log line, and Web
UI screen in a video comes from those runs. They are a production aid, not
part of rotari or its tests.

All videos have English narration by Kokoro TTS (voice `af_heart`), burned-in
subtitles, and an `.srt` beside the video, and no music. The short is
vertical (1080×1920) for promotion; feature videos are landscape (1920×1080),
one per documentation page, linked from that page.

## Episodes and documentation

| # | Video | Page it accompanies | Script | State (2026-10-24) |
| --- | --- | --- | --- | --- |
| 01 | The short: one job needs no rotari, many do | README, Getting started | `01_short.sh` | done, 78 s (96 s with the Slurm act) |
| 02 | Notifications: desktop, Inbox, Slack | `docs/NOTIFICATIONS.md` | `02_notifications.sh` | written, not yet captured in landscape |
| 03 | Outputs: Artifacts, previews, TensorBoard, Files | `docs/OUTPUTS.md` | `03_outputs.sh` | done, 53 s |
| 04 | Rerunning failures and diagnosis | `docs/RECOVERING.md`, `docs/LOCAL_DIAGNOSIS.md` | | planned (the short's sweep act) |
| 05 | Local, SSH, Slurm | `docs/EXECUTORS.md` | | planned (the short's Slurm act) |
| 06 | Arrays, matrices, dependencies | `docs/RUNNING.md` | | planned |
| 07 | The Web UI across projects, Activity | `docs/WEB.md` | | planned |
| 08 | History and comparison (lineage) | `docs/INSPECT.md` | | planned |
| 09 | AI agents (CLI guide, MCP) | `docs/MCP.md` | | planned |

`docs/OUTPUTS.md` was split out of `docs/INSPECT.md` so that video 03 has one
page to accompany.

## The short (01)

### Message

Rotari makes the experiment cycle (write code, submit jobs, check how they
ended, fix and resubmit failures, inspect results) less tedious. With one job a
terminal is enough, so the video opens by saying so: rotari pays off when
something becomes plural.

- **Places:** projects in different directories. Without rotari, one terminal
  tab each and `cd` to check; with rotari, every project of a state directory
  is on one Web UI page.
- **Numbers:** a 48-run sweep where 8 runs fail for two reasons (a GPU fault
  and CUDA out of memory). Rotari groups failures by diagnosed cause with a fix
  each; after the batch size is fixed, a plain `rotari retry` reruns only the
  8 failures while the 40 successful runs carry forward. Retrying one cause
  (`--filter-diagnosis`) was tried and dropped as too specialized for a first
  video.
- **Machines (optional act):** Slurm and local jobs are submitted and watched
  differently by hand; with rotari only `-e local` / `-e slurm` differs, and
  one run can hold both.

Claims stay within what rotari does: diagnosis covers known log patterns only,
the Web UI reads one basedir at a time (other basedirs are in the sidebar), and
rotari is not a workflow engine. The retry succeeds because the fixture's
failures really are fixed: the GPU fault happens on the first attempt only, and
the OOM depends on `max_batch` in `fixture/sweep.conf`.

### Storyboard

Each scene's on-screen caption and narration line (`say`, with `sayAt` when
it should not start right away) are in `render.py` (`timeline()`); that file
is the source of truth. The lines were translated from a reviewed Japanese
draft, summarized here:

| Time | Scene | Narration |
| --- | --- | --- |
| 0:00 | `python train.py` in one terminal: "One job?" | One job? Watching it in a terminal is enough. |
| 0:06 | Title: "You don't need rotari. / But research isn't one job." | You don't need rotari. But research is never just one job. |
| 0:11 | Four tabs (`asr`, `tts`, `vision`, `nlp`) with `tail`; vision has crashed | Each project lives in its own directory… something broke. |
| 0:18 | `cd` + `rotari add -p NAME` + `rotari run --async` in each directory | With rotari, you submit from any directory, |
| 0:26 | Web UI All projects, live: vision turns failed | and every project shows up on one page… |
| 0:34 | `rotari add --array 1-48 ./sweep.sh`, `rotari run --async` | Now a parameter sweep. Forty-eight runs. |
| 0:41 | Run page live: failures appear | Eight of them fail. |
| 0:49 | Two log lines: GPU fault, CUDA OOM | For different reasons. Do you open every log? |
| 0:53 | Run page Failure causes: three groups, fix and retry command each | Rotari groups the failures by cause, and suggests a fix for each. |
| 0:58 | `sed -i 's/max_batch=128/max_batch=64/' sweep.conf`, `rotari retry -p llm-sweep` | Fix the batch size, and rerun only the failed jobs… |
| 1:06 | Retry run page: 8 rerun, 40 carried, 48 succeeded | All forty-eight done. |
| (opt.) | Slurm (`sbatch`/`squeue`/`cat`) above local (`nohup`/`ps`/`tail`) | Preprocessing runs locally, training on Slurm… |
| (opt.) | `rotari add -e local …` and `-e slurm --depends-on …`; run page jobs table with workstation and gpu-node01 | …Only the executor changes. One run, two machines. |
| 1:10 | All projects again | More jobs. Still one place to look. |
| 1:13 | Logo and GitHub URL | Rotari. Spend your time on research, not on babysitting jobs. |

## Narration

`render.py --narration KOKORO_DIR` (which `make-video.sh` passes) speaks each
line with [Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M) through
`kokoro-onnx` on the CPU, about 2 s per line. `narration.py` caches each clip
in `WORK/capture/narration/` by text, voice, and speed, so editing one line
synthesizes only that line. A scene shorter than its line is lengthened; the
clips are mixed into one track normalized to -1 dBFS and muxed as AAC. Each
line is also burned in below the frame and written to `OUT.srt`.

`--voice` picks another Kokoro voice (`af_heart` is the default; `am_michael`,
`bf_emma`, and `bm_george` are among the others) and `--speed` changes the
pace. Nobody has listened to the result in this pipeline's checks: review the
pronunciation of "rotari" and the CLI terms by ear before publishing.

## Files

| File | Role |
| --- | --- |
| `01_short.sh`, `02_notifications.sh`, `03_outputs.sh` | Make one video each into `out/`; the usual entry points (`defaults.sh` sets their directories) |
| `setup.sh TOOLS` | Install Playwright and kokoro-onnx into `.venv`; fetch Chrome, the Kokoro model, and the Slurm image into `TOOLS` |
| `make-video.sh TOOLS WORK OUT.mp4 [opts]` | Build rotari, capture, and render the short |
| `capture.sh` | Enter the namespaces, start Slurm, run `capture.py` |
| `capture.py` | Run the scenario; record terminal sessions and Web UI screenshots |
| `slurm-up.sh` | Start slurmdbd, slurmctld, and slurmd from the image; write client wrappers |
| `render.py`, `player.html` | The short's scene timeline, and the HTML player every video renders through (portrait or landscape) |
| `renderer.py` | Shared rendering: options, narration, player page, frames into ffmpeg |
| `common.py` | Shared capture helpers: commands, terminal sessions (`Term`), Web UI frames |
| `narration.py` | Kokoro TTS clips, scene fitting, the audio track, and subtitles |
| `fixture/` | Fake jobs: `train.py`, per-project `train.sh`, `sweep.sh`, `prep.sh`, `train-gpu.sh` |
| `fullpage.py` | Framing aid: full-page screenshots of a captured state, run in a namespace |
| `make-episode.sh [--display] TOOLS WORK OUT.mp4 CAPTURE.py RENDER.py` | Build rotari, capture, and render one feature video |
| `capture-web.sh [--display]` | Enter the namespaces and a namespace-owned `/`, with Xvfb, D-Bus, and dunst when asked, and run a capture script |
| `capture_notifications.py`, `render_notifications.py` | Video 02 (the capture needs Xvfb, dunst, google-chrome) |
| `capture_outputs.py`, `render_outputs.py` | Video 03 (a headless browser is enough) |
| `assets/slack/` | Slack screenshots of one capture's webhook message, for video 02 |
| `newroot.sh`, `dunstrc` | A namespace-owned `/` for `--allow-exec`; the desktop notification style |
| `tts_samples.py`, `fixture/tts_train.py` | Kokoro clips and the fake TTS job that "produces" them |
| `send-webhook.py` | Post the Slack payloads a capture recorded to your real webhook |

## Making a video

Each video has a numbered script that does everything (fetch the tools on
first use, build rotari, capture, narrate, render) and writes
`scripts/promo-video/out/NN_name.mp4` with an `.srt` beside it:

```sh
scripts/promo-video/01_short.sh           # out/01_short.mp4
scripts/promo-video/02_notifications.sh   # out/02_notifications.mp4 (needs Xvfb)
scripts/promo-video/03_outputs.sh         # out/03_outputs.mp4
scripts/promo-video/01_short.sh --with-slurm          # include the Slurm act
scripts/promo-video/01_short.sh --voice am_michael    # another narrator
scripts/promo-video/03_outputs.sh --preview 20 45     # PNGs in the work directory
```

The tools go to `PROMO_TOOLS` (default `~/.cache/rotari-promo`, about 600 MB)
and captures to `PROMO_WORK` (default `$TMPDIR/rotari-promo-$USER`); set `GO`
when `go` is not on `PATH` or in `/usr/local/go/bin`. Options after the
script name go to the episode's renderer. `out/` is not tracked.

Underneath, `setup.sh TOOLS` prepares the tools, and `make-video.sh` (the
short) and `make-episode.sh` (feature videos) capture and render one video
into a given path:

```sh
scripts/promo-video/setup.sh "$TMPDIR/promo-tools"
scripts/promo-video/make-video.sh "$TMPDIR/promo-tools" "$TMPDIR/promo-work" out.mp4
scripts/promo-video/setup.sh --with-slurm "$TMPDIR/promo-tools"   # also fetch the Slurm image
```

Capturing takes about 2 minutes and rendering about 5. To change only
timing, captions, narration, or layout, render the existing capture again
(leave out `--narration` for a silent cut):

```sh
PLAYWRIGHT_BROWSERS_PATH="$TOOLS/pw" .venv/bin/python \
  scripts/promo-video/render.py "$WORK/capture" . out.mp4 --narration "$TOOLS/kokoro"
```

For a feature video, run its `render_*.py` on `$PROMO_WORK/NN_name/capture`
the same way (video 02 also takes `--slack scripts/promo-video/assets/slack`).

Requirements: Linux with unprivileged user namespaces, `ffmpeg` with libx264,
`curl`, `unzip`, Go, and the repository's `.venv` (created with uv; `setup.sh`
adds Playwright, kokoro-onnx, and soundfile to it with `uv pip install`). Set
`PYTHON` to use another interpreter. The Slurm act also needs `singularity`
or `apptainer`; only then do `setup.sh --with-slurm` and `make-video.sh
--with-slurm` start Slurm, so hosts without singularity can make the default
cut. The Kokoro model is 350 MB, Chrome about 270 MB unpacked, and the Slurm
image 1.5 GB unpacked.

`.venv/bin/python` is a symlink to the system interpreter. The repository is
shared between hosts, so point it at a path every host has
(`/usr/bin/python3.10`); a link to `/usr/bin/python` broke on a host without
that name.

## How the capture is isolated

`capture.sh` runs everything in user, UTS, mount, network, and PID namespaces,
so nothing outside `WORK` changes and nothing private reaches the screen:

- The host name is `workstation`; Slurm jobs see `gpu-node01`. Real host
  names, which rotari records and the Web UI shows, never appear.
- `/home` is a tmpfs with `WORK/home` mounted at `/home/me`, so paths read
  `/home/me/asr`. Rotari's default basedir and `ROTARI_MASTERDIR` are inside
  it. Do not run the scenario's rotari commands outside the namespace: a
  global `~/.config/rotari/config.toml` or `ROTARI_*` variable would send jobs
  to the user's real state directory, as happened once while this was built.
- The network namespace has only loopback, plus a dummy interface with an IPv4
  address. Without that, `getaddrinfo` with `AI_ADDRCONFIG` resolves no IPv4
  names and Slurm cannot find its controller. The daemons' ports cannot clash
  with the host's.
- Exiting `capture.py` ends the PID namespace and every daemon in it.

## Pitfalls already solved

- **No Docker:** the docker socket needs the docker group. `setup.sh` unpacks
  the image `scheduler-integration.sh` uses with singularity instead, and
  `slurm-up.sh` runs it by chroot as namespace root.
- **setgroups:** an unprivileged user namespace refuses `setgroups`, so slurmd
  exits. `setup.sh` preloads a shim in the image that makes it succeed.
- **MariaDB 5.5:** no `CREATE USER IF NOT EXISTS`; the grant creates the user.
  It runs with `--skip-networking`, since the host may already use port 3306.
- **Static rotari:** batch jobs run inside the CentOS 7 image, whose glibc is
  too old for a cgo build, so `make-video.sh` builds with `CGO_ENABLED=0`.
- **TLS-inspecting proxy:** `playwright install` fails on it, and uv needs
  `--native-tls`; `setup.sh` downloads the matching Chrome Headless Shell and
  the Kokoro model files with curl.
- **Readable frames:** the Web UI is captured 600 CSS px wide at scale 2.
  Full-page shots pan between page positions that `capture.py` records
  (`logs.json` `offsets_*`). `SQUEUE_FORMAT` narrows `squeue` for the split
  screen. The pipeline run's jobs table is narrowed through its Columns menu.
- **Fonts:** the player's IBM Plex faces are embedded as data URIs, and
  `render.py` stops if any face fails to load instead of rendering in a
  fallback font. Chrome's `FontFace` API refuses `file://` font URLs.

## Feature videos

Feature videos are landscape: the player puts the caption on the left, the
frame on the right, and the narration subtitle under it. The Web UI is
captured 1100×782 CSS px at scale 2, which keeps Files and Artifacts tables
whole. Each capture runs through `capture-web.sh`, so `newroot.sh` gives the
namespace its own `/`: `rotari web --allow-exec` checks that every directory
from `/` to the launcher file is owned by the user or root, and in a user
namespace the real `/` looks owned by nobody. Web UIs started with
`--allow-exec` use a fixed token and are signed in to over
`http://127.0.0.1` before recording (loopback HTTP sign-in is allowed since
2026-10-23). The fake TTS job writes its outputs with f-strings
(`f"{out}/loss.png"`), as training code commonly does: artifact discovery
skips f-strings, so only `--out outputs/exp0415` is listed, while plain
literals such as `"loss.png"` were listed as missing paths under the working
directory.

### Notifications (02)

A TTS run is checked by hand ("Is it done yet?"), then submitted with rotari;
the moment it ends the desktop shows the Web UI's notification, the Inbox
lists it, and Slack gets the webhook message ("Away from your desk?").

```sh
scripts/promo-video/02_notifications.sh
.venv/bin/python scripts/promo-video/send-webhook.py "$PROMO_WORK/02_notifications/capture"
```

- **Desktop notification:** `capture-web.sh --display` starts Xvfb
  (2200×1564), a D-Bus session, and dunst (1.5 syntax in `dunstrc`) inside the
  namespace. A headed `/usr/bin/google-chrome` at scale 2 opens the run page,
  turns on the Web UI's notifications, and ffmpeg x11grab records the screen
  (`-draw_mouse 0`) while the run ends. Chrome runs as the namespace root, so
  it needs `--no-sandbox`; `--test-type` hides the warning bar, `--lang=en-US`
  with Translate disabled keeps a translation prompt out of the frame, and
  the profile turns the password manager off. Xvfb runs with `-nolock` (its
  lock file would land in the host's shared `/tmp`) and `-nocursor`.
  `render_notifications.py` finds the notification's arrival by comparing the
  top right of each frame with the first.
- **Slack:** the namespace has no network, so the capture's
  `notifications.toml` points the webhook at a local recorder
  (`WORK/capture/webhook/NNN.json`), with the same fields as the user's own
  settings. Afterwards `send-webhook.py` posts those exact payloads to the
  `[webhook] url` of `~/.config/rotari/notifications.toml` without printing
  it. Screenshots taken in Slack then (the desktop notification, without the
  pointer over it, and the message, cropped above the input box) replace
  `assets/slack/`. Every capture makes a new run ID, so a regenerated video
  matches the screenshots only after this. A Slack screen is never drawn by
  the pipeline itself.

### Outputs (03)

A training run writes its outputs; the job's Artifacts list them, a plot is
previewed and a sample played (briefly, in the Table viewer with its
waveform and spectrogram, which the video does not name), TensorBoard is
launched on the run's `tb/` row, and then the run's Files view opens the
experiment directory, where TensorBoard on `outputs/` compares every
experiment.

```sh
scripts/promo-video/03_outputs.sh
```

- **Workspace:** the capture runs `rotari init` in `~/tts`, so the basedir is
  `~/tts/.rotari-state` and Files opens `~/tts`. Three earlier experiments
  (`exp0412_lr1e-3`, `exp0413_lr3e-4`, `exp0414_bs32`) are made with the fake
  job's `--floor`, so their curves differ in TensorBoard.
- **Launching:** Artifacts and Files rows have their own launch buttons. The
  capture presses the `tb` row's **TensorBoard**, and in Files the first
  **TensorBoard** in the view, the one beside the directory's path. The run
  toolbar has launch buttons too, so lookups stay inside `#output-modal`.
  TensorBoard comes from `.venv` on `PATH`, and its settings side pane is
  closed before the shot. The confirmation shows "Runs as root@workstation":
  the namespace's user.

## Next steps

- Listen to the narration and adjust wording, voice, or speed; consider
  quiet background music.
- The run page job timeline's axis labels overlap at narrow widths and are
  visible in the video; see `development/ISSUES.md`.
- Capture video 02 in landscape on a host with Xvfb, dunst, and
  google-chrome, then refresh `assets/slack/` from its run.
- Videos 04 to 09 (see [Episodes and documentation](#episodes-and-documentation)):
  04 and 05 can reuse the short's sweep and Slurm acts in landscape.
