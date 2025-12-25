//go:build windows

package server

// getDiskUsageUnix is a stub for Windows (not implemented)
func getDiskUsageUnix() (used, total uint64) {
	return 0, 0
}
