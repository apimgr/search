//go:build linux

package server

import (
	"os"
	"strconv"
	"strings"
)

func readCPUCounters() (total, idle uint64, ok bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	lineEnd := strings.IndexByte(string(data), '\n')
	if lineEnd < 0 {
		lineEnd = len(data)
	}
	fields := strings.Fields(string(data[:lineEnd]))
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	for i, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		// Linux guest and guest_nice are already included in user and nice.
		if i < 8 {
			total += value
		}
		if i == 3 || i == 4 {
			idle += value
		}
	}
	return total, idle, total > 0
}

func getMemoryUsagePlatform() (used, total uint64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var availableKiB uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = value * 1024
		case "MemAvailable:":
			availableKiB = value
		}
	}
	available := availableKiB * 1024
	if total == 0 || available > total {
		return 0, 0
	}
	return total - available, total
}
