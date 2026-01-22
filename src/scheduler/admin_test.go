package scheduler

import (
	"context"
	"testing"
	"time"
)

// NewAdminAdapter tests

func TestNewAdminAdapter(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	if adapter == nil {
		t.Fatal("NewAdminAdapter() returned nil")
	}
	if adapter.scheduler != s {
		t.Error("Adapter should reference the scheduler")
	}
}

func TestNewAdminAdapterNilScheduler(t *testing.T) {
	adapter := NewAdminAdapter(nil)

	if adapter == nil {
		t.Fatal("NewAdminAdapter() returned nil")
	}
	if adapter.scheduler != nil {
		t.Error("Adapter should have nil scheduler")
	}
}

// IsRunning tests

func TestAdminAdapterIsRunning(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	if adapter.IsRunning() {
		t.Error("Scheduler should not be running initially")
	}

	s.Start()

	if !adapter.IsRunning() {
		t.Error("Scheduler should be running after Start()")
	}

	s.Stop()

	if adapter.IsRunning() {
		t.Error("Scheduler should not be running after Stop()")
	}
}

func TestAdminAdapterIsRunningNilScheduler(t *testing.T) {
	adapter := NewAdminAdapter(nil)

	if adapter.IsRunning() {
		t.Error("IsRunning() should return false for nil scheduler")
	}
}

// GetTasks tests

func TestAdminAdapterGetTasks(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task := &Task{
		ID:          "test.task",
		Name:        "Test Task",
		Description: "A test task",
		Schedule:    "@every 1h",
		TaskType:    TaskTypeLocal,
		Skippable:   true,
		Run:         func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	tasks := adapter.GetTasks()

	if len(tasks) != 1 {
		t.Fatalf("GetTasks() returned %d tasks, want 1", len(tasks))
	}

	if tasks[0].ID != "test.task" {
		t.Errorf("ID = %q, want %q", tasks[0].ID, "test.task")
	}
	if tasks[0].Name != "Test Task" {
		t.Errorf("Name = %q, want %q", tasks[0].Name, "Test Task")
	}
	if tasks[0].Description != "A test task" {
		t.Errorf("Description = %q, want %q", tasks[0].Description, "A test task")
	}
	if tasks[0].TaskType != "local" {
		t.Errorf("TaskType = %q, want %q", tasks[0].TaskType, "local")
	}
	if !tasks[0].Skippable {
		t.Error("Skippable should be true")
	}
}

func TestAdminAdapterGetTasksNilScheduler(t *testing.T) {
	adapter := NewAdminAdapter(nil)

	tasks := adapter.GetTasks()

	if tasks != nil {
		t.Errorf("GetTasks() should return nil for nil scheduler, got %v", tasks)
	}
}

func TestAdminAdapterGetTasksEmpty(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	tasks := adapter.GetTasks()

	if len(tasks) != 0 {
		t.Errorf("GetTasks() should return empty slice, got %d tasks", len(tasks))
	}
}

func TestAdminAdapterGetTasksMultiple(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task1 := &Task{
		ID:       "task1",
		Name:     "Task 1",
		Schedule: "@every 1h",
		Run:      func(ctx context.Context) error { return nil },
	}
	task2 := &Task{
		ID:       "task2",
		Name:     "Task 2",
		Schedule: "@every 2h",
		Run:      func(ctx context.Context) error { return nil },
	}

	s.Register(task1)
	s.Register(task2)

	tasks := adapter.GetTasks()

	if len(tasks) != 2 {
		t.Errorf("GetTasks() returned %d tasks, want 2", len(tasks))
	}
}

func TestAdminAdapterGetTasksAllFields(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task := &Task{
		ID:          "full.task",
		Name:        "Full Task",
		Description: "Complete task",
		Schedule:    "@every 1h",
		TaskType:    TaskTypeGlobal,
		Skippable:   true,
		MaxRetries:  5,
		Run:         func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	// Set some runtime state
	s.mu.Lock()
	s.tasks["full.task"].RunCount = 10
	s.tasks["full.task"].FailCount = 2
	s.tasks["full.task"].LastStatus = StatusSuccess
	s.tasks["full.task"].LastError = "previous error"
	s.mu.Unlock()

	tasks := adapter.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(tasks))
	}

	info := tasks[0]
	if info.RunCount != 10 {
		t.Errorf("RunCount = %d, want 10", info.RunCount)
	}
	if info.FailCount != 2 {
		t.Errorf("FailCount = %d, want 2", info.FailCount)
	}
	if info.LastStatus != "success" {
		t.Errorf("LastStatus = %q, want %q", info.LastStatus, "success")
	}
	if info.LastError != "previous error" {
		t.Errorf("LastError = %q, want %q", info.LastError, "previous error")
	}
	if info.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", info.MaxRetries)
	}
}

// GetTask tests

func TestAdminAdapterGetTask(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task := &Task{
		ID:          "test.task",
		Name:        "Test Task",
		Description: "A test task",
		Schedule:    "@every 1h",
		TaskType:    TaskTypeLocal,
		Skippable:   true,
		Run:         func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	info, err := adapter.GetTask("test.task")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if info == nil {
		t.Fatal("GetTask() returned nil")
	}

	if info.ID != "test.task" {
		t.Errorf("ID = %q, want %q", info.ID, "test.task")
	}
	if info.Name != "Test Task" {
		t.Errorf("Name = %q, want %q", info.Name, "Test Task")
	}
	if info.Description != "A test task" {
		t.Errorf("Description = %q, want %q", info.Description, "A test task")
	}
}

func TestAdminAdapterGetTaskNilScheduler(t *testing.T) {
	adapter := NewAdminAdapter(nil)

	info, err := adapter.GetTask("test.task")
	if err != nil {
		t.Errorf("GetTask() should not return error for nil scheduler, got %v", err)
	}
	if info != nil {
		t.Errorf("GetTask() should return nil for nil scheduler, got %v", info)
	}
}

func TestAdminAdapterGetTaskNotFound(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	_, err := adapter.GetTask("nonexistent")
	if err == nil {
		t.Error("GetTask() should return error for nonexistent task")
	}
}

func TestAdminAdapterGetTaskAllFields(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task := &Task{
		ID:          "full.task",
		Name:        "Full Task",
		Description: "Complete task",
		Schedule:    "@every 1h",
		TaskType:    TaskTypeGlobal,
		Skippable:   true,
		MaxRetries:  5,
		Run:         func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	// Set runtime state
	s.mu.Lock()
	s.tasks["full.task"].RunCount = 10
	s.tasks["full.task"].FailCount = 2
	s.tasks["full.task"].RetryCount = 1
	s.mu.Unlock()

	info, err := adapter.GetTask("full.task")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}

	if info.RunCount != 10 {
		t.Errorf("RunCount = %d, want 10", info.RunCount)
	}
	if info.FailCount != 2 {
		t.Errorf("FailCount = %d, want 2", info.FailCount)
	}
	if info.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", info.RetryCount)
	}
	if info.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", info.MaxRetries)
	}
}

// Enable tests

func TestAdminAdapterEnable(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task := &Task{
		ID:        "test.task",
		Name:      "Test Task",
		Schedule:  "@every 1h",
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	// Disable first
	s.Disable("test.task")

	// Enable via adapter
	err := adapter.Enable("test.task")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	info, _ := s.GetTask("test.task")
	if !info.Enabled {
		t.Error("Task should be enabled after Enable()")
	}
}

func TestAdminAdapterEnableNilScheduler(t *testing.T) {
	adapter := NewAdminAdapter(nil)

	err := adapter.Enable("test.task")
	if err != nil {
		t.Errorf("Enable() should not return error for nil scheduler, got %v", err)
	}
}

func TestAdminAdapterEnableNotFound(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	err := adapter.Enable("nonexistent")
	if err == nil {
		t.Error("Enable() should return error for nonexistent task")
	}
}

func TestAdminAdapterEnableNotSkippable(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task := &Task{
		ID:        "test.task",
		Name:      "Test Task",
		Schedule:  "@every 1h",
		Skippable: false,
		Run:       func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	err := adapter.Enable("test.task")
	if err == nil {
		t.Error("Enable() should return error for non-skippable task")
	}
}

// Disable tests

func TestAdminAdapterDisable(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task := &Task{
		ID:        "test.task",
		Name:      "Test Task",
		Schedule:  "@every 1h",
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	// Disable via adapter
	err := adapter.Disable("test.task")
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	info, _ := s.GetTask("test.task")
	if info.Enabled {
		t.Error("Task should be disabled after Disable()")
	}
}

func TestAdminAdapterDisableNilScheduler(t *testing.T) {
	adapter := NewAdminAdapter(nil)

	err := adapter.Disable("test.task")
	if err != nil {
		t.Errorf("Disable() should not return error for nil scheduler, got %v", err)
	}
}

func TestAdminAdapterDisableNotFound(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	err := adapter.Disable("nonexistent")
	if err == nil {
		t.Error("Disable() should return error for nonexistent task")
	}
}

func TestAdminAdapterDisableNotSkippable(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	task := &Task{
		ID:        "test.task",
		Name:      "Test Task",
		Schedule:  "@every 1h",
		Skippable: false,
		Run:       func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	err := adapter.Disable("test.task")
	if err == nil {
		t.Error("Disable() should return error for non-skippable task")
	}
}

// RunNow tests

func TestAdminAdapterRunNow(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	ran := make(chan bool, 1)
	task := &Task{
		ID:       "test.task",
		Name:     "Test Task",
		Schedule: "@every 1h",
		TaskType: TaskTypeLocal,
		Run: func(ctx context.Context) error {
			ran <- true
			return nil
		},
	}
	s.Register(task)
	s.Start()
	defer s.Stop()

	err := adapter.RunNow("test.task")
	if err != nil {
		t.Fatalf("RunNow() error = %v", err)
	}

	// Wait for task
	select {
	case <-ran:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Task was not run")
	}
}

func TestAdminAdapterRunNowNilScheduler(t *testing.T) {
	adapter := NewAdminAdapter(nil)

	err := adapter.RunNow("test.task")
	if err != nil {
		t.Errorf("RunNow() should not return error for nil scheduler, got %v", err)
	}
}

func TestAdminAdapterRunNowNotFound(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	err := adapter.RunNow("nonexistent")
	if err == nil {
		t.Error("RunNow() should return error for nonexistent task")
	}
}

// Integration tests

func TestAdminAdapterFullWorkflow(t *testing.T) {
	s := New(nil, "node1")
	adapter := NewAdminAdapter(s)

	// Register task
	task := &Task{
		ID:        "workflow.task",
		Name:      "Workflow Task",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}
	s.Register(task)
	s.Start()
	defer s.Stop()

	// Verify initial state
	if !adapter.IsRunning() {
		t.Error("Scheduler should be running")
	}

	tasks := adapter.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(tasks))
	}

	info, _ := adapter.GetTask("workflow.task")
	if !info.Enabled {
		t.Error("Task should be enabled initially")
	}

	// Disable task
	adapter.Disable("workflow.task")
	info, _ = adapter.GetTask("workflow.task")
	if info.Enabled {
		t.Error("Task should be disabled")
	}

	// Enable task
	adapter.Enable("workflow.task")
	info, _ = adapter.GetTask("workflow.task")
	if !info.Enabled {
		t.Error("Task should be enabled")
	}

	// Run task
	err := adapter.RunNow("workflow.task")
	if err != nil {
		t.Errorf("RunNow() error = %v", err)
	}
}
