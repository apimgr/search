package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

// ClusterScheduler extends Scheduler with cluster-safe distributed locking
// Per AI.md PART 9: Database-backed scheduler with cluster-safe locking
type ClusterScheduler struct {
	*Scheduler
	db         *sql.DB
	nodeID     string
	hostname   string
	lockTTL    time.Duration
	mu         sync.RWMutex
	isLeader   bool
	leaderChan chan struct{}
}

// TaskExecution represents a task execution record in the database
type TaskExecution struct {
	ID           int64     `json:"id"`
	TaskName     string    `json:"task_name"`
	NodeID       string    `json:"node_id"`
	Hostname     string    `json:"hostname"`
	Status       string    `json:"status"` // running, completed, failed
	StartedAt    time.Time `json:"started_at"`
	CompletedAt  time.Time `json:"completed_at,omitempty"`
	Error        string    `json:"error,omitempty"`
	ScheduledAt  time.Time `json:"scheduled_at"`
}

// TaskLock represents a distributed lock for a task
type TaskLock struct {
	TaskName  string    `json:"task_name"`
	NodeID    string    `json:"node_id"`
	Hostname  string    `json:"hostname"`
	AcquiredAt time.Time `json:"acquired_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// NewClusterScheduler creates a new cluster-aware scheduler
func NewClusterScheduler(db *sql.DB, nodeID string) (*ClusterScheduler, error) {
	hostname, _ := os.Hostname()
	if nodeID == "" {
		nodeID = fmt.Sprintf("%s-%d", hostname, time.Now().UnixNano())
	}

	cs := &ClusterScheduler{
		Scheduler:  New(db, nodeID),
		db:         db,
		nodeID:     nodeID,
		hostname:   hostname,
		lockTTL:    5 * time.Minute,
		leaderChan: make(chan struct{}),
	}

	if err := cs.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize scheduler schema: %w", err)
	}

	return cs, nil
}

// initSchema creates necessary database tables for cluster scheduling
func (cs *ClusterScheduler) initSchema() error {
	queries := []string{
		// Task locks table for distributed locking
		`CREATE TABLE IF NOT EXISTS scheduler_locks (
			task_name TEXT PRIMARY KEY,
			node_id TEXT NOT NULL,
			hostname TEXT NOT NULL,
			acquired_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL
		)`,

		// Task execution history
		`CREATE TABLE IF NOT EXISTS scheduler_executions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_name TEXT NOT NULL,
			node_id TEXT NOT NULL,
			hostname TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			error TEXT,
			scheduled_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Index for quick lookups
		`CREATE INDEX IF NOT EXISTS idx_scheduler_executions_task
		 ON scheduler_executions(task_name, started_at DESC)`,

		// Task state table for tracking next runs across cluster
		`CREATE TABLE IF NOT EXISTS scheduler_state (
			task_name TEXT PRIMARY KEY,
			last_run DATETIME,
			next_run DATETIME,
			last_node_id TEXT,
			last_hostname TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range queries {
		if _, err := cs.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute schema query: %w", err)
		}
	}

	return nil
}

// AcquireLock attempts to acquire a distributed lock for a task
func (cs *ClusterScheduler) AcquireLock(ctx context.Context, taskName string) (bool, error) {
	now := time.Now()
	expiresAt := now.Add(cs.lockTTL)

	// First, clean up expired locks
	_, err := cs.db.ExecContext(ctx,
		`DELETE FROM scheduler_locks WHERE expires_at < ?`, now)
	if err != nil {
		log.Printf("[ClusterScheduler] Warning: failed to clean expired locks: %v", err)
	}

	// Try to insert a new lock (will fail if lock exists)
	result, err := cs.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO scheduler_locks
		 (task_name, node_id, hostname, acquired_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)`,
		taskName, cs.nodeID, cs.hostname, now, expiresAt)
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	if rowsAffected > 0 {
		return true, nil
	}

	// Lock exists, check if it's ours or expired
	var lock TaskLock
	err = cs.db.QueryRowContext(ctx,
		`SELECT task_name, node_id, hostname, acquired_at, expires_at
		 FROM scheduler_locks WHERE task_name = ?`, taskName).Scan(
		&lock.TaskName, &lock.NodeID, &lock.Hostname,
		&lock.AcquiredAt, &lock.ExpiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			// Lock was deleted, retry
			return cs.AcquireLock(ctx, taskName)
		}
		return false, err
	}

	// If lock is ours, extend it
	if lock.NodeID == cs.nodeID {
		_, err = cs.db.ExecContext(ctx,
			`UPDATE scheduler_locks SET expires_at = ? WHERE task_name = ? AND node_id = ?`,
			expiresAt, taskName, cs.nodeID)
		return err == nil, err
	}

	// Lock belongs to another node
	return false, nil
}

// ReleaseLock releases a distributed lock
func (cs *ClusterScheduler) ReleaseLock(ctx context.Context, taskName string) error {
	_, err := cs.db.ExecContext(ctx,
		`DELETE FROM scheduler_locks WHERE task_name = ? AND node_id = ?`,
		taskName, cs.nodeID)
	return err
}

// RecordExecution records a task execution
func (cs *ClusterScheduler) RecordExecution(ctx context.Context, taskName string, scheduledAt time.Time) (int64, error) {
	result, err := cs.db.ExecContext(ctx,
		`INSERT INTO scheduler_executions
		 (task_name, node_id, hostname, status, started_at, scheduled_at)
		 VALUES (?, ?, ?, 'running', ?, ?)`,
		taskName, cs.nodeID, cs.hostname, time.Now(), scheduledAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CompleteExecution marks an execution as completed
func (cs *ClusterScheduler) CompleteExecution(ctx context.Context, executionID int64, err error) error {
	status := "completed"
	errStr := ""
	if err != nil {
		status = "failed"
		errStr = err.Error()
	}

	_, dbErr := cs.db.ExecContext(ctx,
		`UPDATE scheduler_executions
		 SET status = ?, completed_at = ?, error = ?
		 WHERE id = ?`,
		status, time.Now(), errStr, executionID)
	return dbErr
}

// UpdateTaskState updates the shared task state
func (cs *ClusterScheduler) UpdateTaskState(ctx context.Context, taskName string, lastRun, nextRun time.Time) error {
	_, err := cs.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO scheduler_state
		 (task_name, last_run, next_run, last_node_id, last_hostname, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		taskName, lastRun, nextRun, cs.nodeID, cs.hostname, time.Now())
	return err
}

// GetTaskState gets the shared task state
func (cs *ClusterScheduler) GetTaskState(ctx context.Context, taskName string) (*ClusterTaskState, error) {
	var state ClusterTaskState
	err := cs.db.QueryRowContext(ctx,
		`SELECT task_name, last_run, next_run, last_node_id, last_hostname, updated_at
		 FROM scheduler_state WHERE task_name = ?`, taskName).Scan(
		&state.TaskName, &state.LastRun, &state.NextRun,
		&state.LastNodeID, &state.LastHostname, &state.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

// ClusterTaskState represents shared task state across the cluster
type ClusterTaskState struct {
	TaskName     string    `json:"task_name"`
	LastRun      time.Time `json:"last_run"`
	NextRun      time.Time `json:"next_run"`
	LastNodeID   string    `json:"last_node_id"`
	LastHostname string    `json:"last_hostname"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GetMissedJobs returns tasks that were missed while the cluster was down
// Per AI.md PART 9: Catchup for missed jobs
func (cs *ClusterScheduler) GetMissedJobs(ctx context.Context) ([]*ClusterTaskState, error) {
	rows, err := cs.db.QueryContext(ctx,
		`SELECT task_name, last_run, next_run, last_node_id, last_hostname, updated_at
		 FROM scheduler_state
		 WHERE next_run < ?`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missed []*ClusterTaskState
	for rows.Next() {
		var state ClusterTaskState
		if err := rows.Scan(
			&state.TaskName, &state.LastRun, &state.NextRun,
			&state.LastNodeID, &state.LastHostname, &state.UpdatedAt); err != nil {
			return nil, err
		}
		missed = append(missed, &state)
	}
	return missed, rows.Err()
}

// RunWithLock runs a task with distributed locking
func (cs *ClusterScheduler) RunWithLock(ctx context.Context, task *Task) error {
	taskName := string(task.ID)

	// Try to acquire lock
	acquired, err := cs.AcquireLock(ctx, taskName)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !acquired {
		log.Printf("[ClusterScheduler] Task %s is running on another node, skipping", taskName)
		return nil
	}

	// Ensure lock is released when done
	defer func() {
		if err := cs.ReleaseLock(context.Background(), taskName); err != nil {
			log.Printf("[ClusterScheduler] Warning: failed to release lock for %s: %v", taskName, err)
		}
	}()

	// Record execution start
	scheduledAt := time.Now()
	execID, err := cs.RecordExecution(ctx, taskName, scheduledAt)
	if err != nil {
		log.Printf("[ClusterScheduler] Warning: failed to record execution: %v", err)
	}

	// Run the task
	taskErr := task.Run(ctx)

	// Record completion
	if execID > 0 {
		if err := cs.CompleteExecution(ctx, execID, taskErr); err != nil {
			log.Printf("[ClusterScheduler] Warning: failed to complete execution record: %v", err)
		}
	}

	// Update shared state - use task's calculated NextRun
	if err := cs.UpdateTaskState(ctx, taskName, time.Now(), task.NextRun); err != nil {
		log.Printf("[ClusterScheduler] Warning: failed to update task state: %v", err)
	}

	return taskErr
}

// StartCluster starts the cluster-aware scheduler
func (cs *ClusterScheduler) StartCluster() {
	cs.mu.Lock()
	if cs.Scheduler.running {
		cs.mu.Unlock()
		return
	}
	cs.Scheduler.running = true
	cs.mu.Unlock()

	// Check for missed jobs on startup
	go cs.catchupMissedJobs()

	cs.Scheduler.wg.Add(1)
	go cs.runCluster()
}

// catchupMissedJobs runs any tasks that were missed
func (cs *ClusterScheduler) catchupMissedJobs() {
	ctx := context.Background()

	missed, err := cs.GetMissedJobs(ctx)
	if err != nil {
		log.Printf("[ClusterScheduler] Error checking missed jobs: %v", err)
		return
	}

	for _, state := range missed {
		cs.mu.RLock()
		task, ok := cs.tasks[TaskID(state.TaskName)]
		cs.mu.RUnlock()

		if !ok || !task.Enabled {
			continue
		}

		log.Printf("[ClusterScheduler] Running missed job: %s (was scheduled for %s)",
			state.TaskName, state.NextRun.Format(time.RFC3339))

		if err := cs.RunWithLock(ctx, task); err != nil {
			log.Printf("[ClusterScheduler] Missed job %s failed: %v", state.TaskName, err)
		}
	}
}

// runCluster is the main cluster-aware scheduler loop
func (cs *ClusterScheduler) runCluster() {
	defer cs.Scheduler.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cs.Scheduler.ctx.Done():
			return
		case now := <-ticker.C:
			cs.checkAndRunClusterTasks(now)
		}
	}
}

// checkAndRunClusterTasks checks and runs due tasks with cluster locking
func (cs *ClusterScheduler) checkAndRunClusterTasks(now time.Time) {
	cs.mu.RLock()
	var dueTasks []*Task
	for _, task := range cs.tasks {
		if task.Enabled && now.After(task.NextRun) {
			dueTasks = append(dueTasks, task)
		}
	}
	cs.mu.RUnlock()

	for _, task := range dueTasks {
		go func(t *Task) {
			// Use 30 minute timeout for task execution
			ctx, cancel := context.WithTimeout(cs.Scheduler.ctx, 30*time.Minute)
			defer cancel()

			if err := cs.RunWithLock(ctx, t); err != nil {
				log.Printf("[ClusterScheduler] Task %s failed: %v", t.ID, err)
			}
		}(task)
	}
}

// GetExecutionHistory returns recent executions for a task
func (cs *ClusterScheduler) GetExecutionHistory(ctx context.Context, taskName string, limit int) ([]*TaskExecution, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := cs.db.QueryContext(ctx,
		`SELECT id, task_name, node_id, hostname, status, started_at, completed_at, error, scheduled_at
		 FROM scheduler_executions
		 WHERE task_name = ?
		 ORDER BY started_at DESC
		 LIMIT ?`, taskName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var executions []*TaskExecution
	for rows.Next() {
		var exec TaskExecution
		var completedAt sql.NullTime
		var errStr sql.NullString

		if err := rows.Scan(
			&exec.ID, &exec.TaskName, &exec.NodeID, &exec.Hostname,
			&exec.Status, &exec.StartedAt, &completedAt, &errStr, &exec.ScheduledAt); err != nil {
			return nil, err
		}

		if completedAt.Valid {
			exec.CompletedAt = completedAt.Time
		}
		if errStr.Valid {
			exec.Error = errStr.String
		}

		executions = append(executions, &exec)
	}

	return executions, rows.Err()
}

// CleanupOldExecutions removes old execution records
func (cs *ClusterScheduler) CleanupOldExecutions(ctx context.Context, retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention)

	result, err := cs.db.ExecContext(ctx,
		`DELETE FROM scheduler_executions WHERE started_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// NodeID returns the current node's ID
func (cs *ClusterScheduler) NodeID() string {
	return cs.nodeID
}

// Hostname returns the current node's hostname
func (cs *ClusterScheduler) Hostname() string {
	return cs.hostname
}

// SetLockTTL sets the lock time-to-live
func (cs *ClusterScheduler) SetLockTTL(ttl time.Duration) {
	cs.lockTTL = ttl
}
