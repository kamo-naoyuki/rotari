#!/usr/bin/env bash
# Prepare the tools the promo video pipeline needs:
#
#   setup.sh [--with-slurm] TOOLS
#
#   .venv           Playwright, kokoro-onnx, and soundfile, installed into the
#                   repository's virtual environment (PYTHON overrides it)
#   TOOLS/pw        Chrome Headless Shell for that Playwright version
#   TOOLS/kokoro    Kokoro-82M TTS model and voices (ONNX, about 350 MB)
#   TOOLS/slurm-sb  with --with-slurm: the Slurm image used by
#                   scheduler-integration.sh, unpacked by singularity, with a
#                   setgroups shim preloaded (for make-video.sh --with-slurm)
#
# Needs uv (or pip in the virtual environment), curl, unzip, and, for Slurm,
# singularity (or apptainer) and unprivileged user namespaces. Docker is not
# used: it needs the docker group. Chrome is fetched with curl rather than
# `playwright install`, whose Node downloader rejects TLS-inspecting proxies
# that curl and uv --native-tls accept.
set -euo pipefail

with_slurm=false
if [[ "${1:-}" == --with-slurm ]]; then
    with_slurm=true
    shift
fi
if [[ $# -ne 1 ]]; then
    echo "usage: $0 [--with-slurm] TOOLS" >&2
    exit 2
fi
mkdir -p "$1"
tools=$(cd "$1" && pwd)
repo=$(cd "$(dirname "$0")/../.." && pwd)
python=${PYTHON:-$repo/.venv/bin/python}

playwright_version=1.64.0
kokoro_onnx_version=0.6.1
kokoro_models=https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0
slurm_image=docker://giovtorres/docker-centos7-slurm:latest

packages=("playwright==$playwright_version" "kokoro-onnx==$kokoro_onnx_version" soundfile)
if ! "$python" -c "import playwright, kokoro_onnx, soundfile" 2>/dev/null; then
    if command -v uv >/dev/null; then
        uv pip install --native-tls --python "$python" "${packages[@]}"
    else
        "$python" -m pip install "${packages[@]}"
    fi
fi

mkdir -p "$tools/kokoro"
for file in kokoro-v1.0.onnx voices-v1.0.bin; do
    if [[ ! -s "$tools/kokoro/$file" ]]; then
        curl -fsSL -o "$tools/kokoro/$file.part" "$kokoro_models/$file"
        mv "$tools/kokoro/$file.part" "$tools/kokoro/$file"
    fi
done

read -r revision browser_version < <(
    "$python" <<'EOF'
import json, pathlib, playwright
package = pathlib.Path(playwright.__file__).parent / "driver" / "package"
browsers = json.loads((package / "browsers.json").read_text())["browsers"]
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

if $with_slurm; then
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
fi

echo "tools ready in $tools"
