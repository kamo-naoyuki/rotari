#!/usr/bin/env bash
# Replace / with a tmpfs owned by the namespace's root, then run COMMAND.
#
#   newroot.sh COMMAND [ARGS...]     (as root in a user and mount namespace)
#
# rotari web --allow-exec checks that every directory from / down to the
# launcher file is owned by the user or root (OpenSSH StrictModes). In a user
# namespace the real root is unmapped and / looks owned by nobody, so the
# check fails. This builds a new root whose / and /home belong to the
# namespace's root, binds the host's system and data directories into it,
# and pivots into it; /home/me must already be mounted.
set -euo pipefail
new=$(mktemp -d)
mount -t tmpfs -o mode=755 tmpfs "$new"
for d in usr etc opt var data tmp run srv; do
    if [[ -d "/$d" ]]; then
        mkdir -p "$new/$d"
        mount --rbind "/$d" "$new/$d"
    fi
done
for link in bin sbin lib lib32 lib64 libx32; do
    if [[ -L "/$link" ]]; then
        ln -s "$(readlink "/$link")" "$new/$link"
    elif [[ -d "/$link" ]]; then
        mkdir -p "$new/$link"
        mount --rbind "/$link" "$new/$link"
    fi
done
mkdir -p "$new/home/me" "$new/dev" "$new/sys" "$new/proc" "$new/root"
mount --rbind /home/me "$new/home/me"
mount --rbind /dev "$new/dev"
mount --rbind /sys "$new/sys"
mount -t proc proc "$new/proc"
mkdir -p "$new/.old"
cd "$new"
pivot_root . .old
umount -l /.old
rmdir /.old
cd /
exec "$@"
