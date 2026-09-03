//go:build !windows

package server

import (
	"syscall"
)

// getDiskUsageUnix returns disk usage using syscall.Statfs (Unix only)
func getDiskUsageUnix() (used, total uint64) {
	var stat syscall.Statfs_t

	// Check root filesystem
	if err := syscall.Statfs("/", &stat); err != nil {
		return 0, 0
	}

	// Calculate total and used bytes
	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used = total - free

	return used, total
}
