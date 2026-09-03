// Package signal provides cross-platform signal handling for graceful shutdown.
// Per AI.md PART 7: Signal handling must be platform-dependent with build tags.
package signal

import (
	"context"
	"log/slog"
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
	ShutdownFunc func(ctx context.Context) error
	PIDFile      string
	// Default: 30s
	InFlightTimeout time.Duration
	// Default: 10s
	ChildTimeout time.Duration
	// Default: 5s
	DatabaseTimeout time.Duration
	// Default: 2s
	LogFlushTimeout time.Duration
	// Called on SIGUSR1 (Unix only)
	OnReopenLogs func()
	// Called on SIGUSR2 (Unix only)
	OnDumpStatus func()
	// Called during shutdown
	OnCloseDatabase func()
	// Called during shutdown
	OnFlushLogs func()
	// Returns child process PIDs
	GetChildPIDs func() []int
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

// Setup configures signal handling with the given config.
// It returns a channel that is closed when graceful shutdown completes;
// main() should receive from it and then call os.Exit(0).
func Setup(cfg ShutdownConfig) <-chan struct{} {
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

	done := make(chan struct{})
	setupSignals(cfg, done)
	return done
}

// gracefulShutdown performs orderly shutdown (cross-platform).
// It closes done when all shutdown steps complete, signalling main() to call os.Exit(0).
// Per AI.md PART 7: Shutdown sequence with specific timeouts.
// Per AI.md PART 7: os.Exit() is forbidden outside main(); done channel conveys exit intent.
func gracefulShutdown(cfg ShutdownConfig, done chan struct{}) {
	// Step 1-3: Set shutdown flag for health checks (returns 503)
	setShuttingDown(true)
	slog.Info("Shutdown flag set, health checks now return 503")

	// Step 4: Stop accepting new connections, wait for in-flight requests
	ctx, cancel := context.WithTimeout(context.Background(), cfg.InFlightTimeout)
	defer cancel()

	// Use ShutdownFunc if provided, otherwise use Server.Shutdown
	if cfg.ShutdownFunc != nil {
		slog.Info("Calling shutdown function", "timeout", cfg.InFlightTimeout)
		if err := cfg.ShutdownFunc(ctx); err != nil {
			slog.Error("Shutdown function error", "err", err)
		} else {
			slog.Info("Shutdown function completed successfully")
		}
	} else if cfg.Server != nil {
		slog.Info("Waiting for in-flight requests", "timeout", cfg.InFlightTimeout)
		if err := cfg.Server.Shutdown(ctx); err != nil {
			slog.Error("HTTP server shutdown error", "err", err)
		} else {
			slog.Info("HTTP server stopped accepting connections")
		}
	}

	// Step 5: Close database connections
	if cfg.OnCloseDatabase != nil {
		slog.Info("Closing database connections", "timeout", cfg.DatabaseTimeout)
		dbDone := make(chan struct{})
		go func() {
			cfg.OnCloseDatabase()
			close(dbDone)
		}()
		select {
		case <-dbDone:
			slog.Info("Database connections closed")
		case <-time.After(cfg.DatabaseTimeout):
			slog.Warn("Database close timeout exceeded, continuing shutdown")
		}
	}

	// Step 6: Flush logs and metrics
	if cfg.OnFlushLogs != nil {
		slog.Info("Flushing logs", "timeout", cfg.LogFlushTimeout)
		logDone := make(chan struct{})
		go func() {
			cfg.OnFlushLogs()
			close(logDone)
		}()
		select {
		case <-logDone:
			slog.Info("Logs flushed")
		case <-time.After(cfg.LogFlushTimeout):
			slog.Warn("Log flush timeout exceeded, skipping")
		}
	}

	// Step 7-8: Stop child processes (platform-specific, handled by caller or stopChildProcesses)
	if cfg.GetChildPIDs != nil {
		pids := cfg.GetChildPIDs()
		if len(pids) > 0 {
			slog.Info("Stopping child processes", "count", len(pids), "timeout", cfg.ChildTimeout)
			stopChildProcesses(pids, cfg.ChildTimeout)
		}
	}

	// Step 9: Remove PID file
	if cfg.PIDFile != "" {
		if err := os.Remove(cfg.PIDFile); err != nil && !os.IsNotExist(err) {
			slog.Error("Failed to remove PID file", "path", cfg.PIDFile, "err", err)
		} else {
			slog.Info("Removed PID file", "path", cfg.PIDFile)
		}
	}

	// Step 10: Signal completion so main() can call os.Exit(0)
	slog.Info("Graceful shutdown complete")
	close(done)
}

// reopenLogs calls the log reopen callback if configured
func reopenLogs(cfg ShutdownConfig) {
	if cfg.OnReopenLogs != nil {
		cfg.OnReopenLogs()
	} else {
		slog.Warn("Log reopen requested but no handler configured")
	}
}

// dumpStatus calls the status dump callback if configured
func dumpStatus(cfg ShutdownConfig) {
	if cfg.OnDumpStatus != nil {
		cfg.OnDumpStatus()
	} else {
		slog.Warn("Status dump requested but no handler configured")
	}
}
