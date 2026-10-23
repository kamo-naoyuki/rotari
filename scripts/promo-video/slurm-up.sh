#!/usr/bin/env bash
# Start a one-node Slurm cluster from an unpacked image, inside the capture
# namespace (we are root there, mapped to one host user, with a private
# network namespace so the daemons' ports cannot clash with the host's).
#
#   slurm-up.sh SANDBOX BIN_DIR
#
# Every daemon runs as root in a chroot of SANDBOX; slurmctld and slurmd get
# their own UTS namespaces, so batch jobs see the host name gpu-node01.
# BIN_DIR receives sbatch/squeue/sacct/... wrappers that run the client
# commands in the chroot from the same working directory, so rotari on the
# host side can use them. /home/me is shared with the chroot. setup.sh
# prepares SANDBOX, including the setgroups shim the daemons need.
set -euo pipefail
sb=$1
bin=$2

# Start from an empty controller state; job IDs then begin at 1 on every capture.
find "$sb/var/spool/slurmctld" -mindepth 1 -delete 2>/dev/null || true

mount --rbind /dev "$sb/dev"
mount -t proc proc "$sb/proc"
mount --rbind /sys "$sb/sys"
mkdir -p "$sb/home/me"
mount --bind /home/me "$sb/home/me"
mount -t tmpfs tmpfs "$sb/tmp"
chmod 1777 "$sb/tmp"

echo "127.0.0.1 localhost slurmctl gpu-node01" >"$sb/etc/hosts"

cat >"$sb/etc/slurm/slurm.conf" <<'EOF'
ClusterName=lab
SlurmctldHost=slurmctl
SlurmUser=root
AuthType=auth/munge
StateSaveLocation=/var/spool/slurmctld
SlurmdSpoolDir=/var/spool/slurmd
SlurmctldPidFile=/var/run/slurm/slurmctld.pid
SlurmdPidFile=/var/run/slurm/slurmd.pid
SlurmctldLogFile=/var/log/slurm/slurmctld.log
SlurmdLogFile=/var/log/slurm/slurmd.log
ProctrackType=proctrack/linuxproc
TaskPlugin=task/none
SwitchType=switch/none
MpiDefault=none
ReturnToService=2
SchedulerType=sched/backfill
SelectType=select/cons_tres
SelectTypeParameters=CR_CPU
AccountingStorageType=accounting_storage/slurmdbd
JobAcctGatherType=jobacct_gather/none
MinJobAge=300
SlurmctldTimeout=60
SlurmdTimeout=60
NodeName=gpu-node01 NodeHostName=slurmctl NodeAddr=127.0.0.1 CPUs=8 RealMemory=8000 State=UNKNOWN
PartitionName=gpu Nodes=gpu-node01 Default=YES MaxTime=INFINITE State=UP
EOF
sed -i 's/^SlurmUser=.*/SlurmUser=root/' "$sb/etc/slurm/slurmdbd.conf"
chmod 600 "$sb/etc/slurm/slurmdbd.conf"
mkdir -p "$sb/var/spool/slurmctld" "$sb/var/spool/slurmd" "$sb/var/run/slurm" \
    "$sb/var/log/slurm" "$sb/var/run/munge" "$sb/var/lib/munge" "$sb/var/log/munge"

in_sb() { chroot "$sb" /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin HOME=/root "$@"; }

in_sb /usr/sbin/munged --force </dev/null >/dev/null 2>&1
if [[ ! -d "$sb/var/lib/mysql/mysql" ]]; then
    in_sb /usr/bin/mysql_install_db --user=root >/dev/null 2>&1
fi
in_sb /bin/sh -c '/usr/bin/mysqld_safe --user=root --skip-networking </dev/null >/dev/null 2>&1 &'
for _ in $(seq 60); do
    in_sb mysql -e 'SELECT 1' >/dev/null 2>&1 && break
    sleep 0.5
done
in_sb mysql -NBe "CREATE DATABASE IF NOT EXISTS slurm_acct_db;
  GRANT ALL ON slurm_acct_db.* TO 'slurm'@'localhost' IDENTIFIED BY 'password';"
in_sb /usr/sbin/slurmdbd </dev/null >/dev/null 2>&1
for _ in $(seq 60); do
    (echo >/dev/tcp/127.0.0.1/6819) 2>/dev/null && break
    sleep 0.5
done
in_sb sacctmgr -i add cluster lab >/dev/null 2>&1 || true
unshare --uts sh -c 'hostname slurmctl; exec "$@"' _ chroot "$sb" /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/sbin/slurmctld </dev/null >/dev/null 2>&1
unshare --uts sh -c 'hostname gpu-node01; exec "$@"' _ chroot "$sb" /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /usr/sbin/slurmd -N gpu-node01 </dev/null >/dev/null 2>&1
for _ in $(seq 60); do
    in_sb sinfo -h -o %t 2>/dev/null | grep -qx idle && break
    sleep 0.5
done
in_sb sinfo

mkdir -p "$bin"
for c in sbatch squeue sacct scancel scontrol sinfo; do
    cat >"$bin/$c" <<EOF
#!/bin/sh
# Run $c in the Slurm chroot from the same working directory.
exec /usr/sbin/chroot "$sb" /usr/bin/env PATH=/usr/sbin:/usr/bin:/sbin:/bin LC_ALL=C \\
    /bin/sh -c 'cd "\$0" && exec $c "\$@"' "\$PWD" "\$@"
EOF
    chmod +x "$bin/$c"
done
