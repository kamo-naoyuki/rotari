"""Render the notifications episode (landscape): know the moment a run ends.

    render_notifications.py CAPTURE_DIR REPO_DIR OUT.mp4 --slack DIR
                      [--narration KOKORO_DIR [--voice VOICE] [--speed X]]
                      [--preview SECONDS...]

Builds the scene timeline from capture_notifications.py's material and renders
it with renderer.py. It accompanies docs/NOTIFICATIONS.md. DIR holds two screenshots taken in Slack of the run's
webhook message: notification.png (the desktop notification) and
message.png (the message, cropped to it).
"""

import json
import shutil
from pathlib import Path

import renderer
from PIL import Image, ImageChops, ImageStat

ARGS = renderer.options(
    lambda parser: parser.add_argument(
        "--slack", required=True, metavar="DIR", help="Slack screenshots"
    )
)
CAP = ARGS.capture_dir


def notified_at(frames):
    """Seconds into the recording when the desktop notification appears.

    The notification is drawn at the top right of the screen; the first frame
    whose top right differs from the first frame's is taken as its arrival.
    """
    first = Image.open(f"{CAP}/{frames[0][1]}").convert("L")
    width, height = first.size
    box = (width // 3, 0, width, height // 3)
    first = first.crop(box)
    for t, path in frames:
        frame = Image.open(f"{CAP}/{path}").convert("L").crop(box)
        if ImageStat.Stat(ImageChops.difference(first, frame)).mean[0] > 12:
            return t
    raise SystemExit("no desktop notification in the recording")


def timeline():
    terms = {
        t["name"]: t for t in json.loads(Path(f"{CAP}/terminals.json").read_text())
    }
    web = json.loads(Path(f"{CAP}/web.json").read_text())
    slack = Path(f"{CAP}/slack")
    slack.mkdir(exist_ok=True)
    for name in ("notification.png", "message.png"):
        shutil.copy(Path(ARGS.slack) / name, slack / name)

    def term(name, title, captions, speed=1.0, extra=0.6, **more):
        t = terms[name]
        return {
            "kind": "term",
            "title": title,
            "events": t["events"],
            "speed": speed,
            "dur": t["duration"] / speed + extra,
            "captions": captions,
            **more,
        }

    def webscene(name, dur, captions, frames=None, **more):
        return {
            "kind": "web",
            "frames": frames or web[name],
            "dur": dur,
            "captions": captions,
            **more,
        }

    # Start the run-end recording 3 s before the notification.
    at = notified_at(web["finish"])
    lead = 3.0
    finish = [[t - (at - lead), p] for t, p in web["finish"] if t >= at - lead]

    return [
        # Hook: checking on a run by hand, again and again.
        term(
            "wait",
            "~/tts",
            [[0, "Is it done yet?"], [5.0, "Is it done <em>now?</em>"]],
            speed=1.3,
            say="Is it done yet? You keep checking, again and again.",
        ),
        # Act 1: know when it ends.
        term(
            "submit",
            "~/tts",
            [[0, "With rotari,<br>submit as usual."]],
            speed=1.4,
            extra=0.3,
            say="With rotari, submit as usual, and get on with your day.",
        ),
        webscene(
            "finish",
            lead + 4.5,
            [[0, "Training…"], [lead, "Done.<br><em>Your desktop tells you.</em>"]],
            frames=finish,
            say="The moment the run finishes, you get a notification.",
            sayAt=lead,
        ),
        webscene(
            "inbox",
            2.8,
            [[0, "Every result<br>also lands in the Inbox."]],
            say="It also lands in the Inbox.",
        ),
        {
            "kind": "image",
            "dur": 5.0,
            "captions": [[0, "Away from your desk?<br><em>Get it in Slack.</em>"]],
            "images": [
                {"src": "slack/notification.png", "width": 732},
                {"src": "slack/message.png", "width": 1000, "at": 1.2},
            ],
            "say": "Away from your desk? Get it in Slack.",
        },
        # Close.
        {
            "kind": "title",
            "dur": 3.0,
            "lines": [["Stop checking.", "", 0], ["Get told.", "hl", 0.35]],
            "say": "Stop checking. Get told.",
        },
        {
            "kind": "logo",
            "dur": 4.0,
            "tagline": "Notifications",
            "url": "kamo-naoyuki.github.io/rotari/docs/NOTIFICATIONS/",
            "say": "Rotari.",
        },
    ]


renderer.main(ARGS, timeline(), layout="landscape")
