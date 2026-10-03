#!/usr/bin/env python3
"""Drive `rotari mcp` over stdio like an MCP-only agent and print response sizes.

Usage: XDG_STATE_HOME=DIR/xdg agent-trial-mcp.py PATH_TO_ROTARI
[read|write|control], after agent-trial-fixture.sh DIR. The read scenario
triages and compares runs (agent-trial-2026-10-03-m4.md). The write scenario,
run from DIR/work on a fresh fixture, fixes labA and reruns it
(agent-trial-2026-10-03-m6.md). The control scenario, also from DIR/work on a
fresh fixture, cancels a hanging job and recovers a run whose supervisor was
killed (agent-trial-2026-10-03-m7.md).
"""

import json
import re
import subprocess
import sys
import time

proc = subprocess.Popen(
    [sys.argv[1], "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True
)
next_id = 0


def request(method, params=None, notify=False):
    global next_id
    message = {"jsonrpc": "2.0", "method": method}
    if params is not None:
        message["params"] = params
    if not notify:
        next_id += 1
        message["id"] = next_id
    proc.stdin.write(json.dumps(message) + "\n")
    proc.stdin.flush()
    if notify:
        return None
    while True:
        reply = json.loads(proc.stdout.readline())
        if reply.get("id") == next_id:
            return reply


def call(name, arguments, quiet=False):
    reply = request("tools/call", {"name": name, "arguments": arguments})
    result = reply["result"]
    text = json.dumps(result.get("structuredContent", result.get("content")))
    if result.get("isError"):
        sys.exit(f"== {name}: {text}")
    if not quiet:
        shown = re.sub(r'"manifest": "[^"]*"', '"manifest": ...', json.dumps(arguments))
        print(f"== {name} {shown} -> {len(text)} bytes")
    return result.get("structuredContent") or {}


def read_scenario():
    """Triage the worst run, inspect one job, and compare labA's runs."""
    projects = call("rotari_list_projects", {})
    for p in projects["projects"]:
        print(
            "  ",
            p["basedir_name"],
            p["project"],
            p.get("last_run_id"),
            p.get("last_run_status"),
            p.get("last_run_failed", 0),
            "/",
            p.get("last_run_jobs", 0),
        )
    worst = max(projects["projects"], key=lambda p: p.get("last_run_failed", 0))
    summary = call("rotari_run_summary", {"run_id": worst["last_run_id"]})
    for group in summary["summary"].get("failures", []):
        print(
            "  ",
            group["count"],
            group["cause"],
            "e.g.",
            group["example"].get("evidence"),
            "attempt",
            group["example"].get("attempt_id"),
        )
    group = summary["summary"]["failures"][0]
    first_job = group["jobs"][0]["id"]
    call("rotari_get_job_info", {"run_id": worst["last_run_id"], "job_id": first_job})
    lab = [p for p in projects["projects"] if p["basedir_name"] == "labA"][0]
    comparison = call("rotari_compare_runs", {"run_id": lab["last_run_id"]})
    s = comparison["comparison"]["summary"]
    print(
        "   fixed",
        s["fixed"],
        "still failing",
        s["still_failing"],
        "cause changed",
        s["cause_changed"],
    )
    for job in comparison["comparison"]["jobs"]:
        if job["transition"] == "still_failing":
            print("  ", job["name"], job.get("from_cause"), "->", job.get("to_cause"))
    check = call(
        "rotari_check_project",
        {"basedir_ref": lab["basedir_ref"], "project": lab["project"]},
    )
    print(
        "  ",
        check.get("state"),
        "runnable",
        check.get("runnable"),
        "queued",
        check.get("queued"),
    )


def write_scenario():
    """Fix labA's failures, rerun it, and follow and compare the new run."""
    projects = call("rotari_list_projects", {})
    lab = [p for p in projects["projects"] if p["basedir_name"] == "labA"][0]
    target = {"basedir_ref": lab["basedir_ref"], "project": lab["project"]}
    rerun = dict(target, retry=True)
    summary = call("rotari_run_summary", {"run_id": lab["last_run_id"]})
    print("   state", summary["state"])
    for group in summary["summary"]["failures"]:
        print("  ", group["count"], group["cause"], [j["name"] for j in group["jobs"]])
    # The agent fixes the OOM in train.sh with its own file tools, and gives
    # the job that timed out more time by editing the exported manifest.
    with open("train.sh") as script:
        text = script.read()
    with open("train.sh", "w") as script:
        script.write(text.replace("  3|7|11)", "  99)"))
    exported = call("rotari_export_run", {"run_id": lab["last_run_id"]})
    manifest = exported["manifest"].replace("timeout: 5s", "timeout: 60s")
    edit = dict(target, manifest=manifest, overwrite=True)
    plan = call("rotari_preview_import", edit)
    call("rotari_import", dict(edit, if_revision=plan["revision"]))
    preview = call("rotari_preview_run", rerun)
    print("  ", len(preview["execute"]), "of", preview["jobs"], "jobs execute")
    started = call("rotari_start_run", dict(rerun, if_revision=preview["revision"]))
    waits = 0
    while True:
        waits += 1
        followed = call("rotari_wait_run", {"run_id": started["run_id"]})
        if followed["reason"] != "timeout":
            break
    print(
        "   waits",
        waits,
        "reason",
        followed["reason"],
        "state",
        followed["state"],
        followed["summary"]["counts"],
    )
    comparison = call("rotari_compare_runs", {"run_id": started["run_id"]})
    print("  ", comparison["comparison"]["summary"])
    for job in comparison["comparison"]["jobs"]:
        if job["transition"] not in ("fixed", "still_passing"):
            print(
                "  ",
                job.get("name"),
                job["transition"],
                job.get("from_cause"),
                "->",
                job.get("to_cause"),
            )


def start(target, retry):
    """Preview and start a run of target, returning its run ID."""
    rerun = dict(target, retry=retry)
    preview = call("rotari_preview_run", rerun)
    print("  ", len(preview["execute"]), "of", preview["jobs"], "jobs execute")
    started = call("rotari_start_run", dict(rerun, if_revision=preview["revision"]))
    return started["run_id"]


def control_scenario():
    """Cancel a hanging job, then recover a run whose supervisor died."""
    # The environment: the data server that task 12 waits for never answers.
    with open("train.sh") as script:
        text = script.read()
    with open("train.sh", "w") as script:
        script.write(text.replace("sleep 30", "sleep 600"))
    projects = call("rotari_list_projects", {})
    lab = [p for p in projects["projects"] if p["basedir_name"] == "labA"][0]
    target = {"basedir_ref": lab["basedir_ref"], "project": lab["project"]}
    # The agent drops the 5s timeout that killed task 12, and reruns.
    exported = call("rotari_export_run", {"run_id": lab["last_run_id"]})
    manifest = exported["manifest"].replace("    timeout: 5s\n", "")
    edit = dict(target, manifest=manifest, overwrite=True)
    plan = call("rotari_preview_import", edit)
    call("rotari_import", dict(edit, if_revision=plan["revision"]))
    run_id = start(target, True)
    waited = call("rotari_wait_run", {"run_id": run_id, "timeout_seconds": 20, "until_failure": True})
    print("   reason", waited["reason"], "state", waited["state"], waited["summary"]["counts"])
    waited = call("rotari_wait_run", {"run_id": run_id, "timeout_seconds": 10})
    print("   reason", waited["reason"], "state", waited["state"], waited["summary"]["counts"])
    if waited["reason"] == "timeout":
        preview = call("rotari_preview_job_control", {"run_id": run_id, "operation": "cancel"})
        print("   would cancel", preview["job_ids"])
        call("rotari_cancel", {"run_id": run_id, "job_ids": preview["job_ids"]})
        waited = call("rotari_wait_run", {"run_id": run_id})
        print("   reason", waited["reason"], "state", waited["state"], waited["summary"]["counts"])
        for group in waited["summary"].get("failures", []):
            print("  ", group["count"], group["cause"], [j["name"] for j in group["jobs"]])

    # The environment: the supervisor of the next run is killed, and its
    # hanging job keeps running.
    run_id = start(target, True)
    time.sleep(3)
    subprocess.run(["pkill", "-9", "-f", "__server --basedir .*/labA"], check=False)
    waited = call("rotari_wait_run", {"run_id": run_id, "timeout_seconds": 20})
    print("   reason", waited["reason"], "state", waited["state"], waited["summary"]["counts"])
    projects = call("rotari_list_projects", {})
    print("   labA state", [p["state"] for p in projects["projects"] if p["basedir_name"] == "labA"])
    reset = call("rotari_preview_reset", target)
    print("  ", {k: reset.get(k) for k in ("cleared", "interrupted_run_id", "interrupted_run", "jobs_may_be_running")})
    if reset.get("jobs_may_be_running"):
        preview = call("rotari_preview_job_control", {"run_id": run_id, "operation": "cancel"})
        print("   would cancel", preview["job_ids"])
        call("rotari_cancel", {"run_id": run_id, "job_ids": preview["job_ids"]})
        time.sleep(2)
        reset = call("rotari_preview_reset", target)
        print("  ", {k: reset.get(k) for k in ("cleared", "interrupted_run_id", "interrupted_run", "jobs_may_be_running")})
    call("rotari_reset", dict(target, if_revision=reset["revision"], recover_interrupted=True))
    check = call("rotari_check_project", target)
    print("   check", check["state"], "runnable", check["runnable"])


request(
    "initialize",
    {
        "protocolVersion": "2025-06-18",
        "capabilities": {},
        "clientInfo": {"name": "trial", "version": "1"},
    },
)
request("notifications/initialized", notify=True)
tools = request("tools/list")["result"]["tools"]
print(f"== tools/list -> {len(json.dumps(tools))} bytes: {[t['name'] for t in tools]}")
{"read": read_scenario, "write": write_scenario, "control": control_scenario}[
    sys.argv[2] if len(sys.argv) > 2 else "read"
]()
proc.stdin.close()
proc.wait()
