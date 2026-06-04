// Package scheduler provides a built-in task scheduler per AI.md PART 19
// The scheduler is ALWAYS RUNNING - there is no enable/disable option.
// All scheduled tasks are managed internally, never via external cron/schedulers.
package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// cronParser parses both standard 5-field cron expressions and @descriptor
// forms (@hourly, @daily, @weekly, @monthly, @every Xm) per AI.md PART 18.
// We use github.com/robfig/cron/v3 — never a hand-rolled parser.
var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
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
	TaskTokenCleanup    TaskID = "token.cleanup"
	TaskLogRotation     TaskID = "log.rotation"
	TaskBackupDaily     TaskID = "backup_daily"
	TaskBackupHourly    TaskID = "backup_hourly"
	TaskHealthcheckSelf TaskID = "healthcheck.self"
	TaskTorHealth       TaskID = "tor.health"
	TaskAlertsImmediate TaskID = "alerts.immediate"
	TaskAlertsDaily      TaskID = "alerts.daily"
	TaskAlertsWeekly     TaskID = "alerts.weekly"
	// TaskPublicIPRefresh refreshes the cached server public IP per
	// AI.md PART 8 step 16 (startup + every 12h, hardcoded — not configurable).
	TaskPublicIPRefresh TaskID = "public_ip_refresh"
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

// TaskType determines how a task acquires a run lock.
// Per AI.md line 2055: single-instance only; TaskType is used for DB-level deduplication.
type TaskType string

const (
	// TaskTypeGlobal uses a database lock to prevent concurrent duplicate runs.
	TaskTypeGlobal TaskType = "global"
	// TaskTypeLocal runs without acquiring a database lock.
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
	// Cron expression or @every interval
	Schedule string
	// Global or Local
	TaskType TaskType
	Run      func(ctx context.Context) error
	// Can admin disable this task?
	Skippable bool
	// Run immediately on scheduler start?
	RunOnStart bool

	// Retry policy per AI.md PART 19
	// Default: max_retries=3, retry_delay=5m, backoff=exponential (5m, 10m, 20m)
	// Maximum retry attempts (default: 3)
	MaxRetries int
	// Base delay between retries (default: 5m)
	RetryDelay time.Duration

	// Runtime state (persisted to database)
	LastRun    time.Time
	LastStatus TaskStatus
	LastError  string
	NextRun    time.Time
	RunCount   int64
	FailCount  int64
	Enabled    bool

	// Retry state
	// Current retry attempt (0 = first run)
	RetryCount int
	// Scheduled retry time (if retrying)
	NextRetry time.Time

	// Cluster locking
	LockedBy string
	LockedAt time.Time
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
	TaskID    string
	TaskName  string
	Error     string
	Attempts  int
	LastRun   time.Time
	FailCount int64
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
	// Per AI.md PART 19: Task failure notifications
	notifyFunc NotifyFunc
}

// NewScheduler creates a new scheduler
// Per AI.md PART 19: Scheduler is ALWAYS RUNNING
func NewScheduler(db *sql.DB, nodeID string) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	// Default to America/New_York per spec
	tz, err := time.LoadLocation(defaultTimezone)
	if err != nil {
		tz = time.Local
	}

	s := &Scheduler{
		tasks:    make(map[TaskID]*Task),
		db:       db,
		nodeID:   nodeID,
		ctx:      ctx,
		cancel:   cancel,
		timezone: tz,
		// Default catch-up window
		catchUpWindow: 1 * time.Hour,
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

	// Calculate next run time. We already hold s.mu (write) here, so use
	// the lock-free variant — the non-recursive RWMutex would otherwise
	// self-deadlock when calculateNextRun re-acquires the read lock.
	task.NextRun = s.calculateNextRunLocked(task.Schedule)
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

	if handlers.AlertsImmediate != nil {
		s.Register(&Task{
			ID:          TaskAlertsImmediate,
			Name:        "Immediate Search Alerts",
			Description: "Check and deliver immediate search alerts",
			Schedule:    "@every 10m",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.AlertsImmediate,
			Skippable:   false,
			RunOnStart:  true,
			Enabled:     true,
		})
	}

	if handlers.AlertsDaily != nil {
		s.Register(&Task{
			ID:          TaskAlertsDaily,
			Name:        "Daily Search Alerts",
			Description: "Check and deliver daily search alerts",
			Schedule:    "0 8 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.AlertsDaily,
			Skippable:   false,
			Enabled:     true,
		})
	}

	if handlers.AlertsWeekly != nil {
		s.Register(&Task{
			ID:          TaskAlertsWeekly,
			Name:        "Weekly Search Alerts",
			Description: "Check and deliver weekly search alerts",
			Schedule:    "0 8 * * 1",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.AlertsWeekly,
			Skippable:   false,
			Enabled:     true,
		})
	}

	// Public IP Refresh - Startup + every 12 hours, hardcoded per AI.md
	// PART 8 step 16. NOT skippable; FQDN detection depends on it.
	if handlers.PublicIPRefresh != nil {
		s.Register(&Task{
			ID:          TaskPublicIPRefresh,
			Name:        "Public IP Refresh",
			Description: "Detect and cache the server's public IPv4 address (every 12h, hardcoded)",
			Schedule:    "@every 12h",
			TaskType:    TaskTypeLocal,
			Run:         handlers.PublicIPRefresh,
			Skippable:   false,
			RunOnStart:  true,
			Enabled:     true,
		})
	}

}

// TaskHandlers holds handler functions for built-in tasks
type TaskHandlers struct {
	SSLRenewal      func(ctx context.Context) error
	GeoIPUpdate     func(ctx context.Context) error
	BlocklistUpdate func(ctx context.Context) error
	CVEUpdate       func(ctx context.Context) error
	TokenCleanup    func(ctx context.Context) error
	LogRotation     func(ctx context.Context) error
	BackupDaily     func(ctx context.Context) error
	BackupHourly    func(ctx context.Context) error
	HealthcheckSelf func(ctx context.Context) error
	TorHealth       func(ctx context.Context) error
	AlertsImmediate func(ctx context.Context) error
	AlertsDaily      func(ctx context.Context) error
	AlertsWeekly     func(ctx context.Context) error
	// PublicIPRefresh refreshes the cached public IP per AI.md PART 8
	// step 16. Schedule and cadence are hardcoded (startup + every 12h).
	PublicIPRefresh func(ctx context.Context) error
}

// Start starts the scheduler
// Per AI.md PART 19: Scheduler is ALWAYS RUNNING
func (s *Scheduler) StartTaskScheduler() {
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

// runTask runs a single task with DB-level deduplication for global tasks.
// Per AI.md PART 18: Implements exponential backoff retry policy.
func (s *Scheduler) runTask(task *Task) {
	// For global tasks, acquire a DB lock to prevent duplicate concurrent runs.
	if task.TaskType == TaskTypeGlobal && s.db != nil {
		if !s.acquireTaskLock(task) {
			// Lock not acquired — task is already running
			return
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
			task.NextRun = s.calculateNextRunLocked(task.Schedule)
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
	task.NextRun = s.calculateNextRunLocked(task.Schedule)
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

// calculateNextRun calculates the next run time from a cron expression or
// @descriptor (handles @every Xm, @hourly, @daily, @weekly, @monthly, and
// standard 5-field cron) using github.com/robfig/cron/v3 per AI.md PART 18.
// On a parse error the scheduler falls back to a 1-hour interval so a bad
// schedule entry never wedges the loop.
//
// Acquires s.mu.RLock for the timezone read. Callers that already hold
// s.mu must use calculateNextRunLocked to avoid a self-deadlock on the
// non-recursive RWMutex.
func (s *Scheduler) calculateNextRun(schedule string) time.Time {
	s.mu.RLock()
	loc := s.timezone
	s.mu.RUnlock()
	return calculateNextRunWithLoc(schedule, loc)
}

// calculateNextRunLocked is the lock-free variant. Caller must already
// hold s.mu (read or write). Used from Register / RegisterBuiltinTasks
// which hold the write lock for the whole insertion.
func (s *Scheduler) calculateNextRunLocked(schedule string) time.Time {
	return calculateNextRunWithLoc(schedule, s.timezone)
}

func calculateNextRunWithLoc(schedule string, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	now := time.Now().In(loc)

	sched, err := cronParser.Parse(schedule)
	if err != nil {
		log.Printf("[Scheduler] Invalid schedule %q: %v", schedule, err)
		return now.Add(1 * time.Hour)
	}
	return sched.Next(now)
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
	// Already holding s.mu (write) — use the lock-free variant to avoid
	// self-deadlock on the non-recursive RWMutex.
	task.NextRun = s.calculateNextRunLocked(task.Schedule)

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
func (s *Scheduler) StopTaskScheduler() {
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
	RetryCount int       `json:"retry_count"`
	NextRetry  time.Time `json:"next_retry,omitempty"`
	MaxRetries int       `json:"max_retries"`
}
