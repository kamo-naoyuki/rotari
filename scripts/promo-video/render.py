"""Render the rotari promo short ("You don't need rotari for one job").

    render.py CAPTURE_DIR REPO_DIR OUT.mp4 [--with-slurm] [--preview SECONDS...]

Builds the scene timeline from capture.py's material, writes
CAPTURE_DIR/player.html from player.html, steps it frame by frame in headless
Chrome, and pipes the frames to ffmpeg (1080x1920, 30 fps). With --preview,
writes one PNG per given time instead of a video.
"""

import json
import os
import subprocess
import sys
from pathlib import Path

from playwright.sync_api import sync_playwright

CAP, REPO, OUT = sys.argv[1:4]
PREVIEW = (
    [float(x) for x in sys.argv[5:] if x[0].isdigit()]
    if "--preview" in sys.argv
    else None
)
FPS = 30
HERE = os.path.dirname(os.path.abspath(__file__))


def timeline():
    def load(name):
        return json.loads(Path(f"{CAP}/{name}").read_text())

    terms = {t["name"]: t for t in load("terminals.json")}
    web = load("web.json")
    logs = load("logs.json")

    def term(name, title, captions, speed=1.0, extra=0.6):
        t = terms[name]
        return {
            "kind": "term",
            "title": title,
            "events": t["events"],
            "speed": speed,
            "dur": t["duration"] / speed + extra,
            "captions": captions,
        }

    def webscene(name, dur, captions, speed=1.0, pan=None, css=False):
        return {
            "kind": "web",
            "frames": web[name],
            "dur": dur,
            "speed": speed,
            "captions": captions,
            "pan": pan,
            "panCss": css,
            "cssWidth": 600,
        }

    # Full-page shots pan between page positions recorded by capture.py; the
    # frame shows 810 CSS px of a 600 px wide page.
    sweep, retry, pipe = (
        logs[f"offsets_{k}"] for k in ("sweep_done", "retry", "pipeline")
    )

    # Act 3, Slurm and local side by side; left out unless --with-slurm.
    ACT3 = [
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
                [0, "Slurm here. Local there.<br><em>Two ways to run and watch.</em>"]
            ],
        },
        term(
            "pipeline",
            "~/pipeline",
            [[0, "With rotari:<br><em>same commands, -e local or -e slurm.</em>"]],
            speed=1.25,
            extra=0.4,
        ),
        webscene(
            "pipeline",
            4.0,
            [[0, "One run, two machines.<br><em>workstation + gpu-node01</em>"]],
            pan=[[0.4, pipe["cards"] - 16], [2.4, pipe["jobs_bottom"] - 790]],
            css=True,
        ),
    ]
    scenes = [
        # Act 0: one job is fine.
        term("one", "~/llm", [[0, "One job?"]], extra=1.0),
        {
            "kind": "title",
            "dur": 2.6,
            "lines": [["You don't", "", 0], ["need rotari.", "hl", 0.35]],
        },
        {
            "kind": "title",
            "dur": 2.6,
            "lines": [["But research", "", 0], ["isn't one job.", "soft", 0.35]],
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
        },
        term(
            "submit4",
            "~",
            [[0, "With rotari, submit<br>from any directory…"]],
            speed=2.6,
            extra=0.4,
        ),
        webscene(
            "projects",
            8.0,
            [
                [0, "…and see every project<br>on one page."],
                [3.8, "Vision failed.<br><em>You see it at a glance.</em>"],
            ],
            speed=15 / 8.0,
        ),
        # Act 2: a large sweep with mixed failures.
        term(
            "sweep", "~/llm-sweep", [[0, "A parameter sweep:<br>48 runs."]], extra=0.4
        ),
        webscene(
            "sweep_live",
            8.0,
            [[0, "Watch them run."], [4.4, "48 runs.<br><em>9 failed.</em>"]],
            speed=22 / 8.0,
        ),
        {
            "kind": "logs",
            "dur": 4.6,
            "captions": [[0, "…for 3 different reasons."]],
            "cards": [
                ["task 7", logs["gpu"]],
                ["task 4", logs["oom"]],
                ["task 22", logs["module"]],
            ],
        },
        webscene(
            "sweep_done",
            5.0,
            [[0, "Rotari groups them by cause,<br><em>with a fix for each.</em>"]],
            pan=[[0.5, sweep["cards"] - 16], [3.6, sweep["timeline"] - 790]],
            css=True,
        ),
        term(
            "retry", "~/llm-sweep", [[0, "Rerun just the<br>GPU failures."]], extra=0.8
        ),
        webscene(
            "retry",
            3.6,
            [[0, "3 rerun, 45 untouched.<br><em>The other 6 need a code fix.</em>"]],
            pan=[[0.4, retry["cards"] - 16], [2.6, retry["cards"] + 110]],
            css=True,
        ),
        *(ACT3 if "--with-slurm" in sys.argv else []),
        # Close.
        webscene("projects_end", 3.4, [[0, "More jobs.<br><em>Still one place.</em>"]]),
        {
            "kind": "logo",
            "dur": 5.0,
            "tagline": "Spend your time on research,<br>not on babysitting jobs.",
            "install": "brew install kamo-naoyuki/tap/rotari",
            "url": "github.com/kamo-naoyuki/rotari",
        },
    ]
    start = 0.0
    for s in scenes:
        s["start"] = round(start, 3)
        start += s["dur"]
    return scenes, start


def main():
    scenes, total = timeline()
    logo = Path(f"{REPO}/docs/assets/rotari-logo-dark.svg").read_text()
    fonts = f"file://{REPO}/internal/webui/assets/fonts"
    html = Path(f"{HERE}/player.html").read_text()
    html = (
        html.replace("__FONTS__", fonts)
        .replace("__LOGO__", logo)
        .replace("__SCENES__", json.dumps(scenes))
    )
    with open(f"{CAP}/player.html", "w") as f:
        f.write(html)
    for s in scenes:
        print(f"{s['start']:6.1f}  {s['dur']:5.1f}  {s['kind']}", file=sys.stderr)
    print(f"total {total:.1f}s", file=sys.stderr)

    with sync_playwright() as p:
        b = p.chromium.launch(args=["--no-sandbox", "--allow-file-access-from-files"])
        page = b.new_page(viewport={"width": 1080, "height": 1920})
        page.goto(f"file://{CAP}/player.html")
        page.evaluate("document.fonts.ready")
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
