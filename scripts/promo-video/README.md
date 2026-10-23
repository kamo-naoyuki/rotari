# Promo video pipeline

These scripts make rotari's vertical promo short (1080×1920, 30 fps) from
real runs: rotari, its Web UI, and a one-node Slurm cluster run in Linux
namespaces, and a headless browser renders the material into frames for
ffmpeg. Every command, log line, and Web UI screen in the video comes from
those runs. They are a production aid, not part of rotari or its tests.

Two videos exist (2026-10-23), both with English narration by Kokoro TTS
(voice `af_heart`), burned-in subtitles, and an `.srt` beside the video, and
no music:

- **The short** (78 s; `--with-slurm` adds the Slurm act, about 96 s): one
  job needs no rotari, many do. Sections below describe it unless they say
  otherwise.
- **Series 1, "results"** (56 s): know when a run ends, then look at its
  results. See [The results episode](#the-results-episode).

## Message

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

## Storyboard

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
| `01_short.sh`, `02_results.sh` | Make one video each into `out/`; the usual entry points (`defaults.sh` sets their directories) |
| `setup.sh TOOLS` | Install Playwright and kokoro-onnx into `.venv`; fetch Chrome, the Kokoro model, and the Slurm image into `TOOLS` |
| `make-video.sh TOOLS WORK OUT.mp4 [opts]` | Build rotari, capture, and render the short |
| `capture.sh` | Enter the namespaces, start Slurm, run `capture.py` |
| `capture.py` | Run the scenario; record terminal sessions and Web UI screenshots |
| `slurm-up.sh` | Start slurmdbd, slurmctld, and slurmd from the image; write client wrappers |
| `render.py`, `player.html` | The short's scene timeline, and the HTML player both episodes render through |
| `renderer.py` | Shared rendering: options, narration, player page, frames into ffmpeg |
| `common.py` | Shared capture helpers: commands, terminal sessions (`Term`), Web UI frames |
| `narration.py` | Kokoro TTS clips, scene fitting, the audio track, and subtitles |
| `fixture/` | Fake jobs: `train.py`, per-project `train.sh`, `sweep.sh`, `prep.sh`, `train-gpu.sh` |
| `fullpage.py` | Framing aid: full-page screenshots of a captured state, run in a namespace |
| `make-results.sh TOOLS WORK OUT.mp4 SLACK_DIR` | Build rotari, capture, and render the results episode |
| `assets/slack/` | Slack screenshots of run `20261023-075013-adb3b347`'s webhook message |
| `capture-results.sh`, `capture_results.py` | Capture the results episode (needs Xvfb, dunst, google-chrome) |
| `newroot.sh`, `dunstrc` | A namespace-owned `/` for `--allow-exec`; the desktop notification style |
| `render_results.py` | The results episode's scene timeline |
| `tts_samples.py`, `fixture/tts_train.py` | Kokoro clips and the fake TTS job that "produces" them |
| `send-webhook.py` | Post the Slack payloads a capture recorded to your real webhook |

## Making a video

Each video has a numbered script that does everything (fetch the tools on
first use, build rotari, capture, narrate, render) and writes
`scripts/promo-video/out/NN_name.mp4` with an `.srt` beside it:

```sh
scripts/promo-video/01_short.sh       # the short, out/01_short.mp4
scripts/promo-video/02_results.sh     # the results episode, out/02_results.mp4
scripts/promo-video/01_short.sh --with-slurm          # include the Slurm act
scripts/promo-video/01_short.sh --voice am_michael    # another narrator
scripts/promo-video/02_results.sh --preview 20 45     # PNGs in the work directory
```

The tools go to `PROMO_TOOLS` (default `~/.cache/rotari-promo`, about 600 MB)
and captures to `PROMO_WORK` (default `$TMPDIR/rotari-promo-$USER`); set `GO`
when `go` is not on `PATH` or in `/usr/local/go/bin`. Options after the
script name go to the episode's renderer. `out/` is not tracked.

Underneath, `setup.sh TOOLS` prepares the tools and `make-video.sh` /
`make-results.sh` capture and render one video into a given path:

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

## The results episode

The second video covers the end of the experiment cycle: a TTS training run
finishes, the desktop shows a notification, the Web UI Inbox lists it, Slack
gets the webhook message, and the outputs (a loss plot, then an audio sample
from the evaluation table) are opened from the run page. The hook is only
"Is it done yet?": checking a log again and again.

```sh
scripts/promo-video/02_results.sh
.venv/bin/python scripts/promo-video/send-webhook.py "$PROMO_WORK/02_results/capture"
```

The committed `assets/slack/` screenshots belong to run
`20261023-075013-adb3b347`; every capture makes a new run ID, so the run page
and the Slack message in a regenerated video differ there. To make them agree,
send the new capture's payload as above, screenshot the Slack notification and
message, crop the message above the input box, and replace `assets/slack/`.

- **Desktop notification:** `capture-results.sh` starts Xvfb (1200x1620),
  a D-Bus session, and dunst (1.5 syntax in `dunstrc`) inside the namespace.
  A headed `/usr/bin/google-chrome` at scale 2 opens the run page, turns on
  the Web UI's notifications, and ffmpeg x11grab records the screen while the
  run ends. Chrome runs as the namespace root, so it needs `--no-sandbox`;
  `--test-type` hides the warning bar, and `--lang=en-US` with Translate
  disabled keeps a translation prompt out of the frame. Xvfb runs with
  `-nolock` (its lock file would land in the host's shared `/tmp`) and
  `-nocursor`. `render_results.py` finds the notification's arrival by
  comparing the top right of each frame with the first.
- **Slack:** the namespace has no network, so the capture's
  `notifications.toml` points the webhook at a local recorder
  (`WORK/capture/webhook/NNN.json`), with the same fields as the user's own
  settings. Afterwards `send-webhook.py` posts those exact payloads to the
  `[webhook] url` of `~/.config/rotari/notifications.toml` without printing
  it. Screenshots taken in Slack then (the desktop notification and the
  message, cropped above the input box) go in SLACK_DIR. A Slack screen is
  never drawn by the pipeline itself.
- **Artifacts:** the fake job writes its outputs with f-strings
  (`f"{out}/loss.png"`), as training code commonly does. Artifact discovery
  skips f-strings, so only `--out outputs/exp0415` is listed; plain literals such
  as `"loss.png"` were listed as missing paths under the working directory.
  The Table viewer's media paths resolve from "this file's directory".
- **Audio:** a sample is opened from `eval.csv` in the Table viewer, which
  draws its waveform and spectrogram, and plays while frames are recorded;
  the same WAV (`WORK/capture/sample.wav`) is mixed into the track as a scene
  `sfx`. The viewer is shown briefly and not named or explained: it needs a
  table of results and serves a narrower need than the episode's message.
- **TensorBoard is left out.** Launching needs `rotari web --allow-exec`, whose
  browser login required HTTPS until loopback HTTP sign-in was allowed
  (2026-10-23); a TensorBoard scene can now be added. `newroot.sh` already
  solves the other obstacle:
  `--allow-exec` checks that every directory from `/` to the launcher file is
  owned by the user or root, and in a user namespace the real `/` looks
  owned by nobody.

## Next steps

- Listen to the narration and adjust wording, voice, or speed; consider
  quiet background music.
- The run page job timeline's axis labels overlap at narrow widths and are
  visible in the video; see `development/ISSUES.md`.
- A longer intro and more of the series (clusters, history and lineage,
  agents) can reuse this pipeline with new scenes.
