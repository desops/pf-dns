package pledge

import (
	"syscall"
	"unsafe"
)

func pledge(promises string, paths []string) (err error) {
	promise, err := syscall.BytePtrFromString(promises)
	if err != nil {
		return err
	}
	_, _, e := syscall.Syscall(108, uintptr(unsafe.Pointer(promise)), uintptr(unsafe.Pointer(nil)), 0)
	return e
}
