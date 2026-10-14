#!/usr/bin/env bash
# Starts a scheduler container and runs the TestSchedulerContainer* tests in
# cmd/rotari against it; the scheduler integration workflow runs it too. Extra
# arguments go to go test, for example -run to select tests.
#
# LSF Community Edition is not downloaded: pass the package downloaded from
# IBM, either the archive or a directory it was extracted into. The image
# built from scripts/lsf holds no LSF software and installs it at startup.
set -euo pipefail

usage() {
    cat >&2 <<EOF
usage: $0 slurm|pbs|sge [GO_TEST_ARGS...]
       $0 lsf --accept-license LSF_PACKAGE [GO_TEST_ARGS...]
  --accept-license  accept the LSF license for the silent install
  LSF_DISTRIBUTION  file name of the LSF distribution archive, when the package has several
  SCHEDULER_TEST_RUNNER  go (default) or gotestsum, which reads GOTESTSUM_FORMAT
EOF
}

executor=${1:-}
case "$executor" in
slurm | pbs | sge)
    shift
    ;;
lsf)
    if [[ "${2:-}" != --accept-license || -z "${3:-}" || ! -e "${3:-}" ]]; then
        usage
        exit 2
    fi
    lsf_package=$3
    shift 3
    if [[ "$(id -u)" == 0 ]]; then
        echo "run as a non-root user: LSF does not run root's jobs by default" >&2
        exit 1
    fi
    ;;
*)
    usage
    exit 2
    ;;
esac

runner=${SCHEDULER_TEST_RUNNER:-go}
if [[ "$runner" != go && "$runner" != gotestsum ]]; then
    usage
    exit 2
fi
for command in docker go "$runner"; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "$command not found" >&2
        exit 1
    fi
done

workspace=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
container="rotari-${executor}-$$"
work=$(mktemp -d)
state_dir="$work/state"
mkdir -p "$state_dir"
chmod 0777 "$state_dir"

cleanup() {
    local status=$?
    if docker container inspect "$container" >/dev/null 2>&1; then
        if [[ $status -ne 0 ]]; then
            docker logs "$container" >&2 || true
        fi
        # Container users own files in /state that the host user cannot remove.
        docker exec --user root "$container" find /state -mindepth 1 -delete >/dev/null 2>&1 || true
        docker rm --force "$container" >/dev/null 2>&1 || true
    fi
    rm -rf "$work" || echo "could not remove $work" >&2
    exit "$status"
}
trap cleanup EXIT

scheduler_setup=
case "$executor" in
slurm)
    docker run --detach --name "$container" --hostname slurmctl \
        --interactive --tty --cap-add SYS_ADMIN \
        --volume "$state_dir:/state" \
        giovtorres/docker-centos7-slurm:latest >/dev/null
    scheduler_user=root
    ready='sinfo'
    ;;
pbs)
    docker run --detach --name "$container" --cap-add SYS_RESOURCE \
        --volume "$state_dir:/state" \
        naoso5/test-openpbs:latest >/dev/null
    scheduler_user=testuser
    ready='qstat -Bf'
    ;;
sge)
    # Pinned amd64 digest: the CentOS 7 image, last rebuilt in 2021, must not change under CI.
    docker run --detach --name "$container" --hostname docker \
        --cap-add SYS_ADMIN \
        --volume "$state_dir:/state" \
        drmaa/gridengine@sha256:bebb328113b2f4b6eb14d01d18f4a5714ffd3c41659df6b0013d7e1b63588415 \
        /bin/bash -lc 'exec sleep infinity' >/dev/null
    scheduler_user=root
    scheduler_setup='. /opt/sge/default/common/settings.sh'
    ready='qstat -xml && qconf -sql | grep -qx all.q'
    ;;
lsf)
    if [[ -d "$lsf_package" ]]; then
        distrib=$(cd "$lsf_package" && pwd)
    else
        distrib="$work/distrib"
        mkdir -p "$distrib"
        tar -xf "$lsf_package" -C "$distrib"
    fi
    docker build --tag rotari-lsf-test \
        --build-arg TEST_UID="$(id -u)" --build-arg TEST_GID="$(id -g)" \
        "$workspace/scripts/lsf"
    docker run --detach --name "$container" --hostname lsfhost \
        --env LSF_DISTRIBUTION \
        --volume "$distrib:/lsf-distrib:ro" --volume "$state_dir:/state" \
        rotari-lsf-test >/dev/null
    scheduler_user=rotari
    scheduler_setup='. /opt/lsf/conf/profile.lsf'
    ready='bhosts | awk "NR > 1 && \$2 == \"ok\" { found = 1 } END { exit !found }"'
    ;;
esac

echo "waiting for $executor to start" >&2
for attempt in {1..150}; do
    if [[ "$(docker container inspect --format '{{.State.Running}}' "$container")" != true ]]; then
        echo "the $executor container stopped" >&2
        exit 1
    fi
    if [[ "$executor" != pbs || "$(docker container inspect --format '{{.State.Health.Status}}' "$container")" == healthy ]] &&
        docker exec --user "$scheduler_user" "$container" sh -lc "${scheduler_setup:+$scheduler_setup; }$ready" >/dev/null 2>&1; then
        break
    fi
    if [[ "$attempt" == 150 ]]; then
        echo "$executor did not become ready" >&2
        exit 1
    fi
    sleep 2
done

cd "$workspace"
export ROTARI_SCHEDULER_CONTAINER_TEST=1 \
    ROTARI_SCHEDULER_EXECUTOR="$executor" \
    SCHEDULER_CONTAINER="$container" \
    SCHEDULER_USER="$scheduler_user" \
    SCHEDULER_SETUP="$scheduler_setup" \
    SCHEDULER_STATE_DIR="$state_dir"
test_args=(./cmd/rotari -run '^TestSchedulerContainer' -count=1)
if [[ "$runner" == gotestsum ]]; then
    gotestsum -- "${test_args[@]}" "$@"
else
    go test "${test_args[@]}" -v "$@"
fi
