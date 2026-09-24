"""Console-script entry point: run the bundled `rotari` executable, or fall
back to one found on PATH (for installs built without a bundled binary).
"""

from __future__ import annotations

import os
import pathlib
import sys


def _bundled_binary() -> pathlib.Path | None:
    path = pathlib.Path(__file__).with_name("_bin") / "rotari"
    return path if path.is_file() else None


def _binary_on_path() -> str | None:
    # Exclude this very console script so a missing bundled binary can still
    # fall back to a real `rotari` located elsewhere on PATH.
    self_path = pathlib.Path(sys.argv[0]).resolve()
    for directory in os.environ.get("PATH", "").split(os.pathsep):
        if not directory:
            continue
        candidate = pathlib.Path(directory) / "rotari"
        if (
            candidate.is_file()
            and os.access(candidate, os.X_OK)
            and candidate.resolve() != self_path
        ):
            return str(candidate)
    return None


def main() -> None:
    bundled = _bundled_binary()
    if bundled is not None:
        os.chmod(bundled, 0o755)
        target = str(bundled)
    else:
        target = _binary_on_path()
        if target is None:
            sys.exit(
                "rotari: no bundled executable in this wheel and no `rotari` "
                "found on PATH; install a platform-specific wheel from the "
                "package index or place `rotari` on PATH"
            )
    os.execv(target, [target, *sys.argv[1:]])


if __name__ == "__main__":
    main()
