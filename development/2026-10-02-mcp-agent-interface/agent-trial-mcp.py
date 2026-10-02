#!/usr/bin/env python3
"""Drive `rotari mcp` over stdio like an MCP-only agent and print response sizes.

Usage: XDG_STATE_HOME=DIR/xdg agent-trial-mcp.py PATH_TO_ROTARI, after
agent-trial-fixture.sh DIR. See agent-trial-2026-10-03-m4.md.
"""

import json
import subprocess
import sys

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


def call(name, arguments):
    reply = request("tools/call", {"name": name, "arguments": arguments})
    result = reply["result"]
    text = json.dumps(result.get("structuredContent", result.get("content")))
    print(
        f"== {name} {arguments} -> {len(text)} bytes, "
        f"error={result.get('isError', False)}"
    )
    return result.get("structuredContent") or {}


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
info = call(
    "rotari_get_job_info", {"run_id": worst["last_run_id"], "job_id": first_job}
)
labA = [p for p in projects["projects"] if p["basedir_name"] == "labA"][0]
comparison = call("rotari_compare_runs", {"run_id": labA["last_run_id"]})
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
    {"basedir_ref": labA["basedir_ref"], "project": labA["project"]},
)
print(
    "  ",
    check.get("state"),
    "runnable",
    check.get("runnable"),
    "queued",
    check.get("queued"),
)
proc.stdin.close()
proc.wait()
