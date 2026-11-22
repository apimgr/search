//go:build windows

package server

import (
	"syscall"
	"unsafe"
)

var (
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func readCPUCounters() (total, idle uint64, ok bool) {
	var idleTime, kernelTime, userTime syscall.Filetime
	result, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleTime)),
		uintptr(unsafe.Pointer(&kernelTime)),
		uintptr(unsafe.Pointer(&userTime)),
	)
	if result == 0 {
		return 0, 0, false
	}
	idle = uint64(idleTime.HighDateTime)<<32 | uint64(idleTime.LowDateTime)
	kernel := uint64(kernelTime.HighDateTime)<<32 | uint64(kernelTime.LowDateTime)
	user := uint64(userTime.HighDateTime)<<32 | uint64(userTime.LowDateTime)
	total = kernel + user
	return total, idle, total > 0
}

func getMemoryUsagePlatform() (used, total uint64) {
	status := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	result, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if result == 0 || status.TotalPhys == 0 || status.AvailPhys > status.TotalPhys {
		return 0, 0
	}
	return status.TotalPhys - status.AvailPhys, status.TotalPhys
}
