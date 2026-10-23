#!/usr/bin/env bash
# Capture material for the rotari promo short.
#
#   capture.sh WORK ROTARI_STATIC_BINARY SLURM_SANDBOX
#
# Runs capture.py in user, UTS, mount, network and PID namespaces: host name
# "workstation", WORK/home mounted at /home/me, a private network with only
# loopback and a dummy IPv4 address, and a one-node Slurm cluster started by
# slurm-up.sh. Everything in the namespace stops when capture.py exits.
# Output lands in WORK/capture. PYTHON names an interpreter with Playwright
# (default python3), and PLAYWRIGHT_BROWSERS_PATH where its Chrome is.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
work=$(mkdir -p "$1" && cd "$1" && pwd)
rotari=$(realpath "$2")
sandbox=$(realpath "$3")
mkdir -p "$work/home"
exec unshare --user --map-root-user --uts --mount --net --pid --fork --mount-proc bash -c '
    set -e
    ip link set lo up
    ip link add dummy0 type dummy
    ip addr add 10.77.0.1/24 dev dummy0
    ip link set dummy0 up
    hostname workstation
    mount -t tmpfs tmpfs /home
    mkdir -p /home/me
    mount --bind "$1/home" /home/me
    "$2/slurm-up.sh" "$4" /home/me/sbin >"$1/slurm-up.log" 2>&1
    "$5" "$2/capture.py" "$2/fixture" "$1/capture" "$3"
' _ "$work" "$here" "$rotari" "$sandbox" "${PYTHON:-python3}"
