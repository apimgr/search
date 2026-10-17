//go:build windows

package server

import (
	"syscall"
	"unsafe"
)

// kernel32 GetDiskFreeSpaceExW is the Windows equivalent of statfs; it is
// resolved lazily so the binary still loads on systems without it.
var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceE = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// getDiskUsageUnix returns disk usage in bytes (used, total) for the volume
// backing path. Per AI.md PART 20 the system_disk_* metrics are reported for
// the data directory.
func getDiskUsageUnix(path string) (used, total uint64) {
	if path == "" {
		path = `C:\`
	}

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	ret, _, _ := procGetDiskFreeSpaceE.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return 0, 0
	}

	return totalBytes - totalFreeBytes, totalBytes
}
