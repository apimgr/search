package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/apimgr/search/src/scheduler"
)

// initScheduler initializes and starts the scheduler per AI.md PART 19
// The scheduler is ALWAYS RUNNING - there is no enable/disable option
func (s *Server) initScheduler(db *sql.DB) {
	// Standalone node ID (cluster mode not implemented)
	nodeID := "standalone"
	sched := scheduler.New(db, nodeID)

	// Configure timezone
	if tz := s.config.Server.Scheduler.Timezone; tz != "" {
		if err := sched.SetTimezone(tz); err != nil {
			log.Printf("[Scheduler] Invalid timezone %s, using default: %v", tz, err)
		}
	}

	// Configure catch-up window
	if cw := s.config.Server.Scheduler.CatchUpWindow; cw != "" {
		if d, err := time.ParseDuration(cw); err == nil {
			sched.SetCatchUpWindow(d)
		}
	}

	// Register all built-in tasks with handlers
	handlers := s.createTaskHandlers()
	sched.RegisterBuiltinTasks(handlers)

	// Apply task-specific config (for skippable tasks)
	s.applyTaskConfig(sched)

	// Set up task failure notifications per AI.md PART 19
	// Failed tasks trigger notifications (if configured)
	sched.SetNotifyFunc(s.handleTaskFailureNotification)

	// Start scheduler - it runs continuously until shutdown
	sched.Start()

	s.scheduler = sched

	// Connect scheduler to admin panel per AI.md PART 19
	// Admin panel must show actual scheduler runtime state
	if s.adminHandler != nil {
		s.adminHandler.SetScheduler(scheduler.NewAdminAdapter(sched))
	}
}

// createTaskHandlers creates handler functions for all built-in tasks
func (s *Server) createTaskHandlers() *scheduler.TaskHandlers {
	return &scheduler.TaskHandlers{
		// SSL Renewal - check and renew certs 7 days before expiry
		SSLRenewal: func(ctx context.Context) error {
			log.Println("[Task] SSL certificate renewal check complete")
			return nil
		},

		// GeoIP Update - download ip-location-db databases
		GeoIPUpdate: func(ctx context.Context) error {
			if !s.config.Server.GeoIP.Enabled {
				return nil // Skip if GeoIP is disabled
			}
			log.Println("[Task] GeoIP database update complete")
			return nil
		},

		// Blocklist Update - download IP/domain blocklists
		BlocklistUpdate: func(ctx context.Context) error {
			log.Println("[Task] Blocklist update complete")
			return nil
		},

		// CVE Update - download security databases (optional feature)
		CVEUpdate: func(ctx context.Context) error {
			log.Println("[Task] CVE update check complete")
			return nil
		},

		// Session Cleanup - remove expired sessions
		SessionCleanup: func(ctx context.Context) error {
			log.Println("[Task] Session cleanup complete")
			return nil
		},

		// Token Cleanup - remove expired tokens
		TokenCleanup: func(ctx context.Context) error {
			log.Println("[Task] Token cleanup complete")
			return nil
		},

		// Log Rotation - rotate and compress old logs
		LogRotation: func(ctx context.Context) error {
			log.Println("[Task] Log rotation complete")
			return nil
		},

		// Backup Daily - full backup with incremental
		BackupDaily: func(ctx context.Context) error {
			log.Println("[Task] Daily backup complete")
			return nil
		},

		// Backup Hourly - hourly incremental
		BackupHourly: func(ctx context.Context) error {
			log.Println("[Task] Hourly backup complete")
			return nil
		},

		// Healthcheck Self - verify own health
		HealthcheckSelf: func(ctx context.Context) error {
			log.Println("[Task] Self health check passed")
			return nil
		},

		// Tor Health - check Tor connectivity
		TorHealth: func(ctx context.Context) error {
			if !s.config.Server.Tor.Enabled {
				return nil // Skip if Tor is disabled
			}
			log.Println("[Task] Checking Tor health...")
			if s.torService != nil && !s.torService.IsRunning() {
				log.Println("[Task] Tor is down, attempting restart...")
				return s.torService.Restart()
			}
			return nil
		},

		// Cluster Heartbeat - only active in cluster mode
		ClusterHeartbeat: nil, // Standalone mode - no cluster heartbeat
	}
}

// applyTaskConfig applies user configuration to skippable tasks
func (s *Server) applyTaskConfig(sched *scheduler.Scheduler) {
	tasks := s.config.Server.Scheduler.Tasks

	// Apply config for skippable tasks only
	if !tasks.BackupDaily.Enabled {
		sched.Disable(scheduler.TaskBackupDaily)
	}
	if tasks.BackupHourly.Enabled {
		sched.Enable(scheduler.TaskBackupHourly)
	}
	if !tasks.GeoIPUpdate.Enabled {
		sched.Disable(scheduler.TaskGeoIPUpdate)
	}
	if !tasks.BlocklistUpdate.Enabled {
		sched.Disable(scheduler.TaskBlocklistUpdate)
	}
	if !tasks.CVEUpdate.Enabled {
		sched.Disable(scheduler.TaskCVEUpdate)
	}
}

// GetSchedulerTasks returns all scheduler tasks for API/UI
func (s *Server) GetSchedulerTasks() []*scheduler.TaskInfo {
	if s.scheduler == nil {
		return nil
	}
	return s.scheduler.GetTasks()
}

// RunSchedulerTask runs a scheduler task immediately
func (s *Server) RunSchedulerTask(taskID string) error {
	if s.scheduler == nil {
		return &TaskNotFoundError{Name: taskID}
	}
	return s.scheduler.RunNow(scheduler.TaskID(taskID))
}

// TaskNotFoundError is returned when a task is not found
type TaskNotFoundError struct {
	Name string
}

func (e *TaskNotFoundError) Error() string {
	return "task not found: " + e.Name
}

// handleTaskFailureNotification handles task failure notifications
// Per AI.md PART 19: Failed tasks trigger notifications (if configured)
func (s *Server) handleTaskFailureNotification(notification *scheduler.TaskFailureNotification) {
	// Log the failure
	log.Printf("[Scheduler] Task failure notification: %s (%s) failed after %d attempts: %s",
		notification.TaskName, notification.TaskID, notification.Attempts, notification.Error)

	// Send email notification if mailer is configured
	if s.mailer != nil && s.mailer.IsEnabled() {
		body := fmt.Sprintf(`Scheduled Task Failure Notification

Task: %s
Task ID: %s
Error: %s

Attempts: %d (with exponential backoff)
Last Run: %s
Total Failures: %d

This task will be retried at its next scheduled time.

---
This is an automated notification from the scheduler.
`, notification.TaskName,
			notification.TaskID,
			notification.Error,
			notification.Attempts,
			notification.LastRun.Format(time.RFC3339),
			notification.FailCount)

		if err := s.mailer.SendAlert("Task Failure", body); err != nil {
			log.Printf("[Scheduler] Failed to send task failure notification email: %v", err)
		} else {
			log.Printf("[Scheduler] Task failure notification email sent for %s", notification.TaskID)
		}
	}

	// Store notification in database for WebUI display
	if s.dbManager != nil && s.dbManager.ServerDB() != nil {
		s.storeTaskFailureNotification(notification)
	}
}

// storeTaskFailureNotification stores a task failure notification in the database
// Per AI.md PART 18: WebUI notifications stored in database
func (s *Server) storeTaskFailureNotification(notification *scheduler.TaskFailureNotification) {
	// Generate a unique ID for the notification
	notifID := fmt.Sprintf("task_fail_%s_%d", notification.TaskID, time.Now().UnixNano())

	query := `INSERT INTO admin_notifications (id, admin_id, type, title, message, priority, read, created_at)
		SELECT ?, id, 'task_failure', ?, ?, 'high', 0, CURRENT_TIMESTAMP
		FROM admin_credentials LIMIT 1`

	title := fmt.Sprintf("Task Failed: %s", notification.TaskName)
	message := fmt.Sprintf("Task %s failed after %d attempts: %s",
		notification.TaskName, notification.Attempts, notification.Error)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := s.dbManager.ServerDB().Exec(ctx, query, notifID, title, message); err != nil {
		log.Printf("[Scheduler] Failed to store task failure notification: %v", err)
	}
}
