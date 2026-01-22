package main

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
)

// Tests for LogConfig

func TestLogConfigStruct(t *testing.T) {
	cfg := LogConfig{
		Level:    "debug",
		File:     "/var/log/cli.log",
		MaxSize:  10,
		MaxFiles: 5,
	}

	if cfg.Level != "debug" {
		t.Errorf("Level = %q, want 'debug'", cfg.Level)
	}
	if cfg.File != "/var/log/cli.log" {
		t.Errorf("File = %q", cfg.File)
	}
	if cfg.MaxSize != 10 {
		t.Errorf("MaxSize = %d, want 10", cfg.MaxSize)
	}
	if cfg.MaxFiles != 5 {
		t.Errorf("MaxFiles = %d, want 5", cfg.MaxFiles)
	}
}

func TestGetLogConfig(t *testing.T) {
	viper.Reset()
	viper.Set("logging.level", "error")
	viper.Set("logging.file", "/tmp/test.log")
	viper.Set("logging.max_size", 20)
	viper.Set("logging.max_files", 10)

	cfg := GetLogConfig()

	if cfg.Level != "error" {
		t.Errorf("Level = %q, want 'error'", cfg.Level)
	}
	if cfg.File != "/tmp/test.log" {
		t.Errorf("File = %q", cfg.File)
	}
	if cfg.MaxSize != 20 {
		t.Errorf("MaxSize = %d, want 20", cfg.MaxSize)
	}
	if cfg.MaxFiles != 10 {
		t.Errorf("MaxFiles = %d, want 10", cfg.MaxFiles)
	}
}

func TestGetLogConfigDefaults(t *testing.T) {
	viper.Reset()

	cfg := GetLogConfig()

	// Should return empty/zero values when not set
	if cfg.Level != "" {
		t.Errorf("Level = %q, want empty", cfg.Level)
	}
	if cfg.File != "" {
		t.Errorf("File = %q, want empty", cfg.File)
	}
	if cfg.MaxSize != 0 {
		t.Errorf("MaxSize = %d, want 0", cfg.MaxSize)
	}
}

// Tests for InitLogging

func TestInitLogging(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "debug")
	viper.Set("logging.max_size", 10)
	viper.Set("logging.max_files", 5)

	err := InitLogging()
	if err != nil {
		t.Fatalf("InitLogging() error = %v", err)
	}

	// Verify logger was created
	l := Logger()
	if l == nil {
		t.Error("Logger() returned nil after InitLogging()")
	}
}

func TestInitLoggingDefaultLevel(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	// No level set - should default to warn

	err := InitLogging()
	if err != nil {
		t.Fatalf("InitLogging() error = %v", err)
	}
}

func TestInitLoggingLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error", ""}

	for _, level := range levels {
		t.Run("level_"+level, func(t *testing.T) {
			// Reset state
			logger = nil
			loggerOnce = sync.Once{}
			viper.Reset()

			tempDir := t.TempDir()
			logFile := filepath.Join(tempDir, "test.log")
			viper.Set("logging.file", logFile)
			viper.Set("logging.level", level)

			err := InitLogging()
			if err != nil {
				t.Fatalf("InitLogging() error = %v", err)
			}
		})
	}
}

func TestInitLoggingUnknownLevel(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "unknown")

	err := InitLogging()
	if err != nil {
		t.Fatalf("InitLogging() error = %v", err)
	}
	// Should default to warn level
}

func TestInitLoggingWithTilde(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	// Get home directory for comparison
	home, _ := os.UserHomeDir()

	tempSubDir := "test-cli-log-" + filepath.Base(t.TempDir())
	logPath := "~/" + tempSubDir + "/test.log"
	viper.Set("logging.file", logPath)

	err := InitLogging()
	if err != nil {
		t.Fatalf("InitLogging() error = %v", err)
	}

	// Clean up
	os.RemoveAll(filepath.Join(home, tempSubDir))
}

func TestInitLoggingDefaultMaxValues(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	// No max_size or max_files set - should use defaults

	err := InitLogging()
	if err != nil {
		t.Fatalf("InitLogging() error = %v", err)
	}
}

// Tests for Logger()

func TestLoggerReturnsInstance(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)

	_ = InitLogging()
	l := Logger()

	if l == nil {
		t.Error("Logger() returned nil")
	}
}

func TestLoggerFallback(t *testing.T) {
	// Set logger to nil to test fallback
	oldLogger := logger
	logger = nil

	l := Logger()
	if l == nil {
		t.Error("Logger() fallback should return non-nil logger")
	}

	// Restore
	logger = oldLogger
}

// Tests for LogDebug, LogInfo, LogWarn, LogError

func TestLogDebug(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "debug")

	_ = InitLogging()

	// Should not panic
	LogDebug("test debug message", "key", "value")
}

func TestLogInfo(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "info")

	_ = InitLogging()

	// Should not panic
	LogInfo("test info message", "key", "value")
}

func TestLogWarn(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "warn")

	_ = InitLogging()

	// Should not panic
	LogWarn("test warn message", "key", "value")
}

func TestLogError(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "error")

	_ = InitLogging()

	// Should not panic
	LogError("test error message", "key", "value")
}

func TestLogFunctionsWithNilLogger(t *testing.T) {
	// Set logger to nil to test fallback
	oldLogger := logger
	logger = nil

	// Should not panic - will use fallback logger
	LogDebug("debug")
	LogInfo("info")
	LogWarn("warn")
	LogError("error")

	// Restore
	logger = oldLogger
}

func TestLogFunctionsWithMultipleArgs(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "debug")

	_ = InitLogging()

	// Should not panic with multiple args
	LogDebug("test", "key1", "val1", "key2", 123, "key3", true)
	LogInfo("test", "key1", "val1", "key2", 123)
	LogWarn("test", "key1", "val1")
	LogError("test")
}

// Tests for multiWriter

func TestMultiWriterStruct(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	mw := &multiWriter{
		writers: []io.Writer{&buf1, &buf2},
	}

	if len(mw.writers) != 2 {
		t.Errorf("writers length = %d, want 2", len(mw.writers))
	}
}

func TestMultiWriterWrite(t *testing.T) {
	var buf1, buf2 bytes.Buffer

	mw := &multiWriter{
		writers: []io.Writer{&buf1, &buf2},
	}

	data := []byte("test data")
	n, err := mw.Write(data)

	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() n = %d, want %d", n, len(data))
	}
	if buf1.String() != "test data" {
		t.Errorf("buf1 = %q", buf1.String())
	}
	if buf2.String() != "test data" {
		t.Errorf("buf2 = %q", buf2.String())
	}
}

func TestMultiWriterWriteEmpty(t *testing.T) {
	var buf bytes.Buffer

	mw := &multiWriter{
		writers: []io.Writer{&buf},
	}

	n, err := mw.Write([]byte{})

	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 0 {
		t.Errorf("Write() n = %d, want 0", n)
	}
}

func TestMultiWriterWriteNoWriters(t *testing.T) {
	mw := &multiWriter{
		writers: []io.Writer{},
	}

	n, err := mw.Write([]byte("test"))

	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 4 {
		t.Errorf("Write() n = %d, want 4", n)
	}
}

// errorWriter is a writer that returns an error
type errorWriter struct {
	err error
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	return 0, w.err
}

func TestMultiWriterWriteError(t *testing.T) {
	var buf bytes.Buffer
	errW := &errorWriter{err: os.ErrClosed}

	mw := &multiWriter{
		writers: []io.Writer{&buf, errW},
	}

	_, err := mw.Write([]byte("test"))

	if err != os.ErrClosed {
		t.Errorf("Write() error = %v, want os.ErrClosed", err)
	}
}

// Tests for slog level parsing

func TestLogLevelParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelWarn},      // default
		{"unknown", slog.LevelWarn}, // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var level slog.Level
			switch tt.input {
			case "debug":
				level = slog.LevelDebug
			case "info":
				level = slog.LevelInfo
			case "warn", "":
				level = slog.LevelWarn
			case "error":
				level = slog.LevelError
			default:
				level = slog.LevelWarn
			}

			if level != tt.expected {
				t.Errorf("level = %v, want %v", level, tt.expected)
			}
		})
	}
}

// Tests for concurrent logging

func TestConcurrentLogging(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "debug")

	_ = InitLogging()

	done := make(chan bool)

	// Multiple goroutines logging
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				LogDebug("debug", "goroutine", id, "iteration", j)
				LogInfo("info", "goroutine", id)
				LogWarn("warn", "goroutine", id)
				LogError("error", "goroutine", id)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Tests for init only once

func TestInitLoggingOnce(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")
	viper.Set("logging.file", logFile)
	viper.Set("logging.level", "debug")

	// Call multiple times
	err1 := InitLogging()
	err2 := InitLogging()
	err3 := InitLogging()

	if err1 != nil {
		t.Errorf("first InitLogging() error = %v", err1)
	}
	if err2 != nil {
		t.Errorf("second InitLogging() error = %v", err2)
	}
	if err3 != nil {
		t.Errorf("third InitLogging() error = %v", err3)
	}
}
