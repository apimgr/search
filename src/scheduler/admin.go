// Package scheduler provides admin panel integration
// Per AI.md PART 19: Admin panel must show actual scheduler runtime state
package scheduler

import "github.com/apimgr/search/src/admin"

// AdminAdapter wraps Scheduler to implement admin.SchedulerManager interface
// Per AI.md PART 19: Admin panel integration with scheduler
type AdminAdapter struct {
	scheduler *Scheduler
}

// NewAdminAdapter creates an adapter for admin panel integration
func NewAdminAdapter(s *Scheduler) *AdminAdapter {
	return &AdminAdapter{scheduler: s}
}

// IsRunning returns whether the scheduler is running
func (a *AdminAdapter) IsRunning() bool {
	if a.scheduler == nil {
		return false
	}
	return a.scheduler.IsRunning()
}

// GetTasks returns all registered tasks with runtime state
func (a *AdminAdapter) GetTasks() []*admin.SchedulerTaskInfo {
	if a.scheduler == nil {
		return nil
	}

	tasks := a.scheduler.GetTasks()
	result := make([]*admin.SchedulerTaskInfo, len(tasks))
	for i, t := range tasks {
		result[i] = &admin.SchedulerTaskInfo{
			ID:          t.ID,
			Name:        t.Name,
			Description: t.Description,
			Schedule:    t.Schedule,
			TaskType:    t.TaskType,
			LastRun:     t.LastRun,
			LastStatus:  t.LastStatus,
			LastError:   t.LastError,
			NextRun:     t.NextRun,
			RunCount:    t.RunCount,
			FailCount:   t.FailCount,
			Enabled:     t.Enabled,
			Skippable:   t.Skippable,
			RetryCount:  t.RetryCount,
			NextRetry:   t.NextRetry,
			MaxRetries:  t.MaxRetries,
		}
	}
	return result
}

// GetTask returns a specific task by ID
func (a *AdminAdapter) GetTask(id string) (*admin.SchedulerTaskInfo, error) {
	if a.scheduler == nil {
		return nil, nil
	}

	t, err := a.scheduler.GetTask(TaskID(id))
	if err != nil {
		return nil, err
	}

	return &admin.SchedulerTaskInfo{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Schedule:    t.Schedule,
		TaskType:    t.TaskType,
		LastRun:     t.LastRun,
		LastStatus:  t.LastStatus,
		LastError:   t.LastError,
		NextRun:     t.NextRun,
		RunCount:    t.RunCount,
		FailCount:   t.FailCount,
		Enabled:     t.Enabled,
		Skippable:   t.Skippable,
		RetryCount:  t.RetryCount,
		NextRetry:   t.NextRetry,
		MaxRetries:  t.MaxRetries,
	}, nil
}

// Enable enables a task
func (a *AdminAdapter) Enable(id string) error {
	if a.scheduler == nil {
		return nil
	}
	return a.scheduler.Enable(TaskID(id))
}

// Disable disables a task
func (a *AdminAdapter) Disable(id string) error {
	if a.scheduler == nil {
		return nil
	}
	return a.scheduler.Disable(TaskID(id))
}

// RunNow triggers immediate execution of a task
func (a *AdminAdapter) RunNow(id string) error {
	if a.scheduler == nil {
		return nil
	}
	return a.scheduler.RunNow(TaskID(id))
}
