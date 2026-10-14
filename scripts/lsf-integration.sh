#!/usr/bin/env bash
# Runs the scheduler container tests against IBM Spectrum LSF Community
# Edition in a local Docker container. LSF is not downloaded: pass the package
# you downloaded from IBM, either the archive or a directory it was extracted
# into. Extra arguments go to go test, for example -run to select tests.
set -euo pipefail

usage() {
    echo "usage: $0 --accept-license LSF_PACKAGE [GO_TEST_ARGS...]" >&2
    echo "  --accept-license  accept the LSF license for the silent install" >&2
    echo "  LSF_DISTRIBUTION  file name of the distribution archive, when the package has several" >&2
}

if [[ "${1:-}" != --accept-license ]]; then
    usage
    exit 2
fi
shift
package=${1:-}
if [[ -z "$package" || ! -e "$package" ]]; then
    usage
    exit 2
fi
shift

for command in docker go; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "$command not found" >&2
        exit 1
    fi
done
if [[ "$(id -u)" == 0 ]]; then
    echo "run as a non-root user: LSF does not run root's jobs by default" >&2
    exit 1
fi

workspace=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
image=rotari-lsf-test
container="rotari-lsf-$$"
work=$(mktemp -d)
state_dir="$work/state"
mkdir -p "$state_dir"

cleanup() {
    local status=$?
    if [[ $status -ne 0 ]] && docker container inspect "$container" >/dev/null 2>&1; then
        docker logs "$container" >&2 || true
    fi
    docker rm --force "$container" >/dev/null 2>&1 || true
    rm -rf "$work" || echo "could not remove $work" >&2
    exit "$status"
}
trap cleanup EXIT

if [[ -d "$package" ]]; then
    distrib=$(cd "$package" && pwd)
else
    distrib="$work/distrib"
    mkdir -p "$distrib"
    tar -xf "$package" -C "$distrib"
fi

docker build --tag "$image" \
    --build-arg TEST_UID="$(id -u)" --build-arg TEST_GID="$(id -g)" \
    "$workspace/scripts/lsf"
docker run --detach --name "$container" --hostname lsfhost \
    --env LSF_DISTRIBUTION \
    --volume "$distrib:/lsf-distrib:ro" --volume "$state_dir:/state" \
    "$image" >/dev/null

echo "waiting for LSF to install and start" >&2
for attempt in {1..150}; do
    if [[ "$(docker container inspect --format '{{.State.Running}}' "$container")" != true ]]; then
        echo "the LSF container stopped" >&2
        exit 1
    fi
    if docker exec --user rotari "$container" sh -lc \
        'bhosts 2>/dev/null | awk "NR > 1 && \$2 == \"ok\" { found = 1 } END { exit !found }"'; then
        break
    fi
    if [[ "$attempt" == 150 ]]; then
        echo "LSF did not report an ok host" >&2
        exit 1
    fi
    sleep 2
done
docker exec --user rotari "$container" sh -lc 'lsid; bhosts; bqueues'

cd "$workspace"
ROTARI_SCHEDULER_CONTAINER_TEST=1 \
    ROTARI_SCHEDULER_EXECUTOR=lsf \
    SCHEDULER_CONTAINER="$container" \
    SCHEDULER_USER=rotari \
    SCHEDULER_SETUP='. /opt/lsf/conf/profile.lsf' \
    SCHEDULER_STATE_DIR="$state_dir" \
    go test ./cmd/rotari -run '^TestSchedulerContainer' -count=1 -v "$@"
