package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
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

func TestSchedulerStopWithoutStart(t *testing.T) {
	s := New(nil, "node1")

	// Stop without start - should be no-op
	s.Stop()

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

func TestSchedulerDisableNotFound(t *testing.T) {
	s := New(nil, "node1")

	err := s.Disable("nonexistent")
	if err == nil {
		t.Error("Disable() should fail for nonexistent task")
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

func TestSchedulerGetTasksWithDefaultMaxRetries(t *testing.T) {
	s := New(nil, "node1")

	// Task with no MaxRetries set (should default to 3)
	task := &Task{
		ID:       "task1",
		Name:     "Task 1",
		Schedule: "@every 1h",
		Run:      func(ctx context.Context) error { return nil },
	}
	s.Register(task)

	tasks := s.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("GetTasks() returned %d tasks, want 1", len(tasks))
	}

	if tasks[0].MaxRetries != DefaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", tasks[0].MaxRetries, DefaultMaxRetries)
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

func TestSchedulerGetTaskWithDefaultMaxRetries(t *testing.T) {
	s := New(nil, "node1")

	// Task with no MaxRetries set
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

	if info.MaxRetries != DefaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", info.MaxRetries, DefaultMaxRetries)
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

func TestCalculateNextRunEveryInvalidInterval(t *testing.T) {
	s := New(nil, "node1")
	s.SetTimezone("UTC")

	// Invalid duration should fall back to 1 hour
	next := s.calculateNextRun("@every invalid")
	now := time.Now()

	// Should be about 1 hour from now (fallback)
	delta := next.Sub(now)
	if delta < 59*time.Minute || delta > 61*time.Minute {
		t.Errorf("Invalid interval should default to 1h, got delta %v", delta)
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
		{"@weekly", func(next time.Time) bool {
			return next.After(now) && next.Before(now.Add(8*24*time.Hour))
		}},
		{"@monthly", func(next time.Time) bool {
			return next.After(now) && next.Before(now.Add(32*24*time.Hour))
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

func TestCalculateNextRunWeeklyOnSunday(t *testing.T) {
	s := New(nil, "node1")
	loc, _ := time.LoadLocation("UTC")
	s.timezone = loc

	// Test @weekly when today is Sunday
	// Find next Sunday at midnight
	now := time.Now().In(loc)
	daysUntilSunday := (7 - int(now.Weekday())) % 7

	next := s.calculateNextRun("@weekly")

	// Should be this coming Sunday or next Sunday
	if next.Before(now) {
		t.Errorf("@weekly returned time in the past: %v", next)
	}
	if next.Weekday() != time.Sunday {
		t.Errorf("@weekly should return Sunday, got %v", next.Weekday())
	}

	_ = daysUntilSunday
}

func TestCalculateNextRunCron(t *testing.T) {
	s := New(nil, "node1")
	s.SetTimezone("UTC")

	// Simple cron expressions
	tests := []string{
		"0 * * * *",   // Every hour at :00
		"*/5 * * * *", // Every 5 minutes
		"0 0 * * *",   // Daily at midnight
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

func TestCalculateNextRunCronInvalid(t *testing.T) {
	s := New(nil, "node1")
	s.SetTimezone("UTC")

	// Invalid cron should fall back to 1 hour
	next := s.calculateNextRun("invalid cron expression here today")
	now := time.Now()

	// Should be about 1 hour from now (fallback)
	delta := next.Sub(now)
	if delta < 59*time.Minute || delta > 61*time.Minute {
		t.Errorf("Invalid cron should default to 1h, got delta %v", delta)
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
		{"100", 0, 59, 0, true},   // Out of range
		{"invalid", 0, 59, 0, true},
		{"-5", 0, 59, 0, true},    // Invalid number
		{"*/0", 0, 59, 0, true},   // Zero step
		{"*/-1", 0, 59, 0, true},  // Negative step
		{"*/abc", 0, 59, 0, true}, // Invalid step value
		{"10-5", 0, 59, 0, true},  // Start > end
		{"5-100", 0, 59, 0, true}, // End out of range
		{"-1-5", 0, 59, 0, true},  // Start out of range
		{"5-10/0", 0, 59, 0, true},  // Zero step in range
		{"5-10/abc", 0, 59, 0, true}, // Invalid step in range
		{"a-10", 0, 59, 0, true},     // Invalid range start
		{"5-b", 0, 59, 0, true},      // Invalid range end
		{"5-10-15", 0, 59, 0, true},  // Multiple dashes without step
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

func TestParseCronFieldListWithRanges(t *testing.T) {
	// Test list containing ranges: "1,5-10,15"
	result, err := parseCronField("1,5-10,15", 0, 59)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Should have 1, 5,6,7,8,9,10, 15 = 8 values
	if len(result) != 8 {
		t.Errorf("Result length = %d, want 8", len(result))
	}
}

func TestParseCronFieldListWithSteps(t *testing.T) {
	// Test list containing steps: "0,*/15"
	result, err := parseCronField("0,*/15", 0, 59)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// Should have 0 + 0,15,30,45 = 5 values (0 appears twice but is counted)
	if len(result) < 4 {
		t.Errorf("Result length = %d, want at least 4", len(result))
	}
}

func TestParseCronFieldListWithInvalidPart(t *testing.T) {
	// Test list with invalid part
	_, err := parseCronField("1,invalid,15", 0, 59)
	if err == nil {
		t.Error("Expected error for list with invalid part")
	}
}

func TestParseCronExpression(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"0 0 * * *", false},    // Valid: daily at midnight
		{"*/15 * * * *", false}, // Valid: every 15 minutes
		{"0 3 * * 0", false},    // Valid: Sunday at 3am
		{"0 0 1 * *", false},    // Valid: first of month
		{"invalid", true},       // Invalid: not enough fields
		{"0 0 * * * *", true},   // Invalid: too many fields
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

func TestParseCronExpressionInvalidFields(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now().In(loc)

	tests := []struct {
		name string
		expr string
	}{
		{"invalid minute", "invalid 0 * * *"},
		{"invalid hour", "0 invalid * * *"},
		{"invalid day", "0 0 invalid * *"},
		{"invalid month", "0 0 * invalid *"},
		{"invalid weekday", "0 0 * * invalid"},
		{"minute out of range", "60 0 * * *"},
		{"hour out of range", "0 24 * * *"},
		{"day out of range", "0 0 32 * *"},
		{"month out of range", "0 0 * 13 *"},
		{"weekday out of range", "0 0 * * 7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCronExpression(tt.expr, now, loc)
			if err == nil {
				t.Errorf("Expected error for %s", tt.name)
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
		SSLRenewal:      func(ctx context.Context) error { return nil },
		GeoIPUpdate:     func(ctx context.Context) error { return nil },
		SessionCleanup:  func(ctx context.Context) error { return nil },
		HealthcheckSelf: func(ctx context.Context) error { return nil },
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

func TestRegisterBuiltinTasksAllHandlers(t *testing.T) {
	s := New(nil, "node1")

	handlers := &TaskHandlers{
		SSLRenewal:       func(ctx context.Context) error { return nil },
		GeoIPUpdate:      func(ctx context.Context) error { return nil },
		BlocklistUpdate:  func(ctx context.Context) error { return nil },
		CVEUpdate:        func(ctx context.Context) error { return nil },
		SessionCleanup:   func(ctx context.Context) error { return nil },
		TokenCleanup:     func(ctx context.Context) error { return nil },
		LogRotation:      func(ctx context.Context) error { return nil },
		BackupDaily:      func(ctx context.Context) error { return nil },
		BackupHourly:     func(ctx context.Context) error { return nil },
		HealthcheckSelf:  func(ctx context.Context) error { return nil },
		TorHealth:        func(ctx context.Context) error { return nil },
		ClusterHeartbeat: func(ctx context.Context) error { return nil },
	}

	s.RegisterBuiltinTasks(handlers)

	tasks := s.GetTasks()
	if len(tasks) != 12 {
		t.Errorf("Expected 12 builtin tasks, got %d", len(tasks))
	}

	// Check BackupHourly is disabled by default
	hourlyTask, err := s.GetTask(TaskBackupHourly)
	if err != nil {
		t.Fatalf("BackupHourly task not found: %v", err)
	}
	if hourlyTask.Enabled {
		t.Error("BackupHourly should be disabled by default")
	}
}

func TestRegisterBuiltinTasksNilHandlers(t *testing.T) {
	s := New(nil, "node1")

	// Register with all nil handlers - should not crash
	handlers := &TaskHandlers{}
	s.RegisterBuiltinTasks(handlers)

	tasks := s.GetTasks()
	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks with nil handlers, got %d", len(tasks))
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

	var attempts int32
	task := &Task{
		ID:         "retry.test",
		Name:       "Retry Test",
		Schedule:   "@every 1h",
		TaskType:   TaskTypeLocal,
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond, // Short delay for testing
		Run: func(ctx context.Context) error {
			atomic.AddInt32(&attempts, 1)
			if atomic.LoadInt32(&attempts) < 3 {
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

	if atomic.LoadInt32(&attempts) < 3 {
		t.Errorf("Task ran %d times, expected at least 3 (initial + 2 retries)", attempts)
	}
}

func TestSchedulerTaskRetryWithDefaultValues(t *testing.T) {
	s := New(nil, "node1")

	var attempts int32
	task := &Task{
		ID:       "retry.default",
		Name:     "Retry Default",
		Schedule: "@every 1h",
		TaskType: TaskTypeLocal,
		// No MaxRetries or RetryDelay set - should use defaults
		Run: func(ctx context.Context) error {
			atomic.AddInt32(&attempts, 1)
			return nil
		},
	}

	s.Register(task)
	s.Start()

	// Run the task
	s.RunNow("retry.default")

	// Wait for task to complete
	time.Sleep(100 * time.Millisecond)

	s.Stop()

	if atomic.LoadInt32(&attempts) < 1 {
		t.Error("Task should have run at least once")
	}
}

func TestSchedulerTaskFailureNotification(t *testing.T) {
	s := New(nil, "node1")

	var notificationReceived bool
	var mu sync.Mutex
	s.SetNotifyFunc(func(n *TaskFailureNotification) {
		mu.Lock()
		notificationReceived = true
		mu.Unlock()
	})

	task := &Task{
		ID:         "fail.test",
		Name:       "Fail Test",
		Schedule:   "@every 1h",
		TaskType:   TaskTypeLocal,
		MaxRetries: 0, // No retries
		RetryDelay: 1 * time.Millisecond,
		Run: func(ctx context.Context) error {
			return errors.New("permanent error")
		},
	}

	s.Register(task)
	s.Start()

	s.RunNow("fail.test")

	// Wait for notification
	time.Sleep(200 * time.Millisecond)

	s.Stop()

	mu.Lock()
	defer mu.Unlock()
	if !notificationReceived {
		t.Error("Failure notification should have been received")
	}
}

func TestSchedulerCheckAndRunTasks(t *testing.T) {
	s := New(nil, "node1")

	ran := make(chan bool, 1)
	task := &Task{
		ID:       "check.test",
		Name:     "Check Test",
		Schedule: "@every 1ms", // Very short interval
		TaskType: TaskTypeLocal,
		Run: func(ctx context.Context) error {
			select {
			case ran <- true:
			default:
			}
			return nil
		},
	}

	s.Register(task)

	// Set next run to the past so it's immediately due
	s.mu.Lock()
	s.tasks["check.test"].NextRun = time.Now().Add(-time.Minute)
	s.mu.Unlock()

	s.Start()

	// Wait for task to be picked up
	select {
	case <-ran:
		// Success
	case <-time.After(3 * time.Second):
		t.Error("Task was not run by checkAndRunTasks")
	}

	s.Stop()
}

func TestSchedulerRunTaskContextCancellation(t *testing.T) {
	s := New(nil, "node1")

	taskStarted := make(chan bool, 1)
	task := &Task{
		ID:         "cancel.test",
		Name:       "Cancel Test",
		Schedule:   "@every 1h",
		TaskType:   TaskTypeLocal,
		MaxRetries: 5,
		RetryDelay: 100 * time.Millisecond,
		Run: func(ctx context.Context) error {
			taskStarted <- true
			// Wait for context cancellation
			<-ctx.Done()
			return ctx.Err()
		},
	}

	s.Register(task)
	s.Start()

	s.RunNow("cancel.test")

	// Wait for task to start
	<-taskStarted

	// Stop scheduler (cancels context)
	s.Stop()

	// Test should complete without hanging
}

func TestSchedulerRunStartupTasks(t *testing.T) {
	s := New(nil, "node1")

	ran := make(chan bool, 1)
	task := &Task{
		ID:         "startup.test",
		Name:       "Startup Test",
		Schedule:   "@every 1h",
		TaskType:   TaskTypeLocal,
		RunOnStart: true,
		Run: func(ctx context.Context) error {
			select {
			case ran <- true:
			default:
			}
			return nil
		},
	}

	s.Register(task)
	s.Start()

	// Wait for startup task
	select {
	case <-ran:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Startup task was not run")
	}

	s.Stop()
}

func TestSchedulerCatchUpMissedTasks(t *testing.T) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")
	s.SetCatchUpWindow(2 * time.Hour)

	ran := make(chan bool, 1)
	task := &Task{
		ID:       "catchup.test",
		Name:     "Catchup Test",
		Schedule: "@every 1h",
		TaskType: TaskTypeLocal,
		Run: func(ctx context.Context) error {
			select {
			case ran <- true:
			default:
			}
			return nil
		},
	}

	s.Register(task)

	// Set last run to be before the catch-up window and next run in the past
	s.mu.Lock()
	s.tasks["catchup.test"].LastRun = time.Now().Add(-3 * time.Hour)
	s.tasks["catchup.test"].NextRun = time.Now().Add(-30 * time.Minute)
	s.mu.Unlock()

	s.Start()

	// Wait for catch-up
	select {
	case <-ran:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Catch-up task was not run")
	}

	s.Stop()
}

func TestSchedulerCatchUpMissedTasksNoDatabase(t *testing.T) {
	s := New(nil, "node1")

	// This should not panic
	s.catchUpMissedTasks()
}

func TestSchedulerCatchUpMissedTasksDisabledTask(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")
	s.SetCatchUpWindow(2 * time.Hour)

	task := &Task{
		ID:        "disabled.catchup",
		Name:      "Disabled Catchup",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run: func(ctx context.Context) error {
			t.Error("Disabled task should not be run")
			return nil
		},
	}

	s.Register(task)

	// Disable the task
	s.Disable("disabled.catchup")

	// Set times for catch-up
	s.mu.Lock()
	s.tasks["disabled.catchup"].LastRun = time.Now().Add(-3 * time.Hour)
	s.tasks["disabled.catchup"].NextRun = time.Now().Add(-30 * time.Minute)
	s.mu.Unlock()

	// Catch-up should skip disabled tasks
	s.catchUpMissedTasks()

	// Give it a moment
	time.Sleep(100 * time.Millisecond)
}

// Database operation tests

func TestSchedulerWithDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")

	task := &Task{
		ID:        "db.test",
		Name:      "DB Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s.Register(task)
	s.Start()

	// Enable/disable should persist
	s.Disable("db.test")

	// Verify task is disabled
	info, _ := s.GetTask("db.test")
	if info.Enabled {
		t.Error("Task should be disabled")
	}

	s.Enable("db.test")

	// Verify task is enabled
	info, _ = s.GetTask("db.test")
	if !info.Enabled {
		t.Error("Task should be enabled")
	}

	s.Stop()
}

func TestSchedulerLoadTaskState(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// First scheduler - create and save state
	s1 := New(db, "node1")

	task := &Task{
		ID:        "persist.test",
		Name:      "Persist Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s1.Register(task)
	s1.Start()

	// Disable the task
	s1.Disable("persist.test")

	s1.Stop()

	// Second scheduler - should load persisted state
	s2 := New(db, "node1")

	task2 := &Task{
		ID:        "persist.test",
		Name:      "Persist Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s2.Register(task2)

	// Check if state was loaded
	info, _ := s2.GetTask("persist.test")
	if info.Enabled {
		t.Error("Task should be disabled from persisted state")
	}
}

func TestSchedulerLoadTaskStateNonSkippable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")

	// First register and run a skippable task to create DB state
	task := &Task{
		ID:        "nonskip.test",
		Name:      "NonSkip Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s.Register(task)
	s.Start()
	s.Disable("nonskip.test")
	s.Stop()

	// Second scheduler with non-skippable version
	s2 := New(db, "node1")

	task2 := &Task{
		ID:        "nonskip.test",
		Name:      "NonSkip Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: false, // Now non-skippable
		Run:       func(ctx context.Context) error { return nil },
	}

	s2.Register(task2)

	// Non-skippable task should remain enabled regardless of DB state
	info, _ := s2.GetTask("nonskip.test")
	if !info.Enabled {
		t.Error("Non-skippable task should remain enabled")
	}
}

func TestSchedulerSaveTaskStateWithNextRetry(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")
	s.Start()

	task := &Task{
		ID:       "retry.save",
		Name:     "Retry Save",
		Schedule: "@every 1h",
		TaskType: TaskTypeLocal,
		Run:      func(ctx context.Context) error { return nil },
	}

	s.Register(task)

	// Manually set NextRetry
	s.mu.Lock()
	s.tasks["retry.save"].NextRetry = time.Now().Add(5 * time.Minute)
	s.mu.Unlock()

	// Save state
	s.saveTaskState(s.tasks["retry.save"])

	s.Stop()
}

func TestSchedulerGlobalTaskWithLocking(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")

	ran := make(chan bool, 1)
	task := &Task{
		ID:       "global.test",
		Name:     "Global Test",
		Schedule: "@every 1h",
		TaskType: TaskTypeGlobal, // Global task requires locking
		Run: func(ctx context.Context) error {
			select {
			case ran <- true:
			default:
			}
			return nil
		},
	}

	s.Register(task)
	s.Start()

	s.RunNow("global.test")

	// Wait for task
	select {
	case <-ran:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Global task was not run")
	}

	s.Stop()
}

func TestSchedulerAcquireReleaseLock(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")
	s.Start()

	task := &Task{
		ID:       "lock.test",
		Name:     "Lock Test",
		Schedule: "@every 1h",
		TaskType: TaskTypeGlobal,
		Run:      func(ctx context.Context) error { return nil },
	}

	s.Register(task)

	// Acquire lock
	acquired := s.acquireTaskLock(task)
	if !acquired {
		t.Error("Should be able to acquire lock")
	}

	// Release lock
	s.releaseTaskLock(task)

	s.Stop()
}

func TestSchedulerLockTakeoverExpired(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// First node
	s1 := New(db, "node1")
	s1.Start()

	task := &Task{
		ID:       "takeover.test",
		Name:     "Takeover Test",
		Schedule: "@every 1h",
		TaskType: TaskTypeGlobal,
		Run:      func(ctx context.Context) error { return nil },
	}

	s1.Register(task)

	// Acquire lock with node1
	s1.acquireTaskLock(task)

	// Simulate expired lock by updating locked_at in the past
	db.Exec("UPDATE scheduler_tasks SET locked_at = ? WHERE task_id = ?",
		time.Now().Add(-10*time.Minute), "takeover.test")

	s1.Stop()

	// Second node should be able to take over expired lock
	s2 := New(db, "node2")
	s2.Start()

	task2 := &Task{
		ID:       "takeover.test",
		Name:     "Takeover Test",
		Schedule: "@every 1h",
		TaskType: TaskTypeGlobal,
		Run:      func(ctx context.Context) error { return nil },
	}

	s2.Register(task2)

	acquired := s2.acquireTaskLock(task2)
	if !acquired {
		t.Error("Node2 should be able to take over expired lock")
	}

	s2.Stop()
}

func TestSchedulerReacquireOwnLock(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")
	s.Start()

	task := &Task{
		ID:       "reacquire.test",
		Name:     "Reacquire Test",
		Schedule: "@every 1h",
		TaskType: TaskTypeGlobal,
		Run:      func(ctx context.Context) error { return nil },
	}

	s.Register(task)

	// Acquire lock first time
	acquired1 := s.acquireTaskLock(task)
	if !acquired1 {
		t.Error("Should acquire lock first time")
	}

	// Re-acquire same lock (should succeed - same node)
	acquired2 := s.acquireTaskLock(task)
	if !acquired2 {
		t.Error("Should be able to re-acquire own lock")
	}

	s.Stop()
}

func TestSchedulerRunLoop(t *testing.T) {
	s := New(nil, "node1")

	ran := make(chan bool, 10)
	task := &Task{
		ID:       "loop.test",
		Name:     "Loop Test",
		Schedule: "@every 1ms",
		TaskType: TaskTypeLocal,
		Run: func(ctx context.Context) error {
			select {
			case ran <- true:
			default:
			}
			return nil
		},
	}

	s.Register(task)

	// Set next run to past
	s.mu.Lock()
	s.tasks["loop.test"].NextRun = time.Now().Add(-time.Second)
	s.mu.Unlock()

	s.Start()

	// Wait for multiple runs
	count := 0
	timeout := time.After(3 * time.Second)
	for count < 2 {
		select {
		case <-ran:
			count++
		case <-timeout:
			break
		}
	}

	s.Stop()

	if count < 1 {
		t.Errorf("Task should have run at least once, ran %d times", count)
	}
}

func TestSchedulerTaskRetryAllExhausted(t *testing.T) {
	s := New(nil, "node1")

	var attempts int32
	var notificationReceived bool
	var mu sync.Mutex

	s.SetNotifyFunc(func(n *TaskFailureNotification) {
		mu.Lock()
		notificationReceived = true
		mu.Unlock()
	})

	task := &Task{
		ID:         "exhaust.test",
		Name:       "Exhaust Test",
		Schedule:   "@every 1h",
		TaskType:   TaskTypeLocal,
		MaxRetries: 1, // Only 1 retry
		RetryDelay: 5 * time.Millisecond,
		Run: func(ctx context.Context) error {
			atomic.AddInt32(&attempts, 1)
			return errors.New("always fail")
		},
	}

	s.Register(task)
	s.Start()

	s.RunNow("exhaust.test")

	// Wait for retries to exhaust
	time.Sleep(500 * time.Millisecond)

	s.Stop()

	// Should have run 2 times (initial + 1 retry)
	if atomic.LoadInt32(&attempts) < 2 {
		t.Errorf("Task ran %d times, expected at least 2", attempts)
	}

	mu.Lock()
	defer mu.Unlock()
	if !notificationReceived {
		t.Error("Failure notification should be received after all retries exhausted")
	}
}

func TestSchedulerTaskSuccessNoNotification(t *testing.T) {
	s := New(nil, "node1")

	notificationReceived := false
	s.SetNotifyFunc(func(n *TaskFailureNotification) {
		notificationReceived = true
	})

	ran := make(chan bool, 1)
	task := &Task{
		ID:       "success.test",
		Name:     "Success Test",
		Schedule: "@every 1h",
		TaskType: TaskTypeLocal,
		Run: func(ctx context.Context) error {
			ran <- true
			return nil
		},
	}

	s.Register(task)
	s.Start()

	s.RunNow("success.test")

	<-ran
	time.Sleep(100 * time.Millisecond)

	s.Stop()

	if notificationReceived {
		t.Error("No notification should be received for successful task")
	}
}

func TestSchedulerInitDatabaseMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create old schema without retry columns
	_, err = db.Exec(`
		CREATE TABLE scheduler_tasks (
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
			locked_by TEXT,
			locked_at DATETIME
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create old schema: %v", err)
	}

	// Insert a row
	_, err = db.Exec("INSERT INTO scheduler_tasks (task_id, task_name, schedule) VALUES (?, ?, ?)",
		"test.task", "Test", "@every 1h")
	if err != nil {
		t.Fatalf("Failed to insert test row: %v", err)
	}

	// Create scheduler - should migrate schema
	s := New(db, "node1")
	s.Start()
	s.Stop()

	// Verify columns were added (no error means columns exist)
	_, err = db.Exec("UPDATE scheduler_tasks SET retry_count = 0, next_retry = NULL WHERE task_id = ?", "test.task")
	if err != nil {
		t.Errorf("Migration failed - retry columns not added: %v", err)
	}
}

// Edge cases and error handling

func TestSchedulerIsRunningConcurrent(t *testing.T) {
	s := New(nil, "node1")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.IsRunning()
			}
		}()
	}

	s.Start()
	wg.Wait()
	s.Stop()
}

func TestSchedulerConcurrentOperations(t *testing.T) {
	s := New(nil, "node1")

	var wg sync.WaitGroup

	// Register tasks concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			task := &Task{
				ID:       TaskID("concurrent." + string(rune('0'+idx))),
				Name:     "Concurrent Task",
				Schedule: "@every 1h",
				Run:      func(ctx context.Context) error { return nil },
			}
			s.Register(task)
		}(i)
	}

	wg.Wait()

	s.Start()

	// Get tasks concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.GetTasks()
		}()
	}

	wg.Wait()
	s.Stop()
}

func TestSchedulerLoadTaskStateWithFutureNextRun(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")
	s.Start()

	task := &Task{
		ID:        "future.test",
		Name:      "Future Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s.Register(task)

	// Save state with future next_run
	futureTime := time.Now().Add(2 * time.Hour)
	_, err = db.Exec("UPDATE scheduler_tasks SET next_run = ? WHERE task_id = ?", futureTime, "future.test")
	if err != nil {
		t.Fatalf("Failed to update next_run: %v", err)
	}

	s.Stop()

	// New scheduler should load the future next_run
	s2 := New(db, "node1")

	task2 := &Task{
		ID:        "future.test",
		Name:      "Future Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s2.Register(task2)

	info, _ := s2.GetTask("future.test")
	if info.NextRun.Before(time.Now().Add(time.Hour)) {
		t.Error("NextRun should be loaded from database (future time)")
	}
}

func TestSchedulerLoadTaskStateWithPastNextRun(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")
	s.Start()

	task := &Task{
		ID:        "past.test",
		Name:      "Past Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s.Register(task)

	// Save state with past next_run
	pastTime := time.Now().Add(-2 * time.Hour)
	_, err = db.Exec("UPDATE scheduler_tasks SET next_run = ? WHERE task_id = ?", pastTime, "past.test")
	if err != nil {
		t.Fatalf("Failed to update next_run: %v", err)
	}

	s.Stop()

	// New scheduler should recalculate next_run (not use past time)
	s2 := New(db, "node1")

	task2 := &Task{
		ID:        "past.test",
		Name:      "Past Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s2.Register(task2)

	// Past next_run should be ignored, recalculated instead
	info, _ := s2.GetTask("past.test")
	if info.NextRun.Before(time.Now()) {
		t.Error("NextRun in the past should be recalculated")
	}
}

func TestSchedulerLoadTaskStateWithAllFields(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	s := New(db, "node1")
	s.Start()

	task := &Task{
		ID:        "allfields.test",
		Name:      "All Fields Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s.Register(task)

	// Update all fields
	now := time.Now()
	futureRetry := now.Add(10 * time.Minute)
	_, err = db.Exec(`UPDATE scheduler_tasks SET
		last_run = ?, last_status = ?, last_error = ?,
		run_count = ?, fail_count = ?, retry_count = ?, next_retry = ?
		WHERE task_id = ?`,
		now, "failed", "test error", 5, 2, 1, futureRetry, "allfields.test")
	if err != nil {
		t.Fatalf("Failed to update fields: %v", err)
	}

	s.Stop()

	// New scheduler should load all fields
	s2 := New(db, "node1")

	task2 := &Task{
		ID:        "allfields.test",
		Name:      "All Fields Test",
		Schedule:  "@every 1h",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run:       func(ctx context.Context) error { return nil },
	}

	s2.Register(task2)

	s2.mu.RLock()
	t2 := s2.tasks["allfields.test"]
	s2.mu.RUnlock()

	if t2.RunCount != 5 {
		t.Errorf("RunCount = %d, want 5", t2.RunCount)
	}
	if t2.FailCount != 2 {
		t.Errorf("FailCount = %d, want 2", t2.FailCount)
	}
	if t2.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", t2.RetryCount)
	}
	if t2.LastError != "test error" {
		t.Errorf("LastError = %q, want %q", t2.LastError, "test error")
	}
}

func TestSchedulerCheckAndRunTasksNoDueTasks(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:       "future.task",
		Name:     "Future Task",
		Schedule: "@every 24h",
		TaskType: TaskTypeLocal,
		Run: func(ctx context.Context) error {
			t.Error("Task should not run - not due yet")
			return nil
		},
	}

	s.Register(task)

	// Set next run to far future
	s.mu.Lock()
	s.tasks["future.task"].NextRun = time.Now().Add(24 * time.Hour)
	s.mu.Unlock()

	// Check should find no due tasks
	s.checkAndRunTasks(time.Now())

	time.Sleep(100 * time.Millisecond)
}

func TestSchedulerCheckAndRunTasksDisabledTask(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:        "disabled.task",
		Name:      "Disabled Task",
		Schedule:  "@every 1ms",
		TaskType:  TaskTypeLocal,
		Skippable: true,
		Run: func(ctx context.Context) error {
			t.Error("Disabled task should not run")
			return nil
		},
	}

	s.Register(task)

	// Disable the task
	s.Disable("disabled.task")

	// Set next run to past
	s.mu.Lock()
	s.tasks["disabled.task"].NextRun = time.Now().Add(-time.Minute)
	s.mu.Unlock()

	// Check should skip disabled task
	s.checkAndRunTasks(time.Now())

	time.Sleep(100 * time.Millisecond)
}

func TestSchedulerRunStartupTasksDisabled(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:         "startup.disabled",
		Name:       "Startup Disabled",
		Schedule:   "@every 1h",
		TaskType:   TaskTypeLocal,
		RunOnStart: true,
		Skippable:  true,
		Run: func(ctx context.Context) error {
			t.Error("Disabled startup task should not run")
			return nil
		},
	}

	s.Register(task)
	s.Disable("startup.disabled")

	// Run startup tasks - disabled task should not run
	s.runStartupTasks()

	time.Sleep(100 * time.Millisecond)
}

func TestSchedulerRunStartupTasksNoRunOnStart(t *testing.T) {
	s := New(nil, "node1")

	task := &Task{
		ID:         "startup.false",
		Name:       "Startup False",
		Schedule:   "@every 1h",
		TaskType:   TaskTypeLocal,
		RunOnStart: false,
		Run: func(ctx context.Context) error {
			t.Error("Task with RunOnStart=false should not run on startup")
			return nil
		},
	}

	s.Register(task)

	// Run startup tasks - task with RunOnStart=false should not run
	s.runStartupTasks()

	time.Sleep(100 * time.Millisecond)
}
