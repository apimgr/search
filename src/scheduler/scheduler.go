// Package scheduler provides a built-in task scheduler per AI.md PART 18.
// The scheduler is ALWAYS RUNNING - there is no enable/disable option.
// All scheduled tasks are managed internally, never via external cron/schedulers.
package scheduler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/apimgr/search/src/common/closeutil"
)

// defaultTimezone is the default timezone for the scheduler (allows testing)
var defaultTimezone = "America/New_York"

// TaskID represents a unique task identifier
type TaskID string

// Built-in task IDs per AI.md PART 18 (canonical underscore names)
const (
	TaskSSLRenewal      TaskID = "ssl_renewal"
	TaskGeoIPUpdate     TaskID = "geoip_update"
	TaskBlocklistUpdate TaskID = "blocklist_update"
	TaskCVEUpdate       TaskID = "cve_update"
	TaskUpdateCheck     TaskID = "update_check"
	TaskTokenCleanup    TaskID = "token_cleanup"
	TaskLogRotation     TaskID = "log_rotation"
	TaskBackupDaily     TaskID = "backup_daily"
	TaskBackupHourly    TaskID = "backup_hourly"
	TaskHealthcheckSelf TaskID = "healthcheck_self"
	TaskTorHealth       TaskID = "tor_health"
	TaskI2PHealth       TaskID = "i2p_health"
	TaskAlertsImmediate TaskID = "alerts_immediate"
	TaskAlertsDaily     TaskID = "alerts_daily"
	TaskAlertsWeekly    TaskID = "alerts_weekly"
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

// Default retry policy values per AI.md PART 18
const (
	DefaultMaxRetries = 3
	DefaultRetryDelay = 5 * time.Minute
)

// shutdownGracePeriod bounds how long StopTaskScheduler waits for running
// tasks to finish per AI.md PART 18 "Shutdown Behavior" ("Wait for running
// tasks to complete (max 30 seconds)"). On timeout the node's locks are
// force-released and still-running tasks are marked for retry on next start.
const shutdownGracePeriod = 30 * time.Second

// historyRetainPerTask is how many execution history rows are kept per task.
// Per AI.md PART 18 the CLI shows recent history (`scheduler history <id>`);
// older rows are pruned so the table cannot grow without bound.
const historyRetainPerTask = 50

// slowTaskRetryDelay is the retry delay for tasks whose AI.md PART 18 config
// block sets retry_on_fail with retry_delay: 1h (blocklist_update, cve_update).
const slowTaskRetryDelay = time.Hour

// lockRetryCooldown bounds how soon a TaskTypeGlobal task becomes eligible
// again after a failed lock acquisition (another node holds it, or the lock
// query itself errored, e.g. DB contention/timeout). Without this, the 1s
// scheduler ticker re-selects the task as "due" on every tick (NextRun is
// never advanced on this path), launching a fresh goroutine that hammers
// acquireTaskLock every second — which can itself cause and perpetuate the
// very DB contention ("context deadline exceeded") that caused the first
// failure. Per AI.md PART 18 retry_delay intent, back off instead.
const lockRetryCooldown = 30 * time.Second

// Task represents a scheduled task per AI.md PART 18
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

	// Retry policy per AI.md PART 18
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

	// DB-level deduplication lock (per AI.md line 2055: single instance)
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
// Per AI.md PART 18: Failed tasks trigger notifications (if configured)
type TaskFailureNotification struct {
	TaskID    string
	TaskName  string
	Error     string
	Attempts  int
	LastRun   time.Time
	FailCount int64
}

// NotifyFunc is a callback function for task failure notifications
// Per AI.md PART 18: Failed tasks trigger notifications (if configured)
type NotifyFunc func(notification *TaskFailureNotification)

// TaskExecution describes one completed task execution.
// Per AI.md PART 18 "Task Execution Flow": every execution (success, failure,
// or skipped) is written to the audit log.
type TaskExecution struct {
	TaskID    string
	TaskName  string
	Status    TaskStatus
	Error     string
	Attempts  int
	StartedAt time.Time
	Duration  time.Duration
}

// AuditFunc is a callback invoked after every task execution so the host
// application can write the event to its audit log.
// Per AI.md PART 18 Implementation Requirements: "Audit logging - All task
// executions logged".
type AuditFunc func(execution *TaskExecution)

// TaskHistoryEntry is one persisted execution history record.
// Per AI.md PART 18 CLI Commands: `{project_name} scheduler history <id>`.
type TaskHistoryEntry struct {
	TaskID     string        `json:"task_id"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Duration   time.Duration `json:"duration"`
	Status     string        `json:"status"`
	Success    bool          `json:"success"`
	Error      string        `json:"error,omitempty"`
}

// Scheduler manages periodic tasks per AI.md PART 18
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
	// Per AI.md PART 18: Task failure notifications
	notifyFunc NotifyFunc
	// Per AI.md PART 18: every task execution is written to the audit log
	auditFunc AuditFunc
	// taskWG tracks in-progress task goroutines so shutdown can wait for
	// them per AI.md PART 18 "Shutdown Behavior" (max 30 seconds).
	taskWG sync.WaitGroup
	// inFlight tracks tasks with a runTask goroutine currently executing
	// (including one still blocked trying to acquire the DB lock). Without
	// this, a slow/contended lock query (up to its own context timeout)
	// leaves NextRun unchanged for that whole window, so every 1s tick in
	// between dispatches ANOTHER goroutine for the same still-"due" task —
	// a thundering herd that saturates the DB further and can perpetuate
	// the very contention that caused the first failure.
	inFlight map[TaskID]bool
}

// NewScheduler creates a new scheduler
// Per AI.md PART 18: Scheduler is ALWAYS RUNNING
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
		inFlight:      make(map[TaskID]bool),
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
// Per AI.md PART 18: Failed tasks trigger notifications (if configured)
func (s *Scheduler) SetNotifyFunc(fn NotifyFunc) {
	s.mu.Lock()
	s.notifyFunc = fn
	s.mu.Unlock()
}

// SetAuditFunc sets the callback invoked after every task execution.
// Per AI.md PART 18: all task executions are logged to the audit log.
func (s *Scheduler) SetAuditFunc(fn AuditFunc) {
	s.mu.Lock()
	s.auditFunc = fn
	s.mu.Unlock()
}

// SetTaskSchedule overrides a task's schedule from configuration.
// Per AI.md PART 18 "Task Configuration": every built-in task's schedule is
// configurable through server.scheduler.tasks.<task>.schedule.
func (s *Scheduler) SetTaskSchedule(id TaskID, schedule string) error {
	if schedule == "" {
		return fmt.Errorf("task schedule is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	task.Schedule = schedule
	task.NextRun = s.calculateNextRunLocked(schedule)

	if s.db != nil {
		s.saveTaskState(task)
	}

	return nil
}

// SetTaskRetry overrides a task's retry policy from configuration.
// Per AI.md PART 18 "Retry Policy": max_retries (default 3) and retry_delay
// (default 5m, exponential backoff). Zero or negative values keep the default.
func (s *Scheduler) SetTaskRetry(id TaskID, maxRetries int, retryDelay time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %s", id)
	}

	if maxRetries > 0 {
		task.MaxRetries = maxRetries
	}
	if retryDelay > 0 {
		task.RetryDelay = retryDelay
	}

	return nil
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

// RegisterBuiltinTasks registers all required tasks per AI.md PART 18
func (s *Scheduler) RegisterBuiltinTasks(handlers *TaskHandlers) {
	// SSL Renewal - Daily at 03:00, NOT skippable
	if handlers.SSLRenewal != nil {
		if err := s.Register(&Task{
			ID:          TaskSSLRenewal,
			Name:        "SSL Certificate Renewal",
			Description: "Check and renew SSL certificates 7 days before expiry",
			Schedule:    "0 3 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.SSLRenewal,
			Skippable:   false,
			RunOnStart:  true,
			Enabled:     true,
		}); err != nil {
			slog.Error("failed to register SSL renewal task", "error", err)
		}
	}

	// GeoIP Update - Weekly Sunday at 03:00, skippable
	if handlers.GeoIPUpdate != nil {
		if err := s.Register(&Task{
			ID:          TaskGeoIPUpdate,
			Name:        "GeoIP Database Update",
			Description: "Download and update ip-location-db GeoIP databases",
			Schedule:    "0 3 * * 0",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.GeoIPUpdate,
			Skippable:   true,
			Enabled:     true,
		}); err != nil {
			slog.Error("failed to register GeoIP update task", "error", err)
		}
	}

	// Blocklist Update - Daily at 04:00, skippable
	if handlers.BlocklistUpdate != nil {
		if err := s.Register(&Task{
			ID:          TaskBlocklistUpdate,
			Name:        "Blocklist Update",
			Description: "Download and update IP/domain blocklists",
			Schedule:    "0 4 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.BlocklistUpdate,
			Skippable:   true,
			Enabled:     true,
			// Per AI.md PART 18 task config: retry_on_fail with retry_delay 1h
			RetryDelay: slowTaskRetryDelay,
		}); err != nil {
			slog.Error("failed to register blocklist update task", "error", err)
		}
	}

	// CVE Update - Daily at 05:00, skippable
	if handlers.CVEUpdate != nil {
		if err := s.Register(&Task{
			ID:          TaskCVEUpdate,
			Name:        "CVE Database Update",
			Description: "Download and update CVE/security databases",
			Schedule:    "0 5 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.CVEUpdate,
			Skippable:   true,
			Enabled:     true,
			// Per AI.md PART 18 task config: retry_on_fail with retry_delay 1h
			RetryDelay: slowTaskRetryDelay,
		}); err != nil {
			slog.Error("failed to register CVE update task", "error", err)
		}
	}

	// Update Check - Daily at 06:00, skippable
	// Per AI.md PART 18/22: notify-only check for a newer release; honors
	// update.defer_days and stays notify-only unless update.auto_install is set.
	if handlers.UpdateCheck != nil {
		if err := s.Register(&Task{
			ID:          TaskUpdateCheck,
			Name:        "Update Check",
			Description: "Check release channel for a newer version (notify-only by default)",
			Schedule:    "0 6 * * *",
			TaskType:    TaskTypeGlobal,
			Run:         handlers.UpdateCheck,
			Skippable:   true,
			Enabled:     true,
		}); err != nil {
			slog.Error("failed to register update check task", "error", err)
		}
	}

	// Token Cleanup - Every 15 minutes, NOT skippable
	if handlers.TokenCleanup != nil {
		if err := s.Register(&Task{
			ID:          TaskTokenCleanup,
			Name:        "Token Cleanup",
			Description: "Remove expired tokens",
			Schedule:    "@every 15m",
			TaskType:    TaskTypeLocal,
			Run:         handlers.TokenCleanup,
			Skippable:   false,
			Enabled:     true,
		}); err != nil {
			slog.Error("failed to register token cleanup task", "error", err)
		}
	}

	// Log Rotation - Daily at 00:00, NOT skippable
	if handlers.LogRotation != nil {
		s.registerBuiltinTask(&Task{
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
		s.registerBuiltinTask(&Task{
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
		s.registerBuiltinTask(task)
		// Disable after registration (Register sets Enabled=true by default)
		task.Enabled = false
	}

	// Health Check Self - Every 5 minutes, NOT skippable
	if handlers.HealthcheckSelf != nil {
		s.registerBuiltinTask(&Task{
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
		s.registerBuiltinTask(&Task{
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

	// I2P Health - Every 10 minutes, only registered when I2P is opt-in enabled
	// Per AI.md PART 18 Built-in Tasks: i2p_health runs only when the operator
	// has enabled I2P (PART 31.2 - I2P is opt-in, disabled by default), so the
	// host application supplies this handler only in that case.
	if handlers.I2PHealth != nil {
		s.registerBuiltinTask(&Task{
			ID:          TaskI2PHealth,
			Name:        "I2P Health Check",
			Description: "Check I2P provider, restart if needed",
			Schedule:    "@every 10m",
			TaskType:    TaskTypeLocal,
			Run:         handlers.I2PHealth,
			Skippable:   false,
			RunOnStart:  true,
			Enabled:     true,
		})
	}

	if handlers.AlertsImmediate != nil {
		s.registerBuiltinTask(&Task{
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
		s.registerBuiltinTask(&Task{
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
		s.registerBuiltinTask(&Task{
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
		s.registerBuiltinTask(&Task{
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

func (s *Scheduler) registerBuiltinTask(task *Task) {
	if err := s.Register(task); err != nil {
		slog.Error("failed to register built-in scheduler task", "task_id", task.ID, "error", err)
	}
}

// TaskHandlers holds handler functions for built-in tasks
type TaskHandlers struct {
	SSLRenewal      func(ctx context.Context) error
	GeoIPUpdate     func(ctx context.Context) error
	BlocklistUpdate func(ctx context.Context) error
	CVEUpdate       func(ctx context.Context) error
	UpdateCheck     func(ctx context.Context) error
	TokenCleanup    func(ctx context.Context) error
	LogRotation     func(ctx context.Context) error
	BackupDaily     func(ctx context.Context) error
	BackupHourly    func(ctx context.Context) error
	HealthcheckSelf func(ctx context.Context) error
	TorHealth       func(ctx context.Context) error
	// I2PHealth is supplied only when I2P is enabled (AI.md PART 31.2:
	// I2P is opt-in and disabled by default).
	I2PHealth       func(ctx context.Context) error
	AlertsImmediate func(ctx context.Context) error
	AlertsDaily     func(ctx context.Context) error
	AlertsWeekly    func(ctx context.Context) error
	// PublicIPRefresh refreshes the cached public IP per AI.md PART 8
	// step 16. Schedule and cadence are hardcoded (startup + every 12h).
	PublicIPRefresh func(ctx context.Context) error
}

// Start starts the scheduler
// Per AI.md PART 18: Scheduler is ALWAYS RUNNING
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

	slog.Info("Scheduler started - always running per AI.md PART 18")
}

// run is the main scheduler loop.
// Launched via `go s.run()` at startup with no other goroutine watching it —
// an unrecovered panic here would silently kill the loop (and, since Go
// terminates the whole process on any unrecovered panic, the entire server)
// with no automatic restart. Recover and log instead of crashing.
func (s *Scheduler) run() {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Scheduler loop panicked - recovered to prevent server crash",
				"panic", r,
				"stack", string(debug.Stack()))
		}
	}()

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
	s.mu.Lock()
	var dueTasks []*Task
	for _, task := range s.tasks {
		if task.Enabled && now.After(task.NextRun) && !s.inFlight[task.ID] {
			dueTasks = append(dueTasks, task)
		}
	}
	s.mu.Unlock()

	for _, task := range dueTasks {
		// beginTask marks the task in-flight under the lock, so a lock query
		// that blocks for several seconds can't let subsequent ticks pile up
		// more goroutines for the same task before it finishes.
		if !s.beginTask(task) {
			continue
		}
		go s.runTask(task)
	}
}

// beginTask reserves a task for execution: it rejects a duplicate run while one
// is already in flight, refuses to start new work once shutdown has begun, and
// registers the run with taskWG so StopTaskScheduler can wait for it.
// Every caller that reaches runTask must go through beginTask first.
func (s *Scheduler) beginTask(task *Task) bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
	}

	s.mu.Lock()
	if s.inFlight[task.ID] {
		s.mu.Unlock()
		return false
	}
	s.inFlight[task.ID] = true
	s.mu.Unlock()

	s.taskWG.Add(1)
	return true
}

// runTask runs a single task with DB-level deduplication for global tasks.
// Per AI.md PART 18: Implements exponential backoff retry policy.
//
// Always launched via `go s.runTask(task)`, so a panic here is on its own
// goroutine — left unrecovered, Go terminates the entire process regardless
// of which goroutine panicked, taking the whole server down. Recovering here
// keeps a single misbehaving task handler from causing a full outage.
func (s *Scheduler) runTask(task *Task) {
	// Release the shutdown WaitGroup slot reserved by beginTask.
	defer s.taskWG.Done()

	// Clear the in-flight marker on every exit path (success, failure, lock
	// not acquired, or panic) so the task becomes eligible for pickup again.
	defer func() {
		s.mu.Lock()
		delete(s.inFlight, task.ID)
		s.mu.Unlock()
	}()

	// Per AI.md PART 18 "Task Execution Flow": a triggered task (scheduled or
	// manual) is skipped when it is disabled.
	s.mu.RLock()
	enabled := task.Enabled
	s.mu.RUnlock()
	if !enabled {
		skipStart := time.Now()
		s.mu.Lock()
		task.LastStatus = StatusSkipped
		s.mu.Unlock()

		if s.db != nil {
			s.saveTaskState(task)
		}

		s.recordHistory(task, skipStart, time.Now(), StatusSkipped, "task is disabled")
		s.emitAudit(task, StatusSkipped, "task is disabled", 0, skipStart, 0)
		slog.Info("Task skipped - disabled", "task", task.ID)
		return
	}

	// execStart is the wall-clock start of this execution; it is the history
	// and audit start timestamp for every exit path, including a panic.
	execStart := time.Now()

	defer func() {
		if r := recover(); r != nil {
			slog.Error("Task panicked - recovered to prevent server crash",
				"task", task.ID,
				"panic", r,
				"stack", string(debug.Stack()))

			s.mu.Lock()
			task.LastStatus = StatusFailed
			task.LastError = fmt.Sprintf("panic: %v", r)
			task.FailCount++
			task.NextRun = s.calculateNextRunLocked(task.Schedule)
			notifyFn := s.notifyFunc
			failCount := task.FailCount
			s.mu.Unlock()

			if s.db != nil {
				s.saveTaskState(task)
			}

			panicErr := fmt.Sprintf("panic: %v", r)
			s.recordHistory(task, execStart, time.Now(), StatusFailed, panicErr)
			s.emitAudit(task, StatusFailed, panicErr, 1, execStart, time.Since(execStart))

			if notifyFn != nil {
				notifyFn(&TaskFailureNotification{
					TaskID:    string(task.ID),
					TaskName:  task.Name,
					Error:     fmt.Sprintf("panic: %v", r),
					Attempts:  1,
					LastRun:   task.LastRun,
					FailCount: failCount,
				})
			}
		}
	}()

	// For global tasks, acquire a DB lock to prevent duplicate concurrent runs.
	if task.TaskType == TaskTypeGlobal && s.db != nil {
		if !s.acquireTaskLock(task) {
			// Lock not acquired — either another node is already running the
			// task, or the lock query itself failed (e.g. DB contention).
			// Back off instead of leaving NextRun untouched, which would let
			// the 1s ticker re-select and re-attempt this task on every
			// single tick (see lockRetryCooldown).
			s.mu.Lock()
			task.NextRun = time.Now().Add(lockRetryCooldown)
			s.mu.Unlock()
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

			slog.Info("Task retry scheduled", "task", task.ID, "attempt", attempt, "max_retries", maxRetries, "backoff", backoffDelay)

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
		attemptStart := time.Now()
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
		lastErr = task.Run(ctx)
		cancel()
		attemptEnd := time.Now()

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

			s.recordHistory(task, attemptStart, attemptEnd, StatusSuccess, "")
			s.emitAudit(task, StatusSuccess, "", attempt+1, attemptStart, attemptEnd.Sub(attemptStart))

			slog.Info("Task completed successfully", "task", task.ID)
			return
		}

		slog.Warn("Task attempt failed", "task", task.ID, "attempt", attempt+1, "max_attempts", maxRetries+1, "err", lastErr)
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

	s.recordHistory(task, execStart, time.Now(), StatusFailed, lastErr.Error())
	s.emitAudit(task, StatusFailed, lastErr.Error(), maxRetries+1, execStart, time.Since(execStart))

	slog.Error("Task failed after all attempts", "task", task.ID, "attempts", maxRetries+1, "err", lastErr)

	// Per AI.md PART 18: Failed tasks trigger notifications (if configured)
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
// standard 5-field cron) using only Go's standard library.
// Per AI.md PART 18: "Use Go's time/ticker — No external cron libraries required."
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

// parseCronField parses a single cron field and returns a boolean bitmap indexed
// from fieldMin. Supports: * (any), n (single), a-b (range), */n (step), and
// comma-separated combinations of the above.
func parseCronField(field string, fieldMin, fieldMax int) ([]bool, error) {
	size := fieldMax - fieldMin + 1
	result := make([]bool, size)

	for _, part := range strings.Split(field, ",") {
		if part == "*" {
			for i := range result {
				result[i] = true
			}
			return result, nil
		}
		// */step
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step %q", part)
			}
			for v := fieldMin; v <= fieldMax; v += step {
				result[v-fieldMin] = true
			}
			continue
		}
		// range: a-b
		if dashIdx := strings.IndexByte(part, '-'); dashIdx >= 0 {
			a, err1 := strconv.Atoi(part[:dashIdx])
			b, err2 := strconv.Atoi(part[dashIdx+1:])
			if err1 != nil || err2 != nil || a > b {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			for v := a; v <= b; v++ {
				if v >= fieldMin && v <= fieldMax {
					result[v-fieldMin] = true
				}
			}
			continue
		}
		// single integer
		v, err := strconv.Atoi(part)
		if err != nil || v < fieldMin || v > fieldMax {
			return nil, fmt.Errorf("invalid value %q (range %d-%d)", part, fieldMin, fieldMax)
		}
		result[v-fieldMin] = true
	}
	return result, nil
}

// calculateNextRunWithLoc computes the next execution time for a cron schedule
// using only Go's standard library — no external cron library.
// Per AI.md PART 18: "Use Go's time/ticker — No external cron libraries required."
// Supports @every <duration>, @hourly, @daily, @weekly, @monthly, and standard
// 5-field cron expressions (minute hour day-of-month month day-of-week).
func calculateNextRunWithLoc(schedule string, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	now := time.Now().In(loc)

	// Handle @every <duration> directly.
	if strings.HasPrefix(schedule, "@every ") {
		d, err := time.ParseDuration(strings.TrimPrefix(schedule, "@every "))
		if err != nil {
			slog.Warn("Invalid @every duration", "schedule", schedule, "err", err)
			return now.Add(time.Hour)
		}
		return now.Add(d)
	}

	// Map @descriptor aliases to standard 5-field cron expressions.
	switch schedule {
	case "@hourly":
		schedule = "0 * * * *"
	case "@daily":
		schedule = "0 0 * * *"
	case "@weekly":
		schedule = "0 0 * * 0"
	case "@monthly":
		schedule = "0 0 1 * *"
	}

	// Parse 5-field cron: minute hour day-of-month month day-of-week
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		slog.Warn("Invalid cron expression (expected 5 fields)", "schedule", schedule)
		return now.Add(time.Hour)
	}

	// minute 0-59, hour 0-23, dom 1-31, month 1-12, dow 0-6 (0=Sunday)
	minBits, err0 := parseCronField(fields[0], 0, 59)
	hrBits, err1 := parseCronField(fields[1], 0, 23)
	domBits, err2 := parseCronField(fields[2], 1, 31)
	monBits, err3 := parseCronField(fields[3], 1, 12)
	dowBits, err4 := parseCronField(fields[4], 0, 6)

	for _, err := range []error{err0, err1, err2, err3, err4} {
		if err != nil {
			slog.Warn("Invalid cron field", "schedule", schedule, "err", err)
			return now.Add(time.Hour)
		}
	}

	// Walk forward minute by minute from now+1 (max ~1 year = 525600 minutes).
	t := now.Truncate(time.Minute).Add(time.Minute)
	for range 525600 {
		mo := int(t.Month())   // 1-12
		d := t.Day()           // 1-31
		dw := int(t.Weekday()) // 0=Sunday
		h := t.Hour()          // 0-23
		mi := t.Minute()       // 0-59

		// Skip invalid days-of-month (e.g., Feb 30) — jump to next month.
		if d > 28 {
			lastDay := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, loc).Day()
			if d > lastDay {
				t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
				continue
			}
		}

		if monBits[mo-1] && domBits[d-1] && dowBits[dw] && hrBits[h] && minBits[mi] {
			return t
		}
		t = t.Add(time.Minute)
	}

	slog.Warn("Could not determine next run within 1 year", "schedule", schedule)
	return now.Add(time.Hour)
}

// catchUpMissedTasks runs tasks that were missed during downtime
func (s *Scheduler) catchUpMissedTasks() {
	if s.db == nil {
		return
	}

	now := time.Now()
	catchUpDeadline := now.Add(-s.catchUpWindow)

	s.mu.RLock()
	missed := make([]*Task, 0)
	for _, task := range s.tasks {
		if !task.Enabled {
			continue
		}

		// Per AI.md PART 18 "Startup Behavior": a task is caught up only when
		// its scheduled run is in the past AND the miss is still inside the
		// catch-up window. A run missed by longer than the window is dropped -
		// the task simply waits for its next scheduled slot.
		if task.NextRun.Before(now) && task.NextRun.After(catchUpDeadline) {
			missed = append(missed, task)
		}
	}
	s.mu.RUnlock()

	// Queue missed tasks in order of their original scheduled time.
	sort.Slice(missed, func(i, j int) bool {
		return missed[i].NextRun.Before(missed[j].NextRun)
	})

	for _, task := range missed {
		if !s.beginTask(task) {
			continue
		}
		slog.Info("Catching up missed task", "task", task.ID, "last_run", task.LastRun, "missed_run", task.NextRun)
		go s.runTask(task)
	}
}

// runStartupTasks runs tasks marked with RunOnStart
// Per AI.md PART 18 "Startup Behavior": tasks execute "in order of original scheduled time"
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
	sort.Slice(startupTasks, func(i, j int) bool {
		return startupTasks[i].NextRun.Before(startupTasks[j].NextRun)
	})

	for _, task := range startupTasks {
		if !s.beginTask(task) {
			continue
		}
		slog.Info("Running startup task", "task", task.ID)
		s.runTask(task)
	}
}

// Database operations for persistent state

// initDatabase creates the scheduler_tasks table if not exists
func (s *Scheduler) initDatabase() {
	// Per AI.md PART 10: bulk/DDL operations use 60s context timeout
	initCtx, initCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer initCancel()

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
	if _, err := s.db.ExecContext(initCtx, query); err != nil {
		slog.Error("Failed to create tasks table", "err", err)
	}

	// Idempotent migrations — add columns missing from older schema versions
	// SQLite silently errors if the column already exists; errors are intentionally ignored
	for _, column := range []struct{ name, definition string }{
		{"task_name", "TEXT"}, {"schedule", "TEXT"}, {"last_status", "TEXT"},
		{"fail_count", "INTEGER DEFAULT 0"}, {"retry_count", "INTEGER DEFAULT 0"},
		{"next_retry", "DATETIME"}, {"locked_by", "TEXT"}, {"locked_at", "DATETIME"},
	} {
		query := "ALTER TABLE scheduler_tasks ADD COLUMN " + column.name + " " + column.definition
		if _, err := s.db.ExecContext(initCtx, query); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			slog.Warn("failed to apply scheduler schema migration", "column", column.name, "error", err)
		}
	}

	// Execution history per AI.md PART 18 (task detail "RECENT HISTORY" and
	// the `scheduler history <id>` CLI command). Schema matches the
	// scheduler_history migration in src/database/migrations.go.
	historyQuery := `
		CREATE TABLE IF NOT EXISTS scheduler_history (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			started_at DATETIME NOT NULL,
			finished_at DATETIME,
			success INTEGER NOT NULL DEFAULT 0,
			error TEXT
		)
	`
	if _, err := s.db.ExecContext(initCtx, historyQuery); err != nil {
		slog.Error("Failed to create scheduler history table", "err", err)
	}

	// Idempotent migration — status distinguishes skipped runs from failures,
	// which the migration's success flag alone cannot express
	if _, err := s.db.ExecContext(initCtx, "ALTER TABLE scheduler_history ADD COLUMN status TEXT"); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
		slog.Warn("failed to add scheduler history status column", "error", err)
	}
	if _, err := s.db.ExecContext(initCtx, "CREATE INDEX IF NOT EXISTS idx_scheduler_history_task ON scheduler_history(task_id, started_at)"); err != nil {
		slog.Warn("failed to create scheduler history index", "error", err)
	}
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

	// Per AI.md PART 10: SELECT queries must use context with 5s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.db.QueryRowContext(ctx, query, string(task.ID)).Scan(
		&lastRun, &lastStatus, &lastError, &nextRun, &runCount, &failCount, &enabled, &retryCount, &nextRetry,
	)

	if err == sql.ErrNoRows {
		// New task, insert initial state
		s.saveTaskState(task)
		return
	}

	if err != nil {
		slog.Warn("Failed to load task state", "task", task.ID, "err", err)
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

	// Per AI.md PART 10: INSERT/UPDATE queries must use context with 10s timeout
	saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer saveCancel()
	_, err := s.db.ExecContext(saveCtx, query,
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
		slog.Error("Failed to save task state", "task", task.ID, "err", err)
	}
}

// newHistoryID generates the random identifier for a history row. The
// scheduler_history table keys on a TEXT id (see src/database/migrations.go).
func newHistoryID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// A failed CSPRNG read must not lose the history record; fall back to
		// a timestamp-derived id, which is still unique per task execution.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

// recordHistory persists one execution record and prunes older rows for the
// same task. Per AI.md PART 18 the CLI exposes this as
// `{project_name} scheduler history <id>` and the task detail view's
// "RECENT HISTORY" section.
func (s *Scheduler) recordHistory(task *Task, startedAt, finishedAt time.Time, status TaskStatus, errMsg string) {
	if s.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	insert := `INSERT INTO scheduler_history (id, task_id, started_at, finished_at, success, error, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	success := 0
	if status == StatusSuccess {
		success = 1
	}

	if _, err := s.db.ExecContext(ctx, insert, newHistoryID(), string(task.ID), startedAt, finishedAt, success, errMsg, string(status)); err != nil {
		slog.Error("Failed to record task history", "task", task.ID, "err", err)
		return
	}

	prune := `DELETE FROM scheduler_history WHERE task_id = ? AND id NOT IN (
		SELECT id FROM scheduler_history WHERE task_id = ? ORDER BY started_at DESC LIMIT ?)`
	if _, err := s.db.ExecContext(ctx, prune, string(task.ID), string(task.ID), historyRetainPerTask); err != nil {
		slog.Warn("Failed to prune task history", "task", task.ID, "err", err)
	}
}

// GetHistory returns the most recent execution records for a task, newest
// first. Per AI.md PART 18 CLI Commands: `scheduler history <id>`.
func (s *Scheduler) GetHistory(id TaskID, limit int) ([]*TaskHistoryEntry, error) {
	if s.db == nil {
		return nil, fmt.Errorf("scheduler history requires a database")
	}
	if limit <= 0 || limit > historyRetainPerTask {
		limit = historyRetainPerTask
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `SELECT started_at, finished_at, success, error, status
		FROM scheduler_history WHERE task_id = ? ORDER BY started_at DESC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, string(id), limit)
	if err != nil {
		return nil, fmt.Errorf("query task history: %w", err)
	}
	defer closeutil.Close(rows)

	entries := make([]*TaskHistoryEntry, 0, limit)
	for rows.Next() {
		var startedAt time.Time
		var finishedAt sql.NullTime
		var success int
		var errMsg, status sql.NullString

		if err := rows.Scan(&startedAt, &finishedAt, &success, &errMsg, &status); err != nil {
			return nil, fmt.Errorf("scan task history: %w", err)
		}

		entry := &TaskHistoryEntry{
			TaskID:    string(id),
			StartedAt: startedAt,
			Success:   success == 1,
			Status:    status.String,
			Error:     errMsg.String,
		}
		if finishedAt.Valid {
			entry.FinishedAt = finishedAt.Time
			entry.Duration = finishedAt.Time.Sub(startedAt)
		}
		if entry.Status == "" {
			entry.Status = string(StatusFailed)
			if entry.Success {
				entry.Status = string(StatusSuccess)
			}
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read task history: %w", err)
	}

	return entries, nil
}

// emitAudit hands a completed execution to the audit callback.
// Per AI.md PART 18 Implementation Requirements: all task executions are
// logged to the audit log.
func (s *Scheduler) emitAudit(task *Task, status TaskStatus, errMsg string, attempts int, startedAt time.Time, duration time.Duration) {
	s.mu.RLock()
	auditFn := s.auditFunc
	s.mu.RUnlock()

	if auditFn == nil {
		return
	}

	auditFn(&TaskExecution{
		TaskID:    string(task.ID),
		TaskName:  task.Name,
		Status:    status,
		Error:     errMsg,
		Attempts:  attempts,
		StartedAt: startedAt,
		Duration:  duration,
	})
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

	// Per AI.md PART 10: UPDATE queries must use context with 10s timeout
	lockCtx, lockCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer lockCancel()
	result, err := s.db.ExecContext(lockCtx, query, s.nodeID, now, string(task.ID), s.nodeID, now.Add(-lockTimeout))
	if err != nil {
		slog.Warn("Failed to acquire lock", "task", task.ID, "err", err)
		return false
	}

	rows, _ := result.RowsAffected()
	return rows > 0
}

// releaseTaskLock releases the distributed lock for a task
func (s *Scheduler) releaseTaskLock(task *Task) {
	query := `UPDATE scheduler_tasks SET locked_by = NULL, locked_at = NULL WHERE task_id = ? AND locked_by = ?`
	// Per AI.md PART 10: UPDATE queries must use context with 10s timeout
	relCtx, relCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer relCancel()
	if _, err := s.db.ExecContext(relCtx, query, string(task.ID), s.nodeID); err != nil {
		slog.Warn("failed to release scheduler task lock", "task_id", task.ID, "error", err)
	}
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

	// Per AI.md PART 18 "Shutdown Behavior": no new task executions are
	// accepted once shutdown has started. beginTask also rejects a manual run
	// while the same task is already executing.
	if !s.beginTask(task) {
		return fmt.Errorf("task %s is already running or the scheduler is shutting down", id)
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

	// Stop accepting new task executions, then drain the dispatch loop.
	s.cancel()
	s.wg.Wait()

	// Per AI.md PART 18 "Shutdown Behavior": wait for running tasks to
	// complete, for at most 30 seconds. The waiter goroutine always finishes
	// once the last task returns, whether or not the timeout fired first.
	drained := make(chan struct{})
	go func() {
		s.taskWG.Wait()
		close(drained)
	}()

	select {
	case <-drained:
		slog.Info("Scheduler tasks completed before shutdown deadline")
	case <-time.After(shutdownGracePeriod):
		slog.Warn("Scheduler shutdown timeout - forcing lock release", "grace_period", shutdownGracePeriod)
		s.markInterruptedTasks()
	}

	// Release all locks held by this node (force release on the timeout path)
	// and persist final task state.
	s.forceReleaseLocks()
	s.saveAllTaskState()

	slog.Info("Scheduler stopped")
}

// markInterruptedTasks flags tasks still executing at the shutdown deadline so
// they are retried on the next start.
// Per AI.md PART 18 "Shutdown Behavior": "Mark interrupted tasks for retry on
// next start."
func (s *Scheduler) markInterruptedTasks() {
	s.mu.Lock()
	interrupted := make([]*Task, 0)
	for _, task := range s.tasks {
		if task.LastStatus == StatusRunning || task.LastStatus == StatusRetrying {
			task.LastStatus = StatusFailed
			task.LastError = "interrupted by shutdown"
			// Due immediately on the next start so the catch-up pass picks it up.
			task.NextRun = time.Now()
			interrupted = append(interrupted, task)
		}
	}
	s.mu.Unlock()

	for _, task := range interrupted {
		slog.Warn("Task interrupted by shutdown - marked for retry on next start", "task", task.ID)
	}
}

// forceReleaseLocks clears every scheduler lock held by this node.
// Per AI.md PART 18 "Shutdown Behavior": release all locks (force release when
// the grace period expires).
func (s *Scheduler) forceReleaseLocks() {
	if s.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `UPDATE scheduler_tasks SET locked_by = NULL, locked_at = NULL WHERE locked_by = ?`
	if _, err := s.db.ExecContext(ctx, query, s.nodeID); err != nil {
		slog.Error("Failed to release scheduler locks on shutdown", "err", err)
	}
}

// saveAllTaskState persists every task's state at shutdown.
// Per AI.md PART 18 "Shutdown Behavior": "Save scheduler state to database."
func (s *Scheduler) saveAllTaskState() {
	if s.db == nil {
		return
	}

	s.mu.RLock()
	tasks := make([]*Task, 0, len(s.tasks))
	for _, task := range s.tasks {
		tasks = append(tasks, task)
	}
	s.mu.RUnlock()

	for _, task := range tasks {
		s.saveTaskState(task)
	}
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

// Status returns the current status of all registered tasks.
// Used by the debug /scheduler endpoint per AI.md PART 6.
func (s *Scheduler) Status() []*TaskInfo {
	return s.GetTasks()
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

	// Retry state per AI.md PART 18
	RetryCount int       `json:"retry_count"`
	NextRetry  time.Time `json:"next_retry,omitempty"`
	MaxRetries int       `json:"max_retries"`
}
