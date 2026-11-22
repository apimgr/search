//go:build darwin || freebsd

package server

import (
	"encoding/binary"
	"runtime"

	"golang.org/x/sys/unix"
)

func readCPUCounters() (total, idle uint64, ok bool) {
	data, err := unix.SysctlRaw("kern.cp_time")
	if err != nil || len(data) == 0 {
		return 0, 0, false
	}
	width, states := 0, 0
	switch len(data) {
	case 16:
		width, states = 4, 4
	case 20:
		width, states = 4, 5
	case 32:
		width, states = 8, 4
	case 40:
		width, states = 8, 5
	default:
		return 0, 0, false
	}
	for i := 0; i < states; i++ {
		var value uint64
		if width == 8 {
			value = binary.NativeEndian.Uint64(data[i*width : (i+1)*width])
		} else {
			value = uint64(binary.NativeEndian.Uint32(data[i*width : (i+1)*width]))
		}
		total += value
		if i == states-1 {
			idle = value
		}
	}
	return total, idle, total > 0
}

func getMemoryUsagePlatform() (used, total uint64) {
	totalKey := "hw.memsize"
	if runtime.GOOS == "freebsd" {
		totalKey = "hw.physmem"
	}
	total, err := unix.SysctlUint64(totalKey)
	if err != nil || total == 0 {
		return 0, 0
	}
	pageSize := uint64(unix.Getpagesize())
	var availablePages uint64
	keys := []string{"vm.page_free_count", "vm.page_inactive_count", "vm.page_purgeable_count"}
	if runtime.GOOS == "freebsd" {
		keys = []string{"vm.stats.vm.v_free_count", "vm.stats.vm.v_inactive_count", "vm.stats.vm.v_cache_count"}
	}
	for _, key := range keys {
		pages, err := unix.SysctlUint64(key)
		if err == nil {
			availablePages += pages
		}
	}
	available := availablePages * pageSize
	if pageSize == 0 || availablePages == 0 || available > total {
		return 0, 0
	}
	return total - available, total
}
