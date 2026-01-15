package search

import (
	"context"
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

// BaseEngine provides common functionality for engines
type BaseEngine struct {
	config *model.EngineConfig
}

// NewBaseEngine creates a new BaseEngine
func NewBaseEngine(config *model.EngineConfig) *BaseEngine {
	return &BaseEngine{
		config: config,
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
