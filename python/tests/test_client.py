import json
import inspect
from unittest.mock import patch

from rotari import Rotari, RotariError
from rotari.client import CommandResult


def completed(stdout="", stderr="", returncode=0):
    return type("Completed", (), {
        "stdout": stdout,
        "stderr": stderr,
        "returncode": returncode,
    })()


def test_add_builds_safe_argv_with_location_options():
    client = Rotari("rotari", basedir="state", project="demo")
    with patch("subprocess.run", return_value=completed()) as run:
        client.add(["./train.sh", "--epochs", "3"], job_name="train", env=["GPU=0"])

    assert run.call_args.args[0] == [
        "rotari", "add", "--basedir", "state", "--project-name", "demo",
        "--env", "GPU=0", "--job-name", "train", "--",
        "./train.sh", "--epochs", "3",
    ]
    assert "shell" not in run.call_args.kwargs
    assert run.call_args.kwargs["check"] is False


def test_show_decodes_machine_readable_output():
    payload = {"run_id": "run-1", "summary": {"status": "finished"}}
    with patch("subprocess.run", return_value=completed(json.dumps(payload))):
        result = Rotari(basedir="state").show(run_id="run-1")

    assert result == payload


def test_reset_builds_location_aware_argv():
    with patch("subprocess.run", return_value=completed()) as run:
        Rotari("rotari", basedir="state", project="demo").reset(recover=True)

    assert run.call_args.args[0] == [
        "rotari", "reset", "--basedir", "state", "--project-name", "demo", "--recover",
    ]


def test_run_builds_options_from_schema():
    with patch("subprocess.run", return_value=completed()) as run:
        Rotari("rotari").run(run_id="run-1", async_=True, job_ids=["job-1"])

    assert run.call_args.args[0] == [
        "rotari", "run", "--run-id", "run-1", "--job-id", "job-1", "--async",
    ]


def test_wait_returns_failed_run_summary_instead_of_raising():
    payload = {"run_id": "run-1", "status": "failed", "exit_code": 2}
    with patch("subprocess.run", return_value=completed(json.dumps(payload), returncode=2)):
        result = Rotari().wait("nightly")

    assert result == payload


def test_wait_without_selector_lets_cli_find_the_active_run():
    payload = {"run_id": "run-1", "status": "finished", "exit_code": 0}
    with patch("subprocess.run", return_value=completed(json.dumps(payload))) as run:
        result = Rotari(basedir="state", project="build").wait()

    assert result == payload
    assert run.call_args.args[0] == [
        "rotari", "wait", "--basedir", "state", "--project-name", "build", "--json",
    ]


def test_command_raises_for_cli_errors():
    with patch("subprocess.run", return_value=completed(stderr="bad option", returncode=1)):
        try:
            Rotari().command("show")
        except RotariError as error:
            assert error.result.returncode == 1
        else:
            raise AssertionError("RotariError was not raised")


def test_cli_signatures_are_generated_from_schema():
    run_signature = inspect.signature(Rotari.run)
    add_signature = inspect.signature(Rotari.add)
    wait_signature = inspect.signature(Rotari.wait)

    assert "run_id" in run_signature.parameters
    assert "partial_array" in run_signature.parameters
    assert "executor_options" in add_signature.parameters
    assert "selector" in wait_signature.parameters
