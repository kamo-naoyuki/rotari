#!/usr/bin/env bash
# Prepare the tools the promo video pipeline needs, all inside TOOLS:
#
#   setup.sh TOOLS
#
#   TOOLS/pylib     Playwright for Python (pip --target, no venv needed)
#   TOOLS/pw        Chrome Headless Shell for that Playwright version
#   TOOLS/slurm-sb  the Slurm image used by scheduler-integration.sh, unpacked
#                   by singularity, with a setgroups shim preloaded
#
# Needs python3 with pip, curl, unzip, singularity (or apptainer), and
# unprivileged user namespaces. Docker is not used: it needs the docker group.
# Chrome is fetched with curl rather than `playwright install`, whose Node
# downloader rejects TLS-inspecting proxies that curl and pip accept.
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 TOOLS" >&2
    exit 2
fi
mkdir -p "$1"
tools=$(cd "$1" && pwd)

playwright_version=1.64.0
slurm_image=docker://giovtorres/docker-centos7-slurm:latest

if [[ ! -d "$tools/pylib/playwright" ]]; then
    python3 -m pip install --quiet --target "$tools/pylib" "playwright==$playwright_version"
fi

read -r revision browser_version < <(
    python3 - "$tools/pylib" <<'EOF'
import json, sys
browsers = json.load(open(f"{sys.argv[1]}/playwright/driver/package/browsers.json"))["browsers"]
shell = next(b for b in browsers if b["name"] == "chromium-headless-shell")
print(shell["revision"], shell["browserVersion"])
EOF
)
shell_dir="$tools/pw/chromium_headless_shell-$revision"
if [[ ! -e "$shell_dir/INSTALLATION_COMPLETE" ]]; then
    mkdir -p "$shell_dir"
    curl -fsSL -o "$tools/chrome-headless-shell.zip" \
        "https://cdn.playwright.dev/builds/cft/$browser_version/linux64/chrome-headless-shell-linux64.zip"
    unzip -q -o "$tools/chrome-headless-shell.zip" -d "$shell_dir"
    rm "$tools/chrome-headless-shell.zip"
    touch "$shell_dir/INSTALLATION_COMPLETE" "$shell_dir/DEPENDENCIES_VALIDATED"
fi

sandbox="$tools/slurm-sb"
if [[ ! -d "$sandbox/usr/sbin" ]]; then
    singularity=$(command -v singularity || command -v apptainer)
    SINGULARITY_CACHEDIR="$tools/singularity-cache" SINGULARITY_TMPDIR="$tools" \
        "$singularity" build --fix-perms --sandbox "$sandbox" "$slurm_image"
fi
if [[ ! -e "$sandbox/usr/local/lib/nogroups.so" ]]; then
    # slurmd and slurmstepd call setgroups, which an unprivileged user
    # namespace refuses; make it (and initgroups) succeed without effect.
    cat >"$sandbox/usr/local/lib/nogroups.c" <<'EOF'
/* setgroups and initgroups are refused in an unprivileged user namespace; pretend they worked. */
#include <grp.h>
#include <sys/types.h>
int setgroups(size_t size, const gid_t *list) { (void)size; (void)list; return 0; }
int initgroups(const char *user, gid_t group) { (void)user; (void)group; return 0; }
EOF
    unshare --user --map-root-user chroot "$sandbox" /usr/bin/env PATH=/usr/bin:/bin \
        gcc -shared -fPIC -O2 -o /usr/local/lib/nogroups.so /usr/local/lib/nogroups.c
    echo /usr/local/lib/nogroups.so >"$sandbox/etc/ld.so.preload"
fi

echo "tools ready in $tools"
