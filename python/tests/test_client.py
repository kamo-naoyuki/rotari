import inspect
import json
from unittest.mock import patch

import pytest
from rotari import Job, Rotari, RotariError, Run


def completed(stdout="", stderr="", returncode=0):
    return type(
        "Completed",
        (),
        {
            "stdout": stdout,
            "stderr": stderr,
            "returncode": returncode,
        },
    )()


@pytest.mark.parametrize("basedir", [None, "state"])
@pytest.mark.parametrize("project", [None, "demo"])
@pytest.mark.parametrize(
    ("command", "supports_basedir", "supports_project"),
    [
        ("basedirs", False, False),
        ("projects", True, False),
        ("runs", True, True),
        ("show", True, True),
    ],
)
def test_command_injects_only_supported_location_options(
    command, supports_basedir, supports_project, basedir, project
):
    client = Rotari("rotari", basedir=basedir, project=project)
    expected = ["rotari", command]
    if supports_basedir and basedir is not None:
        expected += ["--basedir", basedir]
    if supports_project and project is not None:
        expected += ["--project-name", project]
    with patch("subprocess.run", return_value=completed("output")) as run:
        result = client.command(command)

    assert run.call_args.args[0] == expected
    assert result.args == tuple(expected)
    assert result.stdout == "output"


@pytest.mark.parametrize("command", ["projects", "runs", "show"])
@pytest.mark.parametrize("equals", [False, True])
def test_command_preserves_explicit_location_overrides(command, equals):
    overrides = ["--basedir=other-state"] if equals else ["--basedir", "other-state"]
    if command != "projects":
        overrides += ["--project-name=other"] if equals else ["--project-name", "other"]
    with patch("subprocess.run", return_value=completed()) as run:
        Rotari("rotari", basedir="state", project="demo").command(command, *overrides)

    assert run.call_args.args[0] == ["rotari", command, *overrides]


@pytest.mark.parametrize(
    ("explicit", "defaults"),
    [
        (["--basedir", "other"], ["--project-name", "demo"]),
        (["--project-name=other"], ["--basedir", "state"]),
        (["-basedir=other"], ["--project-name", "demo"]),
        (["-b", "other"], ["--project-name", "demo"]),
        (["-p", "other"], ["--basedir", "state"]),
        (["basedir"], ["--basedir", "state", "--project-name", "demo"]),
        (["--", "--basedir=other"], ["--basedir", "state", "--project-name", "demo"]),
    ],
)
def test_command_preserves_location_defaults_not_overridden(explicit, defaults):
    with patch("subprocess.run", return_value=completed()) as run:
        Rotari(basedir="state", project="demo").command("show", *explicit)

    assert run.call_args.args[0] == ["rotari", "show", *defaults, *explicit]


@pytest.mark.parametrize(
    "arguments",
    [
        ["add", "--job-name", "--basedir", "true"],
        ["add", "--job-name", "-p", "true"],
        ["add", "echo", "--basedir", "other"],
        ["change", "--job-name", "--project-name", "echo", "-b", "other"],
        ["show", "--stream", "--basedir"],
    ],
)
def test_command_does_not_treat_option_values_or_job_arguments_as_locations(arguments):
    with patch("subprocess.run", return_value=completed()) as run:
        Rotari(basedir="state", project="demo").command(*arguments)
    assert run.call_args.args[0] == [
        "rotari",
        arguments[0],
        "--basedir",
        "state",
        "--project-name",
        "demo",
        *arguments[1:],
    ]


def test_add_builds_safe_argv_with_location_options():
    client = Rotari("rotari", basedir="state", project="demo")
    output = (
        "added project=demo job_id=job-1 job_name=train "
        "command=[./train.sh --epochs 3]\n"
    )
    with patch("subprocess.run", return_value=completed(output)) as run:
        job = client.add(
            ["./train.sh", "--epochs", "3"], job_name="train", env=["GPU=0"]
        )

    assert isinstance(job, Job)
    assert job.id == job.job_id == "job-1"
    assert job.name == job.job_name == "train"
    assert job.command == ("./train.sh", "--epochs", "3")
    assert job.args == tuple(run.call_args.args[0])
    assert job.returncode == 0

    assert run.call_args.args[0] == [
        "rotari",
        "add",
        "--basedir",
        "state",
        "--project-name",
        "demo",
        "--quiet=false",
        "--env",
        "GPU=0",
        "--job-name",
        "train",
        "--",
        "./train.sh",
        "--epochs",
        "3",
    ]
    assert "shell" not in run.call_args.kwargs
    assert run.call_args.kwargs["check"] is False


def test_show_decodes_machine_readable_output():
    payload = {"run_id": "run-1", "summary": {"status": "finished"}}
    with patch("subprocess.run", return_value=completed(json.dumps(payload))):
        result = Rotari(basedir="state").show(run_id="run-1")

    assert result == payload


def test_export_can_decode_a_manifest_without_writing_a_file():
    payload = {"version": 1, "jobs": [{"name": "train", "command": ["./train.sh"]}]}
    with patch("subprocess.run", return_value=completed(json.dumps(payload))) as run:
        result = Rotari(basedir="state", project="demo").export(as_dict=True)

    assert result == payload
    assert run.call_args.args[0] == [
        "rotari",
        "export",
        "--basedir",
        "state",
        "--project-name",
        "demo",
        "--format",
        "json",
    ]


def test_import_accepts_a_manifest_dict_without_writing_a_file():
    manifest = {"version": 1, "jobs": [{"name": "train", "command": ["true"]}]}
    with patch("subprocess.run", return_value=completed()) as run:
        Rotari(basedir="state", project="demo").import_(
            manifest, overwrite=True, dry_run=True
        )

    assert run.call_args.args[0] == [
        "rotari",
        "import",
        "--basedir",
        "state",
        "--project-name",
        "demo",
        "--overwrite",
        "--dry-run",
        "-",
    ]
    assert json.loads(run.call_args.kwargs["input"]) == manifest


def test_reset_builds_location_aware_argv():
    with patch("subprocess.run", return_value=completed()) as run:
        Rotari("rotari", basedir="state", project="demo").reset()

    assert run.call_args.args[0] == [
        "rotari",
        "reset",
        "--basedir",
        "state",
        "--project-name",
        "demo",
    ]


def test_unlock_builds_location_aware_argv():
    with patch("subprocess.run", return_value=completed()) as run:
        Rotari("rotari", basedir="state", project="demo").unlock(run_id="run-1")

    assert run.call_args.args[0] == [
        "rotari",
        "unlock",
        "--basedir",
        "state",
        "--project-name",
        "demo",
        "--run-id",
        "run-1",
    ]


def test_run_builds_options_from_schema():
    output = (
        "=== Run started ===\n  Project: demo\n  Run: nightly (run-2)\n"
        "\nCheck status:\n  rotari show --run-id run-2\n"
    )
    with patch("subprocess.run", return_value=completed(output)) as run:
        result = Rotari("rotari").run(
            run_id="run-1", run_name="nightly", async_=True, job_ids=["job-1"]
        )

    assert isinstance(result, Run)
    assert result.id == result.run_id == "run-2"
    assert result.name == result.run_name == "nightly"
    assert result.args == tuple(run.call_args.args[0])

    assert run.call_args.args[0] == [
        "rotari",
        "run",
        "--quiet=false",
        "--run-id",
        "run-1",
        "--run-name",
        "nightly",
        "--job-id",
        "job-1",
        "--async",
    ]


def test_run_passes_false_for_boolean_value_flag():
    with patch("subprocess.run", return_value=completed("  Run ID: run-2\n")) as run:
        Rotari().run(partial_array=False)

    argv = run.call_args.args[0]
    assert "--partial-array=false" in argv
    assert "false" not in argv


def test_unknown_options_are_rejected_before_invoking_cli():
    client = Rotari()
    with patch("subprocess.run") as run:
        with pytest.raises(TypeError, match="job-nam"):
            client.add(["true"], job_nam="train")
        run.assert_not_called()


def test_sync_run_uses_new_run_id_from_progress_not_source_run():
    output = (
        "=== Run started ===\n  Project: demo\n  Run ID: run-2\n"
        "=== Run finished ===\n  Run: nightly (run-2)\n"
    )
    with patch("subprocess.run", return_value=completed(output)):
        result = Rotari().run(run_id="run-1", run_name="nightly")

    assert result.run_id == "run-2"
    assert result.stdout == output


def test_quiet_does_not_hide_ids_and_matrix_does_not_claim_one_id():
    with patch(
        "subprocess.run",
        return_value=completed("added project=demo jobs=2 command=[true]\n"),
    ) as run:
        job = Rotari().add(["true"], job_name="train", matrix=["SEED=1,2"], quiet=True)
    assert job.id is None
    assert job.name is None
    assert job.command == ("true",)
    assert "--quiet" not in run.call_args.args[0]

    output = (
        "=== Run started ===\n  Run: run-3\n"
        "\nCheck status:\n  rotari show --run-id run-3\n"
    )
    with patch("subprocess.run", return_value=completed(output)) as run:
        result = Rotari().run(async_=True, quiet=True)
    assert result.run_id == "run-3"
    assert "--quiet" not in run.call_args.args[0]


def test_failed_run_exposes_started_id_on_error_result():
    client = Rotari()
    with patch(
        "subprocess.run",
        return_value=completed("=== Run started ===\n  Run ID: run-3\n", returncode=1),
    ):
        with pytest.raises(RotariError) as error:
            client.run()
    assert error.value.result.run_id == "run-3"


def test_missing_ids_do_not_silently_produce_unidentified_objects():
    client = Rotari()
    with patch("subprocess.run", return_value=completed("no job ID")):
        with pytest.raises(ValueError, match="job ID"):
            client.add(["true"])

    with patch("subprocess.run", return_value=completed("no run ID")):
        with pytest.raises(ValueError, match="run ID"):
            client.run()


def test_wait_returns_failed_run_summary_instead_of_raising():
    payload = {"run_id": "run-1", "status": "failed", "exit_code": 2}
    with patch(
        "subprocess.run", return_value=completed(json.dumps(payload), returncode=2)
    ):
        result = Rotari().wait("nightly")

    assert result == payload


def test_wait_until_failure_returns_the_running_runs_failures():
    payload = {
        "run_id": "run-1",
        "status": "running",
        "failures": [{"kind": "timeout", "cause": "timeout", "count": 1}],
    }
    with patch(
        "subprocess.run", return_value=completed(json.dumps(payload), returncode=1)
    ) as run:
        result = Rotari().wait("run-1", until_failure=True)

    assert result == payload
    assert run.call_args.args[0] == [
        "rotari",
        "wait",
        "--until-failure",
        "--json",
        "run-1",
    ]


def test_wait_without_selector_lets_cli_find_the_active_run():
    payload = {"run_id": "run-1", "status": "finished", "exit_code": 0}
    with patch("subprocess.run", return_value=completed(json.dumps(payload))) as run:
        result = Rotari(basedir="state", project="build").wait()

    assert result == payload
    assert run.call_args.args[0] == [
        "rotari",
        "wait",
        "--basedir",
        "state",
        "--project-name",
        "build",
        "--json",
    ]


def test_command_raises_for_cli_errors():
    client = Rotari()
    with patch(
        "subprocess.run", return_value=completed(stderr="bad option", returncode=1)
    ):
        with pytest.raises(RotariError) as error:
            client.command("show")
    assert error.value.result.returncode == 1


def test_cli_signatures_are_generated_from_schema():
    run_signature = inspect.signature(Rotari.run)
    add_signature = inspect.signature(Rotari.add)
    wait_signature = inspect.signature(Rotari.wait)
    export_signature = inspect.signature(Rotari.export)

    assert "run_id" in run_signature.parameters
    assert "partial_array" in run_signature.parameters
    assert "executor_options" in add_signature.parameters
    assert "selector" in wait_signature.parameters
    assert "target" in export_signature.parameters
    assert "as_dict" in export_signature.parameters
    assert "manifest" in inspect.signature(Rotari.import_).parameters
