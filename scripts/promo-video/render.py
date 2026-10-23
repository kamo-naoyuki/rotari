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

import json
import sys
from pathlib import Path

import renderer

ARGS = renderer.options(
    lambda parser: parser.add_argument(
        "--with-slurm", action="store_true", help="include the Slurm act"
    )
)
CAP = ARGS.capture_dir


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


renderer.main(ARGS, timeline())
