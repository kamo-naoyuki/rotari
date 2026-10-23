"""Render the rotari promo short ("You don't need rotari for one job").

    render.py CAPTURE_DIR REPO_DIR OUT.mp4 [--with-slurm]
              [--narration KOKORO_DIR [--voice VOICE] [--speed X]]
              [--preview SECONDS...]

Builds the scene timeline from capture.py's material, writes
CAPTURE_DIR/player.html from player.html, steps it frame by frame in headless
Chrome, and pipes the frames to ffmpeg (1080x1920, 30 fps). With
--narration, each scene's `say` line is spoken by Kokoro TTS (models in
KOKORO_DIR), scenes stretch to fit their lines, the lines are burned in as
subtitles, and OUT.srt is written beside the video. With --preview, writes
one PNG per given time instead of a video.
"""

import argparse
import base64
import json
import os
import re
import subprocess
import sys
from pathlib import Path

from playwright.sync_api import sync_playwright

parser = argparse.ArgumentParser()
parser.add_argument("capture_dir")
parser.add_argument("repo_dir")
parser.add_argument("out")
parser.add_argument("--with-slurm", action="store_true", help="include the Slurm act")
parser.add_argument("--narration", metavar="KOKORO_DIR", help="speak each scene's line")
parser.add_argument(
    "--voice", default="af_heart", help="Kokoro voice (default af_heart)"
)
parser.add_argument("--speed", type=float, default=1.0, help="speech speed")
parser.add_argument("--preview", type=float, nargs="+", metavar="SECONDS")
ARGS = parser.parse_args()
if (ARGS.voice != "af_heart" or ARGS.speed != 1.0) and not ARGS.narration:
    parser.error("--voice and --speed need --narration")
CAP, REPO, OUT = ARGS.capture_dir, ARGS.repo_dir, ARGS.out
PREVIEW = ARGS.preview
FPS = 30
HERE = os.path.dirname(os.path.abspath(__file__))


def timeline():
    def load(name):
        return json.loads(Path(f"{CAP}/{name}").read_text())

    terms = {t["name"]: t for t in load("terminals.json")}
    web = load("web.json")
    logs = load("logs.json")

    def term(name, title, captions, speed=1.0, extra=0.6, say=None, say_at=None):
        t = terms[name]
        return {
            "kind": "term",
            "title": title,
            "events": t["events"],
            "speed": speed,
            "dur": t["duration"] / speed + extra,
            "captions": captions,
            "say": say,
            **({"sayAt": say_at} if say_at is not None else {}),
        }

    def webscene(
        name, dur, captions, speed=1.0, pan=None, css=False, say=None, say_at=None
    ):
        return {
            "kind": "web",
            "frames": web[name],
            "dur": dur,
            "speed": speed,
            "captions": captions,
            "pan": pan,
            "panCss": css,
            "cssWidth": 600,
            "say": say,
            **({"sayAt": say_at} if say_at is not None else {}),
        }

    # Full-page shots pan between page positions recorded by capture.py; the
    # frame shows 810 CSS px of a 600 px wide page.
    sweep, retry = logs["offsets_sweep_done"], logs["offsets_retry"]
    if ARGS.with_slurm and "offsets_pipeline" not in logs:
        sys.exit(f"--with-slurm: {CAP} has no Slurm act; capture with Slurm first")
    pipe = logs.get("offsets_pipeline")

    # Act 3, Slurm and local side by side; built only for --with-slurm.
    def act3():
        return [
            {
                "kind": "split",
                "dur": 8.0,
                "speed": 7.6 / 8.0,
                "top": {
                    "label": "Slurm",
                    "title": "~/pipeline",
                    "events": terms["slurm-raw"]["events"],
                },
                "bottom": {
                    "label": "Local",
                    "title": "~/pipeline",
                    "events": terms["local-raw"]["events"],
                },
                "captions": [
                    [
                        0,
                        "Slurm here. Local there.<br><em>Two ways to run and watch.</em>",
                    ]
                ],
                "say": "Preprocessing runs locally, training on Slurm. "
                "Two ways to submit, and two ways to watch.",
            },
            term(
                "pipeline",
                "~/pipeline",
                [[0, "With rotari:<br><em>same commands, -e local or -e slurm.</em>"]],
                speed=1.25,
                extra=0.4,
                say="With rotari, the commands stay the same. Only the executor changes.",
            ),
            webscene(
                "pipeline",
                4.0,
                [[0, "One run, two machines.<br><em>workstation + gpu-node01</em>"]],
                pan=[[0.4, pipe["cards"] - 16], [2.4, pipe["jobs_bottom"] - 790]],
                css=True,
                say="One run, two machines.",
            ),
        ]

    scenes = [
        # Act 0: one job is fine.
        term(
            "one",
            "~/llm",
            [[0, "One job?"]],
            extra=1.0,
            say="One job? Watching it in a terminal is enough.",
            say_at=1.6,
        ),
        {
            "kind": "title",
            "dur": 2.6,
            "lines": [["You don't", "", 0], ["need rotari.", "hl", 0.35]],
            "say": "You don't need rotari.",
        },
        {
            "kind": "title",
            "dur": 2.6,
            "lines": [["But research", "", 0], ["isn't one job.", "soft", 0.35]],
            "say": "But research is never just one job.",
        },
        # Act 1: several projects in several directories.
        {
            "kind": "tabs",
            "dur": 7.2,
            "speed": 1.6,
            "tabs": [
                {"title": d, "events": terms[f"tab-{d}"]["events"]}
                for d in ("asr", "tts", "vision", "nlp")
            ],
            "captions": [
                [0, "4 projects.<br>4 directories. 4 tabs."],
                [3.6, "Something broke.<br><em>Which tab was it?</em>"],
            ],
            "say": "Each project lives in its own directory. So you hop between "
            "terminal tabs, and somewhere, something broke.",
        },
        term(
            "submit4",
            "~",
            [[0, "With rotari, submit<br>from any directory…"]],
            speed=2.6,
            extra=0.4,
            say="With rotari, you submit from any directory,",
            say_at=4.6,
        ),
        webscene(
            "projects",
            8.0,
            [
                [0, "…and see every project<br>on one page."],
                [3.8, "Vision failed.<br><em>You see it at a glance.</em>"],
            ],
            speed=15 / 8.0,
            say="and every project shows up on one page. "
            "You see what failed at a glance.",
        ),
        # Act 2: a large sweep with mixed failures.
        term(
            "sweep",
            "~/llm-sweep",
            [[0, "A parameter sweep:<br>48 runs."]],
            extra=0.4,
            say="Now a parameter sweep. Forty-eight runs.",
        ),
        webscene(
            "sweep_live",
            8.0,
            [[0, "Watch them run."], [4.4, "48 runs.<br><em>8 failed.</em>"]],
            speed=22 / 8.0,
            say="Eight of them fail.",
            say_at=4.3,
        ),
        {
            "kind": "logs",
            "dur": 4.6,
            "captions": [[0, "…for different reasons."]],
            "cards": [
                ["task 7", logs["gpu"]],
                ["task 4", logs["oom"]],
            ],
            "say": "For different reasons. Do you open every log?",
        },
        webscene(
            "sweep_done",
            5.0,
            [[0, "Rotari groups them by cause,<br><em>with a fix for each.</em>"]],
            pan=[[0.5, sweep["cards"] - 16], [3.6, sweep["timeline"] - 790]],
            css=True,
            say="Rotari groups the failures by cause, and suggests a fix for each.",
        ),
        term(
            "retry",
            "~/llm-sweep",
            [[0, "Fix the batch size.<br><em>Rerun only what failed.</em>"]],
            extra=0.8,
            say="Fix the batch size, and rerun only the failed jobs. "
            "The forty that succeeded stay as they are.",
        ),
        webscene(
            "retry",
            3.6,
            [[0, "8 rerun, 40 untouched.<br><em>All 48 done.</em>"]],
            pan=[[0, retry["cards"] - 16]],
            css=True,
            say="All forty-eight done.",
        ),
        *(act3() if ARGS.with_slurm else []),
        # Close.
        webscene(
            "projects_end",
            3.4,
            [[0, "More jobs.<br><em>Still one place.</em>"]],
            say="More jobs. Still one place to look.",
        ),
        {
            "kind": "logo",
            "dur": 5.0,
            "tagline": "Spend your time on research,<br>not on babysitting jobs.",
            "url": "github.com/kamo-naoyuki/rotari",
            "say": "Rotari. Spend your time on research, not on babysitting jobs.",
        },
    ]
    return scenes


def main():
    scenes = timeline()
    audio = None
    if ARGS.narration:
        import narration

        placed, total = narration.narrate(
            scenes, ARGS.narration, f"{CAP}/narration", ARGS.voice, ARGS.speed
        )
        audio = f"{CAP}/narration.wav"
        narration.write_audio(placed, total, audio)
        narration.write_srt(placed, os.path.splitext(OUT)[0] + ".srt")
    else:
        total = 0.0
        for s in scenes:
            s["start"] = round(total, 3)
            total += s["dur"]
    logo = Path(f"{REPO}/docs/assets/rotari-logo-dark.svg").read_text()
    fonts = Path(f"{REPO}/internal/webui/assets/fonts")

    # Chrome refuses web fonts from file:// URLs, so embed them as data URIs.
    def font_uri(match):
        data = base64.b64encode((fonts / match.group(1)).read_bytes()).decode()
        return f"url(data:font/woff2;base64,{data})"

    html = Path(f"{HERE}/player.html").read_text()
    html = re.sub(r"url\(__FONTS__/([^)]+)\)", font_uri, html)
    html = html.replace("__LOGO__", logo).replace("__SCENES__", json.dumps(scenes))
    with open(f"{CAP}/player.html", "w") as f:
        f.write(html)
    for s in scenes:
        print(f"{s['start']:6.1f}  {s['dur']:5.1f}  {s['kind']}", file=sys.stderr)
    print(f"total {total:.1f}s", file=sys.stderr)

    with sync_playwright() as p:
        b = p.chromium.launch(args=["--no-sandbox", "--allow-file-access-from-files"])
        page = b.new_page(viewport={"width": 1080, "height": 1920})
        page.goto(f"file://{CAP}/player.html")
        # Load every face before the first frame, and stop rather than render
        # in a fallback font.
        failed = page.evaluate(
            """Promise.all([...document.fonts].map((f) => f.load().catch(() => f)))
              .then(() => [...document.fonts].filter((f) => f.status !== "loaded")
              .map((f) => f.family + " " + f.weight))"""
        )
        if failed:
            sys.exit(f"fonts did not load: {failed}")
        if PREVIEW:
            for t in PREVIEW:
                page.evaluate(f"render({t})")
                page.screenshot(path=f"{CAP}/preview-{t:05.1f}.png")
            return
        ff = subprocess.Popen(
            [
                "ffmpeg",
                "-y",
                "-loglevel",
                "error",
                "-f",
                "image2pipe",
                "-framerate",
                str(FPS),
                "-c:v",
                "mjpeg",
                "-i",
                "-",
                *(["-i", audio, "-c:a", "aac", "-b:a", "160k"] if audio else []),
                "-c:v",
                "libx264",
                "-pix_fmt",
                "yuv420p",
                "-crf",
                "18",
                "-preset",
                "medium",
                "-movflags",
                "+faststart",
                OUT,
            ],
            stdin=subprocess.PIPE,
        )
        n = int(total * FPS)
        for i in range(n):
            page.evaluate(f"render({i / FPS})")
            ff.stdin.write(page.screenshot(type="jpeg", quality=95))
            if i % 300 == 0:
                print(f"frame {i}/{n}", file=sys.stderr)
        ff.stdin.close()
        ff.wait()
        b.close()


main()
