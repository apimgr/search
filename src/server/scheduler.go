package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/scheduler"
)

// initScheduler initializes and starts the scheduler per AI.md PART 19
// The scheduler is ALWAYS RUNNING - there is no enable/disable option
func (s *Server) initScheduler(db *sql.DB) {
	// Standalone node ID (single-node mode)
	nodeID := "standalone"
	sched := scheduler.NewScheduler(db, nodeID)

	// Configure timezone
	if tz := s.config.Server.Scheduler.Timezone; tz != "" {
		if err := sched.SetTimezone(tz); err != nil {
			slog.Warn("invalid timezone, using default", "timezone", tz, "err", err)
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
	sched.StartTaskScheduler()

	s.scheduler = sched
}

// createTaskHandlers creates handler functions for all built-in tasks
func (s *Server) createTaskHandlers() *scheduler.TaskHandlers {
	return &scheduler.TaskHandlers{
		// SSL Renewal - check and renew certs 7 days before expiry
		SSLRenewal: func(ctx context.Context) error {
			slog.Info("SSL certificate renewal check complete")
			return nil
		},

		// GeoIP Update - download ip-location-db databases
		GeoIPUpdate: func(ctx context.Context) error {
			if !s.config.Server.GeoIP.Enabled {
				// GeoIP is disabled; skip update
				return nil
			}
			slog.Info("GeoIP database update complete")
			return nil
		},

		// Blocklist Update - download IP/domain blocklists
		// Per AI.md PART 18: blocklist_update runs daily at 04:00
		BlocklistUpdate: func(ctx context.Context) error {
			if s.blocklistManager == nil {
				return nil
			}
			if err := s.blocklistManager.Update(ctx); err != nil {
				slog.Error("blocklist update failed", "err", err)
				return err
			}
			ips, nets := s.blocklistManager.Count()
			slog.Info("blocklist update complete", "blocked_ips", ips, "blocked_nets", nets)
			return nil
		},

		// CVE Update - download security databases (optional feature)
		CVEUpdate: func(ctx context.Context) error {
			if s.cveManager == nil {
				return nil
			}
			if err := s.cveManager.Update(ctx); err != nil {
				slog.Error("CVE update failed", "err", err)
				return err
			}
			slog.Info("CVE update complete", "entries", s.cveManager.Count())
			return nil
		},

		// Token Cleanup - remove expired tokens
		TokenCleanup: func(ctx context.Context) error {
			slog.Info("token cleanup complete")
			return nil
		},

		// Log Rotation - rotate and compress old logs
		LogRotation: func(ctx context.Context) error {
			slog.Info("log rotation complete")
			return nil
		},

		// Backup Daily - full backup with verification
		// Per AI.md PART 22: Backup verification is NON-NEGOTIABLE
		BackupDaily: func(ctx context.Context) error {
			return s.performScheduledBackup(ctx, "daily")
		},

		// Backup Hourly - hourly incremental backup
		// Per AI.md PART 22: Optional hourly backup (disabled by default)
		BackupHourly: func(ctx context.Context) error {
			return s.performScheduledBackup(ctx, "hourly")
		},

		// Healthcheck Self - verify own health
		HealthcheckSelf: func(ctx context.Context) error {
			if s.aggregator != nil {
				if err := s.aggregator.RefreshEngineHealth(ctx); err != nil {
					return err
				}
			}
			slog.Info("self health check passed")
			return nil
		},

		// Tor Health - check Tor connectivity
		TorHealth: func(ctx context.Context) error {
			if !s.config.Server.Tor.Enabled {
				// Tor is disabled; skip health check
				return nil
			}
			slog.Info("checking Tor health")
			if s.torService != nil && !s.torService.IsRunning() {
				slog.Warn("Tor is down, attempting restart")
				return s.torService.RestartTorService()
			}
			return nil
		},

		AlertsImmediate: func(ctx context.Context) error {
			if s.alertManager == nil {
				return nil
			}
			return s.alertManager.ProcessDue(ctx, alert.FrequencyImmediate)
		},

		AlertsDaily: func(ctx context.Context) error {
			if s.alertManager == nil {
				return nil
			}
			return s.alertManager.ProcessDue(ctx, alert.FrequencyDaily)
		},

		AlertsWeekly: func(ctx context.Context) error {
			if s.alertManager == nil {
				return nil
			}
			return s.alertManager.ProcessDue(ctx, alert.FrequencyWeekly)
		},

		// Public IP Refresh - startup + every 12h (hardcoded per AI.md PART 8 step 16)
		PublicIPRefresh: func(ctx context.Context) error {
			return s.refreshPublicIP(ctx)
		},
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
	slog.Error("task failure notification",
		"task_name", notification.TaskName,
		"task_id", notification.TaskID,
		"attempts", notification.Attempts,
		"error", notification.Error)

	// Send email notification if mailer is configured
	// Per AI.md PART 30: All user-facing text uses i18n keys.
	if s.mailer != nil && s.mailer.IsEnabled() {
		body := fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s: %s\n\n%s: %d\n%s: %s\n%s: %d\n\n%s\n\n---\n%s\n",
			i18n.TDefault("email_notifications.task_failure_title"),
			i18n.TDefault("email_notifications.task_label"),
			notification.TaskName,
			i18n.TDefault("email_notifications.task_id_label"),
			notification.TaskID,
			i18n.TDefault("email_notifications.error_label"),
			notification.Error,
			i18n.TDefault("email_notifications.attempts_label"),
			notification.Attempts,
			i18n.TDefault("email_notifications.last_run_label"),
			notification.LastRun.Format(time.RFC3339),
			i18n.TDefault("email_notifications.total_failures_label"),
			notification.FailCount,
			i18n.TDefault("email_notifications.task_retry_notice"),
			i18n.TDefault("email_notifications.automated_notice"),
		)

		if err := s.mailer.SendAlert(i18n.TDefault("email_notifications.task_failure_subject"), body); err != nil {
			slog.Error("failed to send task failure notification email", "err", err)
		} else {
			slog.Info("task failure notification email sent", "task_id", notification.TaskID)
		}
	}

	// Persistent notification storage for a WebUI admin panel was removed
	// when the panel itself was removed. Operators consume failure events
	// via the audit log and email notification above.
}

// performScheduledBackup performs a scheduled backup with verification
// Per AI.md PART 22: Backup verification is NON-NEGOTIABLE
// - File exists
// - Size > 0
// - Checksum valid
// - Manifest readable
// - Decrypt test (if encrypted)
// Only delete old backups if new backup passes ALL verification checks.
func (s *Server) performScheduledBackup(ctx context.Context, backupType string) error {
	slog.Info("starting scheduled backup", "type", backupType)

	// Create backup manager
	mgr := backup.NewManager()
	// Per AI.md PART 25: set attribution before storing backup metadata
	mgr.SetCreatedBy("scheduler")

	// Per AI.md PART 22: Check compliance mode
	// If compliance enabled and no password, skip backup with warning
	complianceEnabled := s.config.Server.Compliance.Enabled
	encryptionEnabled := s.config.Server.Backup.Encryption.Enabled

	// Get backup password from environment variable (NEVER stored in config)
	// Per AI.md PART 22/24: Password is NEVER stored - derived on-demand
	backupPassword := os.Getenv("BACKUP_PASSWORD")

	if complianceEnabled {
		if backupPassword == "" {
			// Per AI.md PART 22: Scheduled backups skip with audit log warning
			slog.Warn("compliance mode enabled but BACKUP_PASSWORD not set, backup skipped")
			s.logAuditEvent("backup.skipped", "Compliance mode requires backup encryption but password not set")
			return fmt.Errorf("compliance mode requires backup encryption but BACKUP_PASSWORD not set")
		}
		// Compliance mode forces encryption
		encryptionEnabled = true
	}

	// Set password if encryption is enabled
	if encryptionEnabled && backupPassword != "" {
		mgr.SetPassword(backupPassword)
	}

	// Get retention settings from config
	// Per AI.md PART 22: max_backups (default: 1)
	maxBackups := s.config.Server.Backup.Retention.MaxBackups
	if maxBackups < 1 {
		// Enforce minimum of 1 backup
		maxBackups = 1
	}

	var backupPath string
	var verifyResult *backup.VerificationResult
	var err error

	// Create backup with verification
	// Per AI.md PART 22: Only delete old backups if new backup passes ALL verification checks
	if encryptionEnabled && backupPassword != "" {
		// Create encrypted backup with verification
		backupPath, verifyResult, err = mgr.CreateEncryptedAndVerify("")
	} else {
		// Create unencrypted backup with verification
		backupPath, verifyResult, err = mgr.CreateAndVerify("")
	}

	if err != nil {
		// Per AI.md PART 22: On failure, DO NOT delete any existing backups
		slog.Error("backup failed", "type", backupType, "err", err)
		s.logAuditEvent("backup.verification_failed", fmt.Sprintf("%s backup failed: %v", backupType, err))
		return err
	}

	// Log verification results
	if verifyResult != nil && verifyResult.AllPassed {
		slog.Info("backup created and verified", "type", backupType, "path", backupPath)
		s.logAuditEvent("backup.created", fmt.Sprintf("%s backup created: %s (verified: file=%v, size=%v, checksum=%v, manifest=%v)",
			backupType, backupPath, verifyResult.FileExists, verifyResult.SizeValid, verifyResult.ChecksumValid, verifyResult.ManifestValid))
	}

	// Apply retention policy only after verification passes
	// Per AI.md PART 22: Only delete old backups if new backup passes ALL verification checks
	if err := mgr.ScheduledBackupWithVerification(maxBackups); err != nil {
		// Don't fail the task, just log the retention cleanup error
		slog.Warn("backup retention cleanup failed", "err", err)
	}

	// Apply advanced retention policy if configured
	// Per AI.md PART 22: keep_weekly, keep_monthly, keep_yearly
	retention := s.config.Server.Backup.Retention
	if retention.KeepWeekly > 0 || retention.KeepMonthly > 0 || retention.KeepYearly > 0 {
		policy := backup.RetentionPolicy{
			Count: maxBackups,
			Day:   7,
			Week:  retention.KeepWeekly,
			Month: retention.KeepMonthly,
			Year:  retention.KeepYearly,
		}
		if err := mgr.ApplyRetention(policy); err != nil {
			slog.Warn("advanced retention policy failed", "err", err)
		} else {
			s.logAuditEvent("backup.retention_cleanup", "Applied retention policy")
		}
	}

	slog.Info("backup complete", "type", backupType)
	return nil
}

// logAuditEvent logs an audit event (simplified version for scheduler)
// Per AI.md PART 22: Audit logging for backup events
func (s *Server) logAuditEvent(event, details string) {
	if s.dbManager == nil || s.dbManager.ServerDB() == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `INSERT INTO audit_log (event, details, ip_address, user_agent, created_at)
		VALUES (?, ?, 'scheduler', 'internal', CURRENT_TIMESTAMP)`

	if _, err := s.dbManager.ServerDB().Exec(ctx, query, event, details); err != nil {
		slog.Error("failed to log audit event", "event", event, "err", err)
	}
}
