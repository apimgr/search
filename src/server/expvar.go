package server

import (
	"expvar"
	"runtime"
	"time"
)

var (
	requestCount    = expvar.NewInt("requests_total")
	requestDuration = expvar.NewFloat("requests_duration_seconds")
	errorCount      = expvar.NewInt("errors_total")
	// expvarStartTime is the process start time for uptime reporting.
	expvarStartTime = time.Now()
)

func init() {
	// Publish uptime
	expvar.Publish("uptime_seconds", expvar.Func(func() any {
		return time.Since(expvarStartTime).Seconds()
	}))

	// Publish goroutine count (dynamic — reflects current value on each /debug/vars request)
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))

	// Publish memory stats
	expvar.Publish("memory", expvar.Func(func() any {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return map[string]uint64{
			"alloc":       m.Alloc,
			"total_alloc": m.TotalAlloc,
			"sys":         m.Sys,
			"heap_alloc":  m.HeapAlloc,
			"heap_sys":    m.HeapSys,
		}
	}))
}

// recordRequest records a request for expvar
func recordRequest(duration time.Duration) {
	requestCount.Add(1)
	requestDuration.Add(duration.Seconds())
}

// recordError records an error for expvar
func recordError() {
	errorCount.Add(1)
}
