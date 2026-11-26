package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	clientapi "github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/scheduler"
)

func runMaintenanceScheduler(action, taskID string) {
	if action == "" || action == "help" || action == "--help" {
		printMaintenanceSchedulerHelp()
		return
	}
	needsID := action == "show" || action == "run" || action == "enable" || action == "disable" || action == "history"
	if action != "list" && !needsID {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.scheduler_unknown_action", "action", action))
		printMaintenanceSchedulerHelp()
		exitFunc(2)
		return
	}
	if needsID != (taskID != "") {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.scheduler_task_id_required", "action", action))
		printMaintenanceSchedulerHelp()
		exitFunc(2)
		return
	}
	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	baseURL, err := maintenanceAPIURL(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.scheduler_api_url_failed", "error", err))
		exitFunc(78)
		return
	}
	client := clientapi.NewClient(baseURL, cfg.Server.Token, 15)
	switch action {
	case "list":
		tasks, requestErr := client.GetSchedulerTasks()
		err = requestErr
		if err == nil {
			fmt.Println(serverCLITf("server_cli.maintenance.scheduler_list_header", "task", serverCLIText("server_cli.maintenance.scheduler_task"), "state", serverCLIText("server_cli.maintenance.scheduler_state"), "enabled", serverCLIText("server_cli.maintenance.scheduler_enabled"), "last_run", serverCLIText("server_cli.maintenance.scheduler_last_run"), "schedule", serverCLIText("server_cli.maintenance.scheduler_schedule")))
			for _, task := range tasks {
				enabled, lastRun := serverCLIText("common.no"), serverCLIText("server_cli.maintenance.never")
				if task.Enabled {
					enabled = serverCLIText("common.yes")
				}
				if !task.LastRun.IsZero() {
					lastRun = task.LastRun.Local().Format("2006-01-02 15:04:05")
				}
			fmt.Println(serverCLITf("server_cli.maintenance.scheduler_list_row", "task", task.ID, "state", maintenanceSchedulerStatus(task.LastStatus), "enabled", enabled, "last_run", lastRun, "schedule", task.Schedule))
			}
		}
	case "show":
		var task *scheduler.TaskInfo
		task, err = client.GetSchedulerTask(taskID)
		if err == nil {
			fmt.Print(serverCLITf("server_cli.maintenance.scheduler_show", "task", task.ID, "type", task.TaskType, "name", task.Name, "description", task.Description, "schedule", task.Schedule, "enabled", yesNoCLI(task.Enabled), "skippable", yesNoCLI(task.Skippable), "state", maintenanceSchedulerStatus(task.LastStatus), "last_run", formatSchedulerTimeCLI(task.LastRun), "next_run", formatSchedulerTimeCLI(task.NextRun), "successful", task.RunCount, "failed", task.FailCount))
			if task.LastError != "" {
				fmt.Println(serverCLITf("server_cli.maintenance.scheduler_last_error", "error", task.LastError))
			}
		}
	case "history":
		var history []*scheduler.TaskHistoryEntry
		history, err = client.GetSchedulerHistory(taskID)
		if err == nil {
			if len(history) == 0 {
				fmt.Println(serverCLIText("server_cli.maintenance.scheduler_no_history"))
			}
			for _, entry := range history {
				fmt.Print(serverCLITf("server_cli.maintenance.scheduler_history_row", "date", entry.StartedAt.Local().Format("2006-01-02 15:04:05"), "state", maintenanceSchedulerStatus(entry.Status), "duration", entry.Duration))
				if entry.Error != "" {
					fmt.Print(serverCLITf("server_cli.maintenance.scheduler_history_error", "error", entry.Error))
				}
				fmt.Println()
			}
		}
	case "run":
		err = client.RunSchedulerTask(taskID)
		if err == nil {
			fmt.Println(serverCLITf("server_cli.maintenance.scheduler_ran", "task", taskID))
		}
	case "enable", "disable":
		info, getErr := client.GetSchedulerTask(taskID)
		err = getErr
		if err == nil && !info.Skippable && action == "disable" {
			err = errors.New(serverCLITf("server_cli.maintenance.scheduler_required_task", "task", taskID))
		}
		if err == nil {
			var taskConfig *config.TaskConfig
			taskConfig, err = schedulerTaskConfig(&cfg.Server.Scheduler.Tasks, taskID)
			if err == nil {
				oldEnabled := taskConfig.Enabled
				newEnabled := action == "enable"
				taskConfig.Enabled = newEnabled
				err = cfg.Save(config.GetConfigPath())
				if err == nil {
					event, details := schedulerTaskStateAudit(action, oldEnabled, newEnabled)
					logging.NewManager(config.GetLogDir()).Audit().LogEvent(
						event, logging.AuditCategorySystem, logging.AuditSeverityInfo,
						logging.AuditActor{Type: "operator", Username: pgpActor(), IP: pgpAuditIP},
						&logging.AuditTarget{Type: "scheduler_task", Name: taskID}, "success", details, "")
				}
			}
		}
		if err == nil {
			fmt.Println(serverCLITf("server_cli.maintenance.scheduler_task_changed", "task", taskID, "action", action))
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.scheduler_action_failed", "action", action, "error", err))
		exitCode := 1
		if errors.Is(err, clientapi.ErrConnection) {
			exitCode = 69
		} else if errors.Is(err, clientapi.ErrAuthentication) {
			exitCode = 77
		} else if errors.Is(err, clientapi.ErrNotFound) {
			exitCode = 66
		}
		exitFunc(exitCode)
	}
}

func schedulerTaskStateAudit(action string, oldEnabled, newEnabled bool) (logging.AuditAction, map[string]interface{}) {
	event := logging.AuditActionSchedulerTaskEnabled
	if action == "disable" {
		event = logging.AuditActionSchedulerTaskDisabled
	}
	return event, map[string]interface{}{
		"source":      "cli",
		"old_enabled": oldEnabled,
		"new_enabled": newEnabled,
	}
}

func formatSchedulerTimeCLI(value time.Time) string {
	if value.IsZero() {
		return serverCLIText("server_cli.maintenance.never")
	}
	return value.Local().Format("2006-01-02 15:04:05")
}

func maintenanceSchedulerStatus(status string) string {
	switch strings.ToLower(status) {
	case "success", "failed", "skipped", "running", "retrying":
		return serverCLIText("server_cli.maintenance.scheduler_status_" + strings.ToLower(status))
	case "":
		return serverCLIText("server_cli.maintenance.scheduler_status_unknown")
	default:
		return status
	}
}

func yesNoCLI(value bool) string {
	if value {
		return serverCLIText("common.yes")
	}
	return serverCLIText("common.no")
}

func schedulerTaskConfig(tasks *config.SchedulerTasksConfig, id string) (*config.TaskConfig, error) {
	if tasks == nil {
		return nil, fmt.Errorf("scheduler task configuration is unavailable")
	}
	switch id {
	case "backup_daily":
		return &tasks.BackupDaily, nil
	case "backup_hourly":
		return &tasks.BackupHourly, nil
	case "geoip_update":
		return &tasks.GeoIPUpdate, nil
	case "blocklist_update":
		return &tasks.BlocklistUpdate, nil
	case "cve_update":
		return &tasks.CVEUpdate, nil
	case "update_check":
		return &tasks.UpdateCheck, nil
	case "disk_check":
		return &tasks.DiskCheck, nil
	default:
		return nil, fmt.Errorf("task %q is not configurable as skippable in yaml.d/maintenance.yml", id)
	}
}

func maintenanceAPIURL(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("configuration is unavailable")
	}
	host := "127.0.0.1"
	scheme, port := "http", cfg.Server.Port
	if cfg.Server.HTTPSPort > 0 || cfg.Server.SSL.Enabled {
		scheme = "https"
		host = strings.TrimSpace(cfg.Server.FQDN)
		if host == "" {
			return "", fmt.Errorf("server.fqdn is required for verified HTTPS maintenance access")
		}
		if strings.Contains(host, "://") {
			parsed, err := url.Parse(host)
			if err != nil || parsed.Hostname() == "" {
				return "", fmt.Errorf("invalid server FQDN")
			}
			host = parsed.Hostname()
		}
		if cfg.Server.HTTPSPort > 0 {
			port = cfg.Server.HTTPSPort
		}
	}
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("configured server port is invalid")
	}
	base := scheme + "://" + net.JoinHostPort(strings.Trim(host, "[]"), fmt.Sprint(port))
	basePath := strings.TrimRight(cfg.Server.BaseURL, "/")
	if basePath != "" {
		parsed, err := url.Parse(basePath)
		if err != nil || parsed.IsAbs() || strings.Contains(basePath, "..") {
			return "", fmt.Errorf("configured server baseurl is invalid")
		}
		base += "/" + strings.TrimLeft(basePath, "/")
	}
	return base, nil
}

func printMaintenanceSchedulerHelp() {
	fmt.Println(serverCLIText("server_cli.maintenance.scheduler_help_list"))
	fmt.Println(serverCLIText("server_cli.maintenance.scheduler_help_show"))
	fmt.Println(serverCLIText("server_cli.maintenance.scheduler_help_run"))
	fmt.Println(serverCLIText("server_cli.maintenance.scheduler_help_enable"))
	fmt.Println(serverCLIText("server_cli.maintenance.scheduler_help_disable"))
	fmt.Println(serverCLIText("server_cli.maintenance.scheduler_help_history"))
}
