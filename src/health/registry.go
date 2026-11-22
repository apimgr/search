// Package health provides ordered, cached health checks and public counters.
package health

import (
	"encoding/json"
	"errors"
	"regexp"
	"sync"
)

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// CheckFunc returns one of the public-safe health states: ok, degraded, or error.
type CheckFunc func() string

// StatFunc returns a non-negative aggregate counter.
type StatFunc func() int64

// Check is the cached public view of a registered dependency check.
type Check struct {
	Key      string
	Critical bool
	Status   string
}

// Stat is the cached public view of a registered counter.
type Stat struct {
	Key   string
	Value int64
}

// Field is an extra ordered JSON property produced by the registry.
type Field struct {
	Key   string
	Value any
}

// AppendFields appends ordered JSON properties to a marshaled object.
func AppendFields(base []byte, fields []Field) ([]byte, error) {
	if len(fields) == 0 {
		return base, nil
	}
	if len(base) < 2 || base[len(base)-1] != '}' {
		return nil, errors.New("base JSON value is not an object")
	}
	result := append([]byte(nil), base[:len(base)-1]...)
	for _, field := range fields {
		if !keyPattern.MatchString(field.Key) {
			return nil, errors.New("invalid health JSON key")
		}
		key, err := json.Marshal(field.Key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(field.Value)
		if err != nil {
			return nil, err
		}
		result = append(result, ',')
		result = append(result, key...)
		result = append(result, ':')
		result = append(result, value...)
	}
	return append(result, '}'), nil
}

// Registry retains callbacks in registration order and exposes only cached values.
type Registry struct {
	mu     sync.RWMutex
	checks []checkEntry
	stats  []statEntry
}

var defaultRegistry = NewRegistry()

// Register adds a check to the process-wide registry.
func Register(key string, critical bool, fn CheckFunc) error {
	return defaultRegistry.Register(key, critical, fn)
}

// RegisterStat adds a counter to the process-wide registry.
func RegisterStat(key string, fn StatFunc) error {
	return defaultRegistry.RegisterStat(key, fn)
}

// Refresh reevaluates process-wide callbacks outside request handling.
func Refresh() { defaultRegistry.Refresh() }

// Checks returns process-wide cached checks in registration order.
func Checks() []Check { return defaultRegistry.Checks() }

// Stats returns process-wide cached counters in registration order.
func Stats() []Stat {
	return defaultRegistry.Stats()
}

type checkEntry struct {
	key      string
	critical bool
	fn       CheckFunc
	status   string
}

type statEntry struct {
	key   string
	fn    StatFunc
	value int64
}

// NewRegistry creates an empty ordered health registry.
func NewRegistry() *Registry { return &Registry{} }

// Register adds a dependency check once and captures its initial cached value.
func (r *Registry) Register(key string, critical bool, fn CheckFunc) error {
	if r == nil || !keyPattern.MatchString(key) || fn == nil {
		return errors.New("invalid health check registration")
	}
	status := callCheck(fn)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.checks {
		if entry.key == key {
			return errors.New("duplicate health check registration")
		}
	}
	r.checks = append(r.checks, checkEntry{key: key, critical: critical, fn: fn, status: status})
	return nil
}

// RegisterStat adds a public aggregate counter once in registration order.
func (r *Registry) RegisterStat(key string, fn StatFunc) error {
	if r == nil || !keyPattern.MatchString(key) || fn == nil {
		return errors.New("invalid health stat registration")
	}
	value := callStat(fn)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, entry := range r.stats {
		if entry.key == key {
			return errors.New("duplicate health stat registration")
		}
	}
	r.stats = append(r.stats, statEntry{key: key, fn: fn, value: value})
	return nil
}

// Refresh reevaluates all callbacks; callers schedule this outside request handling.
func (r *Registry) Refresh() {
	if r == nil {
		return
	}
	r.mu.RLock()
	checks := append([]checkEntry(nil), r.checks...)
	stats := append([]statEntry(nil), r.stats...)
	r.mu.RUnlock()
	checkValues := make(map[string]string, len(checks))
	for _, entry := range checks {
		checkValues[entry.key] = callCheck(entry.fn)
	}
	statValues := make(map[string]int64, len(stats))
	for _, entry := range stats {
		statValues[entry.key] = callStat(entry.fn)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.checks {
		if value, ok := checkValues[r.checks[i].key]; ok {
			r.checks[i].status = value
		}
	}
	for i := range r.stats {
		if value, ok := statValues[r.stats[i].key]; ok {
			r.stats[i].value = value
		}
	}
}

// Checks returns cached checks in their registration order.
func (r *Registry) Checks() []Check {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Check, 0, len(r.checks))
	for _, entry := range r.checks {
		if entry.status == "disabled" {
			continue
		}
		result = append(result, Check{Key: entry.key, Critical: entry.critical, Status: entry.status})
	}
	return result
}

// Stats returns cached counters in their registration order.
func (r *Registry) Stats() []Stat {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Stat, 0, len(r.stats))
	for _, entry := range r.stats {
		result = append(result, Stat{Key: entry.key, Value: entry.value})
	}
	return result
}

func callCheck(fn CheckFunc) (status string) {
	defer func() {
		if recover() != nil {
			status = "error"
		}
	}()
	return safeStatus(fn())
}

func callStat(fn StatFunc) (value int64) {
	defer func() {
		if recover() != nil || value < 0 {
			value = 0
		}
	}()
	return fn()
}

func safeStatus(status string) string {
	switch status {
	case "ok", "degraded", "error", "disabled":
		return status
	default:
		return "error"
	}
}
