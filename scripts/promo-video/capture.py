"""Capture material for the rotari promo short ("You don't need rotari for one job").

Run through capture.sh, which enters user, UTS, mount, network and PID
namespaces: the host name is "workstation", /home/me is WORK/home, and, when a
Slurm sandbox is given, a one-node Slurm cluster (gpu-node01) runs from it.
Without Slurm the Act 3 material is skipped.

Writes OUT/terminals.json, OUT/web.json, OUT/logs.json and OUT/web/<scene>/*.png.
"""

import json
import os
import shutil
import subprocess
import sys
import time

from common import Session, only, section
from playwright.sync_api import sync_playwright

FIXTURE, OUT, ROTARI = sys.argv[1:4]
HOME = "/home/me"
PORT = 18789
VIEW = {"width": 600, "height": 810}
PROJECTS = ["asr", "tts", "vision", "nlp"]

ENV = {
    "PATH": f"{HOME}/.local/bin:{HOME}/sbin:/usr/bin:/bin",
    "HOME": HOME,
    "USER": "me",
    "LOGNAME": "me",
    "TERM": "xterm-256color",
    "COLUMNS": "52",
    "LC_ALL": "C",
    "TZ": "UTC",
    "ROTARI_MASTERDIR": f"{HOME}/.rotari-master",
    "ROTARI_RUN_LOCAL_CONCURRENCY": "12",
    # Narrow squeue columns so its output fits half a vertical frame.
    "SQUEUE_FORMAT": "%.5i %.4P %.10j %.5u %.2t %.5M %R",
}
S = Session(ENV, HOME, OUT)


# Show only these columns of the run page's jobs table, through its Columns menu.
COLUMNS = """[...document.querySelectorAll('input[data-column]')].forEach((box) => {
  const want = ['status', 'executor', 'hosts'].includes(box.dataset.column);
  if (box.checked !== want) box.click();
})"""

# Page positions (CSS px from the top) that render.py pans between.
OFFSETS = """(() => {
  const top = (el) => (el ? el.getBoundingClientRect().top + scrollY : null);
  const byText = (t) => [...document.querySelectorAll('*')]
    .find((e) => e.children.length === 0 && e.textContent.trim() === t);
  const tables = [...document.querySelectorAll('table.runs')];
  const last = tables[tables.length - 1];
  return {
    cards: top(document.querySelector('.run-cards')),
    timeline: top(byText('Job timeline')),
    jobs_bottom: last ? last.getBoundingClientRect().bottom + scrollY : null,
    height: document.documentElement.scrollHeight,
  };
})()"""

SCROLL = (
    "window.scrollTo(0, document.querySelector('.run-cards')"
    ".getBoundingClientRect().top + scrollY - 70)"
)


def setup():
    os.makedirs(f"{HOME}/.local/bin", exist_ok=True)
    shutil.copy(ROTARI, f"{HOME}/.local/bin/rotari")
    py = f"{HOME}/.local/bin/python"
    if not os.path.exists(py):
        os.symlink("/usr/bin/python3", py)
    for d in [*PROJECTS, "llm", "llm-sweep", "pipeline"]:
        os.makedirs(f"{HOME}/{d}", exist_ok=True)
    shutil.copy(f"{FIXTURE}/train.py", f"{HOME}/llm")
    for d in PROJECTS:
        shutil.copy(f"{FIXTURE}/train.sh", f"{HOME}/{d}")
    for f in ("sweep.sh", "sweep.conf"):
        shutil.copy(f"{FIXTURE}/{f}", f"{HOME}/llm-sweep")
    for f in ("prep.sh", "train-gpu.sh"):
        shutil.copy(f"{FIXTURE}/{f}", f"{HOME}/pipeline")
    shutil.rmtree(OUT, ignore_errors=True)
    os.makedirs(f"{OUT}/web")


def main():
    setup()
    terms, frames, logs = [], {}, {}

    # Act 0: one job is fine in a terminal.
    t = S.term("one", f"{HOME}/llm")
    t.live("python train.py")
    terms.append(t)

    # Act 1a: the same four projects without rotari, one terminal tab each.
    for d in PROJECTS:
        S.sh("nohup ./train.sh > train.log 2>&1 &", cwd=f"{HOME}/{d}")
    time.sleep(5)
    for d in PROJECTS:
        t = S.term(f"tab-{d}", f"{HOME}/{d}")
        t.run("tail -n 3 train.log", line_s=0.05)
        terms.append(t)
    time.sleep(9)

    web = subprocess.Popen(
        [f"{HOME}/.local/bin/rotari", "web", "--port", str(PORT)],
        cwd=HOME,
        env=ENV,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    time.sleep(1.5)
    base = f"http://127.0.0.1:{PORT}"
    try:
        with sync_playwright() as p:
            b = p.chromium.launch(args=["--no-sandbox"])
            wide = b.new_page(viewport=VIEW, device_scale_factor=2, color_scheme="dark")
            phone = wide

            # Act 1b: submit the four projects with rotari, then watch them on one page.
            t = S.term("submit4")
            for d in PROJECTS:
                t.run(f"cd ~/{d}", cps=70)
                t.run(f"rotari add -p {d} ./train.sh", cps=70, keep=only("added"))
                t.run(
                    f"rotari run -p {d} --async",
                    cps=70,
                    keep=only("=== Run started", "Project:"),
                )
            terms.append(t)
            wide.goto(base + "/")
            wide.wait_for_timeout(800)
            S.record(wide, frames, "projects", 15)
            for d in PROJECTS:
                S.wait_idle(d)

            # Act 2: a 48-run sweep with three kinds of failure.
            t = S.term("sweep", f"{HOME}/llm-sweep")
            t.run("rotari add -p llm-sweep --array 1-48 ./sweep.sh", keep=only("added"))
            t.run(
                "rotari run -p llm-sweep --async",
                keep=only("=== Run started", "Project:", "Run:"),
            )
            terms.append(t)
            rid = S.runs_of("llm-sweep")[-1]
            phone.goto(f"{base}/project/llm-sweep/run/{rid}")
            phone.wait_for_timeout(800)
            phone.evaluate(SCROLL)
            S.record(phone, frames, "sweep_live", 22)
            S.wait_idle("llm-sweep")
            phone.reload()
            phone.wait_for_timeout(1500)
            logs["offsets_sweep_done"] = phone.evaluate(OFFSETS)
            S.record(phone, frames, "sweep_done", 0.1, full=True)

            failed = S.sh("rotari show -p llm-sweep --failed-logs").stdout
            for key, marker in (
                ("gpu", "fallen off the bus"),
                ("oom", "OutOfMemoryError"),
            ):
                logs[key] = next(ln for ln in failed.splitlines() if marker in ln)

            t = S.term("causes", f"{HOME}/llm-sweep")
            t.run(
                "rotari show -p llm-sweep",
                keep=section("Failures by cause", ("Inspect", "To show")),
                line_s=0.06,
            )
            terms.append(t)
            logs["show_full"] = S.sh("rotari show -p llm-sweep").stdout

            # Fix the batch limit, then rerun only what failed.
            t = S.term("retry", f"{HOME}/llm-sweep")
            t.run("sed -i 's/max_batch=128/max_batch=64/' sweep.conf")
            t.run(
                "rotari retry -p llm-sweep",
                keep=only(
                    "Submitted", "Excluded", "progress", "Summary", "Carried", "Status"
                ),
                line_s=0.08,
            )
            terms.append(t)
            rid2 = S.runs_of("llm-sweep")[-1]
            phone.goto(f"{base}/project/llm-sweep/run/{rid2}")
            phone.wait_for_timeout(1500)
            logs["offsets_retry"] = phone.evaluate(OFFSETS)
            S.record(phone, frames, "retry", 0.1, full=True)

            # Act 3, only when capture.sh started Slurm (render.py --with-slurm).
            if os.path.exists(f"{HOME}/sbin/sbatch"):
                # Act 3a: the same work by hand on Slurm (left) and locally (right).
                left = S.term("slurm-raw", f"{HOME}/pipeline")
                left.run("sbatch train-gpu.sh")
                time.sleep(1.0)
                left.run("squeue", real="squeue | sed 's/ root / me   /'")
                right = S.term("local-raw", f"{HOME}/pipeline")
                right.run("nohup ./prep.sh > prep.log 2>&1 &")
                time.sleep(2.5)
                right.run(
                    "ps -o pid,etime,args | grep prep",
                    real="ps -o pid,etime,args | grep '[p]rep'",
                )
                right.run("tail -n 2 prep.log")
                time.sleep(4)
                left.run("cat slurm-*.out")
                terms += [left, right]
                time.sleep(1)
                S.sh("rm -f slurm-*.out prep.log nohup.out", cwd=f"{HOME}/pipeline")

                # Act 3b: one pipeline mixing both, through rotari.
                t = S.term("pipeline", f"{HOME}/pipeline")
                t.run(
                    "rotari add -p pipeline -e local --job-name prep ./prep.sh",
                    cps=40,
                    keep=only("added"),
                )
                t.run(
                    "rotari add -p pipeline -e slurm --depends-on prep ./train-gpu.sh",
                    cps=40,
                    keep=only("added"),
                )
                t.run(
                    "rotari run -p pipeline",
                    keep=only("progress", "Summary"),
                    line_s=0.08,
                )
                terms.append(t)
                rid3 = S.runs_of("pipeline")[-1]
                phone.goto(f"{base}/project/pipeline/run/{rid3}")
                phone.wait_for_timeout(1500)
                logs["columns"] = phone.evaluate(
                    "[...document.querySelectorAll('input[data-column]')].map(b => b.dataset.column)"
                )
                phone.evaluate(COLUMNS)
                phone.wait_for_timeout(500)
                logs["offsets_pipeline"] = phone.evaluate(OFFSETS)
                S.record(phone, frames, "pipeline", 0.1, full=True)
                logs["pipeline_show"] = S.sh("rotari show -p pipeline").stdout

            # Close: every project on one page.
            wide.goto(base + "/")
            wide.wait_for_timeout(1500)
            S.record(wide, frames, "projects_end", 1)
            b.close()
    finally:
        web.terminate()

    with open(f"{OUT}/terminals.json", "w") as f:
        json.dump([t.dump() for t in terms], f, indent=1)
    with open(f"{OUT}/web.json", "w") as f:
        json.dump(frames, f, indent=1)
    with open(f"{OUT}/logs.json", "w") as f:
        json.dump(logs, f, indent=1)


main()
