// Package models defines core data structures and errors
// Per TEMPLATE.md: Standard error definitions and data models
package models

import "errors"

var (
	// Query errors
	ErrEmptyQuery      = errors.New("query text cannot be empty")
	ErrInvalidCategory = errors.New("invalid category")
	
	// Engine errors
	ErrEngineNotFound    = errors.New("engine not found")
	ErrEngineDisabled    = errors.New("engine is disabled")
	ErrEngineUnavailable = errors.New("engine is unavailable")
	ErrEngineTimeout     = errors.New("engine request timed out")
	ErrEngineRateLimit   = errors.New("engine rate limit exceeded")
	
	// Search errors
	ErrNoResults     = errors.New("no results found")
	ErrNoEngines     = errors.New("no engines available")
	ErrSearchTimeout = errors.New("search request timed out")
	
	// Configuration errors
	ErrInvalidConfig = errors.New("invalid configuration")
	ErrMissingConfig = errors.New("missing required configuration")
)
