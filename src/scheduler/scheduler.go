// Package scheduler provides a built-in task scheduler per AI.md PART 19
// The scheduler is ALWAYS RUNNING - there is no enable/disable option.
// All scheduled tasks are managed internally, never via external cron/schedulers.
package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultTimezone is the default timezone for the scheduler (allows testing)
var defaultTimezone = "America/New_York"

// TaskID represents a unique task identifier
type TaskID string

// Built-in task IDs per AI.md PART 19
const (
	TaskSSLRenewal       TaskID = "ssl.renewal"
	TaskGeoIPUpdate      TaskID = "geoip.update"
	TaskBlocklistUpdate  TaskID = "blocklist.update"
	TaskCVEUpdate        TaskID = "cve.update"
	TaskSessionCleanup   TaskID = "session.cleanup"
	TaskTokenCleanup     TaskID = "token.cleanup"
	TaskLogRotation      TaskID = "log.rotation"
	TaskBackupDaily      TaskID = "backup_daily"
	TaskBackupHourly     TaskID = "backup_hourly"
	TaskHealthcheckSelf  TaskID = "healthcheck.self"
	TaskTorHealth        TaskID = "tor.health"
	TaskClusterHeartbeat TaskID = "cluster.heartbeat"
)

// TaskStatus represents task execution status
type TaskStatus string

const (
	StatusSuccess  TaskStatus = "success"
	StatusFailed   TaskStatus = "failed"
	StatusSkipped  TaskStatus = "skipped"
	StatusRunning  TaskStatus = "running"
	StatusRetrying TaskStatus = "retrying"
)

// TaskType determines how tasks run in cluster mode
type TaskType string

const (
	// TaskTypeGlobal runs on ONE node only (leader election)
	TaskTypeGlobal TaskType = "global"
	// TaskTypeLocal runs on EVERY node
	TaskTypeLocal TaskType = "local"
)

// Default retry policy values per AI.md PART 19
const (
	DefaultMaxRetries = 3
	DefaultRetryDelay = 5 * time.Minute
)

// Task represents a scheduled task per AI.md PART 19
type Task struct {
	ID          TaskID
	Name        string
	Description string
	Schedule    string         // Cron expression or @every interval
	TaskType    TaskType       // Global or Local
	Run         func(ctx context.Context) error
	Skippable   bool           // Can admin disable this task?
	RunOnStart  bool           // Run immediately on scheduler start?

	// Retry policy per AI.md PART 19
	// Default: max_retries=3, retry_delay=5m, backoff=exponential (5m, 10m, 20m)
	MaxRetries  int           // Maximum retry attempts (default: 3)
	RetryDelay  time.Duration // Base delay between retries (default: 5m)

	// Runtime state (persisted to database)
	LastRun     time.Time
	LastStatus  TaskStatus
	LastError   string
	NextRun     time.Time
	RunCount    int64
	FailCount   int64
	Enabled     bool

	// Retry state
	RetryCount  int       // Current retry attempt (0 = first run)
	NextRetry   time.Time // Scheduled retry time (if retrying)

	// Cluster locking
	LockedBy    string
	LockedAt    time.Time
}

// TaskState represents persisted task state in database
type TaskState struct {
	TaskID     string
	TaskName   string
	Schedule   string
	LastRun    time.Time
	LastStatus string
	LastError  string
	NextRun    time.Time
	RunCount   int64
	FailCount  int64
	Enabled    bool
	LockedBy   string
	LockedAt   time.Time
}

// TaskFailureNotification contains details about a failed task
// Per AI.md PART 19: Failed tasks trigger notifications (if configured)
type TaskFailureNotification struct {
	TaskID      string
	TaskName    string
	Error       string
	Attempts    int
	LastRun     time.Time
	FailCount   int64
}

// NotifyFunc is a callback function for task failure notifications
// Per AI.md PART 19: Failed tasks trigger notifications (if configured)
type NotifyFunc func(notification *TaskFailureNotification)

// Scheduler manages periodic tasks per AI.md PART 19
// The scheduler is ALWAYS RUNNING - no enable/disable option exists
type Scheduler struct {
	mu            sync.RWMutex
	tasks         map[TaskID]*Task
	db            *sql.DB
	nodeID        string
	ctx           context.Context
	cancel        context.CancelFunc
	running       bool
	wg            sync.WaitGroup
	timezone      *time.Location
	catchUpWindow time.Duration
	notifyFunc    NotifyFunc // Per AI.md PART 19: Task failure notifications
}

// New creates a new scheduler
// Per AI.md PART 19: Scheduler is ALWAYS RUNNING
func New(db *sql.DB, nodeID string) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	// Default to America/New_York per spec
	tz, err := time.LoadLocation(defaultTimezone)
	if err != nil {
		tz = time.Local
	}

	s := &Scheduler{
		tasks:         make(map[TaskID]*Task),
		db:            db,
		nodeID:        nodeID,
		ctx:           ctx,
		cancel:        cancel,
		timezone:      tz,
		catchUpWindow: 1 * time.Hour, // Default catch-up window
	}

	// Initialize database tables early so Register() can save state
	if db != nil {
		s.initDatabase()
	}

	return s
}

// SetTimezone sets the timezone for scheduled tasks
func (s *Scheduler) SetTimezone(tz string) error {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("invalid timezone %s: %w", tz, err)
	}
	s.mu.Lock()
	s.timezone = loc
	s.mu.Unlock()
	return nil
}

// SetCatchUpWindow sets the catch-up window for missed tasks
func (s *Scheduler) SetCatchUpWindow(d time.Duration) {
	s.mu.Lock()
	s.catchUpWindow = d
	s.mu.Unlock()
}

// SetNotifyFunc sets the callback function for task failure notifications
// Per AI.md PART 19: Failed tasks trigger notifications (if configured)
func (s *Scheduler) SetNotifyFunc(fn NotifyFunc) {
	s.mu.Lock()
	s.notifyFunc = fn
	s.mu.Unlock()
}

// Register adds a task to the scheduler
func (s *Scheduler) Register(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	if task.Schedule == "" {
		return fmt.Errorf("task schedule is required")
	}

	if task.Run == nil {
		return fmt.Errorf("task run function is required")
	}

	// Calculate next run time
	task.NextRun = s.calculateNextRun(task.Schedule)
	task.Enabled = true

	s.tasks[task.ID] = task

	// Load persisted state if available
	if s.db != nil {
		s.loadTaskState(task)
	}

	return nil
}

// RegisterBuiltinTasks registers all required tasks per AI.md PART 19
func (s *Scheduler) RegisterBuiltinTasks(handlers *TaskHandlers) {
	// SSL Renewal - Daily at 03:00, NOT skippable
	if handlers.SSLRenewal != nil {
		s.Register(&Task{
			ID:          TaskSSLRenewal,
			Name:        "SSL Certificate Renewal",
			Description: "Check and renew SSL certificates 7 days before expiry",
			Schedule:    "0 3 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.SSLRenewal,
			Skippable:   false,
			RunOnStart:  true,
			Enabled:     true,
		})
	}

	// GeoIP Update - Weekly Sunday at 03:00, skippable
	if handlers.GeoIPUpdate != nil {
		s.Register(&Task{
			ID:          TaskGeoIPUpdate,
			Name:        "GeoIP Database Update",
			Description: "Download and update MaxMind GeoLite2 databases",
			Schedule:    "0 3 * * 0",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.GeoIPUpdate,
			Skippable:   true,
			Enabled:     true,
		})
	}

	// Blocklist Update - Daily at 04:00, skippable
	if handlers.BlocklistUpdate != nil {
		s.Register(&Task{
			ID:          TaskBlocklistUpdate,
			Name:        "Blocklist Update",
			Description: "Download and update IP/domain blocklists",
			Schedule:    "0 4 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.BlocklistUpdate,
			Skippable:   true,
			Enabled:     true,
		})
	}

	// CVE Update - Daily at 05:00, skippable
	if handlers.CVEUpdate != nil {
		s.Register(&Task{
			ID:          TaskCVEUpdate,
			Name:        "CVE Database Update",
			Description: "Download and update CVE/security databases",
			Schedule:    "0 5 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.CVEUpdate,
			Skippable:   true,
			Enabled:     true,
		})
	}

	// Session Cleanup - Every 15 minutes, NOT skippable
	if handlers.SessionCleanup != nil {
		s.Register(&Task{
			ID:          TaskSessionCleanup,
			Name:        "Session Cleanup",
			Description: "Remove expired sessions",
			Schedule:    "@every 15m",
			TaskType:    TaskTypeLocal,
			Run:         handlers.SessionCleanup,
			Skippable:   false,
			Enabled:     true,
		})
	}

	// Token Cleanup - Every 15 minutes, NOT skippable
	if handlers.TokenCleanup != nil {
		s.Register(&Task{
			ID:          TaskTokenCleanup,
			Name:        "Token Cleanup",
			Description: "Remove expired tokens",
			Schedule:    "@every 15m",
			TaskType:    TaskTypeLocal,
			Run:         handlers.TokenCleanup,
			Skippable:   false,
			Enabled:     true,
		})
	}

	// Log Rotation - Daily at 00:00, NOT skippable
	if handlers.LogRotation != nil {
		s.Register(&Task{
			ID:          TaskLogRotation,
			Name:        "Log Rotation",
			Description: "Rotate and compress old logs",
			Schedule:    "0 0 * * *",
			TaskType:    TaskTypeLocal,
			Run:         handlers.LogRotation,
			Skippable:   false,
			Enabled:     true,
		})
	}

	// Backup Daily - Daily at 02:00, skippable
	if handlers.BackupDaily != nil {
		s.Register(&Task{
			ID:          TaskBackupDaily,
			Name:        "Daily Backup",
			Description: "Full backup with daily incremental",
			Schedule:    "0 2 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.BackupDaily,
			Skippable:   true,
			Enabled:     true,
		})
	}

	// Backup Hourly - Hourly, skippable, disabled by default
	if handlers.BackupHourly != nil {
		task := &Task{
			ID:          TaskBackupHourly,
			Name:        "Hourly Backup",
			Description: "Hourly incremental backup",
			Schedule:    "@hourly",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.BackupHourly,
			Skippable:   true,
		}
		s.Register(task)
		// Disable after registration (Register sets Enabled=true by default)
		task.Enabled = false
	}

	// Health Check Self - Every 5 minutes, NOT skippable
	if handlers.HealthcheckSelf != nil {
		s.Register(&Task{
			ID:          TaskHealthcheckSelf,
			Name:        "Self Health Check",
			Description: "Self-health verification",
			Schedule:    "@every 5m",
			TaskType:    TaskTypeLocal,
			Run:         handlers.HealthcheckSelf,
			Skippable:   false,
			RunOnStart:  true,
			Enabled:     true,
		})
	}

	// Tor Health - Every 10 minutes, NOT skippable when Tor installed
	if handlers.TorHealth != nil {
		s.Register(&Task{
			ID:          TaskTorHealth,
			Name:        "Tor Health Check",
			Description: "Check Tor connectivity, restart if needed",
			Schedule:    "@every 10m",
			TaskType:    TaskTypeLocal,
			Run:         handlers.TorHealth,
			Skippable:   false,
			RunOnStart:  true,
			Enabled:     true,
		})
	}

	// Cluster Heartbeat - Every 30 seconds, NOT skippable in cluster mode
	if handlers.ClusterHeartbeat != nil {
		s.Register(&Task{
			ID:          TaskClusterHeartbeat,
			Name:        "Cluster Heartbeat",
			Description: "Cluster node heartbeat",
			Schedule:    "@every 30s",
			TaskType:    TaskTypeLocal,
			Run:         handlers.ClusterHeartbeat,
			Skippable:   false,
			RunOnStart:  true,
			Enabled:     true,
		})
	}
}

// TaskHandlers holds handler functions for built-in tasks
type TaskHandlers struct {
	SSLRenewal       func(ctx context.Context) error
	GeoIPUpdate      func(ctx context.Context) error
	BlocklistUpdate  func(ctx context.Context) error
	CVEUpdate        func(ctx context.Context) error
	SessionCleanup   func(ctx context.Context) error
	TokenCleanup     func(ctx context.Context) error
	LogRotation      func(ctx context.Context) error
	BackupDaily      func(ctx context.Context) error
	BackupHourly     func(ctx context.Context) error
	HealthcheckSelf  func(ctx context.Context) error
	TorHealth        func(ctx context.Context) error
	ClusterHeartbeat func(ctx context.Context) error
}

// Start starts the scheduler
// Per AI.md PART 19: Scheduler is ALWAYS RUNNING
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// Initialize database tables
	if s.db != nil {
		s.initDatabase()
	}

	// Check for missed tasks (catch-up logic)
	s.catchUpMissedTasks()

	// Run tasks marked with RunOnStart
	s.runStartupTasks()

	s.wg.Add(1)
	go s.run()

	log.Println("[Scheduler] Started - always running per AI.md PART 19")
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.checkAndRunTasks(now)
		}
	}
}

// checkAndRunTasks checks and runs due tasks
func (s *Scheduler) checkAndRunTasks(now time.Time) {
	s.mu.RLock()
	var dueTasks []*Task
	for _, task := range s.tasks {
		if task.Enabled && now.After(task.NextRun) {
			dueTasks = append(dueTasks, task)
		}
	}
	s.mu.RUnlock()

	for _, task := range dueTasks {
		go s.runTask(task)
	}
}

// runTask runs a single task with proper locking for cluster mode
// Per AI.md PART 19: Implements exponential backoff retry policy
func (s *Scheduler) runTask(task *Task) {
	// For global tasks in cluster mode, acquire lock first
	if task.TaskType == TaskTypeGlobal && s.db != nil {
		if !s.acquireTaskLock(task) {
			return // Another node is handling this task
		}
		defer s.releaseTaskLock(task)
	}

	// Get retry settings with defaults
	maxRetries := task.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}
	retryDelay := task.RetryDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryDelay
	}

	// Execute with retry logic
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 5m, 10m, 20m per spec
			backoffDelay := retryDelay * time.Duration(1<<(attempt-1))
			s.mu.Lock()
			task.LastStatus = StatusRetrying
			task.RetryCount = attempt
			task.NextRetry = time.Now().Add(backoffDelay)
			s.mu.Unlock()

			if s.db != nil {
				s.saveTaskState(task)
			}

			log.Printf("[Scheduler] Task %s retry %d/%d in %v", task.ID, attempt, maxRetries, backoffDelay)

			// Wait for backoff duration or context cancellation
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(backoffDelay):
			}
		}

		// Update task state to running
		s.mu.Lock()
		task.LastRun = time.Now()
		task.LastStatus = StatusRunning
		task.RetryCount = attempt
		task.NextRetry = time.Time{}
		s.mu.Unlock()

		// Execute task with timeout
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
		lastErr = task.Run(ctx)
		cancel()

		if lastErr == nil {
			// Success - update state and return
			s.mu.Lock()
			task.LastStatus = StatusSuccess
			task.LastError = ""
			task.RetryCount = 0
			task.RunCount++
			task.NextRun = s.calculateNextRun(task.Schedule)
			s.mu.Unlock()

			if s.db != nil {
				s.saveTaskState(task)
			}

			log.Printf("[Scheduler] Task %s completed successfully", task.ID)
			return
		}

		log.Printf("[Scheduler] Task %s attempt %d/%d failed: %v", task.ID, attempt+1, maxRetries+1, lastErr)
	}

	// All retries exhausted - task failed
	s.mu.Lock()
	task.LastStatus = StatusFailed
	task.LastError = lastErr.Error()
	task.RetryCount = 0
	task.FailCount++
	failCount := task.FailCount
	task.NextRun = s.calculateNextRun(task.Schedule)
	notifyFn := s.notifyFunc
	s.mu.Unlock()

	if s.db != nil {
		s.saveTaskState(task)
	}

	log.Printf("[Scheduler] Task %s failed after %d attempts: %v", task.ID, maxRetries+1, lastErr)

	// Per AI.md PART 19: Failed tasks trigger notifications (if configured)
	if notifyFn != nil {
		notifyFn(&TaskFailureNotification{
			TaskID:    string(task.ID),
			TaskName:  task.Name,
			Error:     lastErr.Error(),
			Attempts:  maxRetries + 1,
			LastRun:   task.LastRun,
			FailCount: failCount,
		})
	}
}

// calculateNextRun calculates the next run time from schedule expression
func (s *Scheduler) calculateNextRun(schedule string) time.Time {
	now := time.Now().In(s.timezone)

	// Handle @every intervals
	if strings.HasPrefix(schedule, "@every ") {
		interval := strings.TrimPrefix(schedule, "@every ")
		d, err := time.ParseDuration(interval)
		if err != nil {
			log.Printf("[Scheduler] Invalid interval %s: %v", interval, err)
			return now.Add(1 * time.Hour) // Default fallback
		}
		return now.Add(d)
	}

	// Handle predefined schedules
	switch schedule {
	case "@hourly":
		return now.Truncate(time.Hour).Add(time.Hour)
	case "@daily":
		return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, s.timezone)
	case "@weekly":
		daysUntilSunday := (7 - int(now.Weekday())) % 7
		if daysUntilSunday == 0 && now.Hour() >= 0 {
			daysUntilSunday = 7
		}
		return time.Date(now.Year(), now.Month(), now.Day()+daysUntilSunday, 0, 0, 0, 0, s.timezone)
	case "@monthly":
		return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, s.timezone)
	}

	// Parse cron expression (minute hour day month weekday)
	next, err := parseCronExpression(schedule, now, s.timezone)
	if err != nil {
		log.Printf("[Scheduler] Invalid cron expression %s: %v", schedule, err)
		return now.Add(1 * time.Hour)
	}
	return next
}

// parseCronExpression parses a standard cron expression and returns next run time
// Per AI.md PART 19: Full cron syntax with all 5 fields (minute hour day month weekday)
func parseCronExpression(expr string, from time.Time, loc *time.Location) (time.Time, error) {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return time.Time{}, fmt.Errorf("invalid cron expression: expected 5 fields")
	}

	minute, err := parseCronField(parts[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid minute field: %w", err)
	}

	hour, err := parseCronField(parts[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid hour field: %w", err)
	}

	day, err := parseCronField(parts[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid day field: %w", err)
	}

	month, err := parseCronField(parts[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month field: %w", err)
	}

	weekday, err := parseCronField(parts[4], 0, 6) // 0=Sunday, 6=Saturday
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid weekday field: %w", err)
	}

	// Iterate to find next valid time (check all 5 fields)
	candidate := from.Add(time.Minute)
	// Round down to start of minute
	candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day(),
		candidate.Hour(), candidate.Minute(), 0, 0, loc)

	for i := 0; i < 525600; i++ { // Max 1 year of minutes
		if contains(minute, candidate.Minute()) &&
			contains(hour, candidate.Hour()) &&
			contains(day, candidate.Day()) &&
			contains(month, int(candidate.Month())) &&
			contains(weekday, int(candidate.Weekday())) {
			return candidate, nil
		}
		candidate = candidate.Add(time.Minute)
	}

	return time.Time{}, fmt.Errorf("could not find next run time")
}

// parseCronField parses a single cron field
// Per AI.md PART 19: Supports *, ranges (5-10), lists (5,10,15), steps (*/5, 0-23/2)
func parseCronField(field string, min, max int) ([]int, error) {
	// Handle wildcard
	if field == "*" {
		return makeRange(min, max, 1), nil
	}

	// Handle step on wildcard: */5
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step value: %s", field)
		}
		return makeRange(min, max, step), nil
	}

	// Handle lists: 1,5,10
	if strings.Contains(field, ",") {
		var result []int
		for _, part := range strings.Split(field, ",") {
			vals, err := parseCronField(strings.TrimSpace(part), min, max)
			if err != nil {
				return nil, err
			}
			result = append(result, vals...)
		}
		return result, nil
	}

	// Handle ranges: 5-10 or 5-10/2
	if strings.Contains(field, "-") {
		step := 1
		rangeStr := field

		// Check for step: 5-10/2
		if strings.Contains(field, "/") {
			parts := strings.Split(field, "/")
			rangeStr = parts[0]
			var err error
			step, err = strconv.Atoi(parts[1])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step value: %s", field)
			}
		}

		rangeParts := strings.Split(rangeStr, "-")
		if len(rangeParts) != 2 {
			return nil, fmt.Errorf("invalid range: %s", field)
		}

		start, err := strconv.Atoi(rangeParts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
		}
		end, err := strconv.Atoi(rangeParts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
		}

		if start < min || end > max || start > end {
			return nil, fmt.Errorf("range %d-%d out of bounds [%d-%d]", start, end, min, max)
		}

		return makeRange(start, end, step), nil
	}

	// Handle single number
	val, err := strconv.Atoi(field)
	if err != nil {
		return nil, fmt.Errorf("invalid number: %s", field)
	}
	if val < min || val > max {
		return nil, fmt.Errorf("value %d out of range [%d-%d]", val, min, max)
	}
	return []int{val}, nil
}

// makeRange creates a slice of integers from start to end (inclusive) with given step
func makeRange(start, end, step int) []int {
	var result []int
	for i := start; i <= end; i += step {
		result = append(result, i)
	}
	return result
}

func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// catchUpMissedTasks runs tasks that were missed during downtime
func (s *Scheduler) catchUpMissedTasks() {
	if s.db == nil {
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	catchUpDeadline := now.Add(-s.catchUpWindow)

	for _, task := range s.tasks {
		if !task.Enabled {
			continue
		}

		// Check if task was missed (last run before catch-up window and next run in the past)
		if task.LastRun.Before(catchUpDeadline) && task.NextRun.Before(now) {
			log.Printf("[Scheduler] Catching up missed task: %s (last run: %s)", task.ID, task.LastRun)
			go s.runTask(task)
		}
	}
}

// runStartupTasks runs tasks marked with RunOnStart
// Per AI.md PART 19 line 28760-28761: Tasks execute "in order of original scheduled time"
func (s *Scheduler) runStartupTasks() {
	s.mu.RLock()
	startupTasks := make([]*Task, 0)
	for _, task := range s.tasks {
		if task.Enabled && task.RunOnStart {
			startupTasks = append(startupTasks, task)
		}
	}
	s.mu.RUnlock()

	// Run startup tasks sequentially per spec (in order of original scheduled time)
	for _, task := range startupTasks {
		log.Printf("[Scheduler] Running startup task: %s", task.ID)
		s.runTask(task)
	}
}

// Database operations for persistent state

// initDatabase creates the scheduler_tasks table if not exists
func (s *Scheduler) initDatabase() {
	query := `
		CREATE TABLE IF NOT EXISTS scheduler_tasks (
			task_id TEXT PRIMARY KEY,
			task_name TEXT NOT NULL,
			schedule TEXT NOT NULL,
			last_run DATETIME,
			last_status TEXT,
			last_error TEXT,
			next_run DATETIME,
			run_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0,
			enabled INTEGER DEFAULT 1,
			retry_count INTEGER DEFAULT 0,
			next_retry DATETIME,
			locked_by TEXT,
			locked_at DATETIME
		)
	`
	if _, err := s.db.Exec(query); err != nil {
		log.Printf("[Scheduler] Failed to create tasks table: %v", err)
	}

	// Add retry columns to existing tables (migration)
	s.db.Exec("ALTER TABLE scheduler_tasks ADD COLUMN retry_count INTEGER DEFAULT 0")
	s.db.Exec("ALTER TABLE scheduler_tasks ADD COLUMN next_retry DATETIME")
}

// loadTaskState loads persisted state for a task
func (s *Scheduler) loadTaskState(task *Task) {
	query := `SELECT last_run, last_status, last_error, next_run, run_count, fail_count, enabled, retry_count, next_retry
		FROM scheduler_tasks WHERE task_id = ?`

	var lastRun, nextRun, nextRetry sql.NullTime
	var lastStatus, lastError sql.NullString
	var runCount, failCount int64
	var retryCount int
	var enabled bool

	err := s.db.QueryRow(query, string(task.ID)).Scan(
		&lastRun, &lastStatus, &lastError, &nextRun, &runCount, &failCount, &enabled, &retryCount, &nextRetry,
	)

	if err == sql.ErrNoRows {
		// New task, insert initial state
		s.saveTaskState(task)
		return
	}

	if err != nil {
		log.Printf("[Scheduler] Failed to load task state for %s: %v", task.ID, err)
		return
	}

	// Only restore enabled state for skippable tasks
	if task.Skippable {
		task.Enabled = enabled
	}

	if lastRun.Valid {
		task.LastRun = lastRun.Time
	}
	if lastStatus.Valid {
		task.LastStatus = TaskStatus(lastStatus.String)
	}
	if lastError.Valid {
		task.LastError = lastError.String
	}
	if nextRun.Valid && nextRun.Time.After(time.Now()) {
		task.NextRun = nextRun.Time
	}
	if nextRetry.Valid {
		task.NextRetry = nextRetry.Time
	}
	task.RunCount = runCount
	task.FailCount = failCount
	task.RetryCount = retryCount
}

// saveTaskState persists task state to database
func (s *Scheduler) saveTaskState(task *Task) {
	query := `INSERT OR REPLACE INTO scheduler_tasks
		(task_id, task_name, schedule, last_run, last_status, last_error, next_run, run_count, fail_count, enabled, retry_count, next_retry)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	// Handle zero time for next_retry
	var nextRetry interface{}
	if task.NextRetry.IsZero() {
		nextRetry = nil
	} else {
		nextRetry = task.NextRetry
	}

	_, err := s.db.Exec(query,
		string(task.ID),
		task.Name,
		task.Schedule,
		task.LastRun,
		string(task.LastStatus),
		task.LastError,
		task.NextRun,
		task.RunCount,
		task.FailCount,
		task.Enabled,
		task.RetryCount,
		nextRetry,
	)

	if err != nil {
		log.Printf("[Scheduler] Failed to save task state for %s: %v", task.ID, err)
	}
}

// acquireTaskLock attempts to acquire a distributed lock for a task
func (s *Scheduler) acquireTaskLock(task *Task) bool {
	// Lock timeout is 5 minutes per spec
	lockTimeout := 5 * time.Minute
	now := time.Now()

	// Try to acquire lock or take over expired lock
	query := `UPDATE scheduler_tasks
		SET locked_by = ?, locked_at = ?
		WHERE task_id = ? AND (locked_by IS NULL OR locked_by = ? OR locked_at < ?)`

	result, err := s.db.Exec(query, s.nodeID, now, string(task.ID), s.nodeID, now.Add(-lockTimeout))
	if err != nil {
		log.Printf("[Scheduler] Failed to acquire lock for %s: %v", task.ID, err)
		return false
	}

	rows, _ := result.RowsAffected()
	return rows > 0
}

// releaseTaskLock releases the distributed lock for a task
func (s *Scheduler) releaseTaskLock(task *Task) {
	query := `UPDATE scheduler_tasks SET locked_by = NULL, locked_at = NULL WHERE task_id = ? AND locked_by = ?`
	s.db.Exec(query, string(task.ID), s.nodeID)
}

// Enable enables a task
func (s *Scheduler) Enable(id TaskID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	if !task.Skippable {
		return fmt.Errorf("task %s cannot be enabled/disabled", id)
	}

	task.Enabled = true
	task.NextRun = s.calculateNextRun(task.Schedule)

	if s.db != nil {
		s.saveTaskState(task)
	}

	return nil
}

// Disable disables a task
func (s *Scheduler) Disable(id TaskID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	if !task.Skippable {
		return fmt.Errorf("task %s cannot be disabled - it is required", id)
	}

	task.Enabled = false

	if s.db != nil {
		s.saveTaskState(task)
	}

	return nil
}

// RunNow runs a task immediately
func (s *Scheduler) RunNow(id TaskID) error {
	s.mu.RLock()
	task, ok := s.tasks[id]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	go s.runTask(task)
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.mu.Unlock()

	s.cancel()
	s.wg.Wait()
	log.Println("[Scheduler] Stopped")
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetTasks returns all registered tasks
func (s *Scheduler) GetTasks() []*TaskInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*TaskInfo, 0, len(s.tasks))
	for _, task := range s.tasks {
		maxRetries := task.MaxRetries
		if maxRetries <= 0 {
			maxRetries = DefaultMaxRetries
		}
		tasks = append(tasks, &TaskInfo{
			ID:          string(task.ID),
			Name:        task.Name,
			Description: task.Description,
			Schedule:    task.Schedule,
			TaskType:    string(task.TaskType),
			LastRun:     task.LastRun,
			LastStatus:  string(task.LastStatus),
			LastError:   task.LastError,
			NextRun:     task.NextRun,
			RunCount:    task.RunCount,
			FailCount:   task.FailCount,
			Enabled:     task.Enabled,
			Skippable:   task.Skippable,
			RetryCount:  task.RetryCount,
			NextRetry:   task.NextRetry,
			MaxRetries:  maxRetries,
		})
	}
	return tasks
}

// GetTask returns a specific task
func (s *Scheduler) GetTask(id TaskID) (*TaskInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %s", id)
	}

	maxRetries := task.MaxRetries
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}

	return &TaskInfo{
		ID:          string(task.ID),
		Name:        task.Name,
		Description: task.Description,
		Schedule:    task.Schedule,
		TaskType:    string(task.TaskType),
		LastRun:     task.LastRun,
		LastStatus:  string(task.LastStatus),
		LastError:   task.LastError,
		NextRun:     task.NextRun,
		RunCount:    task.RunCount,
		FailCount:   task.FailCount,
		Enabled:     task.Enabled,
		Skippable:   task.Skippable,
		RetryCount:  task.RetryCount,
		NextRetry:   task.NextRetry,
		MaxRetries:  maxRetries,
	}, nil
}

// TaskInfo represents task information for API/UI
type TaskInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Schedule    string    `json:"schedule"`
	TaskType    string    `json:"task_type"`
	LastRun     time.Time `json:"last_run"`
	LastStatus  string    `json:"last_status"`
	LastError   string    `json:"last_error,omitempty"`
	NextRun     time.Time `json:"next_run"`
	RunCount    int64     `json:"run_count"`
	FailCount   int64     `json:"fail_count"`
	Enabled     bool      `json:"enabled"`
	Skippable   bool      `json:"skippable"`

	// Retry state per AI.md PART 19
	RetryCount  int       `json:"retry_count"`
	NextRetry   time.Time `json:"next_retry,omitempty"`
	MaxRetries  int       `json:"max_retries"`
}
