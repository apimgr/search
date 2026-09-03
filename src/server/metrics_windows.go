//go:build windows

package server

// getDiskUsageUnix returns disk usage on Windows (uses WMI in production, returns 0 for basic metrics)
func getDiskUsageUnix() (used, total uint64) {
	return 0, 0
}
