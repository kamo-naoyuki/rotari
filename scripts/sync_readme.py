#!/usr/bin/env python3
"""Synchronize the Getting started guide into the GitHub README."""

from __future__ import annotations

import argparse
import pathlib
import re

BEGIN = "<!-- BEGIN GETTING STARTED -->"
END = "<!-- END GETTING STARTED -->"
PITCH_BEGIN = "<!-- BEGIN ROTARI VALUE PROPOSITION -->"
PITCH_END = "<!-- END ROTARI VALUE PROPOSITION -->"
PITCH_HEADING = "## Manage jobs, logs, and outputs in one place"
RELATIVE_MARKDOWN_LINK = re.compile(r"\]\((?!https?://)([^)#]+)")
README_LINK_REPLACEMENT = r"](docs/\1"


def readme_pitch(guide: str) -> tuple[str, str]:
    pitch_start = guide.find(f"{PITCH_HEADING}\n")
    installation_start = guide.find("## Installation", pitch_start)
    if pitch_start < 0 or installation_start < 0:
        raise SystemExit(
            "Getting started must contain the rotari value proposition before Installation"
        )
    pitch = guide[pitch_start:installation_start].strip()
    remaining_guide = guide[:pitch_start] + guide[installation_start:]
    return pitch, remaining_guide


def readme_content(repo_dir: pathlib.Path) -> str:
    guide = (repo_dir / "docs" / "GETTING_STARTED.md").read_text(encoding="utf-8")
    guide = re.sub(r"^# Getting started\s*\n", "", guide, count=1)
    _pitch, guide = readme_pitch(guide)
    guide = RELATIVE_MARKDOWN_LINK.sub(README_LINK_REPLACEMENT, guide)
    return f"{BEGIN}\n\n{guide.rstrip()}\n\n{END}"


def update_readme_pitch(readme: str, pitch: str) -> str:
    generated = f"{PITCH_BEGIN}\n\n{pitch}\n\n{PITCH_END}"
    markers = re.compile(
        rf"{re.escape(PITCH_BEGIN)}.*?{re.escape(PITCH_END)}", re.DOTALL
    )
    updated, count = markers.subn(generated, readme, count=1)
    if count == 1:
        return updated

    legacy_start = readme.find("## Why use rotari?\n")
    legacy_end = readme.find("## How is rotari different?", legacy_start)
    if legacy_start < 0 or legacy_end < 0:
        raise SystemExit(
            "README must contain the value proposition or its legacy heading"
        )
    return readme[:legacy_start] + f"{generated}\n\n" + readme[legacy_end:]


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    repo_dir = pathlib.Path(__file__).resolve().parents[1]
    readme_path = repo_dir / "README.md"
    readme = readme_path.read_text(encoding="utf-8")
    guide = (repo_dir / "docs" / "GETTING_STARTED.md").read_text(encoding="utf-8")
    guide = re.sub(r"^# Getting started\s*\n", "", guide, count=1)
    pitch, _ = readme_pitch(guide)
    pitch = RELATIVE_MARKDOWN_LINK.sub(README_LINK_REPLACEMENT, pitch)
    readme = update_readme_pitch(readme, pitch)
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
