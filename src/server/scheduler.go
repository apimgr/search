package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/notify"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/security"
	sigsvc "github.com/apimgr/search/src/signal"
	"github.com/apimgr/search/src/update"
)

// updateNotifyTracker remembers the release the update_check task last
// surfaced to the operator
type updateNotifyTracker struct {
	mu   sync.Mutex
	seen string
}

// markSeen records version as surfaced and reports whether this is the first
// time it has been seen. Per AI.md PART 22 the WARN log line and the
// update_available email fire once per version, when a new eligible version is
// first seen, and are not re-sent on every task run.
func (t *updateNotifyTracker) markSeen(version string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seen == version {
		return false
	}
	t.seen = version
	return true
}

// updateNotifyState is the process-wide fire-once guard for update
// notifications
var updateNotifyState updateNotifyTracker

func updateManualCommand(goos string, startedElevated bool) string {
	command := config.ProjectName + " --update yes"
	if goos == "windows" {
		return command + " (run from an elevated prompt)"
	}
	if !startedElevated {
		return "sudo " + command
	}
	return command
}

func updateVersionHasFailed(entries []logging.AuditEntry, version string) bool {
	for _, entry := range entries {
		if entry.Event != logging.AuditAction("update.failed") || entry.Result != "failure" {
			continue
		}
		if failedVersion, ok := entry.Details["version"].(string); ok && failedVersion == version {
			return true
		}
	}
	return false
}

// installScheduledUpdate runs the full `--update yes` flow for a release the
// update_check task found eligible, as AI.md PART 22 requires when
// server.update.auto_install is true
func (s *Server) installScheduledUpdate(mgr *update.Manager, info *update.UpdateInfo) error {
	if s.logManager != nil && s.logManager.Audit() != nil {
		result, err := s.logManager.Audit().QueryAuditLogs(logging.AuditQueryOptions{
			Event: logging.AuditAction("update.failed"),
		})
		if err != nil {
			return fmt.Errorf("check failed update versions: %w", err)
		}
		if result != nil && updateVersionHasFailed(result.Entries, info.LatestVersion) {
			slog.Warn("scheduled update version previously failed; waiting for a new release", "latest_version", info.LatestVersion)
			return nil
		}
	}
	if runtime.GOOS == "windows" || !config.StartedElevated() {
		manualCommand := updateManualCommand(runtime.GOOS, config.StartedElevated())
		slog.Warn("scheduled update not installed because the process did not start elevated", "latest_version", info.LatestVersion, "manual_command", manualCommand)
		if s.logManager != nil && s.logManager.Audit() != nil {
			s.logManager.Audit().LogEvent(logging.AuditAction("update.install_skipped"), logging.AuditCategorySystem, logging.AuditSeverityInfo, logging.AuditActor{Type: "system", Username: "system"}, nil, "skipped", map[string]interface{}{"reason": "not_elevated", "version": info.LatestVersion}, "")
		}
		return nil
	}
	if info.DownloadURL == "" {
		err := fmt.Errorf("release %s has no binary for this platform", info.LatestVersion)
		slog.Error("auto-install skipped", "latest_version", info.LatestVersion, "err", err)
		s.recordUpdateFailure(info.LatestVersion, err)
		return err
	}

	slog.Info("auto-installing update", "latest_version", info.LatestVersion)

	downloadPath, err := mgr.DownloadUpdate(info.DownloadURL, nil)
	if err != nil {
		slog.Error("auto-install download failed", "latest_version", info.LatestVersion, "err", err)
		s.recordUpdateFailure(info.LatestVersion, err)
		return err
	}

	if err := mgr.StageUpdate(downloadPath, info.AssetName, info.ChecksumURL, info.LatestVersion, info.PublishedAt); err != nil {
		slog.Error("auto-install staging failed", "latest_version", info.LatestVersion, "err", err)
		s.recordUpdateFailure(info.LatestVersion, err)
		return err
	}
	if s.logManager != nil && s.logManager.Audit() != nil {
		s.logManager.Audit().LogEvent(logging.AuditAction("update.staged"), logging.AuditCategorySystem, logging.AuditSeverityInfo, logging.AuditActor{Type: "system", Username: "system"}, nil, "success", map[string]interface{}{"version": info.LatestVersion}, "")
	}
	sigsvc.SetShutdownExitCode(75)
	s.SetShutdownReason("update restart")
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		sigsvc.SetShutdownExitCode(0)
		s.recordUpdateFailure(info.LatestVersion, err)
		return fmt.Errorf("find process for staged update restart: %w", err)
	}
	if err := process.Signal(os.Interrupt); err != nil {
		sigsvc.SetShutdownExitCode(0)
		s.SetShutdownReason("signal")
		s.recordUpdateFailure(info.LatestVersion, err)
		return fmt.Errorf("request graceful shutdown for staged update: %w", err)
	}
	return nil
}

func (s *Server) recordUpdateFailure(version string, cause error) {
	if s.logManager != nil && s.logManager.Audit() != nil {
		s.logManager.Audit().LogEvent(logging.AuditAction("update.failed"), logging.AuditCategorySystem, logging.AuditSeverityError, logging.AuditActor{Type: "system", Username: "system"}, nil, "failure", map[string]interface{}{"version": version}, cause.Error())
	}
	if s.mailer != nil {
		if err := s.mailer.SendUpdateFailed(version, cause.Error()); err != nil {
			slog.Error("failed to send update-failed notification", "latest_version", version, "err", err)
		}
	}
}

// initScheduler initializes and starts the scheduler per AI.md PART 19
// The scheduler is ALWAYS RUNNING - there is no enable/disable option
func (s *Server) initScheduler(db *database.DB) {
	// Standalone node ID (single-node mode)
	nodeID := "standalone"
	sched := scheduler.NewSchedulerWithDB(db, nodeID)

	// Configure timezone
	if tz := s.config.Server.Scheduler.Timezone; tz != "" {
		if err := sched.SetTimezone(tz); err != nil {
			slog.Warn("invalid scheduler timezone, using default", "config_key", "server.scheduler.timezone", "timezone", tz, "default", scheduler.DefaultTimezoneName(), "err", err)
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
	if s.currentHealthRegistry() != nil {
		if err := sched.Register(&scheduler.Task{
			ID:          scheduler.TaskID("health_registry_refresh"),
			Name:        "Refresh Health Registry",
			Description: "Refresh cached dependency checks and aggregate statistics",
			Schedule:    "@every 60s",
			TaskType:    scheduler.TaskTypeLocal,
			Run: func(ctx context.Context) error {
				s.refreshHealthRegistry()
				if s.metrics != nil {
					return s.metrics.PersistRequestStats(ctx)
				}
				return nil
			},
			Skippable:  false,
			RunOnStart: true,
			Enabled:    true,
		}); err != nil {
			slog.Error("health registry refresh task registration failed", "err", err)
		}
	}

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
	handlers := &scheduler.TaskHandlers{
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
		SecurityCleanup: func(ctx context.Context) error {
			return s.cleanupSecurityData(ctx)
		},
		KeyserverRetry: func(ctx context.Context) error {
			if len(s.config.Server.Web.Security.Keyservers) == 0 {
				return nil
			}
			pub, err := security.LoadPublicKey(config.GetConfigDir())
			if err != nil {
				return nil
			}
			results, err := security.PublishKeyserversOnce(ctx, nil, config.GetConfigDir(), pub, s.config.Server.Web.Security.Keyservers, time.Now().UTC())
			for _, result := range results {
				if result.Err != nil {
					slog.Warn("PGP keyserver publication retry failed", "keyserver", result.Keyserver, "err", result.Err)
				}
			}
			return err
		},
		DiskCheckSchedule:     diskCheckSchedule(s.config.Server.Notifications.DiskLow.CheckInterval),
		DatabaseCheckSchedule: databaseCheckSchedule(s.config.Server.Notifications.DatabaseError.CheckInterval),
		DatabaseCheck: func(ctx context.Context) error {
			if !s.config.Server.Notifications.DatabaseError.Enabled || s.dbManager == nil {
				return nil
			}
			probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			err := s.dbManager.Probe(probeCtx)
			s.reportDatabaseStatus(err)
			return err
		},

		// GeoIP Update - download ip-location-db databases
		GeoIPUpdate: func(ctx context.Context) error {
			defer s.refreshHealthRegistry()
			if !s.config.Server.GeoIP.Enabled {
				// GeoIP is disabled; skip update
				return nil
			}
			if s.geoipLookup == nil {
				return nil
			}
			if err := s.geoipLookup.UpdateDatabases(); err != nil {
				slog.Error("GeoIP database update failed", "err", err)
				return err
			}
			slog.Info("GeoIP database update complete")
			return nil
		},

		// Blocklist Update - download IP/domain blocklists
		// Per AI.md PART 18: blocklist_update runs daily at 04:00
		BlocklistUpdate: func(ctx context.Context) error {
			defer s.refreshHealthRegistry()
			if s.blocklistManager == nil {
				return nil
			}
			if err := s.blocklistManager.Update(ctx); err != nil {
				slog.Error("blocklist update failed", "err", err)
				return err
			}
			ips, nets := s.blocklistManager.Count()
			slog.Info("blocklist update complete", "blocked_ips", ips, "blocked_nets", nets, "blocked_domains", s.blocklistManager.DomainCount())
			return nil
		},

		// CVE Update - download security databases (optional feature)
		CVEUpdate: func(ctx context.Context) error {
			defer s.refreshHealthRegistry()
			if s.cveManager == nil {
				return nil
			}
			var updateErrors []error
			if err := s.cveManager.Update(ctx); err != nil {
				slog.Error("CVE update failed", "err", err)
				updateErrors = append(updateErrors, err)
			} else {
				slog.Info("CVE update complete", "entries", s.cveManager.Count())
			}
			trivy := s.config.Data.Trivy
			if trivy.Enabled {
				source := strings.TrimSpace(trivy.Source)
				if source == "" {
					source = "ghcr.io/aquasecurity/trivy-db:2"
				}
				if err := s.cveManager.UpdateTrivyDB(ctx, source); err != nil {
					slog.Error("Trivy database update failed", "err", err)
					updateErrors = append(updateErrors, err)
				}
			}
			return errors.Join(updateErrors...)
		},

		// Update Check - check the configured release channel for a newer version
		// Per AI.md PART 18/22: runs the equivalent of `--update check` against
		// the configured channel, filtered by defer_days. With auto_install
		// false (the default) it only notifies; with auto_install true it runs
		// the full `--update yes` flow for eligible releases.
		UpdateCheck: func(ctx context.Context) error {
			updateCfg := s.config.Server.Update
			branch := update.NormalizeBranch(updateCfg.Branch)

			mgr := update.NewManager()

			// Per AI.md PART 22: defer_days gates the scheduled task only - the
			// newest ELIGIBLE release wins, so a brand-new release never resets
			// the clock for an older release that has already aged past the
			// window. Manual `--update check`/`yes` bypass this entirely.
			info, err := mgr.CheckForUpdatesDeferred(branch, updateCfg.DeferDays)
			if err != nil {
				slog.Error("update check failed", "err", err)
				return err
			}
			if info == nil || !info.Available {
				slog.Info("update check complete", "channel", branch,
					"current_version", mgr.GetCurrentVersion(), "update_available", false)
				return nil
			}

			// Per AI.md PART 22 surfacing rules: the WARN log and the
			// update_available email fire once per version, not on every run
			if updateNotifyState.markSeen(info.LatestVersion) {
				slog.Warn("update available",
					"channel", branch,
					"current_version", info.CurrentVersion,
					"latest_version", info.LatestVersion)

				if s.mailer != nil && s.mailer.IsEnabled() {
					updateURL := s.config.Server.BaseURL + "/update"
					manualCommand := updateManualCommand(runtime.GOOS, config.StartedElevated())
					if err := s.mailer.SendUpdateAvailable(
						info.CurrentVersion,
						info.LatestVersion,
						info.PublishedAt.Format("2006-01-02"),
						info.ReleaseNotes,
						updateURL,
						manualCommand,
					); err != nil {
						slog.Error("failed to send update-available notification", "err", err)
					} else {
						slog.Info("update-available notification email sent", "latest_version", info.LatestVersion)
					}
				}
			}

			if !updateCfg.AutoInstall {
				return nil
			}
			return s.installScheduledUpdate(mgr, info)
		},

		// Token Cleanup - remove expired tokens
		TokenCleanup: func(ctx context.Context) error {
			if s.dbManager == nil || s.dbManager.ServerDB() == nil {
				return errors.New("server database is unavailable for token cleanup")
			}
			removed, err := s.dbManager.ServerDB().CleanupExpiredAPITokens(ctx)
			if err != nil {
				return err
			}
			slog.Info("expired API token cleanup complete", "removed", removed)
			return nil
		},

		// Log Rotation - rotate all active log files using the built-in logger.
		LogRotation: func(ctx context.Context) error {
			if s.logManager == nil {
				return errors.New("log manager is unavailable for log rotation")
			}
			if err := s.logManager.RotateAll(); err != nil {
				return err
			}
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
	if s.config.Server.Notifications.DiskLow.Enabled {
		handlers.DiskCheck = s.checkDiskLow
	}
	if s.i2pManager != nil && s.i2pManager.Enabled() {
		handlers.I2PHealth = func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return s.i2pManager.CheckAndRestart()
		}
	}
	return handlers
}

func diskCheckSchedule(interval string) string {
	if strings.TrimSpace(interval) == "" {
		return "@every 5m"
	}
	duration, err := time.ParseDuration(strings.TrimSpace(interval))
	if err != nil || duration <= 0 {
		slog.Warn("invalid disk-low check interval, using default", "config_key", "server.notifications.disk_low.check_interval", "value", interval, "default", "5m")
		return "@every 5m"
	}
	return "@every " + duration.String()
}

func databaseCheckSchedule(interval string) string {
	if strings.TrimSpace(interval) == "" {
		return "@every 1m"
	}
	duration, err := time.ParseDuration(strings.TrimSpace(interval))
	if err != nil || duration <= 0 {
		slog.Warn("invalid database check interval, using default", "config_key", "server.notifications.database_error.check_interval", "value", interval, "default", "1m")
		return "@every 1m"
	}
	return "@every " + duration.String()
}

type diskLowState struct {
	lastNotification time.Time
}

func (s *Server) checkDiskLow(ctx context.Context) error {
	if s.config == nil || !s.config.Server.Notifications.DiskLow.Enabled {
		return nil
	}
	cfg := s.config.Server.Notifications.DiskLow
	threshold := cfg.ThresholdPercent
	if threshold < 1 || threshold > 100 {
		threshold = 85
	}
	repeat := 24 * time.Hour
	if value := strings.TrimSpace(cfg.Repeat); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			slog.Warn("invalid disk-low repeat interval, using default", "config_key", "server.notifications.disk_low.repeat", "value", value, "default", repeat.String())
		} else {
			repeat = parsed
		}
	}
	paths := []string{config.GetDataDir(), config.GetLogDir(), config.GetBackupDir()}
	seen := make(map[string]struct{}, len(paths))
	now := time.Now()
	var errs []error
	for _, path := range paths {
		select {
		case <-ctx.Done():
			return errors.Join(append(errs, ctx.Err())...)
		default:
		}
		if strings.TrimSpace(path) == "" {
			continue
		}
		volumePath, err := existingDiskPath(path)
		if err != nil {
			slog.Warn("disk space volume could not be resolved", "path", path, "err", err)
			continue
		}
		volume, err := diskVolumeKey(volumePath)
		if err != nil {
			slog.Warn("disk space volume could not be identified", "path", path, "err", err)
			continue
		}
		if _, exists := seen[volume]; exists {
			continue
		}
		seen[volume] = struct{}{}
		used, total, err := backup.DiskUsage(volumePath)
		if err != nil {
			slog.Warn("disk space check failed", "path", path, "err", err)
			continue
		}
		if total == 0 {
			continue
		}
		usedPercent := float64(used) * 100 / float64(total)
		freeBytes := total - used
		if usedPercent < float64(threshold-5) {
			s.diskLowMu.Lock()
			if s.diskLowState != nil {
				if _, exists := s.diskLowState[volume]; exists {
					delete(s.diskLowState, volume)
					s.diskLowMu.Unlock()
					slog.Info("Disk space recovered", "path", path, "used_percent", usedPercent)
					continue
				}
			}
			s.diskLowMu.Unlock()
			continue
		}
		if usedPercent < float64(threshold) {
			continue
		}

		s.diskLowMu.Lock()
		if s.diskLowState == nil {
			s.diskLowState = make(map[string]diskLowState)
		}
		state, notified := s.diskLowState[volume]
		if notified && now.Sub(state.lastNotification) < repeat {
			s.diskLowMu.Unlock()
			continue
		}
		s.diskLowState[volume] = diskLowState{lastNotification: now}
		s.diskLowMu.Unlock()

		slog.Warn("Disk space low", "path", path, "used_percent", usedPercent, "free_bytes", freeBytes)
		if s.mailer != nil && s.mailer.CanSend() {
			if err := s.mailer.SendDiskLow(path, usedPercent, formatDiskBytes(freeBytes), threshold); err != nil {
				slog.Error("disk space low notification email failed", "path", path, "err", err)
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func formatDiskBytes(bytes uint64) string {
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	value := float64(bytes)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value /= 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func (s *Server) cleanupSecurityData(ctx context.Context) error {
	if s.dbManager == nil || s.dbManager.ServerDB() == nil {
		return fmt.Errorf("security cleanup database is unavailable")
	}
	retentionDays := s.config.Server.Security.Reports.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 365
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	deleted, err := security.DeleteClosedReportsBefore(ctx, s.dbManager.ServerDB(), cutoff)
	if err != nil {
		return err
	}
	if deleted > 0 {
		slog.Info("expired closed security reports removed", "count", deleted)
	}
	if retired, err := security.CleanupExpiredPreviousEncryptionKey(ctx, s.dbManager.ServerDB(), s.config.Server.Security.EncryptionKey, time.Now().UTC()); err != nil {
		return fmt.Errorf("retire previous encryption key: %w", err)
	} else if retired {
		slog.Info("expired previous encryption key retired")
	}

	pgpDir := filepath.Join(config.GetConfigDir(), security.PGPDirName)
	expiryPath := filepath.Join(pgpDir, "pgp.old.expires")
	expiryBytes, err := os.ReadFile(expiryPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read retired PGP key expiry: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(string(expiryBytes)))
	if err != nil {
		return fmt.Errorf("parse retired PGP key expiry: %w", err)
	}
	if time.Now().Before(expiresAt) {
		return nil
	}
	privateKeyPath := filepath.Join(pgpDir, "pgp.priv.old.asc.enc")
	if err := os.Remove(privateKeyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove retired PGP private key: %w", err)
	}
	if err := os.Remove(expiryPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove retired PGP key expiry: %w", err)
	}
	slog.Info("retired PGP private key removed after grace period")
	return nil
}

// applyTaskConfig applies user configuration to skippable tasks
func (s *Server) applyTaskConfig(sched *scheduler.Scheduler) {
	if sched == nil {
		return
	}
	tasks := s.config.Server.Scheduler.Tasks
	applySettings := func(id scheduler.TaskID, taskConfig config.TaskConfig) {
		if taskConfig.Schedule != "" {
			if err := sched.SetTaskSchedule(id, taskConfig.Schedule); err != nil {
				slog.Warn("failed to set scheduler task schedule", "task", id, "error", err)
			}
		}
		if err := sched.SetTaskRetryEnabled(id, taskConfig.RetryOnFail); err != nil {
			slog.Warn("failed to set scheduler task retry policy", "task", id, "error", err)
		}
		if taskConfig.RetryDelay != "" {
			delay, err := time.ParseDuration(taskConfig.RetryDelay)
			if err != nil {
				slog.Warn("invalid scheduler task retry delay", "task", id, "delay", taskConfig.RetryDelay, "error", err)
			} else if err := sched.SetTaskRetry(id, 0, delay); err != nil {
				slog.Warn("failed to set scheduler task retry delay", "task", id, "error", err)
			}
		}
	}

	// Apply config for skippable tasks only
	backupDaily := tasks.BackupDaily
	if tasks.Backup.Schedule != "" || tasks.Backup.Enabled || tasks.Backup.RetryOnFail || tasks.Backup.RetryDelay != "" || tasks.Backup.Retention > 0 {
		backupDaily = tasks.Backup
	}
	applySettings(scheduler.TaskBackupDaily, backupDaily)
	applySettings(scheduler.TaskBackupHourly, tasks.BackupHourly)
	applySettings(scheduler.TaskSSLRenewal, tasks.SSLRenewal)
	applySettings(scheduler.TaskGeoIPUpdate, tasks.GeoIPUpdate)
	applySettings(scheduler.TaskBlocklistUpdate, tasks.BlocklistUpdate)
	applySettings(scheduler.TaskCVEUpdate, tasks.CVEUpdate)
	applySettings(scheduler.TaskUpdateCheck, tasks.UpdateCheck)
	applySettings(scheduler.TaskDiskCheck, tasks.DiskCheck)
	applySettings(scheduler.TaskDatabaseCheck, tasks.DatabaseCheck)
	applySettings(scheduler.TaskTokenCleanup, tasks.TokenCleanup)
	applySettings(scheduler.TaskLogRotation, tasks.LogRotation)
	applySettings(scheduler.TaskHealthcheckSelf, tasks.HealthcheckSelf)
	applySettings(scheduler.TaskTorHealth, tasks.TorHealth)
	if _, err := sched.GetTask(scheduler.TaskI2PHealth); err == nil {
		applySettings(scheduler.TaskI2PHealth, tasks.I2PHealth)
	}
	applySettings(scheduler.TaskAlertsImmediate, tasks.AlertsImmediate)
	applySettings(scheduler.TaskAlertsDaily, tasks.AlertsDaily)
	applySettings(scheduler.TaskAlertsWeekly, tasks.AlertsWeekly)
	if backupDaily.Retention > 0 {
		s.config.Server.Backup.Retention.MaxBackups = backupDaily.Retention
	}
	if !tasks.BackupDaily.Enabled {
		if err := sched.Disable(scheduler.TaskBackupDaily); err != nil {
			slog.Warn("failed to disable daily backup task", "error", err)
		}
	}
	if tasks.BackupHourly.Enabled {
		if err := sched.Enable(scheduler.TaskBackupHourly); err != nil {
			slog.Warn("failed to enable hourly backup task", "error", err)
		}
	}
	if !tasks.GeoIPUpdate.Enabled {
		if err := sched.Disable(scheduler.TaskGeoIPUpdate); err != nil {
			slog.Warn("failed to disable GeoIP update task", "error", err)
		}
	}
	if !tasks.BlocklistUpdate.Enabled {
		if err := sched.Disable(scheduler.TaskBlocklistUpdate); err != nil {
			slog.Warn("failed to disable blocklist update task", "error", err)
		}
	}
	if !tasks.CVEUpdate.Enabled {
		if err := sched.Disable(scheduler.TaskCVEUpdate); err != nil {
			slog.Warn("failed to disable CVE update task", "error", err)
		}
	}
	if !tasks.UpdateCheck.Enabled {
		if err := sched.Disable(scheduler.TaskUpdateCheck); err != nil {
			slog.Warn("failed to disable update check task", "error", err)
		}
	}
	if !tasks.DiskCheck.Enabled || !s.config.Server.Notifications.DiskLow.Enabled {
		if err := sched.Disable(scheduler.TaskDiskCheck); err != nil {
			slog.Warn("failed to disable disk check task", "error", err)
		}
	}
	if !tasks.DatabaseCheck.Enabled || !s.config.Server.Notifications.DatabaseError.Enabled {
		if err := sched.Disable(scheduler.TaskDatabaseCheck); err != nil {
			slog.Warn("failed to disable database check task", "error", err)
		}
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

// StartBlocklistUpdateIfMissing queues the first external-data refresh after
// the listener starts, without delaying server startup when feeds are down.
func (s *Server) StartBlocklistUpdateIfMissing() {
	if s.blocklistManager == nil || !s.blocklistManager.NeedsInitialUpdate() {
		return
	}
	if err := s.RunSchedulerTask(string(scheduler.TaskBlocklistUpdate)); err != nil {
		slog.Warn("initial blocklist update could not be scheduled", "error", err)
	}
}

// StartCVEUpdateIfMissing queues the first CVE data refresh without delaying
// server startup when a configured source is unavailable.
func (s *Server) StartCVEUpdateIfMissing() {
	if s.cveManager == nil {
		return
	}
	trivyNeedsUpdate := s.config.Data.Trivy.Enabled && s.cveManager.TrivyHealthStatus() != "ok"
	if !s.cveManager.NeedsInitialUpdate() && !trivyNeedsUpdate {
		return
	}
	if err := s.RunSchedulerTask(string(scheduler.TaskCVEUpdate)); err != nil {
		slog.Warn("initial CVE update could not be scheduled", "error", err)
	}
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
	s.dispatchNotification(notify.Event{Role: "admin", Name: "scheduler.task_failed", Subject: "Scheduled task failed", Body: "Task " + notification.TaskName + " failed after " + fmt.Sprint(notification.Attempts) + " attempt(s).", Severity: "error"})

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
	if s.dbManager != nil && s.dbManager.ServerDB() != nil {
		mgr.SetServerDatabaseSnapshotter(s.dbManager.ServerDB())
	}
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
	complianceEnabled := s.config.Server.Compliance.AnyEnabled()
	backupPassword := s.config.Server.Backup.EncryptionPassword
	encryptionEnabled := backupPassword != ""

	if complianceEnabled {
		if backupPassword == "" {
			// Per AI.md PART 22: Scheduled backups skip with audit log warning
			slog.Warn("compliance mode enabled but server.backup.encryption_password not set, backup skipped")
			s.logAuditEvent("backup.skipped", "Compliance mode requires backup encryption but password not set")
			return fmt.Errorf("compliance mode requires backup encryption; set server.backup.encryption_password in secrets.yml")
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

	// Daily artifacts are derived from one in-memory snapshot. The incremental
	// is written against the previous full before the new full is written.
	snapshotAt := time.Now().UTC()
	if backupType == "daily" {
		incrementalFilename := fmt.Sprintf("search-daily%s", ext)
		fullFilename := fmt.Sprintf("search_backup_%s%s", snapshotAt.Format("2006-01-02"), ext)
		incrementalPath, fullPath, incrementalVerify, fullVerify, skippedIncremental, err := mgr.CreateDailyPairAndVerify(incrementalFilename, fullFilename, snapshotAt)
		if err != nil {
			slog.Error("backup failed", "type", backupType, "err", err)
			s.logAuditEvent("backup.verification_failed", fmt.Sprintf("%s backup failed: %v", backupType, err))
			s.sendBackupFailedNotification(fullFilename, err)
			return err
		}
		if skippedIncremental {
			slog.Info("skipping first daily incremental because no full backup exists")
		}
		if incrementalVerify != nil && incrementalVerify.AllPassed {
			slog.Info("incremental backup created and verified", "type", backupType, "path", incrementalPath)
			s.logAuditEvent("backup.daily_updated", fmt.Sprintf("%s incremental backup updated: %s", backupType, incrementalPath))
			s.sendBackupCompletedNotification(incrementalPath)
		}
		if fullVerify != nil && fullVerify.AllPassed {
			slog.Info("backup created and verified", "type", backupType, "path", fullPath)
			s.logAuditEvent("backup.created", fmt.Sprintf("%s backup created: %s (verified: file=%v, size=%v, checksum=%v, manifest=%v)", backupType, fullPath, fullVerify.FileExists, fullVerify.SizeValid, fullVerify.ChecksumValid, fullVerify.ManifestValid))
			s.sendBackupCompletedNotification(fullPath)
		}
	} else {
		incrementalFilename := fmt.Sprintf("search-hourly%s", ext)
		var incrementalPath string
		var incrementalVerify *backup.VerificationResult
		var err error
		if encryptionEnabled && backupPassword != "" {
			incrementalPath, incrementalVerify, err = mgr.CreateEncryptedIncrementalAndVerify(incrementalFilename, snapshotAt)
		} else {
			incrementalPath, incrementalVerify, err = mgr.CreateIncrementalAndVerify(incrementalFilename, snapshotAt)
		}
		if err != nil {
			if errors.Is(err, backup.ErrNoFullBackup) {
				slog.Info("skipping hourly incremental because no full backup exists")
				return nil
			}
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
	size := "unknown"
	if stat, err := os.Stat(backupPath); err == nil {
		size = backup.BackupInfo{Size: stat.Size()}.FormatSize()
	}
	if s.mailer != nil && s.mailer.IsEnabled() {
		if err := s.mailer.SendBackupCompleted(filepath.Base(backupPath), size); err != nil {
			slog.Error("failed to send backup completed notification email", "err", err)
		}
	}
	s.dispatchNotification(notify.Event{Role: "admin", Name: "backup.completed", Subject: "Backup completed", Body: fmt.Sprintf("Backup %s completed and verified (%s).", filepath.Base(backupPath), size), Severity: "info"})
}

// sendBackupFailedNotification alerts admins that a backup failed. Per AI.md
// PART 21 "Backup Creation Flow" step 8 ("Alert operator via
// server.contact.admin notification channel") and PART 17 "Default
// Templates" → backup_failed. Per PART 17 "Suppression", this suppresses the
// generic scheduler_error email for the same backup task execution (handled
// in handleTaskFailureNotification).
func (s *Server) sendBackupFailedNotification(filename string, backupErr error) {
	if s.mailer != nil && s.mailer.IsEnabled() {
		if err := s.mailer.SendBackupFailed(filename, backupErr.Error()); err != nil {
			slog.Error("failed to send backup failed notification email", "err", err)
		}
	}
	s.dispatchNotification(notify.Event{Role: "admin", Name: "backup.failed", Subject: "Backup failed", Body: "Backup " + filepath.Base(filename) + " failed.", Severity: "error"})
}

// sendSSLRenewedNotification notifies admins that a managed SSL certificate
// was successfully renewed. Per AI.md PART 17 "Default Templates" →
// ssl_renewed and PART 15 (SSL/TLS & Let's Encrypt).
func (s *Server) sendSSLRenewedNotification(domain string, validUntil time.Time) {
	if s.mailer != nil && s.mailer.IsEnabled() {
		if err := s.mailer.SendSSLRenewed(domain, validUntil.Format("2006-01-02")); err != nil {
			slog.Error("failed to send SSL renewed notification email", "err", err)
		}
	}
	s.dispatchNotification(notify.Event{Role: "admin", Name: "ssl.renewed", Subject: "TLS certificate renewed", Body: fmt.Sprintf("The TLS certificate for %s was renewed and is valid until %s.", domain, validUntil.Format("2006-01-02")), Severity: "info"})
}

// sendSSLExpiringNotification warns admins that a managed SSL certificate is
// approaching expiry. Per AI.md PART 17 "Default Templates" → ssl_expiring:
// sent at 7, 3, and 1 days before expiry (PART 15, renewalThreshold).
func (s *Server) sendSSLExpiringNotification(domain string, daysLeft int, expiryDate time.Time) {
	expiresIn := fmt.Sprintf("%d day", daysLeft)
	if daysLeft != 1 {
		expiresIn += "s"
	}
	if s.mailer != nil && s.mailer.IsEnabled() {
		if err := s.mailer.SendSSLExpiring(domain, expiresIn, expiryDate.Format("2006-01-02")); err != nil {
			slog.Error("failed to send SSL expiring notification email", "err", err)
		}
	}
	s.dispatchNotification(notify.Event{Role: "admin", Name: "ssl.expiring", Subject: "TLS certificate expiring", Body: fmt.Sprintf("The TLS certificate for %s expires in %s (%s).", domain, expiresIn, expiryDate.Format("2006-01-02")), Severity: "warning"})
}

// sendSSLRenewalFailedNotification alerts admins that an SSL certificate
// renewal attempt failed. Per AI.md PART 17 "Default Templates" →
// ssl_renewal_failed and its Suppression rule (this suppresses the generic
// scheduler_error notification for the SSLRenewal task execution — see
// handleTaskFailureNotification). nextRetry is the next scheduled run of the
// SSLRenewal task (daily 03:00 per PART 18 Built-in Tasks).
func (s *Server) sendSSLRenewalFailedNotification(domain, errMsg string, daysLeft int, nextRetry time.Time) {
	if s.mailer != nil && s.mailer.IsEnabled() {
		if err := s.mailer.SendSSLRenewalFailed(domain, errMsg, daysLeft, nextRetry.Format(time.RFC3339)); err != nil {
			slog.Error("failed to send SSL renewal failed notification email", "err", err)
		}
	}
	s.dispatchNotification(notify.Event{Role: "admin", Name: "ssl.renewal_failed", Subject: "TLS certificate renewal failed", Body: fmt.Sprintf("The TLS certificate for %s could not be renewed. %d day(s) remain; retry scheduled for %s.", domain, daysLeft, nextRetry.Format(time.RFC3339)), Severity: "error"})
}
