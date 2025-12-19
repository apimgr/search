package engines

import (
	"github.com/apimgr/search/src/models"
	"github.com/apimgr/search/src/search"
)

// Registry holds all available search engines
type Registry struct {
	engines map[string]search.Engine
}

// NewRegistry creates a new engine registry
func NewRegistry() *Registry {
	return &Registry{
		engines: make(map[string]search.Engine),
	}
}

// Register adds an engine to the registry
func (r *Registry) Register(engine search.Engine) {
	r.engines[engine.Name()] = engine
}

// Get returns an engine by name
func (r *Registry) Get(name string) (search.Engine, error) {
	engine, ok := r.engines[name]
	if !ok {
		return nil, models.ErrEngineNotFound
	}
	return engine, nil
}

// GetAll returns all registered engines
func (r *Registry) GetAll() []search.Engine {
	engines := make([]search.Engine, 0, len(r.engines))
	for _, engine := range r.engines {
		engines = append(engines, engine)
	}
	return engines
}

// GetEnabled returns all enabled engines
func (r *Registry) GetEnabled() []search.Engine {
	engines := make([]search.Engine, 0)
	for _, engine := range r.engines {
		if engine.IsEnabled() {
			engines = append(engines, engine)
		}
	}
	return engines
}

// GetForCategory returns all engines that support a category
func (r *Registry) GetForCategory(category models.Category) []search.Engine {
	engines := make([]search.Engine, 0)
	for _, engine := range r.engines {
		if engine.IsEnabled() && engine.SupportsCategory(category) {
			engines = append(engines, engine)
		}
	}
	return engines
}

// GetByNames returns engines by their names
func (r *Registry) GetByNames(names []string) []search.Engine {
	if len(names) == 0 {
		return r.GetEnabled()
	}
	
	engines := make([]search.Engine, 0, len(names))
	for _, name := range names {
		if engine, err := r.Get(name); err == nil && engine.IsEnabled() {
			engines = append(engines, engine)
		}
	}
	return engines
}

// Count returns the total number of registered engines
func (r *Registry) Count() int {
	return len(r.engines)
}

// DefaultRegistry creates and returns a registry with default engines
func DefaultRegistry() *Registry {
	registry := NewRegistry()

	// Register engines in priority order (DuckDuckGo first for privacy, Google second)
	registry.Register(NewDuckDuckGo())
	registry.Register(NewGoogle())
	registry.Register(NewBing())
	registry.Register(NewWikipediaEngine())
	registry.Register(NewQwantEngine())
	registry.Register(NewBrave())
	registry.Register(NewYahoo())
	registry.Register(NewGitHub())
	registry.Register(NewStackOverflow())
	registry.Register(NewReddit())
	registry.Register(NewStartpageEngine())
	registry.Register(NewYouTubeEngine())

	return registry
}
