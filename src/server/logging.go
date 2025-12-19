package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
)

// LogLevel represents logging levels
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the string representation of a log level
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel parses a log level string
func ParseLogLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// Logger provides structured logging
type Logger struct {
	config      *config.Config
	level       LogLevel
	output      io.Writer
	file        *os.File
	mu          sync.Mutex
	format      string
	fields      map[string]interface{}
	colorOutput bool
}

// NewLogger creates a new logger
func NewLogger(cfg *config.Config) *Logger {
	l := &Logger{
		config:      cfg,
		level:       ParseLogLevel(cfg.Server.Logs.Level),
		output:      os.Stdout,
		format:      cfg.Server.Logs.Format,
		fields:      make(map[string]interface{}),
		colorOutput: true,
	}

	// Set up file logging if configured
	if cfg.Server.Logs.File != "" {
		if err := l.setupFileLogging(cfg.Server.Logs.File); err != nil {
			log.Printf("Failed to setup file logging: %v", err)
		}
	}

	return l
}

// setupFileLogging sets up logging to a file
func (l *Logger) setupFileLogging(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Open file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	l.file = file
	l.output = io.MultiWriter(os.Stdout, file)
	l.colorOutput = false // Disable color for file output

	return nil
}

// Close closes the logger
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// WithField returns a logger with an additional field
func (l *Logger) WithField(key string, value interface{}) *Logger {
	newLogger := &Logger{
		config:      l.config,
		level:       l.level,
		output:      l.output,
		file:        l.file,
		format:      l.format,
		fields:      make(map[string]interface{}),
		colorOutput: l.colorOutput,
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value

	return newLogger
}

// WithFields returns a logger with additional fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	newLogger := &Logger{
		config:      l.config,
		level:       l.level,
		output:      l.output,
		file:        l.file,
		format:      l.format,
		fields:      make(map[string]interface{}),
		colorOutput: l.colorOutput,
	}

	// Copy existing fields
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	// Add new fields
	for k, v := range fields {
		newLogger.fields[k] = v
	}

	return newLogger
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// log writes a log entry
func (l *Logger) log(level LogLevel, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Format message if args provided
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}

	// Build log entry
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   msg,
		Fields:    l.fields,
	}

	// Add caller info for error and above
	if level >= LevelError {
		_, file, line, ok := runtime.Caller(2)
		if ok {
			entry.File = filepath.Base(file)
			entry.Line = line
		}
	}

	// Write entry
	if l.format == "json" {
		l.writeJSON(entry)
	} else {
		l.writeText(entry)
	}
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
}

// writeJSON writes a JSON log entry
func (l *Logger) writeJSON(entry LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.output.Write(append(data, '\n'))
}

// writeText writes a text log entry
func (l *Logger) writeText(entry LogEntry) {
	var color string
	var reset string

	if l.colorOutput {
		reset = "\033[0m"
		switch entry.Level {
		case "DEBUG":
			color = "\033[36m" // Cyan
		case "INFO":
			color = "\033[32m" // Green
		case "WARN":
			color = "\033[33m" // Yellow
		case "ERROR":
			color = "\033[31m" // Red
		case "FATAL":
			color = "\033[35m" // Magenta
		}
	}

	// Format: [2024-01-15 10:30:45] [INFO] Message
	timestamp := entry.Timestamp.Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s[%s]%s %s", timestamp, color, entry.Level, reset, entry.Message)

	// Add fields if present
	if len(entry.Fields) > 0 {
		fields := make([]string, 0, len(entry.Fields))
		for k, v := range entry.Fields {
			fields = append(fields, fmt.Sprintf("%s=%v", k, v))
		}
		line += " " + strings.Join(fields, " ")
	}

	// Add file:line for errors
	if entry.File != "" {
		line += fmt.Sprintf(" (%s:%d)", entry.File, entry.Line)
	}

	l.output.Write([]byte(line + "\n"))
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.log(LevelDebug, msg, args...)
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...interface{}) {
	l.log(LevelInfo, msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.log(LevelWarn, msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...interface{}) {
	l.log(LevelError, msg, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(msg string, args ...interface{}) {
	l.log(LevelFatal, msg, args...)
	os.Exit(1)
}

// RequestLogger returns a logger for HTTP requests
func (l *Logger) RequestLogger(method, path, ip string, status int, latency time.Duration) {
	l.WithFields(map[string]interface{}{
		"method":     method,
		"path":       path,
		"ip":         ip,
		"status":     status,
		"latency_ms": latency.Milliseconds(),
	}).Info("HTTP Request")
}

// Rotate rotates the log file
func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	// Close current file
	if err := l.file.Close(); err != nil {
		return err
	}

	// Rename current file with timestamp
	path := l.config.Server.Logs.File
	timestamp := time.Now().Format("20060102-150405")
	rotatedPath := fmt.Sprintf("%s.%s", path, timestamp)

	if err := os.Rename(path, rotatedPath); err != nil {
		return err
	}

	// Open new file
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	l.file = file
	l.output = io.MultiWriter(os.Stdout, file)

	return nil
}
