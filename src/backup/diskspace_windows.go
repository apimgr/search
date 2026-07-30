//go:build windows

package backup

import (
	"fmt"
	"syscall"
	"unsafe"
)

// DiskUsage returns used and total bytes for the volume containing path.
// Per AI.md PART 21: used to enforce disk_threshold and max_total_size checks.
// Uses GetDiskFreeSpaceExW via a lazy DLL load to avoid an extra module
// dependency beyond the standard library.
func DiskUsage(path string) (used, total uint64, err error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW := kernel32.NewProc("GetDiskFreeSpaceExW")

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to convert path: %w", err)
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64

	ret, _, callErr := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if ret == 0 {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceExW failed: %w", callErr)
	}

	total = totalBytes
	used = totalBytes - totalFreeBytes

	return used, total, nil
}
