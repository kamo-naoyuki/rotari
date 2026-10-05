from __future__ import annotations

import inspect
import json
import os
import re
import subprocess
from collections.abc import Mapping, Sequence
from dataclasses import dataclass, field
from typing import overload

from .generated_cli import CLI_SCHEMA

_REQUIRE_OUTPUT = "--quiet=false"


def _cli_option_name(name: str) -> str:
    if name == "async_":
        return "async"
    if name.endswith("_ids"):
        return name[:-1].replace("_", "-")
    if name == "executor_options":
        return "executor-option"
    return name.replace("_", "-")


def _append_flag_arguments(
    arguments: list[str], name: str, flag: Mapping[str, object], value: object
) -> None:
    prefix = f"--{name}"
    values = (
        value
        if flag.get("repeated")
        and isinstance(value, Sequence)
        and not isinstance(value, (str, bytes))
        else (value,)
    )
    for item in values:
        if isinstance(item, bool):
            if item:
                arguments.append(prefix)
        elif flag.get("value_name"):
            arguments.extend((prefix, str(item)))
        else:
            arguments.append(f"{prefix}={item}")


def build_command_arguments(
    command: str,
    options: Mapping[str, object] | None = None,
    positional: Sequence[str] = (),
) -> list[str]:
    """Build command argv from the generated CLI schema."""
    command_spec = next(
        (item for item in CLI_SCHEMA["commands"] if item["name"] == command), None
    )
    if command_spec is None:
        raise ValueError(f"unknown CLI command: {command}")
    flags = {item["name"]: item for item in command_spec.get("flags", ())}
    provided = {
        _cli_option_name(option): value for option, value in (options or {}).items()
    }
    unknown = sorted(provided.keys() - flags.keys())
    if unknown:
        names = ", ".join(unknown)
        raise TypeError(f"unknown option(s) for rotari {command}: {names}")
    arguments = [command]
    for name, flag in flags.items():
        value = provided.get(name)
        if value is not None:
            _append_flag_arguments(arguments, name, flag, value)
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


def _include_signature_flag(command: str, flag: Mapping[str, object]) -> bool:
    if flag["name"] in {"basedir", "project-name"}:
        return False
    return not (flag["name"] == "json" and command in {"check", "wait", "show"})


def _signature_flag(flag: Mapping[str, object]) -> inspect.Parameter:
    name = _python_option_name(str(flag["name"]))
    if flag.get("repeated"):
        default: object = ()
    elif flag.get("value_name"):
        default = None
    else:
        default = False
    return inspect.Parameter(name, inspect.Parameter.KEYWORD_ONLY, default=default)


def _install_cli_signatures() -> None:
    for command_spec in CLI_SCHEMA["commands"]:
        command = command_spec["name"]
        method = getattr(Rotari, "import_" if command == "import" else command, None)
        if method is None:
            continue
        parameters = [
            *_signature_positionals(command),
            *(
                _signature_flag(flag)
                for flag in command_spec.get("flags", ())
                if _include_signature_flag(command, flag)
            ),
        ]
        method.__signature__ = inspect.Signature(parameters)


def _signature_positionals(command: str) -> list[inspect.Parameter]:
    parameters = [inspect.Parameter("self", inspect.Parameter.POSITIONAL_OR_KEYWORD)]
    if command in {"add", "import"}:
        name = "command" if command == "add" else "manifest"
        parameters.append(
            inspect.Parameter(name, inspect.Parameter.POSITIONAL_OR_KEYWORD)
        )
    elif command in {"wait", "show", "cancel", "suspend", "resume", "export"}:
        name = "selector" if command == "wait" else "target"
        parameters.append(
            inspect.Parameter(
                name, inspect.Parameter.POSITIONAL_OR_KEYWORD, default=None
            )
        )
    if command == "show":
        parameters.append(
            inspect.Parameter("run", inspect.Parameter.KEYWORD_ONLY, default=None)
        )
    if command == "export":
        parameters.append(
            inspect.Parameter("as_dict", inspect.Parameter.KEYWORD_ONLY, default=False)
        )
    return parameters


def _reject_managed_json_option(command: str, options: Mapping[str, object]) -> None:
    if "json" in options:
        raise TypeError(f"{command}() always returns JSON; do not pass json=")


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
        if match:
            return match.group(1)
        match = re.search(r"(?m)^ {2}Run ID: ([^\s]+)$", self.stdout)
        if match:
            return match.group(1)
        match = re.search(r"(?m)^ {2}rotari show --run-id ([^\s]+)$", self.stdout)
        if match:
            return match.group(1)
        match = re.search(r"(?m)^ {2}Run: ([^\s(]+)$", self.stdout)
        return match.group(1) if match else None

    def json(self) -> object:
        """Decode the first JSON value printed by the command."""

        return json.loads(self.stdout)


@dataclass(frozen=True)
class Job(CommandResult):
    """A queued command returned by add; matrix additions have no single ID."""

    id: str | None
    name: str | None
    command: tuple[str, ...]
    _client: Rotari | None = field(default=None, repr=False, compare=False)

    @property
    def job_id(self) -> str | None:
        return self.id

    @property
    def job_name(self) -> str | None:
        return self.name

    def show(self, *, run: Run | str | None = None) -> dict[str, object]:
        return self._project().show(self, run=run)

    def cancel(self) -> CommandResult:
        return self._project().cancel(self)

    def suspend(self) -> CommandResult:
        return self._project().suspend(self)

    def resume(self) -> CommandResult:
        return self._project().resume(self)

    def _project(self) -> Rotari:
        if self._client is None:
            raise ValueError("job is not bound to a Rotari client")
        return self._client


@dataclass(frozen=True)
class Run(CommandResult):
    """A newly started run, whether synchronous or asynchronous."""

    id: str
    name: str | None
    _client: Rotari | None = field(default=None, repr=False, compare=False)

    @property
    def run_id(self) -> str:
        return self.id

    @property
    def run_name(self) -> str | None:
        return self.name

    def wait(self, **options: object) -> dict[str, object]:
        return self._project().wait(self, **options)

    def show(self) -> dict[str, object]:
        return self._project().show(self)

    def cancel(self, *, wait: bool = False) -> CommandResult:
        return self._project().cancel(self, wait=wait)

    def _project(self) -> Rotari:
        if self._client is None:
            raise ValueError("run is not bound to a Rotari client")
        return self._client


class RotariError(RuntimeError):
    """A rotari command could not be completed."""

    def __init__(self, result: CommandResult):
        self.result = result
        message = (
            result.stderr.strip() or result.stdout.strip() or "rotari command failed"
        )
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

    def command(
        self,
        *arguments: str,
        check: bool = True,
        input_data: str | None = None,
    ) -> CommandResult:
        """Run an arbitrary rotari subcommand with this client's location."""

        if not arguments:
            raise ValueError("a rotari subcommand is required")
        argv = [
            self.executable,
            arguments[0],
            *self._location_options(),
            *arguments[1:],
        ]
        result = self._invoke(argv, input_data=input_data)
        if check and result.returncode != 0:
            raise RotariError(result)
        return result

    def add(
        self,
        command: Sequence[str],
        **options: object,
    ) -> Job:
        """Add an executable argument list to the current project queue."""

        # The subprocess output is captured, so CLI quiet mode would only
        # hide the ID that add needs to return.
        arguments = build_command_arguments("add", {**options, "quiet": False})
        arguments.insert(1, _REQUIRE_OUTPUT)
        arguments += ["--", *command]
        result = self.command(*arguments)
        # CLI add prints one ID for a single job, but only a count for a matrix.
        match = re.search(r"\bjob_id=([^\s]+)", result.stdout)
        if match is None and not options.get("matrix"):
            raise ValueError("rotari add did not report a job ID")
        name = options.get("job_name") if not options.get("matrix") else None
        return Job(
            result.args,
            result.returncode,
            result.stdout,
            result.stderr,
            id=match.group(1) if match else None,
            name=str(name) if name is not None else None,
            command=tuple(command),
            _client=self,
        )

    def run(self, **options: object) -> Run:
        """Run the current queue with the supplied CLI options."""

        if options.get("partial_array") is not None:
            options["partial_array"] = str(options["partial_array"]).lower()
        # Captured CLI output is the only source of the newly allocated run ID.
        # Do not suppress it, even if the caller requests quiet mode.
        arguments = build_command_arguments("run", {**options, "quiet": False})
        arguments.insert(1, _REQUIRE_OUTPUT)
        result = self.command(*arguments)
        run_id = result.run_id
        if run_id is None:
            raise ValueError("rotari run did not report a run ID")
        name = options.get("run_name")
        return Run(
            result.args,
            result.returncode,
            result.stdout,
            result.stderr,
            id=run_id,
            name=str(name) if name is not None else None,
            _client=self,
        )

    def retry(self, **options: object) -> Run:
        """Retry failed and unfinished jobs using the CLI retry alias."""

        arguments = build_command_arguments("retry", {**options, "quiet": False})
        arguments.insert(1, _REQUIRE_OUTPUT)
        result = self.command(*arguments)
        run_id = result.run_id
        if run_id is None:
            raise ValueError("rotari retry did not report a run ID")
        name = options.get("run_name")
        return Run(
            result.args,
            result.returncode,
            result.stdout,
            result.stderr,
            id=run_id,
            name=str(name) if name is not None else None,
            _client=self,
        )

    def export(
        self,
        target: str | None = None,
        *,
        as_dict: bool = False,
        **options: object,
    ) -> CommandResult | dict[str, object]:
        """Export a workflow manifest, optionally returning it as a dict.

        ``as_dict=True`` selects the CLI's JSON format and decodes its stdout,
        so no manifest file is created.
        """

        if as_dict:
            options = {**options, "format": "json"}
        arguments = build_command_arguments(
            "export", options, [target] if target is not None else ()
        )
        result = self.command(*arguments)
        if not as_dict:
            return result
        value = result.json()
        if not isinstance(value, dict):
            raise TypeError(
                "rotari export --format json returned a non-object JSON value"
            )
        return value

    def import_(
        self,
        manifest: Mapping[str, object],
        **options: object,
    ) -> CommandResult:
        """Import a workflow manifest dict without creating a manifest file."""

        arguments = build_command_arguments("import", options, ["-"])
        return self.command(*arguments, input_data=json.dumps(manifest))

    def reset(self) -> CommandResult:
        """Clear queued jobs for the next run without changing run state."""

        arguments = build_command_arguments("reset", {})
        return self.command(*arguments)

    def unlock(self, *, run_id: str | None = None) -> CommandResult:
        """Recover an interrupted run, keeping the queue.

        Confirm first that the run's jobs have stopped. Rerun its failed and
        unfinished jobs afterwards with ``retry(run_id=...)``.
        """

        arguments = build_command_arguments("unlock", {"run_id": run_id})
        return self.command(*arguments)

    def check(self, **options: object) -> dict[str, object]:
        """Return the CLI project readiness report, including non-runnable states."""

        _reject_managed_json_option("check", options)
        arguments = build_command_arguments(
            "check", {**options, "json": True, "quiet": False}
        )
        result = self.command(*arguments, check=False)
        return self._decode_object(result, "check")

    @overload
    def wait(
        self, selector: Run | str | None = None, **options: object
    ) -> dict[str, object]: ...

    @overload
    def wait(
        self, selector: Sequence[Run | str], **options: object
    ) -> list[dict[str, object]]: ...

    def wait(
        self,
        selector: Run | str | Sequence[Run | str] | None = None,
        **options: object,
    ) -> dict[str, object] | list[dict[str, object]]:
        """Wait for one or more runs; return summaries in selector order."""

        _reject_managed_json_option("wait", options)
        if selector is not None and options.get("run_id") is not None:
            raise ValueError("selector and run_id cannot be used together")
        many, ids = self._wait_targets(selector)
        if many or isinstance(selector, Run):
            options = {**options, "run_id": ids, "json": True}
            positional: Sequence[str] = ()
        else:
            options = {**options, "json": True}
            positional = ids
        arguments = build_command_arguments("wait", options, positional)
        result = self.command(*arguments, check=False)
        summaries = self._wait_summaries(result, ids, many)
        return summaries if many else summaries[0]

    def _wait_targets(
        self, selector: Run | str | Sequence[Run | str] | None
    ) -> tuple[bool, list[str]]:
        many = isinstance(selector, Sequence) and not isinstance(selector, str)
        if many and not selector:
            raise ValueError("wait requires at least one run")
        if isinstance(selector, (Run, str)):
            targets: list[Run | str] = [selector]
        elif selector is None:
            targets = []
        elif isinstance(selector, Sequence):
            targets = list(selector)
        else:
            raise TypeError("wait targets must be Run objects or non-empty strings")
        ids = []
        for target in targets:
            if isinstance(target, Run):
                self._require_owner(target)
                ids.append(target.id)
            elif isinstance(target, str) and target:
                ids.append(target)
            else:
                raise TypeError("wait targets must be Run objects or non-empty strings")
        return many, ids

    @staticmethod
    def _wait_summaries(
        result: CommandResult, ids: list[str], many: bool
    ) -> list[dict[str, object]]:
        try:
            summaries = [
                json.loads(line) for line in result.stdout.splitlines() if line.strip()
            ]
        except json.JSONDecodeError:
            raise RotariError(result) from None
        if not summaries or (many and len(summaries) != len(ids)):
            raise RotariError(result)
        if not all(isinstance(item, dict) for item in summaries):
            raise TypeError("rotari wait --json returned a non-object JSON value")
        if many:
            for expected, summary in zip(ids, summaries, strict=True):
                if summary.get("run_id") != expected:
                    raise ValueError("rotari wait returned runs out of order")
        return summaries

    @overload
    def show(
        self,
        target: Run | Job | None = None,
        *,
        run: Run | str | None = None,
        **options: object,
    ) -> dict[str, object]: ...

    @overload
    def show(
        self,
        target: Sequence[Run] | Sequence[Job],
        *,
        run: Run | str | None = None,
        **options: object,
    ) -> list[dict[str, object]]: ...

    def show(
        self,
        target: Run | Job | Sequence[Run] | Sequence[Job] | None = None,
        *,
        run: Run | str | None = None,
        **options: object,
    ) -> dict[str, object] | list[dict[str, object]]:
        """Query the CLI for a run or a job in the latest saved run."""

        _reject_managed_json_option("show", options)
        if target is None:
            if run is not None:
                raise ValueError("run requires a job target")
            return self._show_one(options)
        if isinstance(target, (Run, Job)):
            return self._show_target(target, run, options)
        if isinstance(target, (str, bytes)) or not isinstance(target, Sequence):
            raise TypeError("show target must be a Job, Run, or a list of them")
        targets = list(target)
        if not targets:
            raise ValueError("show requires at least one target")
        if len({type(item) for item in targets}) != 1:
            raise ValueError("show targets must all be jobs or all be runs")
        return [self._show_target(item, run, options) for item in targets]

    def _show_target(
        self, target: Run | Job, run: Run | str | None, options: Mapping[str, object]
    ) -> dict[str, object]:
        self._require_owner(target)
        if isinstance(target, Run):
            if run is not None or options.get("run_id") or options.get("job_ids"):
                raise ValueError(
                    "a run target cannot be combined with another selector"
                )
            return self._show_one({**options, "run_id": target.id})
        if not isinstance(target, Job):
            raise TypeError("show target must be a Job or Run")
        if target.id is None:
            raise ValueError("job has no single ID (matrix addition)")
        if options.get("run_id") or options.get("job_ids"):
            raise ValueError("use run= to select the run for a job")
        if isinstance(run, Run):
            self._require_owner(run)
            run_id = run.id
        elif run is None:
            run_id = "latest"
        elif isinstance(run, str) and run:
            run_id = run
        else:
            raise TypeError("run must be a Run or non-empty run ID")
        return self._show_one({**options, "run_id": run_id, "job_ids": target.id})

    def _show_one(self, options: Mapping[str, object]) -> dict[str, object]:
        arguments = build_command_arguments("show", {**options, "json": True})
        result = self.command(*arguments)
        return self._decode_object(result, "show")

    @staticmethod
    def _decode_object(result: CommandResult, command: str) -> dict[str, object]:
        try:
            value = result.json()
        except json.JSONDecodeError:
            raise RotariError(result) from None
        if not isinstance(value, dict):
            raise TypeError(f"rotari {command} --json returned a non-object JSON value")
        return value

    def _require_owner(self, target: Run | Job) -> None:
        owner = target._client
        if owner is not None and (
            owner.executable != self.executable
            or owner.basedir != self.basedir
            or owner.project != self.project
            or owner.cwd != self.cwd
            or owner.env != self.env
        ):
            raise ValueError("target belongs to a different Rotari project context")

    def cancel(
        self,
        target: Run | Job | str | Sequence[Job | str] | None = None,
        **options: object,
    ) -> CommandResult:
        """Cancel the project's active run or selected active jobs."""

        return self._control("cancel", target, options)

    def suspend(
        self, target: Job | str | Sequence[Job | str] | None = None, **options: object
    ) -> CommandResult:
        """Suspend selected running jobs."""

        return self._control("suspend", target, options)

    def resume(
        self, target: Job | str | Sequence[Job | str] | None = None, **options: object
    ) -> CommandResult:
        """Resume selected suspended jobs."""

        return self._control("resume", target, options)

    def _control(
        self, operation: str, target: object, options: Mapping[str, object]
    ) -> CommandResult:
        positional = self._control_targets(operation, target, options)
        if (
            operation == "cancel"
            and positional
            and not isinstance(target, Run)
            and options.get("wait")
        ):
            raise ValueError("wait is only available for whole-run cancellation")
        arguments = build_command_arguments(operation, options, positional)
        return self.command(*arguments)

    @staticmethod
    def _has_job_selection(options: Mapping[str, object]) -> bool:
        job_selectors = ("job_ids", "job_name", "stage", "matrix", "filter_state")
        return any(options.get(key) for key in job_selectors) or any(
            key.startswith("filter_") and value for key, value in options.items()
        )

    def _control_targets(
        self, operation: str, target: object, options: Mapping[str, object]
    ) -> list[str]:
        if isinstance(target, Run):
            if operation != "cancel" or self._has_job_selection(options):
                raise ValueError("run selection only supports whole-run cancellation")
            self._require_owner(target)
            return [target.id]
        if target is None:
            if (
                operation == "cancel"
                and self.project is None
                and not options.get("job_ids")
            ):
                raise ValueError("cancel() requires a project or explicit target")
            if operation != "cancel" and not self._has_job_selection(options):
                raise ValueError(f"{operation} requires a job selection")
            return []
        if self._has_job_selection(options):
            raise ValueError("target cannot be combined with another job selector")
        return self._job_ids(operation, target)

    def _job_ids(self, operation: str, target: object) -> list[str]:
        if isinstance(target, Sequence) and not isinstance(target, (str, bytes)):
            if not target:
                raise ValueError(f"{operation} requires at least one job")
            targets = target
        else:
            targets = [target]
        ids = []
        for item in targets:
            if isinstance(item, Job):
                self._require_owner(item)
                if item.id is None:
                    raise ValueError("job has no single ID (matrix addition)")
                ids.append(item.id)
            elif isinstance(item, str) and item:
                ids.append(item)
            else:
                raise TypeError("job control targets must be Job objects or IDs")
        return ids

    def _location_options(self) -> list[str]:
        arguments: list[str] = []
        if self.basedir is not None:
            arguments += ["--basedir", self.basedir]
        if self.project is not None:
            arguments += ["--project-name", self.project]
        return arguments

    def _invoke(
        self, argv: Sequence[str], *, input_data: str | None = None
    ) -> CommandResult:
        process = subprocess.run(
            list(argv),
            cwd=self.cwd,
            env=self.env,
            capture_output=True,
            text=True,
            input=input_data,
            check=False,
        )
        return CommandResult(
            tuple(argv), process.returncode, process.stdout, process.stderr
        )


_install_cli_signatures()
