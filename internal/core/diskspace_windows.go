//go:build windows

package core

import (
	"syscall"
	"unsafe"
)

// freeBytes 返回 dir 所在磁盘卷的可用字节数，通过 kernel32.dll!GetDiskFreeSpaceExW
// 直接调用（标准库 syscall 包，不引入 golang.org/x/sys 之类的额外依赖）。
//
// 这份实现只在 Windows 上编译（//go:build windows），无法在这台 macOS 开发机
// 上验证——已在 issues/03-kairos-asr-paraformer.md 同类场景里用过同样的
// build-tag 隔离手法，见 internal/asr/paraformer_windows.go。
func freeBytes(dir string) (uint64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	dirPtr, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}

	var freeBytesAvailable uint64
	ret, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		0, // lpTotalNumberOfBytes，不需要
		0, // lpTotalNumberOfFreeBytes，不需要
	)
	if ret == 0 {
		return 0, callErr
	}
	return freeBytesAvailable, nil
}
