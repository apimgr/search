// Package main provides CLI logging configuration
// Per AI.md PART 36: Logging configuration (lines 42749-42755)
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/apimgr/search/src/client/paths"
)

var (
	// cliLogger is the structured logger for CLI operations
	logger     *slog.Logger
	loggerOnce sync.Once
)

// LogConfig holds logging configuration
// Per AI.md PART 36 lines 42749-42755
type LogConfig struct {
	Level    string // debug, info, warn, error (default: warn)
	File     string // Log file path (empty = {log_dir}/cli.log)
	MaxSize  int    // Max log file size in MB (default: 10)
	MaxFiles int    // Max log files to keep (default: 5)
}

// GetLogConfig returns logging configuration from viper
func GetLogConfig() LogConfig {
	return LogConfig{
		Level:    viper.GetString("logging.level"),
		File:     viper.GetString("logging.file"),
		MaxSize:  viper.GetInt("logging.max_size"),
		MaxFiles: viper.GetInt("logging.max_files"),
	}
}

// InitLogging initializes the CLI logger with configuration
// Per AI.md PART 36: Comprehensive logging with log rotation
func InitLogging() error {
	var initErr error
	loggerOnce.Do(func() {
		cfg := GetLogConfig()

		// Determine log file path
		logPath := cfg.File
		if logPath == "" {
			logPath = paths.LogFile()
		}

		// Expand ~ to home directory
		if len(logPath) > 0 && logPath[0] == '~' {
			home, _ := os.UserHomeDir()
			logPath = filepath.Join(home, logPath[1:])
		}

		// Ensure parent directory exists
		if err := paths.EnsureFile(logPath, 0600); err != nil {
			initErr = fmt.Errorf("create log dir: %w", err)
			return
		}

		// Set up log rotation with lumberjack
		// Per AI.md PART 36: max_size and max_files config
		maxSize := cfg.MaxSize
		if maxSize == 0 {
			maxSize = 10 // Default 10 MB
		}
		maxFiles := cfg.MaxFiles
		if maxFiles == 0 {
			maxFiles = 5 // Default 5 files
		}

		rotatingWriter := &lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    maxSize, // MB
			MaxBackups: maxFiles,
			MaxAge:     30, // days
			Compress:   true,
		}

		// Parse log level
		// Per AI.md PART 36: debug, info, warn, error (default: warn)
		var level slog.Level
		switch cfg.Level {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn", "":
			level = slog.LevelWarn // Default
		case "error":
			level = slog.LevelError
		default:
			level = slog.LevelWarn
		}

		// Create handler options
		opts := &slog.HandlerOptions{
			Level: level,
		}

		// Create JSON handler for structured logging
		handler := slog.NewJSONHandler(rotatingWriter, opts)

		// Create logger
		logger = slog.New(handler)

		// Also set as default logger
		slog.SetDefault(logger)
	})
	return initErr
}

// Logger returns the CLI logger
func Logger() *slog.Logger {
	if logger == nil {
		// Fallback to stderr if not initialized
		return slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return logger
}

// LogDebug logs a debug message
func LogDebug(msg string, args ...any) {
	Logger().Debug(msg, args...)
}

// LogInfo logs an info message
func LogInfo(msg string, args ...any) {
	Logger().Info(msg, args...)
}

// LogWarn logs a warning message
func LogWarn(msg string, args ...any) {
	Logger().Warn(msg, args...)
}

// LogError logs an error message
func LogError(msg string, args ...any) {
	Logger().Error(msg, args...)
}

// multiWriter creates a writer that writes to multiple destinations
type multiWriter struct {
	writers []io.Writer
}

func (mw *multiWriter) Write(p []byte) (n int, err error) {
	for _, w := range mw.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
	}
	return len(p), nil
}
