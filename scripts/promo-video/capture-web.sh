#!/usr/bin/env bash
# Run an episode's capture script in the capture namespaces.
#
#   capture-web.sh [--display] WORK ROTARI_STATIC_BINARY SAMPLES_DIR SCRIPT.py
#
# Like capture.sh, everything runs in user, UTS, mount, network, and PID
# namespaces with host name "workstation" and WORK/home at /home/me, and
# newroot.sh gives the namespace its own / so `rotari web --allow-exec`
# accepts the launcher file's directories. With --display, the second stage
# also starts Xvfb (DISPLAY :7, $SCREEN, default 2200x1564), a D-Bus session,
# and dunst, so a headed Chrome can show (and ffmpeg can record) the Web UI's
# desktop notification; that needs Xvfb, dunst, and /usr/bin/google-chrome.
# SAMPLES_DIR holds the Kokoro clips the fake TTS job "synthesizes". SCRIPT.py
# is called as SCRIPT FIXTURE OUT ROTARI SAMPLES and writes WORK/capture.
# PYTHON and PLAYWRIGHT_BROWSERS_PATH as for capture.sh.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)

if [[ "${1:-}" == --stage2 ]]; then
    shift
    display=$1
    shift
    if [[ "$display" == no ]]; then
        exec "$PYTHON" "$@"
    fi
    # Inside the new root: a private X socket directory, a display, a
    # session bus, and the notification daemon.
    mount -t tmpfs tmpfs /tmp/.X11-unix
    # -nolock: the lock file would go to the host's shared /tmp and outlive
    # the namespace.
    Xvfb :7 -nolock -nocursor -screen 0 "${SCREEN:-2200x1564}x24" -nolisten tcp >/dev/null 2>&1 &
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
        shift
        exec "$PYTHON" "$@"
    ' _ "$here" "$@"
fi

display=no
if [[ "${1:-}" == --display ]]; then
    display=yes
    shift
fi
if [[ $# -ne 4 ]]; then
    echo "usage: $0 [--display] WORK ROTARI_STATIC_BINARY SAMPLES_DIR SCRIPT.py" >&2
    exit 2
fi
work=$(mkdir -p "$1" && cd "$1" && pwd)
rotari=$(realpath "$2")
samples=$(realpath "$3")
script=$(realpath "$4")
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
    exec "$2/newroot.sh" "$2/capture-web.sh" --stage2 "$6" "$5" "$2/fixture" "$1/capture" "$3" "$4"
' _ "$work" "$here" "$rotari" "$samples" "$script" "$display"
