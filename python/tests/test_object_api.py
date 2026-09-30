"""The public object API delegates to the CLI without fetching attributes."""

import inspect
import json
import os
import tempfile
from typing import Any, cast
from unittest.mock import patch

from rotari import Job, Rotari, RotariError, Run
from rotari.client import CommandResult


def response(text="", code=0):
    return type("Completed", (), {"stdout": text, "stderr": "", "returncode": code})()


def client_objects():
    client = Rotari("rotari", basedir="state", project="demo")
    with patch(
        "subprocess.run", return_value=response("added project=demo job_id=job-1\n")
    ):
        job = client.add(["true"], job_name="one")
    with patch("subprocess.run", return_value=response("  Run ID: run-1\n")):
        run = client.run(async_=True)
    return client, job, run


def test_objects_keep_client_without_implicit_cli_calls():
    client, job, run = client_objects()
    with patch("subprocess.run") as invoke:
        assert job.id == "job-1"
        assert run.id == "run-1"
        assert job.name == "one"
        assert run.name is None
        assert job._client is client
        assert run._client is client
        invoke.assert_not_called()


def test_check_returns_json_even_when_not_runnable():
    client = Rotari("rotari", project="demo")
    payload = {"project": "demo", "runnable": False, "queued": 0}
    with patch(
        "subprocess.run", return_value=response(json.dumps(payload), 1)
    ) as invoke:
        assert client.check(deep=True, quiet=True) == payload
    assert invoke.call_args.args[0] == [
        "rotari", "check", "--project-name", "demo", "--json", "--deep"
    ]
    with patch("subprocess.run", return_value=response("", 1)):
        try:
            client.check()
        except RotariError:
            pass
        else:
            raise AssertionError("missing check JSON was accepted")


def test_retry_returns_new_run_and_object_methods_delegate():
    client, job, run = client_objects()
    with patch("subprocess.run", return_value=response("  Run ID: run-2\n")) as invoke:
        retried = client.retry(run_name="fixed", async_=True, quiet=True)
    assert retried.id == "run-2"
    assert retried.run_name == "fixed"
    assert "--quiet" not in invoke.call_args.args[0]
    assert "--quiet=false" in invoke.call_args.args[0]
    with patch(
        "subprocess.run", return_value=response('{"run_id":"run-1"}\n')
    ) as invoke:
        assert run.wait()["run_id"] == "run-1"
    assert invoke.call_args.args[0][-3:] == ["--run-id", "run-1", "--json"]
    with patch("subprocess.run", return_value=response('{"run_id":"run-1"}')) as invoke:
        assert run.show()["run_id"] == "run-1"
    assert invoke.call_args.args[0][-3:] == ["--run-id", "run-1", "--json"]
    with patch("subprocess.run", return_value=response('{"job_id":"job-1"}')) as invoke:
        assert job.show(run=run)["job_id"] == "job-1"
    assert invoke.call_args.args[0][-5:] == [
        "--run-id", "run-1", "--job-id", "job-1", "--json"
    ]


def test_wait_list_order_and_failure_and_single_compatibility():
    client, _, run = client_objects()
    output = "\n".join(
        json.dumps(item)
        for item in (
            {"run_id": "run-1", "exit_code": 2},
            {"run_id": "run-2", "exit_code": 0},
        )
    ) + "\n"
    with patch("subprocess.run", return_value=response(output, 2)) as invoke:
        result = client.wait([run, "run-2"])
    assert [item["run_id"] for item in result] == ["run-1", "run-2"]
    assert invoke.call_args.args[0][-5:] == [
        "--run-id", "run-1", "--run-id", "run-2", "--json"
    ]
    with patch("subprocess.run", return_value=response('{"run_id":"run-2"}')) as invoke:
        assert client.wait("nightly")["run_id"] == "run-2"
    assert invoke.call_args.args[0][-2:] == ["--json", "nightly"]
    with patch("subprocess.run", return_value=response(output.splitlines()[0], 1)):
        try:
            client.wait([run, "run-2"])
        except RotariError:
            pass
        else:
            raise AssertionError("partial wait results were accepted")

    bad_order = '{"run_id":"run-2"}\n{"run_id":"run-1"}\n'
    with patch("subprocess.run", return_value=response(bad_order)):
        try:
            client.wait([run, "run-2"])
        except ValueError:
            pass
        else:
            raise AssertionError("out-of-order wait results were accepted")
    with patch("subprocess.run", return_value=response("no JSON", 1)):
        try:
            client.wait(run)
        except RotariError:
            pass
        else:
            raise AssertionError("invalid wait JSON was accepted")


def test_show_list_uses_latest_run_and_keeps_order():
    client, job, run = client_objects()
    another = Job(
        job.args, 0, job.stdout, job.stderr, "job-2", "two", ("false",), client
    )

    def get_result(argv, **kwargs):
        return response(json.dumps({"job_id": argv[argv.index("--job-id") + 1]}))

    with patch("subprocess.run", side_effect=get_result) as invoke:
        result = client.show([another, job])
    assert [item["job_id"] for item in result] == ["job-2", "job-1"]
    assert all(
        call.args[0][-5:-3] == ["--run-id", "latest"]
        for call in invoke.call_args_list
    )

    def get_run(argv, **kwargs):
        return response(json.dumps({"run_id": argv[argv.index("--run-id") + 1]}))

    other_run = Run(run.args, 0, "", "", "run-2", None, client)
    with patch("subprocess.run", side_effect=get_run):
        assert [item["run_id"] for item in client.show([run, other_run])] == [
            "run-1", "run-2"
        ]


def test_control_run_jobs_and_object_shortcuts():
    client, job, run = client_objects()
    other = Job(job.args, 0, "", "", "job-2", None, ("true",), client)
    cases = [
        (lambda: client.cancel(), "cancel", []),
        (lambda: run.cancel(wait=True), "cancel", ["--wait", "run-1"]),
        (lambda: client.cancel([job, other]), "cancel", ["job-1", "job-2"]),
        (lambda: job.cancel(), "cancel", ["job-1"]),
        (lambda: job.suspend(), "suspend", ["job-1"]),
        (lambda: client.suspend([job, other]), "suspend", ["job-1", "job-2"]),
        (lambda: job.resume(), "resume", ["job-1"]),
        (lambda: client.resume([job, other]), "resume", ["job-1", "job-2"]),
    ]
    for action, command, ending in cases:
        with patch("subprocess.run", return_value=response("ok")) as invoke:
            assert isinstance(action(), CommandResult)
        argv = invoke.call_args.args[0]
        assert argv[:4] == ["rotari", command, "--basedir", "state"]
        assert argv[6:] == ending, (action, argv)

    with patch("subprocess.run", return_value=response("ok")) as invoke:
        client.cancel(job_ids=["job-1", "job-2"])
    assert invoke.call_args.args[0][-4:] == [
        "--job-id", "job-1", "--job-id", "job-2"
    ]
    with patch("subprocess.run", return_value=response("ok")) as invoke:
        client.suspend(stage="train", yes=True)
    assert invoke.call_args.args[0][-3:] == ["--stage", "train", "--yes"]


def test_invalid_targets_never_invoke_cli():
    client, job, run = client_objects()
    foreign = Rotari(project="other")
    missing = Job(job.args, 0, "", "", None, None, ("true",), client)
    cases = [
        (ValueError, lambda: client.wait([])),
        (ValueError, lambda: client.wait(run, run_id="run-2")),
        (TypeError, lambda: client.wait(cast(Any, job))),
        (TypeError, lambda: client.wait(cast(Any, [run, job]))),
        (TypeError, lambda: client.wait(cast(Any, [job, run]))),
        (ValueError, lambda: client.show([])),
        (ValueError, lambda: client.show(cast(Any, [job, run]))),
        (ValueError, lambda: client.show(cast(Any, [run, job]))),
        (ValueError, lambda: client.show(missing)),
        (ValueError, lambda: client.show(run, run=run)),
        (ValueError, lambda: client.show(job, run_id="run-2")),
        (TypeError, lambda: client.cancel(cast(Any, [job, run]))),
        (TypeError, lambda: client.cancel(cast(Any, [run, job]))),
        (ValueError, lambda: client.cancel(job, wait=True)),
        (ValueError, lambda: client.cancel(run, job_ids=["job-2"])),
        (ValueError, lambda: client.cancel(job, stage="train")),
        (ValueError, lambda: client.cancel(missing)),
        (ValueError, lambda: client.cancel([])),
        (ValueError, lambda: client.suspend()),
        (TypeError, lambda: client.suspend(cast(Any, [job, run]))),
        (TypeError, lambda: client.resume(cast(Any, [run, job]))),
        (ValueError, lambda: client.resume(cast(Any, run))),
        (ValueError, lambda: foreign.show(job)),
        (ValueError, lambda: foreign.show([job])),
        (ValueError, lambda: foreign.cancel(run)),
        (ValueError, lambda: foreign.wait(run)),
        (ValueError, lambda: Rotari().cancel()),
    ]
    with patch("subprocess.run") as invoke:
        for expected, action in cases:
            try:
                action()
            except expected:
                pass
            else:
                raise AssertionError(f"{action} did not reject invalid target")
        invoke.assert_not_called()


def test_signatures_expose_target_and_cli_flags():
    for method in (Rotari.cancel, Rotari.suspend, Rotari.resume, Rotari.show):
        assert "target" in inspect.signature(method).parameters
    assert "run" in inspect.signature(Rotari.show).parameters
    assert "deep" in inspect.signature(Rotari.check).parameters


def test_real_cli_job_lifecycle_when_binary_is_available():
    binary = os.environ.get("ROTARI_TEST_BINARY")
    if binary is None:
        return
    with tempfile.TemporaryDirectory() as directory:
        client = Rotari(
            binary,
            basedir=os.path.join(directory, "state"),
            project="demo",
            env={
                **os.environ,
                "ROTARI_MASTERDIR": os.path.join(directory, "registry"),
                "ROTARI_QUIET": "true",
            },
        )
        job = client.add(["sh", "-c", "sleep 10"], job_name="long")
        assert client.check()["runnable"] is True
        run = client.run(async_=True)
        view = job.show(run=run)
        assert view["job_id"] == job.id
        jobs = view["jobs"]
        assert isinstance(jobs, list)
        assert jobs[0]["job"]["id"] == job.id
        job.cancel()
        summary = run.wait(timeout="15s")
        assert summary["run_id"] == run.id
        assert client.show(job)["run_id"] == run.id
        # The summary can appear just before the supervisor releases the
        # project's active-run lock. Retry only that transition, boundedly.
        for _ in range(30):
            try:
                new_job = client.add(["true"])
                break
            except RotariError as error:
                if "is running" not in str(error):
                    raise
        else:
            raise AssertionError("run stayed active after writing its summary")
        try:
            new_job.show()
        except RotariError:
            pass
        else:
            raise AssertionError("latest run silently fell back to the queue")
        try:
            client.show(new_job, run=run)
        except RotariError:
            pass
        else:
            raise AssertionError("run view accepted a job absent from the run")