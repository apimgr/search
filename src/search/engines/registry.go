package engines

import (
	"sync"

	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
)

// UserAgent is the standard User-Agent string for all search engine requests.
// Windows 11 Edge - consistent across all engines for privacy and compatibility.
const UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0"

// Registry holds all available search engines
type Registry struct {
	mu      sync.RWMutex
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
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engines[engine.Name()] = engine
}

// Get returns an engine by name
func (r *Registry) Get(name string) (search.Engine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	engine, ok := r.engines[name]
	if !ok {
		return nil, model.ErrEngineNotFound
	}
	return engine, nil
}

// GetAll returns all registered engines
func (r *Registry) GetAll() []search.Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	engines := make([]search.Engine, 0, len(r.engines))
	for _, engine := range r.engines {
		engines = append(engines, engine)
	}
	return engines
}

// GetEnabled returns all enabled engines
func (r *Registry) GetEnabled() []search.Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	engines := make([]search.Engine, 0)
	for _, engine := range r.engines {
		if engine.IsEnabled() {
			engines = append(engines, engine)
		}
	}
	return engines
}

// GetForCategory returns all engines that support a category
func (r *Registry) GetForCategory(category model.Category) []search.Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
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

	r.mu.RLock()
	defer r.mu.RUnlock()
	engines := make([]search.Engine, 0, len(names))
	for _, name := range names {
		if engine, ok := r.engines[name]; ok && engine.IsEnabled() {
			engines = append(engines, engine)
		}
	}
	return engines
}

// Count returns the total number of registered engines
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	// Additional engines per IDEA.md
	registry.Register(NewMojeek())
	registry.Register(NewYandex())
	registry.Register(NewBaidu())

	return registry
}
