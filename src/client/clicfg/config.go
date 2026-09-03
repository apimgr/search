// Package clicfg provides a small yaml.v3-backed configuration store for the
// CLI client. It is a drop-in replacement for the subset of the viper API the
// client used: dotted-key access, typed getters, defaults, an explicit-set
// tracker (IsSet), and read/write to a YAML file.
//
// The store is a process-global singleton so both package main (cache,
// logging) and package cmd (commands) share the same configuration state,
// matching the previous viper.* global behavior.
package clicfg

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/apimgr/search/src/client/path"
	"github.com/apimgr/search/src/config"
)

// store holds the configuration state behind a single mutex.
type store struct {
	mu sync.RWMutex
	// defaults holds values registered via SetDefault, keyed by dotted path.
	defaults map[string]any
	// overrides holds values set explicitly via Set, keyed by dotted path.
	overrides map[string]any
	// loaded holds values read from the config file, keyed by dotted path.
	loaded map[string]any
	// configFile is an explicit file path set via SetConfigFile.
	configFile string
	// configPaths are directories searched when configFile is empty.
	configPaths []string
	// configName is the base filename (without extension) to search for.
	configName string
}

// global is the process-wide configuration singleton.
var global = newStore()

// newStore returns an empty initialized store.
func newStore() *store {
	return &store{
		defaults:  make(map[string]any),
		overrides: make(map[string]any),
		loaded:    make(map[string]any),
	}
}

// Reset clears all configuration state, mirroring viper.Reset.
func Reset() {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.defaults = make(map[string]any)
	global.overrides = make(map[string]any)
	global.loaded = make(map[string]any)
	global.configFile = ""
	global.configPaths = nil
	global.configName = ""
}

// Set assigns an explicit value for a dotted key, mirroring viper.Set.
// Explicitly set keys are reported as set by IsSet.
func Set(key string, value any) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.overrides[normalizeKey(key)] = value
}

// SetDefault registers a fallback value for a dotted key, mirroring
// viper.SetDefault. Defaults do not make IsSet return true.
func SetDefault(key string, value any) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.defaults[normalizeKey(key)] = value
}

// IsSet reports whether a key was explicitly set or loaded from the config
// file. Defaults alone do not count as set, matching viper.IsSet semantics.
func IsSet(key string) bool {
	global.mu.RLock()
	defer global.mu.RUnlock()
	k := normalizeKey(key)
	if _, ok := global.overrides[k]; ok {
		return true
	}
	_, ok := global.loaded[k]
	return ok
}

// get resolves a key with priority overrides > loaded > defaults.
func (s *store) get(key string) (any, bool) {
	k := normalizeKey(key)
	if v, ok := s.overrides[k]; ok {
		return v, true
	}
	if v, ok := s.loaded[k]; ok {
		return v, true
	}
	if v, ok := s.defaults[k]; ok {
		return v, true
	}
	return nil, false
}

// GetString returns the string value for a key, or "" if unset.
func GetString(key string) string {
	global.mu.RLock()
	defer global.mu.RUnlock()
	v, ok := global.get(key)
	if !ok {
		return ""
	}
	return toString(v)
}

// GetBool returns the boolean value for a key, or false if unset.
func GetBool(key string) bool {
	global.mu.RLock()
	defer global.mu.RUnlock()
	v, ok := global.get(key)
	if !ok {
		return false
	}
	return toBool(v)
}

// GetInt returns the integer value for a key, or 0 if unset.
func GetInt(key string) int {
	global.mu.RLock()
	defer global.mu.RUnlock()
	v, ok := global.get(key)
	if !ok {
		return 0
	}
	return toInt(v)
}

// SetConfigFile sets an explicit config file path, mirroring
// viper.SetConfigFile.
func SetConfigFile(file string) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.configFile = file
}

// AddConfigPath adds a directory to search for the config file, mirroring
// viper.AddConfigPath.
func AddConfigPath(dir string) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.configPaths = append(global.configPaths, dir)
}

// SetConfigName sets the base filename (without extension) to search for,
// mirroring viper.SetConfigName.
func SetConfigName(name string) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.configName = name
}

// SetConfigType is accepted for API compatibility with viper.SetConfigType.
// Only YAML is supported, so the value is ignored.
func SetConfigType(_ string) {}

// resolveReadPath determines the file path to read for ReadInConfig.
func (s *store) resolveReadPath() string {
	if s.configFile != "" {
		return s.configFile
	}
	name := s.configName
	if name == "" {
		return ""
	}
	for _, dir := range s.configPaths {
		for _, ext := range []string{".yml", ".yaml"} {
			candidate := joinPath(dir, name+ext)
			if fileExists(candidate) {
				return candidate
			}
		}
	}
	return ""
}

// ReadInConfig reads and parses the resolved config file into the loaded map.
// It mirrors viper.ReadInConfig: a missing file is not a fatal error here, the
// caller historically ignored the returned error.
func ReadInConfig() error {
	global.mu.Lock()
	defer global.mu.Unlock()

	readPath := global.resolveReadPath()
	if readPath == "" {
		return nil
	}

	data, err := os.ReadFile(readPath)
	if err != nil {
		return err
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	global.loaded = make(map[string]any)
	flatten("", raw, global.loaded)
	return nil
}

// WriteConfigAs writes the current merged configuration to the given path as
// YAML, mirroring viper.WriteConfigAs. Parent directories are created first.
func WriteConfigAs(file string) error {
	global.mu.RLock()
	merged := global.mergedNested()
	global.mu.RUnlock()

	if err := path.EnsureFile(file, 0600); err != nil {
		return err
	}

	data, err := yaml.Marshal(merged)
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0600)
}

// mergedNested builds a nested map from defaults, loaded, and overrides for
// serialization, with overrides taking highest priority.
func (s *store) mergedNested() map[string]any {
	flat := make(map[string]any)
	for k, v := range s.defaults {
		flat[k] = v
	}
	for k, v := range s.loaded {
		flat[k] = v
	}
	for k, v := range s.overrides {
		flat[k] = v
	}

	nested := make(map[string]any)
	for k, v := range flat {
		setNested(nested, strings.Split(k, "."), v)
	}
	return nested
}

// normalizeKey lowercases keys so lookups are case-insensitive like viper.
func normalizeKey(key string) string {
	return strings.ToLower(key)
}

// flatten recursively flattens a nested map into dotted keys.
func flatten(prefix string, in map[string]any, out map[string]any) {
	for k, v := range in {
		key := normalizeKey(k)
		if prefix != "" {
			key = prefix + "." + key
		}
		if child, ok := v.(map[string]any); ok {
			flatten(key, child, out)
			continue
		}
		if child, ok := v.(map[any]any); ok {
			converted := make(map[string]any, len(child))
			for ck, cv := range child {
				converted[toString(ck)] = cv
			}
			flatten(key, converted, out)
			continue
		}
		out[key] = v
	}
}

// setNested writes a value into a nested map following the key segments.
func setNested(root map[string]any, segments []string, value any) {
	current := root
	for i, seg := range segments {
		if i == len(segments)-1 {
			current[seg] = value
			return
		}
		next, ok := current[seg].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[seg] = next
		}
		current = next
	}
}

// toString converts a value to its string representation.
func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case nil:
		return ""
	default:
		return ""
	}
}

// toBool converts a value to a boolean, accepting common truthy strings.
func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, err := config.ParseBool(strings.TrimSpace(val), false)
		if err != nil {
			return false
		}
		return b
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	default:
		return false
	}
}

// toInt converts a value to an integer where possible.
func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case bool:
		if val {
			return 1
		}
		return 0
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// joinPath joins a directory and filename without importing path/filepath at
// call sites that already hold the lock.
func joinPath(dir, file string) string {
	if dir == "" {
		return file
	}
	if strings.HasSuffix(dir, "/") {
		return dir + file
	}
	return dir + "/" + file
}

// fileExists reports whether a regular file exists at the given path.
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
