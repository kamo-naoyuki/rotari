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


def sh(cmd, cwd=HOME):
    return subprocess.run(
        ["bash", "-c", cmd], cwd=cwd, env=ENV, capture_output=True, text=True
    )


class Term:
    """A terminal session as timed text appends on a virtual clock (seconds)."""

    def __init__(self, name, cwd=HOME):
        self.name = name
        self.cwd = cwd
        self.t = 0.0
        self.events = []

    def emit(self, text):
        self.events.append([round(self.t, 3), text])

    def wait(self, s):
        self.t += s

    def type(self, line, cps=28, pause_after=0.45):
        self.emit("\x01$ \x02")
        self.wait(0.3)
        for ch in line:
            self.emit(ch)
            self.wait(1 / cps)
        self.wait(pause_after)
        self.emit("\n")

    def output(self, text, line_s=0.04, keep=None):
        lines = text.rstrip("\n").split("\n") if text.strip() else []
        if keep is not None:
            lines = keep(lines)
        for ln in lines:
            self.emit(ln + "\n")
            self.wait(line_s)
        self.wait(0.5)

    def run(self, line, real=None, keep=None, cps=28, line_s=0.04):
        self.type(line, cps=cps)
        if line.startswith("cd "):
            self.cwd = os.path.join(HOME, line[3:].replace("~/", "").strip())
            self.wait(0.2)
            return ""
        r = sh(real or line, cwd=self.cwd)
        out = r.stdout + r.stderr
        self.output(out, keep=keep, line_s=line_s)
        return out

    def live(self, line, cps=28):
        """Run a command and emit its output lines at the times they appear."""
        self.type(line, cps=cps)
        p = subprocess.Popen(
            ["bash", "-c", line],
            cwd=self.cwd,
            env=ENV,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
        )
        start, base = time.time(), self.t
        for ln in p.stdout:
            self.t = base + (time.time() - start)
            self.emit(ln)
        p.wait()
        self.t = base + (time.time() - start) + 0.5

    def dump(self):
        return {
            "name": self.name,
            "duration": round(self.t + 0.8, 3),
            "events": self.events,
        }


def section(start, stop=()):
    """Keep output lines from the first one starting with start, up to a stop prefix."""

    def keep(lines):
        out, on = [], False
        for ln in lines:
            if ln.startswith(start):
                on = True
            elif on and any(ln.startswith(s) for s in stop):
                break
            if on:
                out.append(ln)
        return out

    return keep


def only(*prefixes):
    return lambda lines: [ln for ln in lines if ln.lstrip().startswith(prefixes)]


def record(page, frames, name, seconds, fps=6, full=False):
    os.makedirs(f"{OUT}/web/{name}", exist_ok=True)
    out, start, n = [], time.time(), 0
    while time.time() - start <= seconds:
        path = f"web/{name}/{n:04d}.png"
        page.screenshot(path=f"{OUT}/{path}", full_page=full)
        out.append([round(time.time() - start, 3), path])
        n += 1
        time.sleep(max(0, n / fps - (time.time() - start)))
    frames[name] = out


def runs_of(project):
    d = f"{HOME}/.local/state/rotari/projects/{project}/runs"
    for _ in range(50):
        if os.path.isdir(d) and os.listdir(d):
            break
        time.sleep(0.1)
    return sorted(os.listdir(d))


def wait_idle(project, timeout=90):
    end = time.time() + timeout
    while time.time() < end:
        if "Lifecycle: running" not in sh(f"rotari show -p {project}").stdout:
            return
        time.sleep(0.5)
    raise RuntimeError(f"{project} still running")


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
    t = Term("one", f"{HOME}/llm")
    t.live("python train.py")
    terms.append(t)

    # Act 1a: the same four projects without rotari, one terminal tab each.
    for d in PROJECTS:
        sh("nohup ./train.sh > train.log 2>&1 &", cwd=f"{HOME}/{d}")
    time.sleep(5)
    for d in PROJECTS:
        t = Term(f"tab-{d}", f"{HOME}/{d}")
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
            t = Term("submit4")
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
            record(wide, frames, "projects", 15)
            for d in PROJECTS:
                wait_idle(d)

            # Act 2: a 48-run sweep with three kinds of failure.
            t = Term("sweep", f"{HOME}/llm-sweep")
            t.run("rotari add -p llm-sweep --array 1-48 ./sweep.sh", keep=only("added"))
            t.run(
                "rotari run -p llm-sweep --async",
                keep=only("=== Run started", "Project:", "Run:"),
            )
            terms.append(t)
            rid = runs_of("llm-sweep")[-1]
            phone.goto(f"{base}/project/llm-sweep/run/{rid}")
            phone.wait_for_timeout(800)
            phone.evaluate(SCROLL)
            record(phone, frames, "sweep_live", 22)
            wait_idle("llm-sweep")
            phone.reload()
            phone.wait_for_timeout(1500)
            logs["offsets_sweep_done"] = phone.evaluate(OFFSETS)
            record(phone, frames, "sweep_done", 0.1, full=True)

            failed = sh("rotari show -p llm-sweep --failed-logs").stdout
            for key, marker in (
                ("gpu", "fallen off the bus"),
                ("oom", "OutOfMemoryError"),
            ):
                logs[key] = next(ln for ln in failed.splitlines() if marker in ln)

            t = Term("causes", f"{HOME}/llm-sweep")
            t.run(
                "rotari show -p llm-sweep",
                keep=section("Failures by cause", ("Inspect", "To show")),
                line_s=0.06,
            )
            terms.append(t)
            logs["show_full"] = sh("rotari show -p llm-sweep").stdout

            # Fix the batch limit, then rerun only what failed.
            t = Term("retry", f"{HOME}/llm-sweep")
            t.run("sed -i 's/max_batch=128/max_batch=64/' sweep.conf")
            t.run(
                "rotari retry -p llm-sweep",
                keep=only(
                    "Submitted", "Excluded", "progress", "Summary", "Carried", "Status"
                ),
                line_s=0.08,
            )
            terms.append(t)
            rid2 = runs_of("llm-sweep")[-1]
            phone.goto(f"{base}/project/llm-sweep/run/{rid2}")
            phone.wait_for_timeout(1500)
            logs["offsets_retry"] = phone.evaluate(OFFSETS)
            record(phone, frames, "retry", 0.1, full=True)

            # Act 3, only when capture.sh started Slurm (render.py --with-slurm).
            if os.path.exists(f"{HOME}/sbin/sbatch"):
                # Act 3a: the same work by hand on Slurm (left) and locally (right).
                left = Term("slurm-raw", f"{HOME}/pipeline")
                left.run("sbatch train-gpu.sh")
                time.sleep(1.0)
                left.run("squeue", real="squeue | sed 's/ root / me   /'")
                right = Term("local-raw", f"{HOME}/pipeline")
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
                sh("rm -f slurm-*.out prep.log nohup.out", cwd=f"{HOME}/pipeline")

                # Act 3b: one pipeline mixing both, through rotari.
                t = Term("pipeline", f"{HOME}/pipeline")
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
                rid3 = runs_of("pipeline")[-1]
                phone.goto(f"{base}/project/pipeline/run/{rid3}")
                phone.wait_for_timeout(1500)
                logs["columns"] = phone.evaluate(
                    "[...document.querySelectorAll('input[data-column]')].map(b => b.dataset.column)"
                )
                phone.evaluate(COLUMNS)
                phone.wait_for_timeout(500)
                logs["offsets_pipeline"] = phone.evaluate(OFFSETS)
                record(phone, frames, "pipeline", 0.1, full=True)
                logs["pipeline_show"] = sh("rotari show -p pipeline").stdout

            # Close: every project on one page.
            wide.goto(base + "/")
            wide.wait_for_timeout(1500)
            record(wide, frames, "projects_end", 1)
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
