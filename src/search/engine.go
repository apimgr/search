package search

import (
	"context"
	"sync"
	"time"

	"github.com/apimgr/search/src/model"
)

// Engine represents a search engine interface
type Engine interface {
	// Name returns the unique engine name
	Name() string

	// DisplayName returns the human-readable engine name
	DisplayName() string

	// Search performs a search and returns results
	Search(ctx context.Context, query *model.Query) ([]model.Result, error)

	// IsEnabled returns whether the engine is enabled
	IsEnabled() bool

	// GetPriority returns the engine priority (higher = more important)
	GetPriority() int

	// SupportsCategory returns whether the engine supports a category
	SupportsCategory(category model.Category) bool

	// GetConfig returns the engine configuration
	GetConfig() *model.EngineConfig
}

const (
	engineFailureThreshold = 3
	engineCooldownDuration = 10 * time.Minute
)

// EngineHealth tracks runtime health for an engine.
type EngineHealth struct {
	Status              string    `json:"status"`
	Healthy             bool      `json:"healthy"`
	LastChecked         time.Time `json:"last_checked,omitempty"`
	LastSuccess         time.Time `json:"last_success,omitempty"`
	LastFailure         time.Time `json:"last_failure,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
	LastResponseTimeMS  int64     `json:"last_response_time_ms,omitempty"`
	SuccessCount        int64     `json:"success_count"`
	FailureCount        int64     `json:"failure_count"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CooldownUntil       time.Time `json:"cooldown_until,omitempty"`
}

// BaseEngine provides common functionality for engines
type BaseEngine struct {
	config *model.EngineConfig
	mu     sync.RWMutex
	health EngineHealth
}

// NewBaseEngine creates a new BaseEngine
func NewBaseEngine(config *model.EngineConfig) *BaseEngine {
	return &BaseEngine{
		config: config,
		health: EngineHealth{
			Status:  "unknown",
			Healthy: true,
		},
	}
}

// Name returns the engine name
func (e *BaseEngine) Name() string {
	return e.config.Name
}

// DisplayName returns the display name
func (e *BaseEngine) DisplayName() string {
	return e.config.DisplayName
}

// IsEnabled returns whether the engine is enabled
func (e *BaseEngine) IsEnabled() bool {
	return e.config.IsEnabled()
}

// GetPriority returns the engine priority
func (e *BaseEngine) GetPriority() int {
	return e.config.GetPriority()
}

// SupportsCategory returns whether the engine supports a category
func (e *BaseEngine) SupportsCategory(category model.Category) bool {
	return e.config.SupportsCategory(category)
}

// GetConfig returns the engine configuration
func (e *BaseEngine) GetConfig() *model.EngineConfig {
	return e.config
}

// GetHealth returns runtime engine health information.
func (e *BaseEngine) GetHealth() EngineHealth {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.healthSnapshotLocked(time.Now())
}

// CanSearch reports whether the engine should be preferred for live searches.
func (e *BaseEngine) CanSearch(now time.Time) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return !e.health.CooldownUntil.After(now)
}

// RecordSuccess updates runtime health after a successful request.
func (e *BaseEngine) RecordSuccess(duration time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	e.health.LastChecked = now
	e.health.LastSuccess = now
	e.health.LastError = ""
	e.health.SuccessCount++
	e.health.ConsecutiveFailures = 0
	e.health.CooldownUntil = time.Time{}
	if duration > 0 {
		e.health.LastResponseTimeMS = duration.Milliseconds()
	}

	snapshot := e.healthSnapshotLocked(now)
	e.health.Status = snapshot.Status
	e.health.Healthy = snapshot.Healthy
}

// RecordFailure updates runtime health after a failed request.
func (e *BaseEngine) RecordFailure(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	e.health.LastChecked = now
	e.health.LastFailure = now
	e.health.FailureCount++
	e.health.ConsecutiveFailures++
	if err != nil {
		e.health.LastError = err.Error()
	}
	if e.health.ConsecutiveFailures >= engineFailureThreshold {
		e.health.CooldownUntil = now.Add(engineCooldownDuration)
	}

	snapshot := e.healthSnapshotLocked(now)
	e.health.Status = snapshot.Status
	e.health.Healthy = snapshot.Healthy
}

func (e *BaseEngine) healthSnapshotLocked(now time.Time) EngineHealth {
	snapshot := e.health

	switch {
	case snapshot.CooldownUntil.After(now):
		snapshot.Status = "unhealthy"
		snapshot.Healthy = false
	case snapshot.ConsecutiveFailures > 0:
		snapshot.Status = "degraded"
		snapshot.Healthy = true
	case snapshot.SuccessCount == 0 && snapshot.FailureCount == 0:
		snapshot.Status = "unknown"
		snapshot.Healthy = true
	default:
		snapshot.Status = "healthy"
		snapshot.Healthy = true
	}

	return snapshot
}
