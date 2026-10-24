"""Capture material for the outputs episode: Artifacts, previews, TensorBoard, Files.

Run through `capture-web.sh WORK ROTARI SAMPLES capture_outputs.py` (no
display needed). The experiment directory ~/tts is a rotari workspace
(`rotari init`), so its state is in ~/tts/.rotari-state and the run page's
Files view opens ~/tts. Earlier experiments' outputs sit beside the new run's,
so TensorBoard on outputs/ compares them. The Web UI runs with --allow-exec
and is signed in to over loopback HTTP before anything is recorded; a
headless Chrome draws it 1100x782 CSS px at scale 2 for the landscape frame.

    capture_outputs.py FIXTURE OUT ROTARI SAMPLES

Writes OUT/terminals.json, OUT/web.json, OUT/logs.json, OUT/web/<scene>/*.png,
and OUT/sample.wav (the clip played on screen).
"""

import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path

from common import Session, only
from playwright.sync_api import sync_playwright

FIXTURE, OUT, ROTARI, SAMPLES = sys.argv[1:5]
HOME = "/home/me"
EXP = f"{HOME}/tts"
REPO = str(Path(__file__).resolve().parents[2])
PORT = 18795
BASE = f"http://127.0.0.1:{PORT}"
VIEW = {"width": 1100, "height": 782}
RUN_OUT = "outputs/exp0415"
# Earlier experiments, as (directory, where the loss settles).
PAST = [("exp0412_lr1e-3", 0.62), ("exp0413_lr3e-4", 0.47), ("exp0414_bs32", 0.40)]

ENV = {
    "PATH": f"{HOME}/.local/bin:{REPO}/.venv/bin:/usr/bin:/bin",
    "HOME": HOME,
    "USER": "me",
    "LOGNAME": "me",
    "TERM": "xterm-256color",
    "COLUMNS": "68",
    "LC_ALL": "C",
    "TZ": "UTC",
    "ROTARI_MASTERDIR": f"{HOME}/.rotari-master",
    "TTS_SAMPLES": f"{HOME}/.cache/tts-samples",
    "MPLCONFIGDIR": f"{HOME}/.cache/matplotlib",
    # --allow-exec needs a token; this one never leaves the namespace.
    "ROTARI_WEB_AUTH_TOKEN": "promo-capture-token-0123456789",
}
S = Session(ENV, HOME, OUT, basedir=f"{EXP}/.rotari-state", workdir=EXP)


def setup():
    os.makedirs(f"{HOME}/.local/bin", exist_ok=True)
    shutil.copy(ROTARI, f"{HOME}/.local/bin/rotari")
    shutil.copytree(SAMPLES, f"{HOME}/.cache/tts-samples", dirs_exist_ok=True)
    os.makedirs(EXP, exist_ok=True)
    shutil.copy(f"{FIXTURE}/tts_train.py", f"{EXP}/train.py")
    shutil.rmtree(OUT, ignore_errors=True)
    os.makedirs(f"{OUT}/web")
    S.sh("rotari init", cwd=EXP)
    for name, floor in PAST:
        S.sh(
            f"python train.py --epochs 1 --floor {floor} --out outputs/{name}", cwd=EXP
        )


def show_launch_panel(modal, panel):
    """Scroll the listing back left and the launch panel into view."""
    modal.evaluate(
        "m => m.querySelectorAll('*').forEach((e) => { if (e.scrollLeft) e.scrollLeft = 0; })"
    )
    if panel.count():
        panel.scroll_into_view_if_needed()


def launch(page, modal, frames, prefix, choose, panel):
    """Launch TensorBoard from panel's buttons; record its steps and the board.

    choose opens the panel; panel locates the row of launch buttons to use.
    """
    choose()
    page.wait_for_timeout(800)
    show_launch_panel(modal, panel)
    S.record(page, frames, f"{prefix}_menu", 0.5, fps=4)
    panel.get_by_role("button", name="TensorBoard", exact=True).first.click()
    page.wait_for_timeout(800)
    show_launch_panel(modal, modal.locator("button.launch-confirm"))
    S.record(page, frames, f"{prefix}_confirm", 0.5, fps=4)
    modal.locator("button.launch-confirm").first.click()
    modal.locator("a.launch-open").first.wait_for(timeout=90000)
    page.wait_for_timeout(500)
    show_launch_panel(modal, modal.locator("a.launch-open"))
    S.record(page, frames, f"{prefix}_ready", 0.5, fps=4)
    with page.context.expect_page() as opened:
        modal.locator("a.launch-open").first.click()
    board = opened.value
    board.wait_for_load_state()
    # TensorBoard draws its charts after loading the event files.
    board.wait_for_timeout(9000)
    close = board.locator('button[aria-label="Close side pane"]')
    if close.count():
        close.first.click()
        board.wait_for_timeout(800)
    S.record(board, frames, f"{prefix}_board", 0.5, fps=4)
    board.close()


def main():
    setup()
    terms, frames, logs = [], {}, {}

    # Run the experiment from the terminal.
    t = S.term("run")
    t.run(f"rotari add -p tts python train.py --out {RUN_OUT}", keep=only("added"))
    t.run("rotari run -p tts", keep=only("progress", "Summary"), line_s=0.08)
    terms.append(t)

    web = subprocess.Popen(
        [f"{HOME}/.local/bin/rotari", "web", "--port", str(PORT), "--allow-exec"],
        cwd=EXP,
        env=ENV,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    time.sleep(1.5)
    try:
        with sync_playwright() as p:
            b = p.chromium.launch(
                args=["--no-sandbox", "--autoplay-policy=no-user-gesture-required"]
            )
            ctx = b.new_context(
                viewport=VIEW, device_scale_factor=2, color_scheme="dark"
            )
            page = ctx.new_page()
            # Sign in over plain HTTP on loopback, off camera.
            page.goto(f"{BASE}/_auth/login?next=/")
            page.fill('input[name="password"]', ENV["ROTARI_WEB_AUTH_TOKEN"])
            page.locator("form button, form input[type=submit]").first.click()
            page.wait_for_load_state()
            if "/_auth/login" in page.url:
                raise SystemExit(f"sign-in failed: {page.url}")
            rid = S.runs_of("tts")[-1]
            run_page = f"{BASE}/project/tts/run/{rid}"
            page.goto(run_page)
            page.wait_for_timeout(1500)
            modal = page.locator("#output-modal")

            # The job's Artifacts: its output directory, a plot, then TensorBoard.
            page.get_by_role("button", name="Artifacts").first.click()
            page.wait_for_timeout(1200)
            S.record(page, frames, "artifacts", 0.5, fps=4)
            modal.get_by_text(RUN_OUT, exact=True).click()
            page.wait_for_timeout(1200)
            # An opened directory or file is shown below the listing.
            modal.locator("table").last.scroll_into_view_if_needed()
            S.record(page, frames, "outputs_dir", 0.5, fps=4)
            modal.get_by_text("loss.png", exact=True).first.click()
            page.wait_for_timeout(1500)
            modal.locator("img").last.scroll_into_view_if_needed()
            S.record(page, frames, "loss", 0.5, fps=4)
            modal.get_by_text(RUN_OUT, exact=True).click()
            page.wait_for_timeout(800)
            # Each row has its own launch buttons.
            row = modal.locator("tr").filter(has=page.get_by_text("tb", exact=True))
            launch(page, modal, frames, "tb", lambda: None, row)

            # A sample from the evaluation table, briefly.
            page.goto(run_page)
            page.wait_for_timeout(1500)
            page.get_by_role("button", name="Artifacts").first.click()
            page.wait_for_timeout(1200)
            modal.get_by_text(RUN_OUT, exact=True).click()
            page.wait_for_timeout(800)
            row = modal.locator("tr").filter(
                has=page.get_by_text("eval.csv", exact=True)
            )
            with ctx.expect_page() as opened:
                row.get_by_text("Table viewer").first.click()
            table = opened.value
            table.wait_for_timeout(2000)
            table.evaluate(
                """() => { const s = [...document.querySelectorAll('select')]
                  .find((e) => [...e.options].some((o) => o.value === 'file'));
                  s.value = 'file'; s.dispatchEvent(new Event('change', {bubbles: true})); }"""
            )
            table.wait_for_timeout(1500)
            table.get_by_text("samples/LJ001-0001.wav").first.click()
            table.wait_for_timeout(3000)
            table.locator("audio.tv-audio").first.evaluate("a => a.play()")
            S.record(table, frames, "pane", 6.5, fps=8)
            shutil.copy(f"{EXP}/{RUN_OUT}/samples/LJ001-0001.wav", f"{OUT}/sample.wav")
            table.close()

            # The run's Files view: the whole experiment directory, then
            # TensorBoard on outputs/ to compare every experiment.
            page.goto(run_page)
            page.wait_for_timeout(1500)
            page.get_by_role("button", name="Files", exact=True).first.click()
            page.wait_for_timeout(1500)
            S.record(page, frames, "files", 0.5, fps=4)
            modal.get_by_text("outputs", exact=True).first.click()
            page.wait_for_timeout(1500)
            S.record(page, frames, "files_outputs", 0.5, fps=4)
            # The buttons beside the directory's path, the first in the view,
            # launch on outputs/ itself.
            launch(page, modal, frames, "all", lambda: None, modal)
            b.close()
    finally:
        web.terminate()

    Path(f"{OUT}/terminals.json").write_text(
        json.dumps([t.dump() for t in terms], indent=1)
    )
    Path(f"{OUT}/web.json").write_text(json.dumps(frames, indent=1))
    Path(f"{OUT}/logs.json").write_text(json.dumps(logs, indent=1))


main()
