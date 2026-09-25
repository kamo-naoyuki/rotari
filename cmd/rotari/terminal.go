package main

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal reports whether file is a terminal. It asks the terminal driver
// for the file's termios settings, so character devices such as /dev/null are
// not mistaken for an interactive terminal.
func isTerminal(file *os.File) bool {
	conn, err := file.SyscallConn()
	if err != nil {
		return false
	}
	var termios syscall.Termios
	var errno syscall.Errno
	if err := conn.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlReadTermios, uintptr(unsafe.Pointer(&termios)))
	}); err != nil {
		return false
	}
	return errno == 0
}

// stdinIsTerminal decides whether confirmation prompts may read from stdin.
// Tests can replace this package-level hook to exercise the prompts with piped
// input.
var stdinIsTerminal = func() bool {
	return isTerminal(os.Stdin)
}
