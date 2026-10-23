#!/usr/bin/env bash
# Capture material for the "results" episode (notifications and artifacts).
#
#   capture-results.sh WORK ROTARI_STATIC_BINARY SAMPLES_DIR
#
# Like capture.sh, everything runs in user, UTS, mount, network, and PID
# namespaces with host name "workstation" and WORK/home at /home/me. In
# addition, newroot.sh gives the namespace its own /, and the second stage
# starts Xvfb, a D-Bus session, and dunst, so a headed Chrome can show (and
# ffmpeg can record) the Web UI's desktop notification. SAMPLES_DIR holds the
# Kokoro clips the fake TTS job "synthesizes". Output lands in WORK/capture.
# Needs Xvfb, dunst, dbus-run-session, and /usr/bin/google-chrome; PYTHON and
# PLAYWRIGHT_BROWSERS_PATH as for capture.sh.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)

if [[ "${1:-}" == --stage2 ]]; then
    shift
    # Inside the new root: a private X socket directory, a display, a
    # session bus, and the notification daemon.
    mount -t tmpfs tmpfs /tmp/.X11-unix
    # -nolock: the lock file would go to the host's shared /tmp and outlive
    # the namespace.
    Xvfb :7 -nolock -nocursor -screen 0 1200x1620x24 -nolisten tcp >/dev/null 2>&1 &
    for _ in $(seq 50); do
        [[ -S /tmp/.X11-unix/X7 ]] && break
        sleep 0.1
    done
    if [[ ! -S /tmp/.X11-unix/X7 ]]; then
        echo "Xvfb did not start" >&2
        exit 1
    fi
    export DISPLAY=:7
    exec dbus-run-session -- bash -c '
        dunst -config "$1/dunstrc" >/dev/null 2>&1 &
        sleep 0.5
        exec "$2" "$1/capture_results.py" "$1/fixture" "$3" "$4" "$5"
    ' _ "$here" "$PYTHON" "$@"
fi

work=$(mkdir -p "$1" && cd "$1" && pwd)
rotari=$(realpath "$2")
samples=$(realpath "$3")
mkdir -p "$work/home"
export PYTHON=${PYTHON:-python3}
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
    exec "$2/newroot.sh" "$2/capture-results.sh" --stage2 "$1/capture" "$3" "$4"
' _ "$work" "$here" "$rotari" "$samples"
