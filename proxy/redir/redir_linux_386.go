package redir

import (
	"syscall"
	"unsafe"
)

// https://github.com/golang/go/blob/9e6b79a5dfb2f6fe4301ced956419a0da83bd025/src/syscall/syscall_linux_386.go#L196
const GETSOCKOPT = 15

func socketcall(call, a0, a1, a2, a3, a4, a5 uintptr) error {
	var a [6]uintptr
	a[0], a[1], a[2], a[3], a[4], a[5] = a0, a1, a2, a3, a4, a5
	if _, _, errno := syscall.Syscall6(syscall.SYS_SOCKETCALL, call, uintptr(unsafe.Pointer(&a)), 0, 0, 0, 0); errno != 0 {
		return errno
	}
	return nil
}
