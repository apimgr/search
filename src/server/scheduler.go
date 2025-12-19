package server

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
)

// Scheduler manages scheduled tasks
type Scheduler struct {
	config  *config.Config
	tasks   map[string]*ScheduledTask
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
}

// ScheduledTask represents a scheduled task
type ScheduledTask struct {
	Name        string
	Description string
	Interval    time.Duration
	LastRun     time.Time
	NextRun     time.Time
	RunCount    int64
	Enabled     bool
	Handler     func() error
}

// NewScheduler creates a new scheduler
func NewScheduler(cfg *config.Config) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		config: cfg,
		tasks:  make(map[string]*ScheduledTask),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Register adds a new scheduled task
func (s *Scheduler) Register(name, description string, interval time.Duration, handler func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasks[name] = &ScheduledTask{
		Name:        name,
		Description: description,
		Interval:    interval,
		NextRun:     time.Now().Add(interval),
		Enabled:     true,
		Handler:     handler,
	}

	log.Printf("[Scheduler] Registered task: %s (interval: %s)", name, interval)
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	if !s.config.Server.Scheduler.Enabled {
		log.Println("[Scheduler] Scheduler is disabled")
		return
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	log.Println("[Scheduler] Starting scheduler")

	go s.run()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.cancel()
	s.running = false
	log.Println("[Scheduler] Scheduler stopped")
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.checkTasks(now)
		}
	}
}

// checkTasks checks and runs due tasks
func (s *Scheduler) checkTasks(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, task := range s.tasks {
		if !task.Enabled {
			continue
		}

		if now.After(task.NextRun) {
			go s.runTask(name, task)
			task.LastRun = now
			task.NextRun = now.Add(task.Interval)
			task.RunCount++
		}
	}
}

// runTask executes a task
func (s *Scheduler) runTask(name string, task *ScheduledTask) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Scheduler] Task %s panicked: %v", name, r)
		}
	}()

	start := time.Now()
	if err := task.Handler(); err != nil {
		log.Printf("[Scheduler] Task %s failed: %v", name, err)
	} else {
		log.Printf("[Scheduler] Task %s completed in %s", name, time.Since(start))
	}
}

// Enable enables a task
func (s *Scheduler) Enable(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task, ok := s.tasks[name]; ok {
		task.Enabled = true
		return true
	}
	return false
}

// Disable disables a task
func (s *Scheduler) Disable(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if task, ok := s.tasks[name]; ok {
		task.Enabled = false
		return true
	}
	return false
}

// RunNow runs a task immediately
func (s *Scheduler) RunNow(name string) error {
	s.mu.RLock()
	task, ok := s.tasks[name]
	s.mu.RUnlock()

	if !ok {
		return &TaskNotFoundError{Name: name}
	}

	return task.Handler()
}

// GetTasks returns all registered tasks
func (s *Scheduler) GetTasks() []*ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tasks := make([]*ScheduledTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		// Return a copy
		tasks = append(tasks, &ScheduledTask{
			Name:        task.Name,
			Description: task.Description,
			Interval:    task.Interval,
			LastRun:     task.LastRun,
			NextRun:     task.NextRun,
			RunCount:    task.RunCount,
			Enabled:     task.Enabled,
		})
	}
	return tasks
}

// TaskNotFoundError is returned when a task is not found
type TaskNotFoundError struct {
	Name string
}

func (e *TaskNotFoundError) Error() string {
	return "task not found: " + e.Name
}

// DefaultTasks returns common scheduled tasks
func DefaultTasks(cfg *config.Config) []struct {
	Name        string
	Description string
	Interval    time.Duration
	Handler     func() error
} {
	return []struct {
		Name        string
		Description string
		Interval    time.Duration
		Handler     func() error
	}{
		{
			Name:        "cleanup_sessions",
			Description: "Clean up expired sessions",
			Interval:    5 * time.Minute,
			Handler:     func() error { return nil }, // Placeholder
		},
		{
			Name:        "update_geoip",
			Description: "Update GeoIP database",
			Interval:    24 * time.Hour,
			Handler:     func() error { return nil }, // Placeholder
		},
		{
			Name:        "check_engines",
			Description: "Check search engine availability",
			Interval:    30 * time.Minute,
			Handler:     func() error { return nil }, // Placeholder
		},
	}
}
