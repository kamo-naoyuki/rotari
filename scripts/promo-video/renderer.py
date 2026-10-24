"""Turn a scene timeline into the vertical video, shared by the episode scripts.

An episode script builds a list of scenes for player.html and calls
`main(timeline)`; this module parses the common options, narrates, writes
CAPTURE_DIR/player.html, steps it frame by frame in headless Chrome, and pipes
the frames to ffmpeg at 30 fps: 1080x1920 for the portrait short, 1920x1080
for the landscape feature videos.
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

FPS = 30
HERE = os.path.dirname(os.path.abspath(__file__))


def options(extra=None):
    """Parse CAPTURE_DIR REPO_DIR OUT.mp4 and the shared options."""
    parser = argparse.ArgumentParser()
    parser.add_argument("capture_dir")
    parser.add_argument("repo_dir")
    parser.add_argument("out")
    parser.add_argument(
        "--narration", metavar="KOKORO_DIR", help="speak each scene's line"
    )
    parser.add_argument(
        "--voice", default="af_heart", help="Kokoro voice (default af_heart)"
    )
    parser.add_argument("--speed", type=float, default=1.0, help="speech speed")
    parser.add_argument("--preview", type=float, nargs="+", metavar="SECONDS")
    if extra:
        extra(parser)
    args = parser.parse_args()
    if (args.voice != "af_heart" or args.speed != 1.0) and not args.narration:
        parser.error("--voice and --speed need --narration")
    return args


# Frame size of each layout: the short is vertical, feature videos horizontal.
LAYOUTS = {"portrait": (1080, 1920), "landscape": (1920, 1080)}


def main(args, scenes, layout="portrait"):
    cap, repo, out = args.capture_dir, args.repo_dir, args.out
    width, height = LAYOUTS[layout]
    audio = None
    if args.narration:
        import narration

        placed, total = narration.narrate(
            scenes, args.narration, f"{cap}/narration", args.voice, args.speed
        )
        audio = f"{cap}/narration.wav"
        narration.write_audio(placed, total, audio)
        narration.write_srt(placed, os.path.splitext(out)[0] + ".srt")
    else:
        total = 0.0
        for s in scenes:
            s["start"] = round(total, 3)
            total += s["dur"]
    logo = Path(f"{repo}/docs/assets/rotari-logo-dark.svg").read_text()
    fonts = Path(f"{repo}/internal/webui/assets/fonts")

    # Chrome refuses web fonts from file:// URLs, so embed them as data URIs.
    def font_uri(match):
        data = base64.b64encode((fonts / match.group(1)).read_bytes()).decode()
        return f"url(data:font/woff2;base64,{data})"

    html = Path(f"{HERE}/player.html").read_text()
    html = re.sub(r"url\(__FONTS__/([^)]+)\)", font_uri, html)
    html = (
        html.replace("__LOGO__", logo)
        .replace("__SCENES__", json.dumps(scenes))
        .replace("__LAYOUT__", layout)
    )
    with open(f"{cap}/player.html", "w") as f:
        f.write(html)
    for s in scenes:
        print(f"{s['start']:6.1f}  {s['dur']:5.1f}  {s['kind']}", file=sys.stderr)
    print(f"total {total:.1f}s", file=sys.stderr)

    with sync_playwright() as p:
        b = p.chromium.launch(args=["--no-sandbox", "--allow-file-access-from-files"])
        page = b.new_page(viewport={"width": width, "height": height})
        page.goto(f"file://{cap}/player.html")
        # Load every face before the first frame, and stop rather than render
        # in a fallback font.
        failed = page.evaluate(
            """Promise.all([...document.fonts].map((f) => f.load().catch(() => f)))
              .then(() => [...document.fonts].filter((f) => f.status !== "loaded")
              .map((f) => f.family + " " + f.weight))"""
        )
        if failed:
            sys.exit(f"fonts did not load: {failed}")
        if args.preview:
            for t in args.preview:
                page.evaluate(f"render({t})")
                page.screenshot(path=f"{cap}/preview-{t:05.1f}.png")
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
                out,
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
