#!/bin/bash
# Installs LSF from the packages mounted at /lsf-distrib, starts its daemons,
# and keeps the container running.
set -euo pipefail

lsf_top=/opt/lsf
work=/tmp/lsf-install
mkdir -p "$work/distrib"

report_failure() {
    local status=$?
    echo "start-lsf.sh: '$BASH_COMMAND' exited with $status" >&2
    find "$work" "$lsf_top" -name '*.log' -type f 2>/dev/null | while read -r log; do
        echo "== $log" >&2
        tail -n 100 "$log" >&2
    done
}
trap report_failure ERR

installer=$(find /lsf-distrib -name '*lsfinstall*.tar*' -type f -print -quit)
if [[ -z "$installer" ]]; then
    echo "no *lsfinstall*.tar* file under the LSF package" >&2
    exit 1
fi

if [[ -n "${LSF_DISTRIBUTION:-}" ]]; then
    mapfile -t distributions < <(find /lsf-distrib -name "$LSF_DISTRIBUTION" -type f)
else
    mapfile -t distributions < <(find /lsf-distrib -name 'lsf*.tar*' ! -name '*lsfinstall*' -type f)
fi
if [[ ${#distributions[@]} -ne 1 ]]; then
    echo "want one LSF distribution archive, found ${#distributions[@]}:" >&2
    printf '  %s\n' "${distributions[@]}" >&2
    echo "set LSF_DISTRIBUTION to the file name of the one to install" >&2
    exit 1
fi
cp "${distributions[0]}" "$work/distrib/"

gzip -dc "$installer" | tar -xf - -C "$work"
lsfinstall=$(find "$work" -maxdepth 2 -name lsfinstall -type f -print -quit)
if [[ -z "$lsfinstall" ]]; then
    echo "the installer archive $installer has no lsfinstall" >&2
    exit 1
fi

cat > "$work/install.config" <<EOF
LSF_TOP="$lsf_top"
LSF_ADMINS="lsfadmin"
LSF_CLUSTER_NAME="rotari"
LSF_MASTER_LIST="$(hostname)"
LSF_TARDIR="$work/distrib"
LSF_SILENT_INSTALL_TARLIST="ALL"
SILENT_INSTALL="Y"
ACCEPT_LICENSE="Y"
ENABLE_EGO="N"
EOF
entitlement=$(find /lsf-distrib -name '*entitlement*' -type f -print -quit)
if [[ -n "$entitlement" ]]; then
    echo "LSF_ENTITLEMENT_FILE=\"$entitlement\"" >> "$work/install.config"
fi

(cd "$(dirname "$lsfinstall")" && ./lsfinstall -f "$work/install.config" </dev/null)
hostsetup=$(find "$lsf_top" -path '*/install/hostsetup' -type f -print -quit)
if [[ -z "$hostsetup" ]]; then
    echo "lsfinstall left no install/hostsetup under $lsf_top" >&2
    false
fi
"$hostsetup" --top="$lsf_top" --boot=n

ln -sf "$lsf_top/conf/profile.lsf" /etc/profile.d/lsf.sh
set +u
# shellcheck disable=SC1091
. "$lsf_top/conf/profile.lsf"
set -u
"$LSF_SERVERDIR/lsf_daemons" start

echo "LSF started"
exec sleep infinity
