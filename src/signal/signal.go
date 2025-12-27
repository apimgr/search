// Package signal provides cross-platform signal handling for graceful shutdown.
// Per AI.md PART 7: Signal handling must be platform-dependent with build tags.
package signal

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// ShutdownConfig holds configuration for graceful shutdown
type ShutdownConfig struct {
	// Server is the HTTP server to shut down (optional if ShutdownFunc is provided)
	Server *http.Server
	// ShutdownFunc is called to perform shutdown (alternative to Server)
	// If provided, this is called instead of Server.Shutdown
	ShutdownFunc     func(ctx context.Context) error
	PIDFile          string
	InFlightTimeout  time.Duration // Default: 30s
	ChildTimeout     time.Duration // Default: 10s
	DatabaseTimeout  time.Duration // Default: 5s
	LogFlushTimeout  time.Duration // Default: 2s
	OnReopenLogs     func()        // Called on SIGUSR1 (Unix only)
	OnDumpStatus     func()        // Called on SIGUSR2 (Unix only)
	OnCloseDatabase  func()        // Called during shutdown
	OnFlushLogs      func()        // Called during shutdown
	GetChildPIDs     func() []int  // Returns child process PIDs
}

var (
	shuttingDown bool
	shutdownMu   sync.RWMutex
)

// IsShuttingDown returns true if graceful shutdown is in progress
func IsShuttingDown() bool {
	shutdownMu.RLock()
	defer shutdownMu.RUnlock()
	return shuttingDown
}

// setShuttingDown sets the shutdown flag
func setShuttingDown(v bool) {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	shuttingDown = v
}

// Setup configures signal handling with the given config
// This is the main entry point - it calls the platform-specific setupSignals
func Setup(cfg ShutdownConfig) {
	// Apply defaults
	if cfg.InFlightTimeout == 0 {
		cfg.InFlightTimeout = 30 * time.Second
	}
	if cfg.ChildTimeout == 0 {
		cfg.ChildTimeout = 10 * time.Second
	}
	if cfg.DatabaseTimeout == 0 {
		cfg.DatabaseTimeout = 5 * time.Second
	}
	if cfg.LogFlushTimeout == 0 {
		cfg.LogFlushTimeout = 2 * time.Second
	}

	setupSignals(cfg)
}

// gracefulShutdown performs orderly shutdown (cross-platform)
// Per AI.md PART 7: Shutdown sequence with specific timeouts
func gracefulShutdown(cfg ShutdownConfig) {
	// Step 1-3: Set shutdown flag for health checks (returns 503)
	setShuttingDown(true)
	log.Println("Shutdown flag set, health checks now return 503")

	// Step 4: Stop accepting new connections, wait for in-flight requests
	ctx, cancel := context.WithTimeout(context.Background(), cfg.InFlightTimeout)
	defer cancel()

	// Use ShutdownFunc if provided, otherwise use Server.Shutdown
	if cfg.ShutdownFunc != nil {
		log.Printf("Calling shutdown function (timeout: %v)...", cfg.InFlightTimeout)
		if err := cfg.ShutdownFunc(ctx); err != nil {
			log.Printf("Shutdown function error: %v", err)
		} else {
			log.Println("Shutdown function completed successfully")
		}
	} else if cfg.Server != nil {
		log.Printf("Waiting up to %v for in-flight requests...", cfg.InFlightTimeout)
		if err := cfg.Server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		} else {
			log.Println("HTTP server stopped accepting connections")
		}
	}

	// Step 5: Close database connections
	if cfg.OnCloseDatabase != nil {
		log.Printf("Closing database connections (timeout: %v)...", cfg.DatabaseTimeout)
		done := make(chan struct{})
		go func() {
			cfg.OnCloseDatabase()
			close(done)
		}()
		select {
		case <-done:
			log.Println("Database connections closed")
		case <-time.After(cfg.DatabaseTimeout):
			log.Println("WARNING: Database close timeout exceeded, continuing shutdown")
		}
	}

	// Step 6: Flush logs and metrics
	if cfg.OnFlushLogs != nil {
		log.Printf("Flushing logs (timeout: %v)...", cfg.LogFlushTimeout)
		done := make(chan struct{})
		go func() {
			cfg.OnFlushLogs()
			close(done)
		}()
		select {
		case <-done:
			log.Println("Logs flushed")
		case <-time.After(cfg.LogFlushTimeout):
			log.Println("WARNING: Log flush timeout exceeded, skipping")
		}
	}

	// Step 7-8: Stop child processes (platform-specific, handled by caller or stopChildProcesses)
	if cfg.GetChildPIDs != nil {
		pids := cfg.GetChildPIDs()
		if len(pids) > 0 {
			log.Printf("Stopping %d child processes (timeout: %v)...", len(pids), cfg.ChildTimeout)
			stopChildProcesses(pids, cfg.ChildTimeout)
		}
	}

	// Step 9: Remove PID file
	if cfg.PIDFile != "" {
		if err := os.Remove(cfg.PIDFile); err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to remove PID file: %v", err)
		} else {
			log.Printf("Removed PID file: %s", cfg.PIDFile)
		}
	}

	// Step 10: Exit 0
	log.Println("Graceful shutdown complete")
	os.Exit(0)
}

// reopenLogs calls the log reopen callback if configured
func reopenLogs(cfg ShutdownConfig) {
	if cfg.OnReopenLogs != nil {
		cfg.OnReopenLogs()
	} else {
		log.Println("Log reopen requested but no handler configured")
	}
}

// dumpStatus calls the status dump callback if configured
func dumpStatus(cfg ShutdownConfig) {
	if cfg.OnDumpStatus != nil {
		cfg.OnDumpStatus()
	} else {
		log.Println("Status dump requested but no handler configured")
	}
}
