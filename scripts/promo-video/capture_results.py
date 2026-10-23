"""Capture material for the "results" episode: know when a run ends, then look at it.

Run through capture-results.sh, which sets up the namespaces, a private /, an
Xvfb display (:7, 1200x1620), a D-Bus session, and dunst. A headed Chrome
shows the Web UI at scale 2 (600x810 CSS px), so its desktop notification is
drawn by dunst and recorded with ffmpeg x11grab.

    capture_results.py FIXTURE OUT ROTARI SAMPLES

Writes OUT/terminals.json, OUT/web.json, OUT/logs.json, OUT/web/<scene>/*.png,
and OUT/webhook/*.json: the Slack payloads rotari sent, received by a local
recorder instead of Slack (send-webhook.py delivers them later).
"""

import http.server
import json
import os
import shutil
import subprocess
import sys
import threading
import time
from pathlib import Path

from common import Session, only
from playwright.sync_api import sync_playwright

FIXTURE, OUT, ROTARI, SAMPLES = sys.argv[1:5]
HOME = "/home/me"
REPO = str(Path(__file__).resolve().parents[2])
PORT = 18792
HOOK_PORT = 18990
BASE = f"http://127.0.0.1:{PORT}"
SCREEN = "1200x1620"

ENV = {
    "PATH": f"{HOME}/.local/bin:{REPO}/.venv/bin:/usr/bin:/bin",
    "HOME": HOME,
    "USER": "me",
    "LOGNAME": "me",
    "TERM": "xterm-256color",
    "COLUMNS": "52",
    "LC_ALL": "C",
    "TZ": "UTC",
    "ROTARI_MASTERDIR": f"{HOME}/.rotari-master",
    "TTS_SAMPLES": f"{HOME}/.cache/tts-samples",
    "MPLCONFIGDIR": f"{HOME}/.cache/matplotlib",
    # --allow-exec needs a token; this one never leaves the namespace.
    "ROTARI_WEB_AUTH_TOKEN": "promo-capture-token-0123456789",
}
S = Session(ENV, HOME, OUT)

# The fields of the user's own Slack notifications; the URL points at the
# recorder below, so nothing leaves the namespace during the capture.
NOTIFICATIONS = f"""[webhook]
url = "http://127.0.0.1:{HOOK_PORT}/slack"
format = "slack"
job_failure = true
job_success = false
run_failure = true
run_success = true
fields = ["attempt_id", "diagnosis_name", "diagnosis_suggestion", "duration", "exit_code",
  "failure_count", "job_id", "job_name", "job_status", "project", "run_id", "run_name",
  "run_status", "success_count"]
max_jobs = 20

[browser]
job_failure = true
job_success = false
run_failure = true
run_success = true
fields = ["exit_code", "project", "run_status", "duration", "success_count"]
max_jobs = 10
"""


RUN_OUT = "outputs/exp0415"


class Recorder(http.server.BaseHTTPRequestHandler):
    """Saves each webhook POST body as OUT/webhook/NNN.json."""

    count = 0

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        Recorder.count += 1
        Path(f"{OUT}/webhook/{Recorder.count:03d}.json").write_bytes(body)
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok")

    def log_message(self, *args):
        pass


def setup():
    os.makedirs(f"{HOME}/.local/bin", exist_ok=True)
    shutil.copy(ROTARI, f"{HOME}/.local/bin/rotari")
    shutil.copytree(SAMPLES, f"{HOME}/.cache/tts-samples", dirs_exist_ok=True)
    os.makedirs(f"{HOME}/tts", exist_ok=True)
    shutil.copy(f"{FIXTURE}/tts_train.py", f"{HOME}/tts/train.py")
    os.makedirs(f"{HOME}/.config/rotari", exist_ok=True)
    Path(f"{HOME}/.config/rotari/notifications.toml").write_text(NOTIFICATIONS)
    shutil.rmtree(OUT, ignore_errors=True)
    os.makedirs(f"{OUT}/web")
    os.makedirs(f"{OUT}/webhook")


def show_launch_panel(modal):
    """Scroll the Artifacts listing back left and its launch panel into view."""
    modal.evaluate(
        "m => m.querySelectorAll('*').forEach((e) => { if (e.scrollLeft) e.scrollLeft = 0; })"
    )
    panel = modal.locator(".launch-choices").last
    if panel.count():
        panel.scroll_into_view_if_needed()


def x11grab(name, seconds, fps=10):
    """Record the Xvfb screen into OUT/web/NAME as numbered PNGs."""
    os.makedirs(f"{OUT}/web/{name}", exist_ok=True)
    return subprocess.Popen(
        [
            "ffmpeg",
            "-y",
            "-loglevel",
            "error",
            "-f",
            "x11grab",
            # Chrome sets its own cursor, which -nocursor on Xvfb does not hide.
            "-draw_mouse",
            "0",
            "-video_size",
            SCREEN,
            "-framerate",
            str(fps),
            "-i",
            os.environ["DISPLAY"],
            "-t",
            str(seconds),
            f"{OUT}/web/{name}/%04d.png",
        ]
    )


def grabbed(name, fps=10):
    files = sorted(os.listdir(f"{OUT}/web/{name}"))
    return [[round(i / fps, 3), f"web/{name}/{f}"] for i, f in enumerate(files)]


def main():
    setup()
    terms, frames, logs = [], {}, {}
    hook = http.server.HTTPServer(("127.0.0.1", HOOK_PORT), Recorder)
    threading.Thread(target=hook.serve_forever, daemon=True).start()

    # Hook: checking on a run by hand, again and again.
    t = S.term("wait", f"{HOME}/tts")
    S.sh(
        "nohup python train.py --out outputs/exp0414_lr3e-4_bs16 > train.log 2>&1 &",
        cwd=f"{HOME}/tts",
    )
    # Python and matplotlib take a few seconds to start; wait for epoch 1.
    time.sleep(6)
    t.run("tail -n 1 train.log", line_s=0.05)
    t.wait(0.8)
    time.sleep(3.2)
    t.run("tail -n 1 train.log", line_s=0.05)
    t.wait(0.8)
    time.sleep(2.8)
    t.run("tail -n 1 train.log", line_s=0.05)
    t.wait(0.6)
    terms.append(t)
    time.sleep(6)

    web = subprocess.Popen(
        [f"{HOME}/.local/bin/rotari", "web", "--port", str(PORT), "--allow-exec"],
        cwd=HOME,
        env=ENV,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    time.sleep(1.5)
    try:
        # Signing in would otherwise leave Chrome's "Save password?" bubble on
        # screen; turn the password manager off in the profile.
        profile = f"{HOME}/.cache/chrome-profile"
        os.makedirs(f"{profile}/Default", exist_ok=True)
        Path(f"{profile}/Default/Preferences").write_text(
            json.dumps(
                {
                    "credentials_enable_service": False,
                    "profile": {"password_manager_enabled": False},
                }
            )
        )
        with sync_playwright() as p:
            ctx = p.chromium.launch_persistent_context(
                profile,
                executable_path="/usr/bin/google-chrome",
                headless=False,
                no_viewport=True,
                # No window manager runs, so size the window to the screen
                # (in device-independent pixels at scale 2) instead of --kiosk.
                # Chrome runs as the namespace's root, which needs
                # --no-sandbox; --test-type hides the info bar warning about it.
                args=[
                    "--no-sandbox",
                    "--test-type",
                    "--force-device-scale-factor=2",
                    "--window-position=0,0",
                    "--window-size=600,810",
                    "--hide-scrollbars",
                    "--no-first-run",
                    "--disable-session-crashed-bubble",
                    "--lang=en-US",
                    "--disable-features=Translate,TranslateUI",
                    "--autoplay-policy=no-user-gesture-required",
                ],
                env={**os.environ, "LANG": "en_US.UTF-8", "LANGUAGE": "en_US"},
                color_scheme="dark",
            )
            ctx.grant_permissions(["notifications"], origin=BASE)
            page = ctx.pages[0] if ctx.pages else ctx.new_page()
            # Sign in over plain HTTP on loopback, off camera.
            page.goto(f"{BASE}/_auth/login?next=/")
            page.fill('input[name="password"]', ENV["ROTARI_WEB_AUTH_TOKEN"])
            page.locator("form button, form input[type=submit]").first.click()
            page.wait_for_load_state()
            if "/_auth/login" in page.url:
                raise SystemExit(f"sign-in failed: {page.url}")

            # Submit the run from the terminal, then watch it end in the Web UI.
            t = S.term("submit", f"{HOME}/tts")
            t.run(
                f"rotari add -p tts python train.py --out {RUN_OUT}",
                keep=only("added"),
            )
            t.run("rotari run -p tts --async", keep=only("=== Run started", "Project:"))
            terms.append(t)
            rid = S.runs_of("tts")[-1]
            page.goto(f"{BASE}/project/tts/run/{rid}")
            page.wait_for_timeout(1200)
            toggle = page.locator("#notify-toggle")
            if "on" not in toggle.inner_text().lower().split()[-1:]:
                toggle.click()
            page.wait_for_timeout(500)
            logs["notify_toggle"] = toggle.inner_text()
            rec = x11grab("finish", 16)
            S.wait_idle("tts")
            rec.wait()
            frames["finish"] = grabbed("finish")
            page.get_by_role("button", name="Inbox").first.click()
            page.wait_for_timeout(1500)
            S.record(page, frames, "inbox", 1.5, fps=4)
            page.keyboard.press("Escape")
            page.wait_for_timeout(500)

            # Results: artifacts, the plot, then the evaluation table, whose
            # viewer plays a sample with its waveform and spectrogram.
            page.reload()
            page.wait_for_timeout(1500)
            page.get_by_role("button", name="Artifacts").first.click()
            modal = page.locator("#output-modal")
            page.wait_for_timeout(1200)
            S.record(page, frames, "artifacts", 0.5, fps=4)
            modal.get_by_text(RUN_OUT, exact=True).click()
            page.wait_for_timeout(1200)
            S.record(page, frames, "run1", 0.5, fps=4)
            modal.get_by_text("loss.png", exact=True).first.click()
            page.wait_for_timeout(1500)
            S.record(page, frames, "loss", 0.5, fps=4)
            # TensorBoard on the run's tb/ directory, launched from Artifacts.
            modal.get_by_text(RUN_OUT, exact=True).click()
            page.wait_for_timeout(800)
            row = modal.locator("tr").filter(has=page.get_by_text("tb", exact=True))
            row.get_by_role("button", name="Launch with…").click()
            page.wait_for_timeout(800)
            show_launch_panel(modal)
            S.record(page, frames, "launch_menu", 0.5, fps=4)
            # The run toolbar has launch buttons too; use the ones in Artifacts.
            modal.locator(".launch-choices").get_by_role(
                "button", name="TensorBoard", exact=True
            ).first.click()
            page.wait_for_timeout(800)
            show_launch_panel(modal)
            S.record(page, frames, "launch_confirm", 0.5, fps=4)
            modal.locator("button.launch-confirm").first.click()
            modal.locator("a.launch-open").first.wait_for(timeout=90000)
            page.wait_for_timeout(500)
            show_launch_panel(modal)
            S.record(page, frames, "launch_ready", 0.5, fps=4)
            with ctx.expect_page() as opened:
                modal.locator("a.launch-open").first.click()
            board = opened.value
            board.wait_for_load_state()
            # TensorBoard draws its charts after loading the event files.
            board.wait_for_timeout(9000)
            # The settings pane would take a third of a 600 px window.
            close = board.locator('button[aria-label="Close side pane"]')
            if close.count():
                close.first.click()
                board.wait_for_timeout(800)
            S.record(board, frames, "tensorboard", 0.5, fps=4)
            board.close()
            page.reload()
            page.wait_for_timeout(1500)
            page.get_by_role("button", name="Artifacts").first.click()
            page.wait_for_timeout(1200)
            modal.get_by_text(RUN_OUT, exact=True).click()
            page.wait_for_timeout(800)
            row = modal.locator("tr", has_text="eval.csv")
            row.get_by_role("button", name="Launch with…").click()
            page.wait_for_timeout(500)
            with ctx.expect_page() as opened:
                modal.get_by_text("Table viewer").first.click()
            table = opened.value
            table.wait_for_timeout(2000)
            choice = table.evaluate(
                """() => { const s = [...document.querySelectorAll('select')]
                  .find((e) => [...e.options].some((o) => o.value === 'basedir'));
                  return s ? [...s.options].map((o) => o.value + '|' + o.text) : []; }"""
            )
            logs["media_bases"] = choice
            target = next(
                (
                    c.split("|")[0]
                    for c in choice
                    if "eval.csv" in c or "file" in c.lower()
                ),
                None,
            )
            if target:
                table.evaluate(
                    """(v) => { const s = [...document.querySelectorAll('select')]
                      .find((e) => [...e.options].some((o) => o.value === 'basedir'));
                      s.value = v; s.dispatchEvent(new Event('change', {bubbles: true})); }""",
                    target,
                )
                table.wait_for_timeout(1500)
            S.record(table, frames, "table", 0.5, fps=4)
            table.get_by_text("samples/LJ001-0001.wav").first.click()
            # The browser computes the waveform and spectrogram after decoding.
            table.wait_for_timeout(3000)
            table.locator("audio.tv-audio").first.evaluate("a => a.play()")
            S.record(table, frames, "pane", 7.0, fps=8)
            # The clip the viewer played, for the video's audio track.
            shutil.copy(
                f"{HOME}/tts/{RUN_OUT}/samples/LJ001-0001.wav", f"{OUT}/sample.wav"
            )
            ctx.close()
    finally:
        web.terminate()
        hook.shutdown()

    logs["webhooks"] = sorted(os.listdir(f"{OUT}/webhook"))
    Path(f"{OUT}/terminals.json").write_text(
        json.dumps([t.dump() for t in terms], indent=1)
    )
    Path(f"{OUT}/web.json").write_text(json.dumps(frames, indent=1))
    Path(f"{OUT}/logs.json").write_text(json.dumps(logs, indent=1))


main()
