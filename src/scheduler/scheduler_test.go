package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTaskIDConstants(t *testing.T) {
	tests := []struct {
		id   TaskID
		want string
	}{
		{TaskSSLRenewal, "ssl.renewal"},
		{TaskGeoIPUpdate, "geoip.update"},
		{TaskBlocklistUpdate, "blocklist.update"},
		{TaskCVEUpdate, "cve.update"},
		{TaskSessionCleanup, "session.cleanup"},
		{TaskTokenCleanup, "token.cleanup"},
		{TaskLogRotation, "log.rotation"},
		{TaskBackupDaily, "backup_daily"},
		{TaskBackupHourly, "backup_hourly"},
		{TaskHealthcheckSelf, "healthcheck.self"},
		{TaskTorHealth, "tor.health"},
		{TaskClusterHeartbeat, "cluster.heartbeat"},
	}

	for _, tt := range tests {
		if string(tt.id) != tt.want {
			t.Errorf("TaskID = %q, want %q", tt.id, tt.want)
		}
	}
}

func TestTaskStatusConstants(t *testing.T) {
	tests := []struct {
		status TaskStatus
		want   string
	}{
		{StatusSuccess, "success"},
		{StatusFailed, "failed"},
		{StatusSkipped, "skipped"},
		{StatusRunning, "running"},
		{StatusRetrying, "retrying"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("TaskStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

func TestTaskTypeConstants(t *testing.T) {
	if string(TaskTypeGlobal) != "global" {
		t.Errorf("TaskTypeGlobal = %q, want %q", TaskTypeGlobal, "global")
	}
	if string(TaskTypeLocal) != "local" {
		t.Errorf("TaskTypeLocal = %q, want %q", TaskTypeLocal, "local")
	}
}

func TestDefaultRetryConstants(t *testing.T) {
	if DefaultMaxRetries != 3 {
		t.Errorf("DefaultMaxRetries = %d, want 3", DefaultMaxRetries)
	}
	if DefaultRetryDelay != 5*time.Minute {
		t.Errorf("DefaultRetryDelay = %v, want 5m", DefaultRetryDelay)
	}
}

func TestTaskStruct(t *testing.T) {
	task := Task{
		ID:          "test.task",
		Name:        "Test Task",
		Description: "A test task",
		Schedule:    "@every 1h",
		TaskType:    TaskTypeLocal,
		Run:         func(ctx context.Context) error { return nil },
		Skippable:   true,
		RunOnStart:  false,
		MaxRetries:  5,
		RetryDelay:  10 * time.Minute,
		Enabled:     true,
	}

	if string(task.ID) != "test.task" {
		t.Errorf("ID = %q, want %q", task.ID, "test.task")
	}
	if task.Name != "Test Task" {
		t.Errorf("Name = %q, want %q", task.Name, "Test Task")
	}
	if task.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want 5", task.MaxRetries)
	}
}

func TestTaskInfoStruct(t *testing.T) {
	now := time.Now()
	info := TaskInfo{
		ID:          "test.task",
		Name:        "Test Task",
		Description: "A test task",
		Schedule:    "@every 1h",
		TaskType:    "local",
		LastRun:     now,
		LastStatus:  "success",
		NextRun:     now.Add(time.Hour),
		RunCount:    10,
		FailCount:   1,
		Enabled:     true,
		Skippable:   true,
		RetryCount:  0,
		MaxRetries:  3,
	}

	if info.ID != "test.task" {
		t.Errorf("ID = %q, want %q", info.ID, "test.task")
	}
	if info.RunCount != 10 {
		t.Errorf("RunCount = %d, want 10", info.RunCount)
	}
	if !info.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestNew(t *testing.T) {
	s := New(nil, "node1")

	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.nodeID != "node1" {
		t.Errorf("nodeID = %q, want %q", s.nodeID, "node1")
	}
	if s.tasks == nil {
		t.Error("tasks map should not be nil")
	}
	if s.timezone == nil {
		t.Error("timezone should not be nil")
	}
	if s.catchUpWindow != time.Hour {
		t.Errorf("catchUpWindow = %v, want 1h", s.catchUpWindow)
	}
}

func TestSchedulerSetTimezone(t *testing.T) {
	s := New(nil, "node1")

	err := s.SetTimezone("UTC")
	if err != nil {
		t.Fatalf("SetTimezone(UTC) error = %v", err)
	}

	err = s.SetTimezone("America/New_York")
	if err != nil {
		t.Fatalf("SetTimezone(America/New_York) error = %v", err)
	}

	err = s.SetTimezone("Invalid/Timezone")
	if err == nil {
		t.Error("SetTimezone() should fail for invalid timezone")
	}
}

func TestSchedulerSetCatchUpWindow(t *testing.T) {
	s := New(nil, "node1")

	s.SetCatchUpWindow(2 * time.Hour)

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.catchUpWindow != 2*time.Hour {
		t.Errorf("catchUpWindow = %v, want 2h", s.catchUpWindow)
	}
}

func TestSchedulerSetNotifyFunc(t *testing.T) {
	s := New(nil, "node1")

	var notified bool
	s.SetNotifyFunc(func(n *TaskFailureNotification) {
		notified = true
	})

	s.mu.RLock()
	hasNotifyFunc := s.notifyFunc != nil
	s.mu.RUnlock()

	if !hasNotifyFunc {
		t.Error("notifyFunc should be set")
	}

	_ = notified
}

func TestSchedulerRegister(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:       "test.task",
		Name:     "Test",
		Schedule: "@every 1h",
		Run:      func(ctx context.Context) error { return nil },
	}

	err := s.Register(task)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	s.mu.RLock()
	_, exists := s.tasks["test.task"]
	s.mu.RUnlock()

	if !exists {
		t.Error("Task should be registered")
	}
	if !task.Enabled {
		t.Error("Task should be enabled by default")
	}
}

func TestSchedulerRegisterMissingID(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		Name:     "Test",
		Schedule: "@every 1h",
		Run:      func(ctx context.Context) error { return nil },
	}

	err := s.Register(task)
	if err == nil {
		t.Error("Register() should fail without ID")
	}
}

func TestSchedulerRegisterMissingSchedule(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:   "test.task",
		Name: "Test",
		Run:  func(ctx context.Context) error { return nil },
	}

	err := s.Register(task)
	if err == nil {
		t.Error("Register() should fail without schedule")
	}
}

func TestSchedulerRegisterMissingRun(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:       "test.task",
		Name:     "Test",
		Schedule: "@every 1h",
	}

	err := s.Register(task)
	if err == nil {
		t.Error("Register() should fail without run function")
	}
}

func TestSchedulerStartStop(t *testing.T) {
	s := New(nil, "node1")

	s.Start()

	if !s.IsRunning() {
		t.Error("Scheduler should be running after Start()")
	}

	s.Stop()

	if s.IsRunning() {
		t.Error("Scheduler should not be running after Stop()")
	}
}

func TestSchedulerStartTwice(t *testing.T) {
	s := New(nil, "node1")

	s.Start()
	s.Start() // Should be no-op

	if !s.IsRunning() {
		t.Error("Scheduler should still be running")
	}

	s.Stop()
}

func TestSchedulerStopTwice(t *testing.T) {
	s := New(nil, "node1")

	s.Start()
	s.Stop()
	s.Stop() // Should be no-op

	if s.IsRunning() {
		t.Error("Scheduler should not be running")
	}
}

func TestSchedulerEnable(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:        "test.task",
		Name:      "Test",
		Schedule:  "@every 1h",
		Run:       func(ctx context.Context) error { return nil },
		Skippable: true,
	}
	s.Register(task)

	// Disable first
	task.Enabled = false

	err := s.Enable("test.task")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if !task.Enabled {
		t.Error("Task should be enabled after Enable()")
	}
}

func TestSchedulerEnableNotSkippable(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:        "test.task",
		Name:      "Test",
		Schedule:  "@every 1h",
		Run:       func(ctx context.Context) error { return nil },
		Skippable: false,
	}
	s.Register(task)

	err := s.Enable("test.task")
	if err == nil {
		t.Error("Enable() should fail for non-skippable task")
	}
}

func TestSchedulerEnableNotFound(t *testing.T) {
	s := New(nil, "node1")

	err := s.Enable("nonexistent")
	if err == nil {
		t.Error("Enable() should fail for nonexistent task")
	}
}

func TestSchedulerDisable(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:        "test.task",
		Name:      "Test",
		Schedule:  "@every 1h",
		Run:       func(ctx context.Context) error { return nil },
		Skippable: true,
	}
	s.Register(task)

	err := s.Disable("test.task")
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	if task.Enabled {
		t.Error("Task should be disabled after Disable()")
	}
}

func TestSchedulerDisableNotSkippable(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:        "test.task",
		Name:      "Test",
		Schedule:  "@every 1h",
		Run:       func(ctx context.Context) error { return nil },
		Skippable: false,
	}
	s.Register(task)

	err := s.Disable("test.task")
	if err == nil {
		t.Error("Disable() should fail for non-skippable task")
	}
}

func TestSchedulerGetTasks(t *testing.T) {
	s := New(nil, "node1")

	task1 := &Task{ID: "task1", Name: "Task 1", Schedule: "@every 1h", Run: func(ctx context.Context) error { return nil }}
	task2 := &Task{ID: "task2", Name: "Task 2", Schedule: "@every 2h", Run: func(ctx context.Context) error { return nil }}

	s.Register(task1)
	s.Register(task2)

	tasks := s.GetTasks()

	if len(tasks) != 2 {
		t.Errorf("GetTasks() returned %d tasks, want 2", len(tasks))
	}
}

func TestSchedulerGetTask(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:       "test.task",
		Name:     "Test Task",
		Schedule: "@every 1h",
		Run:      func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	info, err := s.GetTask("test.task")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}

	if info.ID != "test.task" {
		t.Errorf("ID = %q, want %q", info.ID, "test.task")
	}
	if info.Name != "Test Task" {
		t.Errorf("Name = %q, want %q", info.Name, "Test Task")
	}
}

func TestSchedulerGetTaskNotFound(t *testing.T) {
	s := New(nil, "node1")

	_, err := s.GetTask("nonexistent")
	if err == nil {
		t.Error("GetTask() should fail for nonexistent task")
	}
}

func TestSchedulerRunNow(t *testing.T) {
	s := New(nil, "node1")

	ran := make(chan bool, 1)
	task := &Task{
		ID:       "test.task",
		Name:     "Test",
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

	err := s.RunNow("test.task")
	if err != nil {
		t.Fatalf("RunNow() error = %v", err)
	}

	// Wait for task to run
	select {
	case <-ran:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Task did not run within timeout")
	}
}

func TestSchedulerRunNowNotFound(t *testing.T) {
	s := New(nil, "node1")

	err := s.RunNow("nonexistent")
	if err == nil {
		t.Error("RunNow() should fail for nonexistent task")
	}
}

func TestCalculateNextRunEveryInterval(t *testing.T) {
	s := New(nil, "node1")
	s.SetTimezone("UTC")

	tests := []struct {
		schedule string
		minDelta time.Duration
		maxDelta time.Duration
	}{
		{"@every 1h", 59 * time.Minute, 61 * time.Minute},
		{"@every 30m", 29 * time.Minute, 31 * time.Minute},
		{"@every 5m", 4 * time.Minute, 6 * time.Minute},
		{"@every 30s", 29 * time.Second, 31 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.schedule, func(t *testing.T) {
			before := time.Now()
			next := s.calculateNextRun(tt.schedule)
			delta := next.Sub(before)

			if delta < tt.minDelta || delta > tt.maxDelta {
				t.Errorf("Next run for %s is %v from now, expected between %v and %v",
					tt.schedule, delta, tt.minDelta, tt.maxDelta)
			}
		})
	}
}

func TestCalculateNextRunPredefined(t *testing.T) {
	s := New(nil, "node1")
	s.SetTimezone("UTC")

	now := time.Now()

	tests := []struct {
		schedule string
		check    func(next time.Time) bool
	}{
		{"@hourly", func(next time.Time) bool {
			return next.After(now) && next.Before(now.Add(2*time.Hour))
		}},
		{"@daily", func(next time.Time) bool {
			return next.After(now) && next.Before(now.Add(25*time.Hour))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.schedule, func(t *testing.T) {
			next := s.calculateNextRun(tt.schedule)
			if !tt.check(next) {
				t.Errorf("Next run for %s = %v, invalid", tt.schedule, next)
			}
		})
	}
}

func TestCalculateNextRunCron(t *testing.T) {
	s := New(nil, "node1")
	s.SetTimezone("UTC")

	// Simple cron expressions
	tests := []string{
		"0 * * * *",  // Every hour at :00
		"*/5 * * * *", // Every 5 minutes
		"0 0 * * *",  // Daily at midnight
	}

	for _, schedule := range tests {
		t.Run(schedule, func(t *testing.T) {
			next := s.calculateNextRun(schedule)
			if next.Before(time.Now()) {
				t.Errorf("Next run for %s is in the past: %v", schedule, next)
			}
		})
	}
}

func TestParseCronField(t *testing.T) {
	tests := []struct {
		field   string
		min     int
		max     int
		wantLen int
		wantErr bool
	}{
		{"*", 0, 59, 60, false},
		{"*/5", 0, 59, 12, false},
		{"0", 0, 59, 1, false},
		{"5,10,15", 0, 59, 3, false},
		{"5-10", 0, 59, 6, false},
		{"5-10/2", 0, 59, 3, false},
		{"100", 0, 59, 0, true}, // Out of range
		{"invalid", 0, 59, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			result, err := parseCronField(tt.field, tt.min, tt.max)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if len(result) != tt.wantLen {
					t.Errorf("Result length = %d, want %d", len(result), tt.wantLen)
				}
			}
		})
	}
}

func TestParseCronExpression(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"0 0 * * *", false},      // Valid: daily at midnight
		{"*/15 * * * *", false},   // Valid: every 15 minutes
		{"0 3 * * 0", false},      // Valid: Sunday at 3am
		{"0 0 1 * *", false},      // Valid: first of month
		{"invalid", true},         // Invalid: not enough fields
		{"0 0 * * * *", true},     // Invalid: too many fields
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			now := time.Now().In(loc)
			_, err := parseCronExpression(tt.expr, now, loc)
			if tt.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestMakeRange(t *testing.T) {
	tests := []struct {
		start int
		end   int
		step  int
		want  []int
	}{
		{0, 5, 1, []int{0, 1, 2, 3, 4, 5}},
		{0, 10, 2, []int{0, 2, 4, 6, 8, 10}},
		{5, 5, 1, []int{5}},
		{0, 59, 15, []int{0, 15, 30, 45}},
	}

	for _, tt := range tests {
		result := makeRange(tt.start, tt.end, tt.step)
		if len(result) != len(tt.want) {
			t.Errorf("makeRange(%d, %d, %d) length = %d, want %d",
				tt.start, tt.end, tt.step, len(result), len(tt.want))
			continue
		}
		for i, v := range result {
			if v != tt.want[i] {
				t.Errorf("makeRange(%d, %d, %d)[%d] = %d, want %d",
					tt.start, tt.end, tt.step, i, v, tt.want[i])
			}
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		slice []int
		val   int
		want  bool
	}{
		{[]int{1, 2, 3}, 2, true},
		{[]int{1, 2, 3}, 4, false},
		{[]int{}, 1, false},
		{[]int{5}, 5, true},
	}

	for _, tt := range tests {
		if got := contains(tt.slice, tt.val); got != tt.want {
			t.Errorf("contains(%v, %d) = %v, want %v", tt.slice, tt.val, got, tt.want)
		}
	}
}

func TestTaskHandlersStruct(t *testing.T) {
	handlers := &TaskHandlers{
		SSLRenewal:       func(ctx context.Context) error { return nil },
		GeoIPUpdate:      func(ctx context.Context) error { return nil },
		SessionCleanup:   func(ctx context.Context) error { return nil },
		HealthcheckSelf:  func(ctx context.Context) error { return nil },
	}

	if handlers.SSLRenewal == nil {
		t.Error("SSLRenewal handler should be set")
	}
	if handlers.GeoIPUpdate == nil {
		t.Error("GeoIPUpdate handler should be set")
	}
}

func TestRegisterBuiltinTasks(t *testing.T) {
	s := New(nil, "node1")

	handlers := &TaskHandlers{
		SSLRenewal:      func(ctx context.Context) error { return nil },
		SessionCleanup:  func(ctx context.Context) error { return nil },
		TokenCleanup:    func(ctx context.Context) error { return nil },
		HealthcheckSelf: func(ctx context.Context) error { return nil },
	}

	s.RegisterBuiltinTasks(handlers)

	tasks := s.GetTasks()
	if len(tasks) == 0 {
		t.Error("No builtin tasks registered")
	}

	// Verify SSL renewal task
	sslTask, err := s.GetTask(TaskSSLRenewal)
	if err != nil {
		t.Fatalf("SSL renewal task not found: %v", err)
	}
	if sslTask.Skippable {
		t.Error("SSL renewal task should not be skippable")
	}
}

func TestTaskFailureNotification(t *testing.T) {
	now := time.Now()
	n := TaskFailureNotification{
		TaskID:    "test.task",
		TaskName:  "Test Task",
		Error:     "test error",
		Attempts:  3,
		LastRun:   now,
		FailCount: 5,
	}

	if n.TaskID != "test.task" {
		t.Errorf("TaskID = %q, want %q", n.TaskID, "test.task")
	}
	if n.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", n.Attempts)
	}
	if n.FailCount != 5 {
		t.Errorf("FailCount = %d, want 5", n.FailCount)
	}
}

func TestTaskStateStruct(t *testing.T) {
	now := time.Now()
	state := TaskState{
		TaskID:     "test.task",
		TaskName:   "Test Task",
		Schedule:   "@every 1h",
		LastRun:    now,
		LastStatus: "success",
		NextRun:    now.Add(time.Hour),
		RunCount:   10,
		FailCount:  1,
		Enabled:    true,
	}

	if state.TaskID != "test.task" {
		t.Errorf("TaskID = %q, want %q", state.TaskID, "test.task")
	}
	if state.RunCount != 10 {
		t.Errorf("RunCount = %d, want 10", state.RunCount)
	}
}

func TestSchedulerTaskRetry(t *testing.T) {
	s := New(nil, "node1")

	attempts := 0
	task := &Task{
		ID:         "retry.test",
		Name:       "Retry Test",
		Schedule:   "@every 1h",
		TaskType:   TaskTypeLocal,
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond, // Short delay for testing
		Run: func(ctx context.Context) error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		},
	}

	s.Register(task)
	s.Start()

	// Run the task manually
	s.RunNow("retry.test")

	// Wait for retries
	time.Sleep(500 * time.Millisecond)

	s.Stop()

	if attempts < 3 {
		t.Errorf("Task ran %d times, expected at least 3 (initial + 2 retries)", attempts)
	}
}
