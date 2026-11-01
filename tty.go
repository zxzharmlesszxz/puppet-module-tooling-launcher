package main

import (
	"os"
	"syscall"
	"unsafe"
)

func isTerminal(file *os.File) bool {
	var state syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), ttyRequest, uintptr(unsafe.Pointer(&state)))
	return errno == 0
}

func terminalColumns(file *os.File) int {
	var size struct{ rows, columns, x, y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0
	}
	return int(size.columns)
}
