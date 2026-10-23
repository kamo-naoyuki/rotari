"""Deliver the webhook payloads a capture recorded to the real Slack webhook.

    send-webhook.py CAPTURE_DIR [NOTIFICATIONS_TOML]

The capture runs without network access, so rotari's webhook goes to a local
recorder (CAPTURE_DIR/webhook/NNN.json). This posts those exact payloads to
the [webhook] url of NOTIFICATIONS_TOML (default
~/.config/rotari/notifications.toml), so the Slack message shown in the video
belongs to the run in the video. The URL is never printed.
"""

import re
import sys
import urllib.request
from pathlib import Path

capture = Path(sys.argv[1])
config = Path(
    sys.argv[2] if len(sys.argv) > 2 else "~/.config/rotari/notifications.toml"
)
text = config.expanduser().read_text()
section = re.search(r"^\[webhook\]\s*$(.*?)(?=^\[|\Z)", text, re.M | re.S)
url = section and re.search(r'^\s*url\s*=\s*"([^"]+)"', section.group(1), re.M)
if not url:
    sys.exit(f"{config}: no [webhook] url")
payloads = sorted((capture / "webhook").glob("*.json"))
if not payloads:
    sys.exit(f"{capture}/webhook: no recorded payloads")
for payload in payloads:
    request = urllib.request.Request(
        url.group(1),
        data=payload.read_bytes(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=20) as response:
        print(
            f"{payload.name}: HTTP {response.status} {response.read(64).decode(errors='replace')}"
        )
