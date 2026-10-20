package launcher

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// maxSymlinks bounds symlink resolution, like the kernel's ELOOP limit.
const maxSymlinks = 40

// OpenTrustedFile opens the file at the absolute path for reading after
// checking, in the manner of OpenSSH StrictModes, that no other OS user can
// write or replace it: the file and every directory from the root down to it
// must be owned by the current user or root and must not be writable by
// group or others. A directory writable by others is accepted only when it
// is sticky (like /tmp), because others cannot rename files they do not own
// there. Symlinks are followed one by one, so the directories holding each
// link are checked as well as the target's.
//
// A missing file returns an error satisfying errors.Is(err, fs.ErrNotExist).
func OpenTrustedFile(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%s: path is not absolute", path)
	}
	resolved, err := resolveTrusted(path, os.Getuid())
	if err != nil {
		return nil, err
	}
	// The directories cannot change under us, so the name still refers to
	// the checked location; O_NOFOLLOW refuses a symlink swapped in by the
	// owner, and the descriptor is what gets checked and read.
	file, err := os.OpenFile(resolved, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil {
		err = checkTrusted(resolved, info, os.Getuid())
	}
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("%s: not a regular file", resolved)
	}
	if err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// resolveTrusted walks path from the root, checking each directory and
// restarting from the root at each symlink, and returns the symlink-free
// path of the final component without checking that component itself.
func resolveTrusted(path string, uid int) (string, error) {
	for range maxSymlinks {
		parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
		current := "/"
		info, err := os.Lstat(current)
		if err != nil {
			return "", err
		}
		if err := checkTrusted(current, info, uid); err != nil {
			return "", err
		}
		// Every iteration ends by returning or by restarting at a symlink.
		for i, part := range parts {
			next := filepath.Join(current, part)
			info, err := os.Lstat(next)
			if err != nil {
				return "", err
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				target, err := os.Readlink(next)
				if err != nil {
					return "", err
				}
				if !filepath.IsAbs(target) {
					// current has no symlinks, so resolving ".." lexically
					// matches the kernel.
					target = filepath.Join(current, target)
				}
				path = filepath.Join(append([]string{target}, parts[i+1:]...)...)
				break
			}
			if i == len(parts)-1 {
				return next, nil
			}
			if !info.IsDir() {
				return "", fmt.Errorf("%s: %w", next, syscall.ENOTDIR)
			}
			if err := checkTrusted(next, info, uid); err != nil {
				return "", err
			}
			current = next
		}
	}
	return "", fmt.Errorf("%s: %w", path, syscall.ELOOP)
}

// checkTrusted rejects a file or directory that another user owns or can
// modify.
func checkTrusted(path string, info fs.FileInfo, uid int) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%s: cannot read owner", path)
	}
	if owner := int(stat.Uid); owner != uid && owner != 0 {
		return fmt.Errorf("%s is owned by uid %d; it must be owned by you (uid %d) or root", path, owner, uid)
	}
	if info.Mode().Perm()&0o022 == 0 {
		return nil
	}
	if info.IsDir() && info.Mode()&fs.ModeSticky != 0 {
		return nil
	}
	return fmt.Errorf("%s is writable by group or others (mode %s); run: chmod go-w %s", path, info.Mode().Perm(), path)
}
