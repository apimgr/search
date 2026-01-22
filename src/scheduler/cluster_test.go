package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// Test structs

func TestTaskExecutionStruct(t *testing.T) {
	now := time.Now()
	exec := TaskExecution{
		ID:          1,
		TaskName:    "test.task",
		NodeID:      "node1",
		Hostname:    "host1",
		Status:      "completed",
		StartedAt:   now,
		CompletedAt: now.Add(time.Second),
		Error:       "",
		ScheduledAt: now,
	}

	if exec.ID != 1 {
		t.Errorf("ID = %d, want 1", exec.ID)
	}
	if exec.TaskName != "test.task" {
		t.Errorf("TaskName = %q, want %q", exec.TaskName, "test.task")
	}
	if exec.Status != "completed" {
		t.Errorf("Status = %q, want %q", exec.Status, "completed")
	}
}

func TestTaskLockStruct(t *testing.T) {
	now := time.Now()
	lock := TaskLock{
		TaskName:   "test.task",
		NodeID:     "node1",
		Hostname:   "host1",
		AcquiredAt: now,
		ExpiresAt:  now.Add(5 * time.Minute),
	}

	if lock.TaskName != "test.task" {
		t.Errorf("TaskName = %q, want %q", lock.TaskName, "test.task")
	}
	if lock.NodeID != "node1" {
		t.Errorf("NodeID = %q, want %q", lock.NodeID, "node1")
	}
}

func TestClusterTaskStateStruct(t *testing.T) {
	now := time.Now()
	state := ClusterTaskState{
		TaskName:     "test.task",
		LastRun:      now,
		NextRun:      now.Add(time.Hour),
		LastNodeID:   "node1",
		LastHostname: "host1",
		UpdatedAt:    now,
	}

	if state.TaskName != "test.task" {
		t.Errorf("TaskName = %q, want %q", state.TaskName, "test.task")
	}
	if state.LastNodeID != "node1" {
		t.Errorf("LastNodeID = %q, want %q", state.LastNodeID, "node1")
	}
}

// NewClusterScheduler tests

func TestNewClusterScheduler(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	if cs == nil {
		t.Fatal("NewClusterScheduler() returned nil")
	}
	if cs.nodeID != "node1" {
		t.Errorf("nodeID = %q, want %q", cs.nodeID, "node1")
	}
	if cs.lockTTL != 5*time.Minute {
		t.Errorf("lockTTL = %v, want 5m", cs.lockTTL)
	}
}

func TestNewClusterSchedulerEmptyNodeID(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	// Should generate a node ID based on hostname
	if cs.nodeID == "" {
		t.Error("nodeID should be auto-generated when empty")
	}
}

func TestNewClusterSchedulerInitSchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	// Verify tables were created
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='scheduler_locks'").Scan(&count)
	if err != nil || count != 1 {
		t.Error("scheduler_locks table was not created")
	}

	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='scheduler_executions'").Scan(&count)
	if err != nil || count != 1 {
		t.Error("scheduler_executions table was not created")
	}

	err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='scheduler_state'").Scan(&count)
	if err != nil || count != 1 {
		t.Error("scheduler_state table was not created")
	}

	_ = cs
}

// AcquireLock tests

func TestClusterSchedulerAcquireLock(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	acquired, err := cs.AcquireLock(ctx, "test.task")
	if err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}
	if !acquired {
		t.Error("Should be able to acquire lock on first attempt")
	}
}

func TestClusterSchedulerAcquireLockAlreadyHeld(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs1, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	cs2, err := NewClusterScheduler(db, "node2")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Node1 acquires lock
	acquired1, err := cs1.AcquireLock(ctx, "test.task")
	if err != nil || !acquired1 {
		t.Fatalf("Node1 should acquire lock")
	}

	// Node2 tries to acquire same lock
	acquired2, err := cs2.AcquireLock(ctx, "test.task")
	if err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}
	if acquired2 {
		t.Error("Node2 should not acquire lock held by node1")
	}
}

func TestClusterSchedulerAcquireLockExtendOwn(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Acquire lock first time
	acquired1, err := cs.AcquireLock(ctx, "test.task")
	if err != nil || !acquired1 {
		t.Fatalf("Should acquire lock first time")
	}

	// Extend own lock
	acquired2, err := cs.AcquireLock(ctx, "test.task")
	if err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}
	if !acquired2 {
		t.Error("Should be able to extend own lock")
	}
}

func TestClusterSchedulerAcquireLockExpired(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs1, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}
	cs1.SetLockTTL(1 * time.Millisecond) // Very short TTL

	cs2, err := NewClusterScheduler(db, "node2")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Node1 acquires lock
	acquired1, _ := cs1.AcquireLock(ctx, "test.task")
	if !acquired1 {
		t.Fatal("Node1 should acquire lock")
	}

	// Wait for lock to expire
	time.Sleep(10 * time.Millisecond)

	// Node2 should be able to acquire expired lock
	acquired2, err := cs2.AcquireLock(ctx, "test.task")
	if err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}
	if !acquired2 {
		t.Error("Node2 should acquire expired lock")
	}
}

func TestClusterSchedulerAcquireLockRetryAfterDelete(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Create and immediately delete lock to test retry path
	_, err = db.Exec(`INSERT INTO scheduler_locks (task_name, node_id, hostname, acquired_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		"test.task", "other_node", "other_host", time.Now(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("Failed to insert lock: %v", err)
	}

	// Acquire should clean up expired and succeed
	acquired, err := cs.AcquireLock(ctx, "test.task")
	if err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}
	if !acquired {
		t.Error("Should acquire lock after cleaning expired")
	}
}

// ReleaseLock tests

func TestClusterSchedulerReleaseLock(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Acquire lock
	cs.AcquireLock(ctx, "test.task")

	// Release lock
	err = cs.ReleaseLock(ctx, "test.task")
	if err != nil {
		t.Fatalf("ReleaseLock() error = %v", err)
	}

	// Verify lock was released
	var count int
	db.QueryRow("SELECT COUNT(*) FROM scheduler_locks WHERE task_name = ?", "test.task").Scan(&count)
	if count != 0 {
		t.Error("Lock should be released")
	}
}

func TestClusterSchedulerReleaseLockNotOwned(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs1, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	cs2, err := NewClusterScheduler(db, "node2")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Node1 acquires lock
	cs1.AcquireLock(ctx, "test.task")

	// Node2 tries to release - should not release node1's lock
	cs2.ReleaseLock(ctx, "test.task")

	// Lock should still exist
	var count int
	db.QueryRow("SELECT COUNT(*) FROM scheduler_locks WHERE task_name = ?", "test.task").Scan(&count)
	if count != 1 {
		t.Error("Lock should not be released by non-owner")
	}
}

// RecordExecution tests

func TestClusterSchedulerRecordExecution(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()
	scheduledAt := time.Now()

	execID, err := cs.RecordExecution(ctx, "test.task", scheduledAt)
	if err != nil {
		t.Fatalf("RecordExecution() error = %v", err)
	}
	if execID <= 0 {
		t.Error("Should return valid execution ID")
	}

	// Verify record was created
	var status string
	db.QueryRow("SELECT status FROM scheduler_executions WHERE id = ?", execID).Scan(&status)
	if status != "running" {
		t.Errorf("status = %q, want %q", status, "running")
	}
}

// CompleteExecution tests

func TestClusterSchedulerCompleteExecutionSuccess(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Record execution
	execID, _ := cs.RecordExecution(ctx, "test.task", time.Now())

	// Complete with success
	err = cs.CompleteExecution(ctx, execID, nil)
	if err != nil {
		t.Fatalf("CompleteExecution() error = %v", err)
	}

	// Verify status
	var status string
	db.QueryRow("SELECT status FROM scheduler_executions WHERE id = ?", execID).Scan(&status)
	if status != "completed" {
		t.Errorf("status = %q, want %q", status, "completed")
	}
}

func TestClusterSchedulerCompleteExecutionFailed(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Record execution
	execID, _ := cs.RecordExecution(ctx, "test.task", time.Now())

	// Complete with error
	taskErr := errors.New("task failed")
	err = cs.CompleteExecution(ctx, execID, taskErr)
	if err != nil {
		t.Fatalf("CompleteExecution() error = %v", err)
	}

	// Verify status and error
	var status, errStr string
	db.QueryRow("SELECT status, error FROM scheduler_executions WHERE id = ?", execID).Scan(&status, &errStr)
	if status != "failed" {
		t.Errorf("status = %q, want %q", status, "failed")
	}
	if errStr != "task failed" {
		t.Errorf("error = %q, want %q", errStr, "task failed")
	}
}

// UpdateTaskState tests

func TestClusterSchedulerUpdateTaskState(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	nextRun := now.Add(time.Hour)

	err = cs.UpdateTaskState(ctx, "test.task", now, nextRun)
	if err != nil {
		t.Fatalf("UpdateTaskState() error = %v", err)
	}

	// Verify state was saved
	var taskName string
	db.QueryRow("SELECT task_name FROM scheduler_state WHERE task_name = ?", "test.task").Scan(&taskName)
	if taskName != "test.task" {
		t.Error("Task state was not saved")
	}
}

func TestClusterSchedulerUpdateTaskStateReplace(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()
	now := time.Now()

	// First update
	cs.UpdateTaskState(ctx, "test.task", now, now.Add(time.Hour))

	// Second update (replace)
	newNextRun := now.Add(2 * time.Hour)
	err = cs.UpdateTaskState(ctx, "test.task", now, newNextRun)
	if err != nil {
		t.Fatalf("UpdateTaskState() error = %v", err)
	}

	// Verify only one row exists
	var count int
	db.QueryRow("SELECT COUNT(*) FROM scheduler_state WHERE task_name = ?", "test.task").Scan(&count)
	if count != 1 {
		t.Errorf("Should have exactly 1 row, got %d", count)
	}
}

// GetTaskState tests

func TestClusterSchedulerGetTaskState(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()
	now := time.Now()
	nextRun := now.Add(time.Hour)

	// Save state
	cs.UpdateTaskState(ctx, "test.task", now, nextRun)

	// Get state
	state, err := cs.GetTaskState(ctx, "test.task")
	if err != nil {
		t.Fatalf("GetTaskState() error = %v", err)
	}
	if state == nil {
		t.Fatal("GetTaskState() returned nil")
	}
	if state.TaskName != "test.task" {
		t.Errorf("TaskName = %q, want %q", state.TaskName, "test.task")
	}
	if state.LastNodeID != "node1" {
		t.Errorf("LastNodeID = %q, want %q", state.LastNodeID, "node1")
	}
}

func TestClusterSchedulerGetTaskStateNotFound(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	state, err := cs.GetTaskState(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetTaskState() error = %v", err)
	}
	if state != nil {
		t.Error("GetTaskState() should return nil for nonexistent task")
	}
}

// GetMissedJobs tests

func TestClusterSchedulerGetMissedJobs(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()
	past := time.Now().Add(-time.Hour)

	// Create missed job (next_run in the past)
	cs.UpdateTaskState(ctx, "missed.task", past.Add(-2*time.Hour), past)

	missed, err := cs.GetMissedJobs(ctx)
	if err != nil {
		t.Fatalf("GetMissedJobs() error = %v", err)
	}
	if len(missed) != 1 {
		t.Errorf("Expected 1 missed job, got %d", len(missed))
	}
	if len(missed) > 0 && missed[0].TaskName != "missed.task" {
		t.Errorf("TaskName = %q, want %q", missed[0].TaskName, "missed.task")
	}
}

func TestClusterSchedulerGetMissedJobsNone(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()
	future := time.Now().Add(time.Hour)

	// Create future job (not missed)
	cs.UpdateTaskState(ctx, "future.task", time.Now(), future)

	missed, err := cs.GetMissedJobs(ctx)
	if err != nil {
		t.Fatalf("GetMissedJobs() error = %v", err)
	}
	if len(missed) != 0 {
		t.Errorf("Expected 0 missed jobs, got %d", len(missed))
	}
}

// RunWithLock tests

func TestClusterSchedulerRunWithLock(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ran := false
	task := &Task{
		ID:       "test.task",
		Name:     "Test Task",
		Schedule: "@every 1h",
		Run: func(ctx context.Context) error {
			ran = true
			return nil
		},
	}

	ctx := context.Background()
	err = cs.RunWithLock(ctx, task)
	if err != nil {
		t.Fatalf("RunWithLock() error = %v", err)
	}
	if !ran {
		t.Error("Task should have run")
	}
}

func TestClusterSchedulerRunWithLockBlocked(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs1, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	cs2, err := NewClusterScheduler(db, "node2")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Node1 acquires lock
	cs1.AcquireLock(ctx, "test.task")

	ran := false
	task := &Task{
		ID:       "test.task",
		Name:     "Test Task",
		Schedule: "@every 1h",
		Run: func(ctx context.Context) error {
			ran = true
			return nil
		},
	}

	// Node2 tries to run - should be blocked
	err = cs2.RunWithLock(ctx, task)
	if err != nil {
		t.Fatalf("RunWithLock() error = %v", err)
	}
	if ran {
		t.Error("Task should not run when lock is held by another node")
	}
}

func TestClusterSchedulerRunWithLockTaskError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	taskErr := errors.New("task failed")
	task := &Task{
		ID:       "fail.task",
		Name:     "Fail Task",
		Schedule: "@every 1h",
		Run: func(ctx context.Context) error {
			return taskErr
		},
	}

	ctx := context.Background()
	err = cs.RunWithLock(ctx, task)
	if err != taskErr {
		t.Errorf("RunWithLock() error = %v, want %v", err, taskErr)
	}

	// Verify execution was recorded as failed
	var status string
	db.QueryRow("SELECT status FROM scheduler_executions WHERE task_name = ?", "fail.task").Scan(&status)
	if status != "failed" {
		t.Errorf("status = %q, want %q", status, "failed")
	}
}

// StartCluster tests

func TestClusterSchedulerStartCluster(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	cs.StartCluster()

	if !cs.Scheduler.running {
		t.Error("Scheduler should be running after StartCluster()")
	}

	cs.Scheduler.Stop()
}

func TestClusterSchedulerStartClusterTwice(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	cs.StartCluster()
	cs.StartCluster() // Second call should be no-op

	if !cs.Scheduler.running {
		t.Error("Scheduler should still be running")
	}

	cs.Scheduler.Stop()
}

// catchupMissedJobs tests

func TestClusterSchedulerCatchupMissedJobs(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ran := make(chan bool, 1)
	task := &Task{
		ID:       "missed.task",
		Name:     "Missed Task",
		Schedule: "@every 1h",
		Run: func(ctx context.Context) error {
			select {
			case ran <- true:
			default:
			}
			return nil
		},
	}

	cs.Register(task)

	// Create missed job state
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	cs.UpdateTaskState(ctx, "missed.task", past.Add(-2*time.Hour), past)

	// Start will trigger catchup
	cs.StartCluster()

	// Wait for catchup
	select {
	case <-ran:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Missed task was not caught up")
	}

	cs.Scheduler.Stop()
}

func TestClusterSchedulerCatchupMissedJobsDisabled(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	task := &Task{
		ID:        "disabled.missed",
		Name:      "Disabled Missed",
		Schedule:  "@every 1h",
		Skippable: true,
		Run: func(ctx context.Context) error {
			t.Error("Disabled task should not run")
			return nil
		},
	}

	cs.Register(task)
	cs.Scheduler.Disable("disabled.missed")

	// Create missed job state
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	cs.UpdateTaskState(ctx, "disabled.missed", past.Add(-2*time.Hour), past)

	// Manually call catchup
	cs.catchupMissedJobs()

	time.Sleep(100 * time.Millisecond)
}

func TestClusterSchedulerCatchupMissedJobsUnknownTask(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	// Create missed job state for unknown task
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	cs.UpdateTaskState(ctx, "unknown.task", past.Add(-2*time.Hour), past)

	// Should not panic
	cs.catchupMissedJobs()
}

// checkAndRunClusterTasks tests

func TestClusterSchedulerCheckAndRunClusterTasks(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ran := make(chan bool, 1)
	task := &Task{
		ID:       "due.task",
		Name:     "Due Task",
		Schedule: "@every 1ms",
		Run: func(ctx context.Context) error {
			select {
			case ran <- true:
			default:
			}
			return nil
		},
	}

	cs.Register(task)

	// Set next run to past
	cs.mu.Lock()
	cs.tasks["due.task"].NextRun = time.Now().Add(-time.Minute)
	cs.mu.Unlock()

	cs.StartCluster()

	select {
	case <-ran:
		// Success
	case <-time.After(3 * time.Second):
		t.Error("Due task was not run")
	}

	cs.Scheduler.Stop()
}

// GetExecutionHistory tests

func TestClusterSchedulerGetExecutionHistory(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Record some executions
	for i := 0; i < 5; i++ {
		execID, _ := cs.RecordExecution(ctx, "test.task", time.Now())
		cs.CompleteExecution(ctx, execID, nil)
	}

	// Get history with limit
	history, err := cs.GetExecutionHistory(ctx, "test.task", 3)
	if err != nil {
		t.Fatalf("GetExecutionHistory() error = %v", err)
	}
	if len(history) != 3 {
		t.Errorf("Expected 3 executions, got %d", len(history))
	}
}

func TestClusterSchedulerGetExecutionHistoryDefaultLimit(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Record some executions
	for i := 0; i < 5; i++ {
		execID, _ := cs.RecordExecution(ctx, "test.task", time.Now())
		cs.CompleteExecution(ctx, execID, nil)
	}

	// Get history with default limit (0 should become 10)
	history, err := cs.GetExecutionHistory(ctx, "test.task", 0)
	if err != nil {
		t.Fatalf("GetExecutionHistory() error = %v", err)
	}
	if len(history) != 5 { // We only have 5 records
		t.Errorf("Expected 5 executions, got %d", len(history))
	}
}

func TestClusterSchedulerGetExecutionHistoryWithError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Record execution with error
	execID, _ := cs.RecordExecution(ctx, "fail.task", time.Now())
	cs.CompleteExecution(ctx, execID, errors.New("task error"))

	history, err := cs.GetExecutionHistory(ctx, "fail.task", 10)
	if err != nil {
		t.Fatalf("GetExecutionHistory() error = %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("Expected 1 execution, got %d", len(history))
	}
	if history[0].Status != "failed" {
		t.Errorf("Status = %q, want %q", history[0].Status, "failed")
	}
	if history[0].Error != "task error" {
		t.Errorf("Error = %q, want %q", history[0].Error, "task error")
	}
}

// CleanupOldExecutions tests

func TestClusterSchedulerCleanupOldExecutions(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ctx := context.Background()

	// Insert old execution directly
	oldTime := time.Now().Add(-48 * time.Hour)
	_, err = db.Exec(`INSERT INTO scheduler_executions
		(task_name, node_id, hostname, status, started_at, scheduled_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"old.task", "node1", "host1", "completed", oldTime, oldTime)
	if err != nil {
		t.Fatalf("Failed to insert old execution: %v", err)
	}

	// Insert recent execution
	cs.RecordExecution(ctx, "recent.task", time.Now())

	// Cleanup executions older than 24 hours
	deleted, err := cs.CleanupOldExecutions(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupOldExecutions() error = %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted, got %d", deleted)
	}

	// Verify recent execution still exists
	var count int
	db.QueryRow("SELECT COUNT(*) FROM scheduler_executions").Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 remaining execution, got %d", count)
	}
}

// NodeID and Hostname tests

func TestClusterSchedulerNodeID(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "test-node-1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	if cs.NodeID() != "test-node-1" {
		t.Errorf("NodeID() = %q, want %q", cs.NodeID(), "test-node-1")
	}
}

func TestClusterSchedulerHostname(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	// Hostname should not be empty
	if cs.Hostname() == "" {
		t.Error("Hostname() should not be empty")
	}
}

// SetLockTTL tests

func TestClusterSchedulerSetLockTTL(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	cs.SetLockTTL(10 * time.Minute)

	if cs.lockTTL != 10*time.Minute {
		t.Errorf("lockTTL = %v, want 10m", cs.lockTTL)
	}
}

// runCluster loop test

func TestClusterSchedulerRunClusterLoop(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ran := make(chan bool, 10)
	task := &Task{
		ID:       "loop.task",
		Name:     "Loop Task",
		Schedule: "@every 1ms",
		Run: func(ctx context.Context) error {
			select {
			case ran <- true:
			default:
			}
			return nil
		},
	}

	cs.Register(task)

	// Set next run to past
	cs.mu.Lock()
	cs.tasks["loop.task"].NextRun = time.Now().Add(-time.Minute)
	cs.mu.Unlock()

	cs.StartCluster()

	// Wait for at least one run
	select {
	case <-ran:
		// Success
	case <-time.After(3 * time.Second):
		t.Error("Task was not run by cluster loop")
	}

	cs.Scheduler.Stop()
}

// Test task execution with error during recording

func TestClusterSchedulerRunWithLockRecordingError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	cs, err := NewClusterScheduler(db, "node1")
	if err != nil {
		t.Fatalf("NewClusterScheduler() error = %v", err)
	}

	ran := false
	task := &Task{
		ID:       "record.error.task",
		Name:     "Record Error Task",
		Schedule: "@every 1h",
		Run: func(ctx context.Context) error {
			ran = true
			return nil
		},
	}

	// Drop the table to cause recording error (but task should still run)
	db.Exec("DROP TABLE scheduler_executions")

	ctx := context.Background()
	cs.RunWithLock(ctx, task)

	// Task should have run despite recording error
	if !ran {
		t.Error("Task should have run despite recording error")
	}
}
