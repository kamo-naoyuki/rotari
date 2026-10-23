"""Full-page screenshots of captured Web UI pages, to choose framing.

fullpage.py ROTARI OUT_DIR WIDTH PATH...   (run inside the capture namespace)
"""

import subprocess
import sys
import time

from playwright.sync_api import sync_playwright

rotari, out, width, paths = sys.argv[1], sys.argv[2], int(sys.argv[3]), sys.argv[4:]
env = {
    "PATH": "/usr/bin:/bin",
    "HOME": "/home/me",
    "ROTARI_MASTERDIR": "/home/me/.rotari-master",
    "TZ": "UTC",
}
web = subprocess.Popen(
    [rotari, "web", "--port", "18790"],
    cwd="/home/me",
    env=env,
    stdout=subprocess.DEVNULL,
    stderr=subprocess.DEVNULL,
)
time.sleep(1.5)
try:
    with sync_playwright() as p:
        b = p.chromium.launch(args=["--no-sandbox"])
        page = b.new_page(
            viewport={"width": width, "height": 800},
            device_scale_factor=1,
            color_scheme="dark",
        )
        for i, path in enumerate(paths):
            page.goto("http://127.0.0.1:18790" + path)
            page.wait_for_timeout(1500)
            page.screenshot(path=f"{out}/full-{i}.png", full_page=True)
            print(path, page.evaluate("document.body.scrollHeight"))
        b.close()
finally:
    web.terminate()
