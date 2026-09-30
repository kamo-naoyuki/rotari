#!/usr/bin/env python3
"""Generate schema-backed Python client option documentation."""

from __future__ import annotations

import argparse
import pathlib
import re
from typing import Any

from rotari.generated_cli import CLI_SCHEMA

BEGIN = "<!-- BEGIN GENERATED CLI OPTIONS -->"
END = "<!-- END GENERATED CLI OPTIONS -->"
COMMANDS = ("add", "run", "retry", "reset", "wait", "show")
SKIP_FLAGS = {"basedir", "project-name"}


def python_option_name(name: str) -> str:
    replacements = {
        "async": "async_",
        "executor-option": "executor_options",
        "job-id": "job_ids",
    }
    return replacements.get(name, name.replace("-", "_"))


def option_type(flag: dict[str, Any]) -> str:
    if flag.get("repeated"):
        return "Sequence[str]"
    if "value_name" not in flag:
        return "bool"
    return "str"


def render_table(command: dict[str, Any]) -> str:
    rows = [
        "| Option | Value | Description |",
        "| --- | --- | --- |",
    ]
    for flag in command.get("flags", ()):
        if flag["name"] in SKIP_FLAGS:
            continue
        description = flag.get("description", "").replace("|", "\\|")
        if flag.get("repeated") and "may be repeated" not in description.lower():
            description += " May be repeated."
        rows.append(
            f"| `{python_option_name(flag['name'])}` | `{option_type(flag)}` | "
            f"{description} |"
        )
    return "\n".join(rows)


def render() -> str:
    sections = [BEGIN, ""]
    for name in COMMANDS:
        command = next(item for item in CLI_SCHEMA["commands"] if item["name"] == name)
        return_type = {
            "add": "Job",
            "run": "Run",
            "retry": "Run",
            "wait": "dict[str, object]",
            "show": "dict[str, object]",
        }.get(name, "CommandResult")
        signature = f"Rotari.{name}(**options: object) -> {return_type}"
        if name == "add":
            signature = "Rotari.add(command: Sequence[str], **options: object) -> Job"
        elif name == "reset":
            signature = "Rotari.reset(*, recover: bool = False) -> CommandResult"
        elif name == "wait":
            signature = (
                "Rotari.wait(selector: Run | str | Sequence[Run | str] | None = None, "
                "**options: object) -> dict[str, object] | list[dict[str, object]]"
            )
        elif name == "show":
            signature = (
                "Rotari.show(target: Run | Job | Sequence[Run] | Sequence[Job] "
                "| None = None, "
                "*, run: Run | str | None = None, **options: object) "
                "-> dict[str, object] | list[dict[str, object]]"
            )
        sections.extend(
            [
                f"## `Rotari.{name}`",
                "",
                "```python",
                signature,
                "```",
                "",
                command.get("description", "").capitalize() + ".",
                "",
                render_table(command),
                "",
            ]
        )
    sections.extend([END, ""])
    return "\n".join(sections)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    docs_path = pathlib.Path(__file__).resolve().parents[1] / "docs" / "python-api.md"
    contents = docs_path.read_text(encoding="utf-8")
    pattern = re.compile(rf"{re.escape(BEGIN)}.*?{re.escape(END)}", re.DOTALL)
    replacement = render().rstrip()
    updated, count = pattern.subn(replacement, contents, count=1)
    if count != 1:
        raise SystemExit("python-api.md must contain exactly one generated marker pair")
    if args.check:
        if updated != contents:
            raise SystemExit("python-api.md is out of sync; run this script")
    else:
        docs_path.write_text(updated, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
