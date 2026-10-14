//go:build linux

package bugreport

import (
	"fmt"
	"syscall"
)

var filesystemNames = map[uint32]string{
	0x6969:     "nfs",
	0xEF53:     "ext2/3/4",
	0x58465342: "xfs",
	0x9123683E: "btrfs",
	0x01021994: "tmpfs",
	0x2FC12FC1: "zfs",
	0x0BD00BD0: "lustre",
	0x47504653: "gpfs",
	0x19830326: "beegfs",
	0xFF534D42: "cifs",
	0xFE534D42: "smb2",
	0x65735546: "fuse",
	0x794C7630: "overlayfs",
	0x00C36400: "ceph",
	0x5346414F: "afs",
}

func filesystemType(path string) string {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "unknown (" + err.Error() + ")"
	}
	magic := uint32(stat.Type)
	if name, ok := filesystemNames[magic]; ok {
		return fmt.Sprintf("%s (0x%x)", name, magic)
	}
	return fmt.Sprintf("0x%x", magic)
}
