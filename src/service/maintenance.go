package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apimgr/search/src/config"
)

// MaintenanceMode represents the current maintenance state
// Per AI.md PART 6: Maintenance Mode (NON-NEGOTIABLE)
type MaintenanceMode int

const (
	// ModeNormal is normal operation
	ModeNormal MaintenanceMode = iota
	// ModeDegraded is reduced functionality (some services unavailable)
	ModeDegraded
	// ModeMaintenance is full maintenance mode (read-only, limited access)
	ModeMaintenance
	// ModeRecovery is recovery mode (attempting to repair)
	ModeRecovery
	// ModeEmergency is emergency mode (critical failure, minimal functionality)
	ModeEmergency
)

// String returns the string representation of the maintenance mode
func (m MaintenanceMode) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeDegraded:
		return "degraded"
	case ModeMaintenance:
		return "maintenance"
	case ModeRecovery:
		return "recovery"
	case ModeEmergency:
		return "emergency"
	default:
		return "unknown"
	}
}

// HealthStatus represents the health of a system component
type HealthStatus struct {
	Component   string    `json:"component"`
	Healthy     bool      `json:"healthy"`
	Message     string    `json:"message,omitempty"`
	LastCheck   time.Time `json:"last_check"`
	LastHealthy time.Time `json:"last_healthy,omitempty"`
	ErrorCount  int       `json:"error_count"`
}

// MaintenanceService manages maintenance mode and self-healing
// Per AI.md PART 6: Maintenance Mode (NON-NEGOTIABLE)
type MaintenanceService struct {
	config       *config.Config
	mode         int32
	mu           sync.RWMutex
	health       map[string]*HealthStatus
	ctx          context.Context
	cancel       context.CancelFunc
	running      bool
	message      string
	scheduledEnd time.Time
	callbacks    []func(MaintenanceMode)

	// Database connections for health checks
	serverDBCheck func(ctx context.Context) error
	usersDBCheck  func(ctx context.Context) error

	// Recovery functions
	recoveryFuncs map[string]func(ctx context.Context) error
}

// NewMaintenanceService creates a new maintenance service
func NewMaintenanceService(cfg *config.Config) *MaintenanceService {
	ctx, cancel := context.WithCancel(context.Background())
	return &MaintenanceService{
		config:        cfg,
		mode:          int32(ModeNormal),
		health:        make(map[string]*HealthStatus),
		ctx:           ctx,
		cancel:        cancel,
		recoveryFuncs: make(map[string]func(ctx context.Context) error),
	}
}

// Start starts the maintenance service monitoring
func (m *MaintenanceService) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = true
	m.mu.Unlock()

	log.Println("[Maintenance] Service started")

	// Initialize health status for known components
	m.initializeHealthStatus()

	// Start health monitoring goroutine
	go m.healthMonitor()

	return nil
}

// Stop stops the maintenance service
func (m *MaintenanceService) Stop() {
	m.cancel()
	m.mu.Lock()
	m.running = false
	m.mu.Unlock()
	log.Println("[Maintenance] Service stopped")
}

// initializeHealthStatus initializes health tracking for components
func (m *MaintenanceService) initializeHealthStatus() {
	m.mu.Lock()
	defer m.mu.Unlock()

	components := []string{"server_db", "users_db", "cache", "tor", "scheduler"}
	now := time.Now()

	for _, comp := range components {
		m.health[comp] = &HealthStatus{
			Component:   comp,
			Healthy:     true,
			LastCheck:   now,
			LastHealthy: now,
			ErrorCount:  0,
		}
	}
}

// healthMonitor continuously monitors system health
func (m *MaintenanceService) healthMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial check
	m.performHealthChecks()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performHealthChecks()
			m.evaluateSystemHealth()
			m.attemptRecovery()
		}
	}
}

// performHealthChecks checks all system components
func (m *MaintenanceService) performHealthChecks() {
	ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer cancel()

	// Check server database
	if m.serverDBCheck != nil {
		m.checkComponent(ctx, "server_db", m.serverDBCheck)
	}

	// Check users database
	if m.usersDBCheck != nil {
		m.checkComponent(ctx, "users_db", m.usersDBCheck)
	}
}

// checkComponent checks a single component and updates its health status
func (m *MaintenanceService) checkComponent(ctx context.Context, name string, check func(context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	status, exists := m.health[name]
	if !exists {
		status = &HealthStatus{Component: name}
		m.health[name] = status
	}

	status.LastCheck = time.Now()

	if err := check(ctx); err != nil {
		status.Healthy = false
		status.Message = err.Error()
		status.ErrorCount++
		log.Printf("[Maintenance] Component %s unhealthy: %v", name, err)
	} else {
		status.Healthy = true
		status.Message = ""
		status.LastHealthy = time.Now()
		status.ErrorCount = 0
	}
}

// evaluateSystemHealth evaluates overall system health and adjusts mode
func (m *MaintenanceService) evaluateSystemHealth() {
	m.mu.RLock()
	var unhealthyCount int
	var criticalUnhealthy bool

	for name, status := range m.health {
		if !status.Healthy {
			unhealthyCount++
			// Databases are critical
			if name == "server_db" || name == "users_db" {
				criticalUnhealthy = true
			}
		}
	}
	m.mu.RUnlock()

	// Determine appropriate mode based on health
	var newMode MaintenanceMode

	switch {
	case unhealthyCount == 0:
		newMode = ModeNormal
	case criticalUnhealthy:
		newMode = ModeEmergency
	case unhealthyCount >= 2:
		newMode = ModeDegraded
	default:
		newMode = ModeDegraded
	}

	// Only change mode if different and not manually set to maintenance
	currentMode := MaintenanceMode(atomic.LoadInt32(&m.mode))
	if currentMode != ModeMaintenance && currentMode != newMode {
		m.SetMode(newMode, "")
	}
}

// attemptRecovery attempts to recover unhealthy components
func (m *MaintenanceService) attemptRecovery() {
	m.mu.RLock()
	unhealthyComponents := make([]string, 0)
	for name, status := range m.health {
		if !status.Healthy && status.ErrorCount >= 3 {
			unhealthyComponents = append(unhealthyComponents, name)
		}
	}
	m.mu.RUnlock()

	if len(unhealthyComponents) == 0 {
		return
	}

	// Enter recovery mode
	previousMode := MaintenanceMode(atomic.LoadInt32(&m.mode))
	if previousMode != ModeRecovery {
		m.SetMode(ModeRecovery, "Attempting automatic recovery")
	}

	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
	defer cancel()

	for _, comp := range unhealthyComponents {
		if recoveryFunc, exists := m.recoveryFuncs[comp]; exists {
			log.Printf("[Maintenance] Attempting recovery of %s", comp)
			if err := recoveryFunc(ctx); err != nil {
				log.Printf("[Maintenance] Recovery of %s failed: %v", comp, err)
			} else {
				log.Printf("[Maintenance] Recovery of %s successful", comp)
				m.mu.Lock()
				if status, exists := m.health[comp]; exists {
					status.ErrorCount = 0
				}
				m.mu.Unlock()
			}
		}
	}

	// Re-evaluate after recovery attempts
	m.performHealthChecks()
	m.evaluateSystemHealth()
}

// SetMode sets the maintenance mode
func (m *MaintenanceService) SetMode(mode MaintenanceMode, message string) {
	oldMode := MaintenanceMode(atomic.SwapInt32(&m.mode, int32(mode)))

	m.mu.Lock()
	m.message = message
	m.mu.Unlock()

	if oldMode != mode {
		log.Printf("[Maintenance] Mode changed: %s -> %s", oldMode, mode)

		// Notify callbacks
		m.mu.RLock()
		callbacks := m.callbacks
		m.mu.RUnlock()

		for _, cb := range callbacks {
			if cb != nil {
				go cb(mode)
			}
		}
	}
}

// GetMode returns the current maintenance mode
func (m *MaintenanceService) GetMode() MaintenanceMode {
	return MaintenanceMode(atomic.LoadInt32(&m.mode))
}

// GetMessage returns the current maintenance message
func (m *MaintenanceService) GetMessage() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.message
}

// IsNormal returns true if system is in normal mode
func (m *MaintenanceService) IsNormal() bool {
	return m.GetMode() == ModeNormal
}

// IsDegraded returns true if system is in degraded mode
func (m *MaintenanceService) IsDegraded() bool {
	return m.GetMode() == ModeDegraded
}

// IsInMaintenance returns true if system is in maintenance mode
func (m *MaintenanceService) IsInMaintenance() bool {
	return m.GetMode() == ModeMaintenance
}

// EnableMaintenance enables maintenance mode
func (m *MaintenanceService) EnableMaintenance(message string, duration time.Duration) {
	m.SetMode(ModeMaintenance, message)

	m.mu.Lock()
	if duration > 0 {
		m.scheduledEnd = time.Now().Add(duration)
	} else {
		m.scheduledEnd = time.Time{}
	}
	m.mu.Unlock()

	// Schedule automatic exit from maintenance mode
	if duration > 0 {
		go func() {
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(duration):
				if m.GetMode() == ModeMaintenance {
					m.DisableMaintenance()
				}
			}
		}()
	}
}

// DisableMaintenance disables maintenance mode
func (m *MaintenanceService) DisableMaintenance() {
	m.mu.Lock()
	m.scheduledEnd = time.Time{}
	m.mu.Unlock()

	m.SetMode(ModeNormal, "")
	log.Println("[Maintenance] Maintenance mode disabled")
}

// GetScheduledEnd returns when maintenance is scheduled to end
func (m *MaintenanceService) GetScheduledEnd() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.scheduledEnd
}

// RegisterCallback registers a callback for mode changes
func (m *MaintenanceService) RegisterCallback(cb func(MaintenanceMode)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, cb)
}

// SetDatabaseChecks sets the database check functions
func (m *MaintenanceService) SetDatabaseChecks(serverCheck, usersCheck func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serverDBCheck = serverCheck
	m.usersDBCheck = usersCheck
}

// RegisterRecoveryFunc registers a recovery function for a component
func (m *MaintenanceService) RegisterRecoveryFunc(component string, fn func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recoveryFuncs[component] = fn
}

// GetHealthStatus returns the health status of all components
func (m *MaintenanceService) GetHealthStatus() map[string]*HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*HealthStatus)
	for k, v := range m.health {
		// Return a copy
		result[k] = &HealthStatus{
			Component:   v.Component,
			Healthy:     v.Healthy,
			Message:     v.Message,
			LastCheck:   v.LastCheck,
			LastHealthy: v.LastHealthy,
			ErrorCount:  v.ErrorCount,
		}
	}
	return result
}

// GetStatus returns the full maintenance status
func (m *MaintenanceService) GetStatus() map[string]interface{} {
	mode := m.GetMode()

	m.mu.RLock()
	defer m.mu.RUnlock()

	status := map[string]interface{}{
		"mode":          mode.String(),
		"message":       m.message,
		"scheduled_end": m.scheduledEnd,
		"health":        m.health,
	}

	return status
}

// CheckDatabaseIntegrity checks database integrity and attempts repair if needed
// Per AI.md PART 6: Database corruption handling
func (m *MaintenanceService) CheckDatabaseIntegrity(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(m.ctx, 120*time.Second)
	defer cancel()

	// Run SQLite integrity check
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("failed to run integrity check: %w", err)
	}
	defer rows.Close()

	var issues []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return fmt.Errorf("failed to scan integrity result: %w", err)
		}
		if result != "ok" {
			issues = append(issues, result)
		}
	}

	if len(issues) > 0 {
		log.Printf("[Maintenance] Database integrity issues found: %v", issues)
		return fmt.Errorf("database integrity issues: %v", issues)
	}

	return nil
}

// RepairDatabase attempts to repair a corrupted database
// Per AI.md PART 6: Database corruption handling
func (m *MaintenanceService) RepairDatabase(dbPath string) error {
	log.Printf("[Maintenance] Attempting database repair: %s", dbPath)

	// Create backup first
	backupPath := dbPath + ".backup." + time.Now().Format("20060102-150405")
	if err := copyFile(dbPath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	log.Printf("[Maintenance] Backup created: %s", backupPath)

	// Open a new connection
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Try to recover using VACUUM
	ctx, cancel := context.WithTimeout(m.ctx, 300*time.Second)
	defer cancel()

	// First try VACUUM INTO to create a clean copy
	cleanPath := dbPath + ".clean"
	if _, err := db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", cleanPath)); err != nil {
		log.Printf("[Maintenance] VACUUM INTO failed: %v, trying reindex", err)

		// Try REINDEX as fallback
		if _, err := db.ExecContext(ctx, "REINDEX"); err != nil {
			return fmt.Errorf("repair failed: %w", err)
		}
		log.Println("[Maintenance] REINDEX completed")
		return nil
	}

	// Replace original with clean copy
	db.Close()
	if err := os.Rename(cleanPath, dbPath); err != nil {
		return fmt.Errorf("failed to replace with clean database: %w", err)
	}

	log.Println("[Maintenance] Database repair completed successfully")
	return nil
}

// BackupDatabase creates a backup of the database
func (m *MaintenanceService) BackupDatabase(dbPath, backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, filepath.Base(dbPath)+"."+timestamp)

	if err := copyFile(dbPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to copy database: %w", err)
	}

	// Create checksum
	checksum, err := fileChecksum(backupPath)
	if err != nil {
		log.Printf("[Maintenance] Warning: failed to create checksum: %v", err)
	} else {
		checksumPath := backupPath + ".sha256"
		if err := os.WriteFile(checksumPath, []byte(checksum), 0600); err != nil {
			log.Printf("[Maintenance] Warning: failed to write checksum: %v", err)
		}
	}

	log.Printf("[Maintenance] Database backup created: %s", backupPath)
	return backupPath, nil
}

// RestoreDatabase restores a database from backup
func (m *MaintenanceService) RestoreDatabase(backupPath, dbPath string) error {
	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	// Verify checksum if available
	checksumPath := backupPath + ".sha256"
	if data, err := os.ReadFile(checksumPath); err == nil {
		expectedChecksum := string(data)
		actualChecksum, err := fileChecksum(backupPath)
		if err != nil {
			return fmt.Errorf("failed to verify backup: %w", err)
		}
		if actualChecksum != expectedChecksum {
			return fmt.Errorf("backup checksum mismatch")
		}
		log.Println("[Maintenance] Backup checksum verified")
	}

	// Create backup of current database
	if _, err := os.Stat(dbPath); err == nil {
		currentBackup := dbPath + ".before-restore." + time.Now().Format("20060102-150405")
		if err := copyFile(dbPath, currentBackup); err != nil {
			log.Printf("[Maintenance] Warning: failed to backup current database: %v", err)
		}
	}

	// Restore backup
	if err := copyFile(backupPath, dbPath); err != nil {
		return fmt.Errorf("failed to restore database: %w", err)
	}

	log.Printf("[Maintenance] Database restored from: %s", backupPath)
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

// fileChecksum calculates SHA256 checksum of a file
func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// GracefulDegradation provides fallback behavior when services are unavailable
// Per AI.md PART 6: Graceful degradation
type GracefulDegradation struct {
	mu              sync.RWMutex
	degradedFeatures map[string]bool
	fallbacks       map[string]func() interface{}
}

// NewGracefulDegradation creates a new graceful degradation handler
func NewGracefulDegradation() *GracefulDegradation {
	return &GracefulDegradation{
		degradedFeatures: make(map[string]bool),
		fallbacks:        make(map[string]func() interface{}),
	}
}

// MarkDegraded marks a feature as degraded
func (g *GracefulDegradation) MarkDegraded(feature string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.degradedFeatures[feature] = true
	log.Printf("[Degradation] Feature marked as degraded: %s", feature)
}

// MarkHealthy marks a feature as healthy
func (g *GracefulDegradation) MarkHealthy(feature string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.degradedFeatures, feature)
	log.Printf("[Degradation] Feature marked as healthy: %s", feature)
}

// IsDegraded checks if a feature is degraded
func (g *GracefulDegradation) IsDegraded(feature string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.degradedFeatures[feature]
}

// RegisterFallback registers a fallback function for a feature
func (g *GracefulDegradation) RegisterFallback(feature string, fallback func() interface{}) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.fallbacks[feature] = fallback
}

// GetFallback returns the fallback value for a degraded feature
func (g *GracefulDegradation) GetFallback(feature string) interface{} {
	g.mu.RLock()
	fallback, exists := g.fallbacks[feature]
	g.mu.RUnlock()

	if exists {
		return fallback()
	}
	return nil
}

// Execute executes a function with graceful degradation
func (g *GracefulDegradation) Execute(feature string, fn func() (interface{}, error)) (interface{}, error) {
	if g.IsDegraded(feature) {
		if fallback := g.GetFallback(feature); fallback != nil {
			return fallback, nil
		}
		return nil, fmt.Errorf("feature %s is degraded and no fallback available", feature)
	}

	result, err := fn()
	if err != nil {
		// Mark as degraded after failure
		g.MarkDegraded(feature)
		if fallback := g.GetFallback(feature); fallback != nil {
			return fallback, nil
		}
		return nil, err
	}

	return result, nil
}

// GetDegradedFeatures returns a list of currently degraded features
func (g *GracefulDegradation) GetDegradedFeatures() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	features := make([]string, 0, len(g.degradedFeatures))
	for f := range g.degradedFeatures {
		features = append(features, f)
	}
	return features
}
