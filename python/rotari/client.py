from __future__ import annotations

import inspect
import json
import os
import re
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Mapping, Sequence

from .generated_cli import CLI_SCHEMA


def _cli_option_name(name: str) -> str:
    if name == "async_":
        return "async"
    if name.endswith("_ids"):
        return name[:-1].replace("_", "-")
    if name == "executor_options":
        return "executor-option"
    return name.replace("_", "-")


def build_command_arguments(
    command: str,
    options: Mapping[str, object] | None = None,
    positional: Sequence[str] = (),
) -> list[str]:
    """Build command argv from the generated CLI schema."""
    command_spec = next((item for item in CLI_SCHEMA["commands"] if item["name"] == command), None)
    if command_spec is None:
        raise ValueError(f"unknown CLI command: {command}")
    flags = {item["name"]: item for item in command_spec.get("flags", ())}
    provided = {_cli_option_name(option): value for option, value in (options or {}).items()}
    arguments = [command]
    for name, flag in flags.items():
        value = provided.get(name)
        if value is None:
            continue
        prefix = f"--{name}"
        values = (
            value
            if flag.get("repeated") and isinstance(value, Sequence) and not isinstance(value, (str, bytes))
            else (value,)
        )
        for item in values:
            if isinstance(item, bool):
                if item:
                    arguments.append(prefix)
            else:
                arguments.extend((prefix, str(item)))
    arguments.extend(positional)
    return arguments


def _python_option_name(name: str) -> str:
    if name == "async":
        return "async_"
    if name == "executor-option":
        return "executor_options"
    if name == "job-id":
        return "job_ids"
    return name.replace("-", "_")


def _install_cli_signatures() -> None:
    for command_spec in CLI_SCHEMA["commands"]:
        command = command_spec["name"]
        method = getattr(Rotari, command, None)
        if method is None:
            continue
        parameters = [inspect.Parameter("self", inspect.Parameter.POSITIONAL_OR_KEYWORD)]
        if command == "add":
            parameters.append(inspect.Parameter("command", inspect.Parameter.POSITIONAL_OR_KEYWORD))
        elif command == "wait":
            parameters.append(inspect.Parameter("selector", inspect.Parameter.POSITIONAL_OR_KEYWORD, default=None))
        for flag in command_spec.get("flags", ()):
            if flag["name"] in {"basedir", "project-name"}:
                continue
            name = _python_option_name(flag["name"])
            if flag.get("repeated"):
                default = ()
            elif flag.get("value_name"):
                default = None
            else:
                default = False
            parameters.append(inspect.Parameter(name, inspect.Parameter.KEYWORD_ONLY, default=default))
        method.__signature__ = inspect.Signature(parameters)


@dataclass(frozen=True)
class CommandResult:
    """Result of one rotari CLI invocation."""

    args: tuple[str, ...]
    returncode: int
    stdout: str
    stderr: str

    @property
    def run_id(self) -> str | None:
        match = re.search(r"\brun_id=([^\s]+)", self.stdout)
        return match.group(1) if match else None

    def json(self) -> object:
        """Decode the first JSON value printed by the command."""

        return json.loads(self.stdout)


class RotariError(RuntimeError):
    """A rotari command could not be completed."""

    def __init__(self, result: CommandResult):
        self.result = result
        message = result.stderr.strip() or result.stdout.strip() or "rotari command failed"
        super().__init__(f"{message} (exit code {result.returncode})")


class Rotari:
    """Thin, process-based client for the rotari executable.

    The Go CLI remains the source of truth. This class only constructs argv,
    starts the process, and decodes the CLI's machine-readable JSON modes. It
    accepts executable argument lists, not Python functions or closures to
    serialize and submit.
    """

    def __init__(
        self,
        executable: str | os.PathLike[str] = "rotari",
        *,
        basedir: str | os.PathLike[str] | None = None,
        project: str | None = None,
        cwd: str | os.PathLike[str] | None = None,
        env: Mapping[str, str] | None = None,
    ) -> None:
        self.executable = os.fspath(executable)
        self.basedir = os.fspath(basedir) if basedir is not None else None
        self.project = project
        self.cwd = os.fspath(cwd) if cwd is not None else None
        self.env = dict(env) if env is not None else None

    def command(self, *arguments: str, check: bool = True) -> CommandResult:
        """Run an arbitrary rotari subcommand with this client's location."""

        if not arguments:
            raise ValueError("a rotari subcommand is required")
        argv = [
            self.executable,
            arguments[0],
            *self._location_options(),
            *arguments[1:],
        ]
        result = self._invoke(argv)
        if check and result.returncode != 0:
            raise RotariError(result)
        return result

    def add(
        self,
        command: Sequence[str],
        **options: object,
    ) -> CommandResult:
        """Add an executable argument list to the current project queue."""

        arguments = build_command_arguments("add", options)
        arguments += ["--", *command]
        return self.command(*arguments)

    def run(self, **options: object) -> CommandResult:
        """Run the current queue with the supplied CLI options."""

        if options.get("partial_array") is not None:
            options["partial_array"] = str(options["partial_array"]).lower()
        arguments = build_command_arguments("run", options)
        return self.command(*arguments)

    def retry(self, **options: object) -> CommandResult:
        """Retry failed and unfinished jobs using the CLI retry alias."""

        arguments = build_command_arguments("retry", options)
        return self.command(*arguments)

    def reset(self, *, recover: bool = False) -> CommandResult:
        """Clear the current queue, optionally recovering an interrupted run."""

        arguments = build_command_arguments("reset", locals())
        return self.command(*arguments)

    def wait(
        self,
        selector: str | None = None,
        **options: object,
    ) -> dict[str, object]:
        """Wait for a run and return its decoded JSON summary."""

        if selector is not None and options.get("run_id") is not None:
            raise ValueError("selector and run_id cannot be used together")
        options = {**options, "json": True}
        arguments = build_command_arguments("wait", options, [selector] if selector is not None else ())
        result = self.command(*arguments, check=False)
        try:
            summary = result.json()
        except json.JSONDecodeError:
            raise RotariError(result) from None
        if not isinstance(summary, dict):
            raise TypeError("rotari wait --json returned a non-object JSON value")
        return summary

    def show(self, **options: object) -> dict[str, object]:
        """Return the decoded JSON view of a project, run, or job."""

        arguments = build_command_arguments("show", {**options, "json": True})
        result = self.command(*arguments)
        value = result.json()
        if not isinstance(value, dict):
            raise TypeError("rotari show --json returned a non-object JSON value")
        return value

    def _location_options(self) -> list[str]:
        arguments: list[str] = []
        if self.basedir is not None:
            arguments += ["--basedir", self.basedir]
        if self.project is not None:
            arguments += ["--project-name", self.project]
        return arguments

    def _invoke(self, argv: Sequence[str]) -> CommandResult:
        process = subprocess.run(
            list(argv),
            cwd=self.cwd,
            env=self.env,
            capture_output=True,
            text=True,
            check=False,
        )
        return CommandResult(tuple(argv), process.returncode, process.stdout, process.stderr)


_install_cli_signatures()
