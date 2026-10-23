"""Helpers shared by the capture scripts: commands, terminal sessions, Web UI frames."""

import os
import subprocess
import time


class Session:
    """Runs scenario commands with one environment and records material into out."""

    def __init__(self, env, home, out):
        self.env = env
        self.home = home
        self.out = out

    def sh(self, cmd, cwd=None):
        return subprocess.run(
            ["bash", "-c", cmd],
            cwd=cwd or self.home,
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
        )

    def term(self, name, cwd=None):
        return Term(self, name, cwd or self.home)

    def record(self, page, frames, name, seconds, fps=6, full=False):
        """Screenshot page fps times a second into out/web/NAME for seconds."""
        os.makedirs(f"{self.out}/web/{name}", exist_ok=True)
        shots, start, n = [], time.time(), 0
        while time.time() - start <= seconds:
            path = f"web/{name}/{n:04d}.png"
            page.screenshot(path=f"{self.out}/{path}", full_page=full)
            shots.append([round(time.time() - start, 3), path])
            n += 1
            time.sleep(max(0, n / fps - (time.time() - start)))
        frames[name] = shots

    def runs_of(self, project):
        d = f"{self.home}/.local/state/rotari/projects/{project}/runs"
        for _ in range(50):
            if os.path.isdir(d) and os.listdir(d):
                break
            time.sleep(0.1)
        return sorted(os.listdir(d))

    def wait_idle(self, project, timeout=90):
        end = time.time() + timeout
        while time.time() < end:
            if "Lifecycle: running" not in self.sh(f"rotari show -p {project}").stdout:
                return
            time.sleep(0.5)
        raise RuntimeError(f"{project} still running")


class Term:
    """A terminal session as timed text appends on a virtual clock (seconds)."""

    def __init__(self, session, name, cwd):
        self.session = session
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
            self.cwd = os.path.join(
                self.session.home, line[3:].replace("~/", "").strip()
            )
            self.wait(0.2)
            return ""
        r = self.session.sh(real or line, cwd=self.cwd)
        out = r.stdout + r.stderr
        self.output(out, keep=keep, line_s=line_s)
        return out

    def live(self, line, cps=28):
        """Run a command and emit its output lines at the times they appear."""
        self.type(line, cps=cps)
        p = subprocess.Popen(
            ["bash", "-c", line],
            cwd=self.cwd,
            env=self.session.env,
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
