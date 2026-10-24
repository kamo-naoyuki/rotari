"""Render the outputs episode (landscape): Artifacts, previews, TensorBoard, Files.

    render_outputs.py CAPTURE_DIR REPO_DIR OUT.mp4
                      [--narration KOKORO_DIR [--voice VOICE] [--speed X]]
                      [--preview SECONDS...]

Builds the scene timeline from capture_outputs.py's material and renders it
with renderer.py. It accompanies docs/OUTPUTS.md.
"""

import json
from pathlib import Path

import renderer

ARGS = renderer.options()
CAP = ARGS.capture_dir


def timeline():
    terms = {
        t["name"]: t for t in json.loads(Path(f"{CAP}/terminals.json").read_text())
    }
    web = json.loads(Path(f"{CAP}/web.json").read_text())

    def term(name, title, captions, **more):
        t = terms[name]
        return {
            "kind": "term",
            "title": title,
            "events": t["events"],
            "dur": t["duration"] + 0.4,
            "captions": captions,
            **more,
        }

    def webscene(name, dur, captions, **more):
        return {
            "kind": "web",
            "frames": web[name],
            "dur": dur,
            "captions": captions,
            **more,
        }

    return [
        {
            "kind": "title",
            "dur": 2.8,
            "lines": [["Your run wrote", "", 0], ["plots, audio, logs.", "soft", 0.35]],
            "say": "Your run wrote plots, audio, and logs.",
        },
        term(
            "run",
            "~/tts",
            [[0, "A training run<br><em>writes its outputs.</em>"]],
            say="Here a training run writes its outputs to one directory.",
        ),
        # A job's Artifacts.
        webscene(
            "artifacts",
            3.4,
            [[0, "Each job lists<br><em>its outputs.</em>"]],
            say="In the Web UI, each job lists the outputs found from its command.",
        ),
        webscene("outputs_dir", 2.4, [[0, "Open a directory,"]]),
        webscene(
            "loss",
            2.8,
            [[0, "preview a plot,"]],
            say="Open them, preview a plot, or listen to a sample.",
        ),
        webscene(
            "pane",
            4.6,
            [[0, "or listen to a sample."]],
            # The frames start as playback starts.
            sfx=[0.0, f"{CAP}/sample.wav"],
        ),
        # TensorBoard from the job's tb/ row.
        webscene(
            "tb_menu",
            2.6,
            [[0, "Launch TensorBoard<br><em>from the row.</em>"]],
            say="Launch TensorBoard right from the row.",
        ),
        webscene(
            "tb_confirm",
            2.8,
            [[0, "See what will run<br><em>before it runs.</em>"]],
            say="You see what will run before it runs.",
        ),
        webscene("tb_ready", 1.0, [[0, "…then open it."]]),
        webscene("tb_board", 3.0, [[0, "…then open it."]], say="Then open it."),
        # The run's Files view of the experiment directory.
        webscene(
            "files",
            3.6,
            [[0, "Files: the whole<br><em>experiment directory.</em>"]],
            say="The run's Files view opens the whole experiment directory,",
        ),
        webscene(
            "files_outputs",
            3.0,
            [[0, "Every experiment's<br><em>outputs.</em>"]],
            say="with every experiment's outputs.",
        ),
        webscene(
            "all_confirm",
            2.2,
            [[0, "Launch TensorBoard<br><em>on all of them,</em>"]],
            say="Launch TensorBoard on all of them,",
        ),
        webscene(
            "all_board",
            4.0,
            [[0, "…and compare<br><em>every run.</em>"]],
            say="and compare every run.",
        ),
        {
            "kind": "logo",
            "dur": 4.0,
            "tagline": "Outputs and artifacts",
            "url": "kamo-naoyuki.github.io/rotari/docs/OUTPUTS/",
            "say": "Rotari.",
        },
    ]


renderer.main(ARGS, timeline(), layout="landscape")
