package server

import "sync"

var cpuCounterState struct {
	sync.Mutex
	initialized bool
	total       uint64
	idle        uint64
}

func getCPUUsage() float64 {
	total, idle, ok := readCPUCounters()
	if !ok {
		return 0
	}
	return cpuUsageFromCounters(total, idle)
}

func cpuUsageFromCounters(total, idle uint64) float64 {
	cpuCounterState.Lock()
	defer cpuCounterState.Unlock()
	if !cpuCounterState.initialized || total <= cpuCounterState.total || idle < cpuCounterState.idle {
		cpuCounterState.total = total
		cpuCounterState.idle = idle
		cpuCounterState.initialized = true
		return 0
	}
	usage, ok := cpuUsageFromCounterDelta(cpuCounterState.total, cpuCounterState.idle, total, idle)
	cpuCounterState.total = total
	cpuCounterState.idle = idle
	if !ok {
		return 0
	}
	return usage
}

func cpuUsageFromCounterDelta(previousTotal, previousIdle, total, idle uint64) (float64, bool) {
	if total <= previousTotal || idle < previousIdle {
		return 0, false
	}
	totalDelta := total - previousTotal
	idleDelta := idle - previousIdle
	if totalDelta == 0 || idleDelta > totalDelta {
		return 0, false
	}
	return float64(totalDelta-idleDelta) / float64(totalDelta) * 100, true
}

func getMemoryUsage() (used, total uint64) {
	return getMemoryUsagePlatform()
}
