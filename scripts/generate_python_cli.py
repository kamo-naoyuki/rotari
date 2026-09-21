#!/usr/bin/env python3
"""Generate the checked-in Python view of the rotari CLI schema."""

from __future__ import annotations

import argparse
import json
import pprint
from pathlib import Path
from typing import Any


def render(schema: dict[str, Any]) -> str:
    payload = pprint.pformat(schema, sort_dicts=True, width=88)
    return (
        "\"\"\"Generated from `rotari schema --json`; do not edit manually.\"\"\"\n\n"
        "from __future__ import annotations\n\n"
        "from typing import Any\n\n\n"
        f"CLI_SCHEMA: dict[str, Any] = {payload}\n"
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    arguments = parser.parse_args()
    schema = json.loads(arguments.input.read_text(encoding="utf-8"))
    if not isinstance(schema, dict) or schema.get("version") != 1:
        parser.error("unsupported CLI schema")
    arguments.output.parent.mkdir(parents=True, exist_ok=True)
    arguments.output.write_text(render(schema), encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
