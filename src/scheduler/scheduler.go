package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Task represents a scheduled task
type Task struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context) error
	LastRun  time.Time
	NextRun  time.Time
	Enabled  bool
	RunOnStart bool
}

// Scheduler manages periodic tasks
type Scheduler struct {
	mu      sync.RWMutex
	tasks   map[string]*Task
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	wg      sync.WaitGroup
}

// Config holds scheduler configuration
type Config struct {
	Enabled bool `yaml:"enabled"`
}

// DefaultConfig returns default scheduler configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled: true,
	}
}

// New creates a new scheduler
func New() *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		tasks:  make(map[string]*Task),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Register adds a new task to the scheduler
func (s *Scheduler) Register(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task.Name == "" {
		return fmt.Errorf("task name is required")
	}

	if task.Interval <= 0 {
		return fmt.Errorf("task interval must be positive")
	}

	if task.Run == nil {
		return fmt.Errorf("task run function is required")
	}

	task.NextRun = time.Now()
	if !task.RunOnStart {
		task.NextRun = task.NextRun.Add(task.Interval)
	}
	task.Enabled = true

	s.tasks[task.Name] = task
	return nil
}

// Unregister removes a task from the scheduler
func (s *Scheduler) Unregister(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tasks, name)
}

// Enable enables a task
func (s *Scheduler) Enable(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[name]
	if !ok {
		return fmt.Errorf("task not found: %s", name)
	}

	task.Enabled = true
	task.NextRun = time.Now().Add(task.Interval)
	return nil
}

// Disable disables a task
func (s *Scheduler) Disable(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[name]
	if !ok {
		return fmt.Errorf("task not found: %s", name)
	}

	task.Enabled = false
	return nil
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.wg.Add(1)
	go s.run()
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
	tasks := make([]*Task, 0)
	for _, task := range s.tasks {
		if task.Enabled && now.After(task.NextRun) {
			tasks = append(tasks, task)
		}
	}
	s.mu.RUnlock()

	for _, task := range tasks {
		s.runTask(task)
	}
}

// runTask runs a single task
func (s *Scheduler) runTask(task *Task) {
	s.mu.Lock()
	task.LastRun = time.Now()
	task.NextRun = time.Now().Add(task.Interval)
	s.mu.Unlock()

	// Run task in goroutine with timeout
	go func() {
		ctx, cancel := context.WithTimeout(s.ctx, task.Interval)
		defer cancel()

		if err := task.Run(ctx); err != nil {
			log.Printf("[Scheduler] Task %s failed: %v", task.Name, err)
		}
	}()
}

// RunNow runs a task immediately
func (s *Scheduler) RunNow(name string) error {
	s.mu.RLock()
	task, ok := s.tasks[name]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", name)
	}

	s.runTask(task)
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
		tasks = append(tasks, &TaskInfo{
			Name:     task.Name,
			Interval: task.Interval,
			LastRun:  task.LastRun,
			NextRun:  task.NextRun,
			Enabled:  task.Enabled,
		})
	}
	return tasks
}

// TaskInfo represents task information (without the run function)
type TaskInfo struct {
	Name     string        `json:"name"`
	Interval time.Duration `json:"interval"`
	LastRun  time.Time     `json:"last_run"`
	NextRun  time.Time     `json:"next_run"`
	Enabled  bool          `json:"enabled"`
}

// Common task factories

// NewCleanupTask creates a session/cache cleanup task
func NewCleanupTask(cleanupFunc func(ctx context.Context) error) *Task {
	return &Task{
		Name:       "cleanup",
		Interval:   1 * time.Hour,
		Run:        cleanupFunc,
		RunOnStart: false,
	}
}

// NewCertRenewalTask creates a certificate renewal check task
func NewCertRenewalTask(renewFunc func(ctx context.Context) error) *Task {
	return &Task{
		Name:       "cert_renewal",
		Interval:   24 * time.Hour,
		Run:        renewFunc,
		RunOnStart: true,
	}
}

// NewStatsAggregationTask creates a stats aggregation task
func NewStatsAggregationTask(aggregateFunc func(ctx context.Context) error) *Task {
	return &Task{
		Name:       "stats_aggregation",
		Interval:   5 * time.Minute,
		Run:        aggregateFunc,
		RunOnStart: false,
	}
}

// NewGeoIPUpdateTask creates a GeoIP database update task
func NewGeoIPUpdateTask(updateFunc func(ctx context.Context) error) *Task {
	return &Task{
		Name:       "geoip_update",
		Interval:   7 * 24 * time.Hour, // Weekly
		Run:        updateFunc,
		RunOnStart: false,
	}
}

// NewLogRotationTask creates a log rotation task
func NewLogRotationTask(rotateFunc func(ctx context.Context) error) *Task {
	return &Task{
		Name:       "log_rotation",
		Interval:   24 * time.Hour,
		Run:        rotateFunc,
		RunOnStart: false,
	}
}
