//go:build !windows

package server

import (
	"syscall"
)

// getDiskUsageUnix returns disk usage in bytes (used, total) for the filesystem
// backing path, using syscall.Statfs (Unix only). Per AI.md PART 20 the
// system_disk_* metrics are reported for the data directory.
func getDiskUsageUnix(path string) (used, total uint64) {
	if path == "" {
		path = "/"
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}

	// Calculate total and used bytes
	total = stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used = total - free

	return used, total
}
