//go:build !windows

package core

import "syscall"

// freeBytes 返回 dir 所在文件系统的可用字节数（syscall.Statfs，darwin/linux）。
// 生产环境是 Windows-only（见 diskspace_windows.go），这份实现存在的唯一目的
// 是让磁盘空间检查逻辑本身能在非 Windows 开发机（这台 macOS）上跑单元测试。
func freeBytes(dir string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
