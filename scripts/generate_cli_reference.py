#!/usr/bin/env python3
"""Generate the CLI reference from the checked-in CLI schema."""

from __future__ import annotations

import argparse
import pathlib
import re

from rotari.generated_cli import CLI_SCHEMA

BEGIN = "<!-- BEGIN GENERATED CLI REFERENCE -->"
END = "<!-- END GENERATED CLI REFERENCE -->"


def render_subcommands(items: list[dict[str, str]]) -> list[str]:
    return [f"| `{item['name']}` | {item['description']} |" for item in items]


def render_flag(flag: dict[str, object]) -> str:
    value = flag.get("value_name", "")
    if flag.get("repeated"):
        value = f"{value} (repeatable)" if value else "repeatable"
    option = f"`--{flag['name']}`"
    if flag.get("short"):
        option = f"`-{flag['short']}` / {option}"
    description = str(flag.get("description", "")).replace("|", "\\|")
    return f"| {option} | `{value}` | `{flag.get('environment', '')}` | {description} |"


def render_command(command: dict[str, object]) -> list[str]:
    name = str(command["name"])
    sections = [f"### `rotari {name}`", "", str(command["description"]), ""]
    positional = command.get("positional")
    if positional:
        sections.extend([f"Usage: `rotari {name} {positional}`", ""])
    subcommands = command.get("subcommands", [])
    if subcommands:
        sections.extend(["| Subcommand | Description |", "| --- | --- |"])
        sections.extend(render_subcommands(subcommands))
        sections.append("")
    flags = command.get("flags", [])
    if flags:
        sections.extend(
            [
                "| Option | Value | Environment | Description |",
                "| --- | --- | --- | --- |",
            ]
        )
        sections.extend(render_flag(flag) for flag in flags)
        sections.append("")
    return sections


def render_commands() -> str:
    sections = ["## Commands", ""]
    for command in CLI_SCHEMA["commands"]:
        sections.extend(render_command(command))
    return "\n".join(sections).rstrip()


def render_environments() -> str:
    sections = [
        "## Environment variables",
        "",
        "`rotari env` prints this list with current values.",
        "",
        "| Variable | CLI default | Job | Array | Description |",
        "| --- | --- | --- | --- | --- |",
    ]
    for item in CLI_SCHEMA.get("environments", []):
        description = item["description"].replace("|", "\\|")
        sections.append(
            f"| `{item['name']}` | {'yes' if item['cli_default'] else 'no'} | "
            f"{'yes' if item['job'] else 'no'} | {'yes' if item['array'] else 'no'} | "
            f"{description} |"
        )
    return "\n".join(sections).rstrip()


def render() -> str:
    return f"{BEGIN}\n\n{render_commands()}\n\n{render_environments()}\n\n{END}"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    path = pathlib.Path(__file__).resolve().parents[1] / "docs" / "CLI_REFERENCE.md"
    contents = path.read_text(encoding="utf-8")
    updated, count = re.subn(
        rf"{re.escape(BEGIN)}.*?{re.escape(END)}",
        render(),
        contents,
        count=1,
        flags=re.DOTALL,
    )
    if count != 1:
        raise SystemExit(
            "CLI_REFERENCE.md must contain exactly one generated marker pair"
        )
    if args.check:
        if updated != contents:
            raise SystemExit("CLI_REFERENCE.md is out of sync; run this script")
    else:
        path.write_text(updated, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
