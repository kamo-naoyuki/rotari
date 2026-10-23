# Promo video pipeline

These scripts make rotari's vertical promo short (1080×1920, 30 fps) from
real runs: rotari, its Web UI, and a one-node Slurm cluster run in Linux
namespaces, and a headless browser renders the material into frames for
ffmpeg. Every command, log line, and Web UI screen in the video comes from
those runs. They are a production aid, not part of rotari or its tests.

Status (2026-10-23): the cut is 77 s with English narration by Kokoro TTS
(voice `af_heart`) and burned-in subtitles, plus an `.srt` beside the video;
there is no music. `--with-slurm` adds the Slurm act (96 s).

## Message

Rotari makes the experiment cycle (write code, submit jobs, check how they
ended, fix and resubmit failures, inspect results) less tedious. With one job a
terminal is enough, so the video opens by saying so: rotari pays off when
something becomes plural.

- **Places:** projects in different directories. Without rotari, one terminal
  tab each and `cd` to check; with rotari, every project of a state directory
  is on one Web UI page.
- **Numbers:** a 48-run sweep that fails for different reasons. Rotari groups
  failures by diagnosed cause with a fix each, and `retry
  --filter-diagnosis` reruns one cause while successful runs carry forward.
- **Machines (optional act):** Slurm and local jobs are submitted and watched
  differently by hand; with rotari only `-e local` / `-e slurm` differs, and
  one run can hold both.

Claims stay within what rotari does: diagnosis covers known log patterns only,
the Web UI reads one basedir at a time (other basedirs are in the sidebar), and
rotari is not a workflow engine. The video's captions say that the 6 remaining
failures need a code fix rather than implying retry fixes everything.

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
| 0:41 | Run page live: failures appear | Nine of them fail. |
| 0:49 | Three log lines: GPU fault, CUDA OOM, missing module | For three different reasons. Do you open every log? |
| 0:53 | Run page Failure causes: three groups, fix and retry command each | Rotari groups the failures by cause, and suggests a fix for each. |
| 0:58 | `rotari retry --filter-diagnosis nvidia-gpu-driver-device-error` | The GPU fault was the machine, so rerun just those three… |
| 1:05 | Retry run page: 45 carried, 6 left | The other six need a code fix. |
| (opt.) | Slurm (`sbatch`/`squeue`/`cat`) above local (`nohup`/`ps`/`tail`) | Preprocessing runs locally, training on Slurm… |
| (opt.) | `rotari add -e local …` and `-e slurm --depends-on …`; run page jobs table with workstation and gpu-node01 | …Only the executor changes. One run, two machines. |
| 1:08 | All projects again | More jobs. Still one place to look. |
| 1:12 | Logo and GitHub URL | Rotari. Spend your time on research, not on babysitting jobs. |

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
| `setup.sh TOOLS` | Install Playwright and kokoro-onnx into `.venv`; fetch Chrome, the Kokoro model, and the Slurm image into `TOOLS` |
| `make-video.sh TOOLS WORK OUT.mp4 [opts]` | Build rotari, capture, render; the usual entry point |
| `capture.sh` | Enter the namespaces, start Slurm, run `capture.py` |
| `capture.py` | Run the scenario; record terminal sessions and Web UI screenshots |
| `slurm-up.sh` | Start slurmdbd, slurmctld, and slurmd from the image; write client wrappers |
| `render.py`, `player.html` | Scene timeline and the HTML player stepped frame by frame into ffmpeg |
| `narration.py` | Kokoro TTS clips, scene fitting, the audio track, and subtitles |
| `fixture/` | Fake jobs: `train.py`, per-project `train.sh`, `sweep.sh`, `prep.sh`, `train-gpu.sh` |
| `fullpage.py` | Framing aid: full-page screenshots of a captured state, run in a namespace |

## Making a video

```sh
scripts/promo-video/setup.sh "$TMPDIR/promo-tools"
scripts/promo-video/make-video.sh "$TMPDIR/promo-tools" "$TMPDIR/promo-work" out.mp4
scripts/promo-video/make-video.sh … out.mp4 --with-slurm         # include the Slurm act
scripts/promo-video/make-video.sh … out.mp4 --voice am_michael   # another narrator
scripts/promo-video/make-video.sh … out.mp4 --preview 20 45 60   # PNGs in WORK/capture
```

Capturing takes about 2 minutes and rendering about 5. To change only
timing, captions, narration, or layout, render the existing capture again
(leave out `--narration` for a silent cut):

```sh
PLAYWRIGHT_BROWSERS_PATH="$TOOLS/pw" .venv/bin/python \
  scripts/promo-video/render.py "$WORK/capture" . out.mp4 --narration "$TOOLS/kokoro"
```

Requirements: Linux with unprivileged user namespaces, `singularity` or
`apptainer`, `ffmpeg` with libx264, `curl`, `unzip`, Go, and the repository's
`.venv` (created with uv; `setup.sh` adds Playwright, kokoro-onnx, and
soundfile to it with `uv pip install`). Set `PYTHON` to use another
interpreter. The Slurm image is 1.5 GB unpacked, the Kokoro model 350 MB, and
Chrome about 125 MB.

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
- **Diagnosis IDs:** `--filter-diagnosis` takes the slug of the diagnosis name,
  such as `nvidia-gpu-driver-device-error`.

## Next steps

- Listen to the narration and adjust wording, voice, or speed; consider
  quiet background music.
- The run page job timeline's axis labels overlap at narrow widths and are
  visible in the video; see `development/ISSUES.md`.
- The closing All projects page lists `pipeline` even without the Slurm act.
- A longer intro and a series (clusters, history and lineage, notifications
  and artifacts, agents) can reuse this pipeline with new scenes.
