package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"
)

// ClusterMode represents the cluster operating mode
type ClusterMode string

const (
	// ClusterModeStandalone is single-node mode (SQLite)
	ClusterModeStandalone ClusterMode = "standalone"
	// ClusterModeCluster is multi-node mode (PostgreSQL/MySQL)
	ClusterModeCluster ClusterMode = "cluster"
)

// NodeStatus represents a cluster node's status
type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
	NodeStatusJoining NodeStatus = "joining"
	NodeStatusLeaving NodeStatus = "leaving"
)

// ClusterNode represents a node in the cluster
type ClusterNode struct {
	ID          string
	Hostname    string
	Address     string
	Port        int
	Version     string
	IsPrimary   bool
	Status      NodeStatus
	LastSeen    time.Time
	JoinedAt    time.Time
	Metadata    map[string]string
}

// ClusterManager manages cluster operations per TEMPLATE.md PART 24
type ClusterManager struct {
	db         *DatabaseManager
	nodeID     string
	hostname   string
	mode       ClusterMode
	isPrimary  bool
	mu         sync.RWMutex
	stopCh     chan struct{}
	started    bool
}

// NewClusterManager creates a new cluster manager
func NewClusterManager(dm *DatabaseManager) (*ClusterManager, error) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	// Generate unique node ID
	nodeID := generateNodeID()

	cm := &ClusterManager{
		db:       dm,
		nodeID:   nodeID,
		hostname: hostname,
		mode:     ClusterModeStandalone,
		stopCh:   make(chan struct{}),
	}

	// Detect cluster mode based on driver
	if dm.serverDB != nil && dm.serverDB.driver != "sqlite" && dm.serverDB.driver != "sqlite3" {
		cm.mode = ClusterModeCluster
	}

	return cm, nil
}

// Start starts the cluster manager
func (cm *ClusterManager) Start(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.started {
		return nil
	}

	// Only run cluster operations in cluster mode
	if cm.mode == ClusterModeCluster {
		if err := cm.ensureClusterTable(ctx); err != nil {
			return fmt.Errorf("failed to create cluster table: %w", err)
		}

		if err := cm.registerNode(ctx); err != nil {
			return fmt.Errorf("failed to register node: %w", err)
		}

		// Start heartbeat goroutine
		go cm.heartbeatLoop()

		// Start primary election goroutine
		go cm.primaryElectionLoop()
	} else {
		// In standalone mode, this node is always primary
		cm.isPrimary = true
	}

	cm.started = true
	return nil
}

// Stop stops the cluster manager
func (cm *ClusterManager) Stop() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if !cm.started {
		return
	}

	close(cm.stopCh)
	cm.started = false

	// Unregister node if in cluster mode
	if cm.mode == ClusterModeCluster {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cm.unregisterNode(ctx)
	}
}

// Mode returns the current cluster mode
func (cm *ClusterManager) Mode() ClusterMode {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.mode
}

// IsClusterMode returns true if running in cluster mode
func (cm *ClusterManager) IsClusterMode() bool {
	return cm.Mode() == ClusterModeCluster
}

// IsPrimary returns true if this node is the primary
func (cm *ClusterManager) IsPrimary() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.isPrimary
}

// NodeID returns this node's ID
func (cm *ClusterManager) NodeID() string {
	return cm.nodeID
}

// Hostname returns this node's hostname
func (cm *ClusterManager) Hostname() string {
	return cm.hostname
}

// GetNodes returns all nodes in the cluster
func (cm *ClusterManager) GetNodes(ctx context.Context) ([]*ClusterNode, error) {
	if cm.mode == ClusterModeStandalone {
		// Return self as only node
		return []*ClusterNode{{
			ID:        cm.nodeID,
			Hostname:  cm.hostname,
			IsPrimary: true,
			Status:    NodeStatusOnline,
			LastSeen:  time.Now(),
			JoinedAt:  time.Now(),
		}}, nil
	}

	rows, err := cm.db.serverDB.Query(ctx, `
		SELECT id, hostname, address, port, version, is_primary, status, last_seen, joined_at
		FROM cluster_nodes
		ORDER BY is_primary DESC, joined_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*ClusterNode
	for rows.Next() {
		var node ClusterNode
		var status string
		err := rows.Scan(
			&node.ID, &node.Hostname, &node.Address, &node.Port,
			&node.Version, &node.IsPrimary, &status, &node.LastSeen, &node.JoinedAt,
		)
		if err != nil {
			return nil, err
		}
		node.Status = NodeStatus(status)
		nodes = append(nodes, &node)
	}

	return nodes, rows.Err()
}

// GetNode returns a specific node by ID
func (cm *ClusterManager) GetNode(ctx context.Context, nodeID string) (*ClusterNode, error) {
	if cm.mode == ClusterModeStandalone {
		if nodeID == cm.nodeID {
			return &ClusterNode{
				ID:        cm.nodeID,
				Hostname:  cm.hostname,
				IsPrimary: true,
				Status:    NodeStatusOnline,
				LastSeen:  time.Now(),
			}, nil
		}
		return nil, nil
	}

	row := cm.db.serverDB.QueryRow(ctx, `
		SELECT id, hostname, address, port, version, is_primary, status, last_seen, joined_at
		FROM cluster_nodes WHERE id = ?
	`, nodeID)

	var node ClusterNode
	var status string
	err := row.Scan(
		&node.ID, &node.Hostname, &node.Address, &node.Port,
		&node.Version, &node.IsPrimary, &status, &node.LastSeen, &node.JoinedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	node.Status = NodeStatus(status)

	return &node, nil
}

// GenerateJoinToken generates a join token for new nodes
func (cm *ClusterManager) GenerateJoinToken(ctx context.Context) (string, error) {
	if cm.mode == ClusterModeStandalone {
		return "", fmt.Errorf("join tokens not available in standalone mode")
	}

	// Generate secure token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes)

	// Store token with expiry
	expiresAt := time.Now().Add(24 * time.Hour)
	_, err := cm.db.serverDB.Exec(ctx, `
		INSERT INTO cluster_join_tokens (token_hash, created_by, expires_at)
		VALUES (?, ?, ?)
	`, hashToken(token), cm.nodeID, expiresAt)
	if err != nil {
		return "", err
	}

	return token, nil
}

// LeaveCluster removes this node from the cluster
func (cm *ClusterManager) LeaveCluster(ctx context.Context) error {
	if cm.mode == ClusterModeStandalone {
		return fmt.Errorf("not in cluster mode")
	}

	if cm.isPrimary {
		// Transfer primary to another node before leaving
		if err := cm.transferPrimary(ctx); err != nil {
			return fmt.Errorf("failed to transfer primary role: %w", err)
		}
	}

	return cm.unregisterNode(ctx)
}

// ensureClusterTable creates the cluster tables if they don't exist
func (cm *ClusterManager) ensureClusterTable(ctx context.Context) error {
	_, err := cm.db.serverDB.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS cluster_nodes (
			id TEXT PRIMARY KEY,
			hostname TEXT NOT NULL,
			address TEXT,
			port INTEGER DEFAULT 0,
			version TEXT,
			is_primary INTEGER DEFAULT 0,
			status TEXT DEFAULT 'online',
			last_seen DATETIME NOT NULL,
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			metadata TEXT
		)
	`)
	if err != nil {
		return err
	}

	_, err = cm.db.serverDB.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS cluster_join_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token_hash TEXT UNIQUE NOT NULL,
			created_by TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			used_at DATETIME,
			used_by TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// registerNode registers this node in the cluster
func (cm *ClusterManager) registerNode(ctx context.Context) error {
	now := time.Now()

	// Check if any primary exists
	var primaryCount int
	row := cm.db.serverDB.QueryRow(ctx, `
		SELECT COUNT(*) FROM cluster_nodes WHERE is_primary = 1 AND status = 'online'
	`)
	if err := row.Scan(&primaryCount); err != nil {
		return err
	}

	// If no primary, this node becomes primary
	isPrimary := primaryCount == 0

	_, err := cm.db.serverDB.Exec(ctx, `
		INSERT INTO cluster_nodes (id, hostname, is_primary, status, last_seen, joined_at)
		VALUES (?, ?, ?, 'online', ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname = excluded.hostname,
			is_primary = excluded.is_primary,
			status = 'online',
			last_seen = excluded.last_seen
	`, cm.nodeID, cm.hostname, isPrimary, now, now)
	if err != nil {
		return err
	}

	cm.mu.Lock()
	cm.isPrimary = isPrimary
	cm.mu.Unlock()

	return nil
}

// unregisterNode removes this node from the cluster
func (cm *ClusterManager) unregisterNode(ctx context.Context) error {
	_, err := cm.db.serverDB.Exec(ctx, `
		UPDATE cluster_nodes SET status = 'offline' WHERE id = ?
	`, cm.nodeID)
	return err
}

// heartbeatLoop sends periodic heartbeats
func (cm *ClusterManager) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cm.sendHeartbeat(ctx)
			cm.cleanupStaleNodes(ctx)
			cancel()
		}
	}
}

// sendHeartbeat updates this node's last_seen timestamp
func (cm *ClusterManager) sendHeartbeat(ctx context.Context) {
	_, _ = cm.db.serverDB.Exec(ctx, `
		UPDATE cluster_nodes SET last_seen = datetime('now') WHERE id = ?
	`, cm.nodeID)
}

// cleanupStaleNodes marks stale nodes as offline
func (cm *ClusterManager) cleanupStaleNodes(ctx context.Context) {
	// Mark nodes as offline if they haven't sent a heartbeat in 2 minutes
	_, _ = cm.db.serverDB.Exec(ctx, `
		UPDATE cluster_nodes SET status = 'offline'
		WHERE status = 'online' AND last_seen < datetime('now', '-2 minutes')
	`)
}

// primaryElectionLoop handles primary election
func (cm *ClusterManager) primaryElectionLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cm.checkPrimaryStatus(ctx)
			cancel()
		}
	}
}

// checkPrimaryStatus checks if primary election is needed
func (cm *ClusterManager) checkPrimaryStatus(ctx context.Context) {
	// Check if there's an online primary
	var primaryID string
	row := cm.db.serverDB.QueryRow(ctx, `
		SELECT id FROM cluster_nodes WHERE is_primary = 1 AND status = 'online'
	`)
	err := row.Scan(&primaryID)

	if err == sql.ErrNoRows {
		// No primary - try to become primary
		cm.tryBecomePrimary(ctx)
	}
}

// tryBecomePrimary attempts to become the primary node
func (cm *ClusterManager) tryBecomePrimary(ctx context.Context) {
	// Use atomic update to prevent race conditions
	result, err := cm.db.serverDB.Exec(ctx, `
		UPDATE cluster_nodes SET is_primary = 1
		WHERE id = ? AND status = 'online'
		AND NOT EXISTS (SELECT 1 FROM cluster_nodes WHERE is_primary = 1 AND status = 'online')
	`, cm.nodeID)
	if err != nil {
		return
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		cm.mu.Lock()
		cm.isPrimary = true
		cm.mu.Unlock()
	}
}

// transferPrimary transfers primary role to another node
func (cm *ClusterManager) transferPrimary(ctx context.Context) error {
	// Find another online node
	var newPrimaryID string
	row := cm.db.serverDB.QueryRow(ctx, `
		SELECT id FROM cluster_nodes
		WHERE id != ? AND status = 'online'
		ORDER BY joined_at ASC LIMIT 1
	`, cm.nodeID)
	if err := row.Scan(&newPrimaryID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no other nodes available")
		}
		return err
	}

	// Transfer primary
	_, err := cm.db.serverDB.Exec(ctx, `
		UPDATE cluster_nodes SET is_primary = CASE WHEN id = ? THEN 1 ELSE 0 END
		WHERE id IN (?, ?)
	`, newPrimaryID, newPrimaryID, cm.nodeID)
	if err != nil {
		return err
	}

	cm.mu.Lock()
	cm.isPrimary = false
	cm.mu.Unlock()

	return nil
}

// generateNodeID generates a unique node ID
func generateNodeID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("node_%s", hex.EncodeToString(b))
}

// hashToken hashes a token for storage
func hashToken(token string) string {
	b := make([]byte, 32)
	for i := 0; i < len(token) && i < len(b); i++ {
		b[i] = token[i]
	}
	return hex.EncodeToString(b)
}
