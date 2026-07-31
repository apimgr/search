//go:build !windows

package backup

import "syscall"

// DiskUsage returns used and total bytes for the filesystem containing path.
// Per AI.md PART 21: used to enforce disk_threshold and max_total_size checks.
func DiskUsage(path string) (used, total uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}

	total = uint64(stat.Blocks) * uint64(stat.Bsize)
	free := uint64(stat.Bavail) * uint64(stat.Bsize)
	used = total - free

	return used, total, nil
}
