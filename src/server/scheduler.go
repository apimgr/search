package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/update"
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
		// Per AI.md PART 18 Built-in Tasks (daily 03:00) and PART 15
		// (SSL/TLS & Let's Encrypt): renews via DNS-01 when the cert is
		// within the renewal threshold, and per PART 17 "Default
		// Templates" sends ssl_renewed on a successful renewal or
		// ssl_expiring at the 7/3/1 day thresholds when it is not.
		SSLRenewal: func(ctx context.Context) error {
			if s.tlsManager == nil || !s.tlsManager.IsEnabled() {
				slog.Info("SSL certificate renewal check complete")
				return nil
			}

			before, err := s.tlsManager.GetCertInfo()
			if err != nil {
				slog.Info("SSL certificate renewal check complete", "err", err)
				return nil
			}
			daysLeft := int(time.Until(before.NotAfter).Hours() / 24)

			if err := s.tlsManager.RenewCertificateDNS01(ctx); err != nil {
				slog.Error("SSL certificate renewal failed", "err", err)
				nextRetry := time.Now().Add(24 * time.Hour)
				if s.scheduler != nil {
					if info, infoErr := s.scheduler.GetTask(scheduler.TaskSSLRenewal); infoErr == nil {
						nextRetry = info.NextRun
					}
				}
				s.sendSSLRenewalFailedNotification(before.Subject, err.Error(), daysLeft, nextRetry)
				return err
			}

			after, err := s.tlsManager.GetCertInfo()
			if err != nil {
				slog.Info("SSL certificate renewal check complete")
				return nil
			}

			if after.NotAfter.After(before.NotAfter) {
				slog.Info("SSL certificate renewed", "domain", after.Subject, "valid_until", after.NotAfter.Format("2006-01-02"))
				s.sendSSLRenewedNotification(after.Subject, after.NotAfter)
				return nil
			}

			if daysLeft == 7 || daysLeft == 3 || daysLeft == 1 {
				s.sendSSLExpiringNotification(after.Subject, daysLeft, after.NotAfter)
			}

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

		// Update Check - notify-only check for a newer release
		// Per AI.md PART 18/22: check the release channel for a newer version
		// and notify admins. This task NEVER installs, even when auto_install
		// is set - actual updates always require explicit operator confirmation
		// via --maintenance update (auto-update without confirmation is forbidden).
		UpdateCheck: func(ctx context.Context) error {
			updateCfg := s.config.Server.Update
			// beta and daily channels both include pre-releases; stable does not
			includePrerelease := updateCfg.Branch == "beta" || updateCfg.Branch == "daily"

			mgr := update.NewManager()
			info, err := mgr.CheckForUpdates(includePrerelease)
			if err != nil {
				slog.Error("update check failed", "err", err)
				return err
			}
			if info == nil || !info.Available {
				slog.Info("update check complete", "current_version", info.CurrentVersion, "update_available", false)
				return nil
			}

			// Per AI.md PART 22: defer_days gates the scheduled task only - a
			// release is eligible once it has been public for defer_days days.
			// Manual `--update check`/`yes` bypass this window entirely.
			if updateCfg.DeferDays > 0 {
				eligibleAt := info.PublishedAt.Add(time.Duration(updateCfg.DeferDays) * 24 * time.Hour)
				if time.Now().UTC().Before(eligibleAt) {
					slog.Info("update check complete", "current_version", info.CurrentVersion,
						"update_available", false, "reason", "deferred", "latest_version", info.LatestVersion,
						"eligible_at", eligibleAt.Format("2006-01-02"))
					return nil
				}
			}

			slog.Warn("update available",
				"current_version", info.CurrentVersion,
				"latest_version", info.LatestVersion)
			// Per AI.md PART 22/features-rules.md: auto_install is read for
			// operator visibility only - the scheduled task never installs.
			if updateCfg.AutoInstall {
				slog.Debug("auto_install is set but update_check remains notify-only; run --maintenance update to install")
			}
			if s.mailer != nil && s.mailer.IsEnabled() {
				updateURL := s.config.Server.BaseURL + "/update"
				if err := s.mailer.SendUpdateAvailable(
					info.CurrentVersion,
					info.LatestVersion,
					info.PublishedAt.Format("2006-01-02"),
					info.ReleaseNotes,
					updateURL,
				); err != nil {
					slog.Error("failed to send update-available notification", "err", err)
				} else {
					slog.Info("update-available notification email sent", "latest_version", info.LatestVersion)
				}
			}
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
	if !tasks.UpdateCheck.Enabled {
		sched.Disable(scheduler.TaskUpdateCheck)
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

	// Per AI.md PART 17 "scheduler_error" Suppression: backup_failed and
	// ssl_renewal_failed each send their own dedicated failure email for the
	// same task execution (backup_failed here at the BackupDaily/BackupHourly
	// handlers via performScheduledBackup; the SSL renewal failure email is
	// sent inline from the SSLRenewal task handler), so the generic
	// scheduler_error notification is suppressed for those task IDs.
	switch scheduler.TaskID(notification.TaskID) {
	case scheduler.TaskBackupDaily, scheduler.TaskBackupHourly, scheduler.TaskSSLRenewal:
		return
	}

	// Send email notification if mailer is configured
	// Per AI.md PART 17 "Default Templates" -> scheduler_error.
	if s.mailer != nil && s.mailer.IsEnabled() {
		nextRun := ""
		if s.scheduler != nil {
			if info, err := s.scheduler.GetTask(scheduler.TaskID(notification.TaskID)); err == nil {
				nextRun = info.NextRun.Format(time.RFC3339)
			}
		}

		if err := s.mailer.SendSchedulerError(notification.TaskName, notification.Error, nextRun); err != nil {
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

	// Per AI.md PART 21: disk-space guard — abort before creating anything when
	// free space is under 2x the most recent backup's size, or usage exceeds
	// disk_threshold (default 90%)
	diskThreshold := s.config.Server.Backup.DiskThreshold
	if diskThreshold <= 0 {
		diskThreshold = 90
	}
	if ok, free, usedPct, err := mgr.CheckDiskSpace(diskThreshold); err != nil {
		slog.Warn("disk space check failed, proceeding with backup", "err", err)
	} else if !ok {
		slog.Warn("skipping scheduled backup, insufficient disk space", "type", backupType, "free_bytes", free, "used_percent", usedPct)
		s.logAuditEvent("backup.skipped_disk_full", fmt.Sprintf("%s backup skipped: free=%d bytes, used=%.1f%%", backupType, free, usedPct))
		return fmt.Errorf("insufficient disk space for backup: free=%d bytes, used=%.1f%%", free, usedPct)
	}

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

	ext := ".tar.gz"
	if encryptionEnabled && backupPassword != "" {
		ext = ".tar.gz.enc"
	}

	// Per AI.md PART 21: the daily job produces a timestamped full backup
	// (search_backup_YYYY-MM-DD<ext>, pruned by max_backups); the hourly job
	// produces only the fixed-name incremental below
	var backupPath string
	var verifyResult *backup.VerificationResult
	var err error

	if backupType == "daily" {
		fullFilename := fmt.Sprintf("search_backup_%s%s", time.Now().Format("2006-01-02"), ext)

		// Create the full backup with verification
		// Per AI.md PART 22: Only delete old backups if new backup passes ALL verification checks
		if encryptionEnabled && backupPassword != "" {
			backupPath, verifyResult, err = mgr.CreateEncryptedAndVerify(fullFilename)
		} else {
			backupPath, verifyResult, err = mgr.CreateAndVerify(fullFilename)
		}

		if err != nil {
			// Per AI.md PART 22: On failure, DO NOT delete any existing backups
			slog.Error("backup failed", "type", backupType, "err", err)
			s.logAuditEvent("backup.verification_failed", fmt.Sprintf("%s backup failed: %v", backupType, err))
			s.sendBackupFailedNotification(fullFilename, err)
			return err
		}

		if verifyResult != nil && verifyResult.AllPassed {
			slog.Info("backup created and verified", "type", backupType, "path", backupPath)
			s.logAuditEvent("backup.created", fmt.Sprintf("%s backup created: %s (verified: file=%v, size=%v, checksum=%v, manifest=%v)",
				backupType, backupPath, verifyResult.FileExists, verifyResult.SizeValid, verifyResult.ChecksumValid, verifyResult.ManifestValid))
			s.sendBackupCompletedNotification(backupPath)
		}
	}

	// Per AI.md PART 21: fixed-name incremental (always exactly one file,
	// replaced each run), excluded entirely from counted retention
	incrementalFilename := fmt.Sprintf("search-%s%s", backupType, ext)
	var incrementalPath string
	var incrementalVerify *backup.VerificationResult
	if encryptionEnabled && backupPassword != "" {
		incrementalPath, incrementalVerify, err = mgr.CreateEncryptedAndVerify(incrementalFilename)
	} else {
		incrementalPath, incrementalVerify, err = mgr.CreateAndVerify(incrementalFilename)
	}

	if err != nil {
		slog.Error("incremental backup failed", "type", backupType, "err", err)
		s.logAuditEvent("backup.verification_failed", fmt.Sprintf("%s incremental backup failed: %v", backupType, err))
		s.sendBackupFailedNotification(incrementalFilename, err)
		return err
	}

	if incrementalVerify != nil && incrementalVerify.AllPassed {
		slog.Info("incremental backup created and verified", "type", backupType, "path", incrementalPath)
		s.logAuditEvent("backup.daily_updated", fmt.Sprintf("%s incremental backup updated: %s", backupType, incrementalPath))
		s.sendBackupCompletedNotification(incrementalPath)
	}

	// Apply retention policy only after verification passes
	// Per AI.md PART 21: exclusive priority-ordered buckets plus a
	// max_total_size size-cap pass overriding count limits
	retention := s.config.Server.Backup.Retention
	policy := backup.RetentionPolicy{
		Count:        maxBackups,
		Week:         retention.KeepWeekly,
		Month:        retention.KeepMonthly,
		Year:         retention.KeepYearly,
		MaxTotalSize: retention.MaxTotalSize,
	}
	if err := mgr.ApplyRetention(policy); err != nil {
		slog.Warn("retention policy failed", "err", err)
	} else {
		s.logAuditEvent("backup.retention_cleanup", "Applied retention policy")
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

// sendBackupCompletedNotification notifies admins that a backup finished and
// was verified. Per AI.md PART 21 "Audit Events" (backup.created logs
// filename/size/verification status) and PART 17 "Default Templates" →
// backup_complete.
func (s *Server) sendBackupCompletedNotification(backupPath string) {
	if s.mailer == nil || !s.mailer.IsEnabled() {
		return
	}
	size := "unknown"
	if stat, err := os.Stat(backupPath); err == nil {
		size = backup.BackupInfo{Size: stat.Size()}.FormatSize()
	}
	if err := s.mailer.SendBackupCompleted(filepath.Base(backupPath), size); err != nil {
		slog.Error("failed to send backup completed notification email", "err", err)
	}
}

// sendBackupFailedNotification alerts admins that a backup failed. Per AI.md
// PART 21 "Backup Creation Flow" step 8 ("Alert operator via
// server.contact.admin notification channel") and PART 17 "Default
// Templates" → backup_failed. Per PART 17 "Suppression", this suppresses the
// generic scheduler_error email for the same backup task execution (handled
// in handleTaskFailureNotification).
func (s *Server) sendBackupFailedNotification(filename string, backupErr error) {
	if s.mailer == nil || !s.mailer.IsEnabled() {
		return
	}
	if err := s.mailer.SendBackupFailed(filename, backupErr.Error()); err != nil {
		slog.Error("failed to send backup failed notification email", "err", err)
	}
}

// sendSSLRenewedNotification notifies admins that a managed SSL certificate
// was successfully renewed. Per AI.md PART 17 "Default Templates" →
// ssl_renewed and PART 15 (SSL/TLS & Let's Encrypt).
func (s *Server) sendSSLRenewedNotification(domain string, validUntil time.Time) {
	if s.mailer == nil || !s.mailer.IsEnabled() {
		return
	}
	if err := s.mailer.SendSSLRenewed(domain, validUntil.Format("2006-01-02")); err != nil {
		slog.Error("failed to send SSL renewed notification email", "err", err)
	}
}

// sendSSLExpiringNotification warns admins that a managed SSL certificate is
// approaching expiry. Per AI.md PART 17 "Default Templates" → ssl_expiring:
// sent at 7, 3, and 1 days before expiry (PART 15, renewalThreshold).
func (s *Server) sendSSLExpiringNotification(domain string, daysLeft int, expiryDate time.Time) {
	if s.mailer == nil || !s.mailer.IsEnabled() {
		return
	}
	expiresIn := fmt.Sprintf("%d day", daysLeft)
	if daysLeft != 1 {
		expiresIn += "s"
	}
	if err := s.mailer.SendSSLExpiring(domain, expiresIn, expiryDate.Format("2006-01-02")); err != nil {
		slog.Error("failed to send SSL expiring notification email", "err", err)
	}
}

// sendSSLRenewalFailedNotification alerts admins that an SSL certificate
// renewal attempt failed. Per AI.md PART 17 "Default Templates" →
// ssl_renewal_failed and its Suppression rule (this suppresses the generic
// scheduler_error notification for the SSLRenewal task execution — see
// handleTaskFailureNotification). nextRetry is the next scheduled run of the
// SSLRenewal task (daily 03:00 per PART 18 Built-in Tasks).
func (s *Server) sendSSLRenewalFailedNotification(domain, errMsg string, daysLeft int, nextRetry time.Time) {
	if s.mailer == nil || !s.mailer.IsEnabled() {
		return
	}
	if err := s.mailer.SendSSLRenewalFailed(domain, errMsg, daysLeft, nextRetry.Format(time.RFC3339)); err != nil {
		slog.Error("failed to send SSL renewal failed notification email", "err", err)
	}
}
