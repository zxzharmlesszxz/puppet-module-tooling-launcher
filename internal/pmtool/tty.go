package pmtool

import (
	"os"
	"syscall"
	"unsafe"
)

func isTerminal(file *os.File) bool {
	var state syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), ttyRequest, uintptr(unsafe.Pointer(&state))) // #nosec G103 -- Synchronous ioctl writes to a correctly sized, live Termios buffer.
	return errno == 0
}

func terminalColumns(file *os.File) int {
	var size struct{ rows, columns, x, y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size))) // #nosec G103 -- Synchronous ioctl writes the four uint16 winsize fields to a live buffer.
	if errno != 0 {
		return 0
	}
	return int(size.columns)
}
