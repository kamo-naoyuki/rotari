#!/usr/bin/env python3
"""Synchronize the Getting started guide into the GitHub README."""

from __future__ import annotations

import argparse
import pathlib
import re

BEGIN = "<!-- BEGIN GETTING STARTED -->"
END = "<!-- END GETTING STARTED -->"


def readme_content(repo_dir: pathlib.Path) -> str:
    guide = (repo_dir / "docs" / "GETTING_STARTED.md").read_text(encoding="utf-8")
    guide = re.sub(r"^# Getting started\s*\n", "", guide, count=1)
    guide = re.sub(r"\]\((?!https?://)([^)#]+)", r"](docs/\1", guide)
    return f"{BEGIN}\n\n{guide.rstrip()}\n\n{END}"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    repo_dir = pathlib.Path(__file__).resolve().parents[1]
    readme_path = repo_dir / "README.md"
    readme = readme_path.read_text(encoding="utf-8")
    replacement = readme_content(repo_dir)
    pattern = re.compile(rf"{re.escape(BEGIN)}.*?{re.escape(END)}", re.DOTALL)
    updated, count = pattern.subn(replacement, readme, count=1)
    if count != 1:
        raise SystemExit("README must contain exactly one Getting started marker pair")
    if args.check:
        if updated != readme:
            raise SystemExit("README is out of sync; run scripts/sync_readme.py")
    else:
        readme_path.write_text(updated, encoding="utf-8")


if __name__ == "__main__":
    main()
