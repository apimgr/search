// Package main provides CLI logging configuration
// Per AI.md PART 32: Logging configuration (lines 42749-42755)
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

	"github.com/apimgr/search/src/client/path"
)

var (
	// cliLogger is the structured logger for CLI operations
	logger     *slog.Logger
	loggerOnce sync.Once
)

// LogConfig holds logging configuration
// Per AI.md PART 32 lines 42749-42755
type LogConfig struct {
	// debug, info, warn, error (default: warn)
	Level string
	// Log file path (empty = {log_dir}/cli.log)
	File string
	// Max log file size in MB (default: 10)
	MaxSize int
	// Max log files to keep (default: 5)
	MaxFiles int
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
// Per AI.md PART 32: Comprehensive logging with log rotation
func InitLogging() error {
	var initErr error
	loggerOnce.Do(func() {
		cfg := GetLogConfig()

		// Determine log file path
		logPath := cfg.File
		if logPath == "" {
			logPath = path.LogFile()
		}

		// Expand ~ to home directory
		if len(logPath) > 0 && logPath[0] == '~' {
			home, _ := os.UserHomeDir()
			logPath = filepath.Join(home, logPath[1:])
		}

		// Ensure parent directory exists
		if err := path.EnsureFile(logPath, 0600); err != nil {
			initErr = fmt.Errorf("create log dir: %w", err)
			return
		}

		// Set up log rotation with lumberjack
		// Per AI.md PART 32: max_size and max_files config
		maxSize := cfg.MaxSize
		if maxSize == 0 {
			// Default 10 MB
			maxSize = 10
		}
		maxFiles := cfg.MaxFiles
		if maxFiles == 0 {
			// Default 5 files
			maxFiles = 5
		}

		rotatingWriter := &lumberjack.Logger{
			Filename: logPath,
			// MB
			MaxSize:    maxSize,
			MaxBackups: maxFiles,
			// days
			MaxAge:   30,
			Compress: true,
		}

		// Parse log level
		// Per AI.md PART 32: debug, info, warn, error (default: warn)
		var level slog.Level
		switch cfg.Level {
		case "debug":
			level = slog.LevelDebug
		case "info":
			level = slog.LevelInfo
		case "warn", "":
			// Default
			level = slog.LevelWarn
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
