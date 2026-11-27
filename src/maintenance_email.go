package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/logging"
)

func runMaintenanceEmail(action string, args []string) {
	if action == "" || action == "help" || action == "--help" {
		printMaintenanceEmailHelp()
		return
	}
	if action != "test" {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.unknown_email_action", "action", action))
		printMaintenanceEmailHelpTo(os.Stderr)
		exitFunc(2)
		return
	}
	dryRun, to, err := parseMaintenanceEmailArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.error", "error", err))
		printMaintenanceEmailHelpTo(os.Stderr)
		exitFunc(2)
		return
	}

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	appName := cfg.Server.Branding.Title
	if appName == "" {
		appName = config.ProjectName
	}
	appURL := cfg.Server.FQDN
	if appURL == "" {
		appURL = "localhost"
	}
	templates := email.NewEmailTemplate()
	if dryRun {
		previews, validationErrors, warnings := templates.ValidateTemplates(appName, appURL, cfg.Server.Notifications.Email.ReplyTo)
		for _, template := range email.GetAllTemplateTypes() {
			preview, ok := previews[template.Type]
			if !ok {
				continue
			}
			fmt.Print(serverCLITf("server_cli.maintenance.email_preview", "template", template.Type, "subject", preview[0], "body", preview[1]))
		}
		for _, validationErr := range validationErrors {
			fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.email_validation_error", "error", validationErr))
		}
		for _, warning := range warnings {
			fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.email_validation_warning", "warning", warning))
		}
		if len(validationErrors) > 0 {
			exitFunc(65)
		}
		return
	}
	if to == "" {
		to = cfg.Server.Notifications.Email.ReplyTo
	}
	if to == "" {
		fmt.Fprintln(os.Stderr, serverCLIText("server_cli.maintenance.email_recipient_required"))
		exitFunc(2)
		return
	}
	mailer, ok := email.NewMailerFromConfig(cfg)
	if !ok {
		fmt.Fprintln(os.Stderr, serverCLIText("server_cli.maintenance.email_smtp_not_configured"))
		exitFunc(69)
		return
	}
	previewSubject, _, err := templates.PreviewTemplate(email.TemplateTest, appName, appURL, cfg.Server.Notifications.Email.ReplyTo)
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.email_test_template_invalid", "error", err))
		exitFunc(65)
		return
	}
	subject := "[TEST] " + previewSubject
	audit := logging.NewManager(config.GetLogDir()).Audit()
	err = mailer.SendTemplateTest(to, email.TemplateTest)
	result := "success"
	severity := logging.AuditSeverityInfo
	if err != nil {
		result = "failure"
		severity = logging.AuditSeverityError
	}
	audit.LogEvent(logging.AuditAction("email.test"), logging.AuditCategorySystem, severity,
		maintenanceAuditActor(),
		&logging.AuditTarget{Type: "email_template", Name: string(email.TemplateTest)}, result,
		map[string]interface{}{"recipient": to, "template": string(email.TemplateTest), "subject": subject}, errString(err))
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.email_test_send_failed", "error", err))
		exitFunc(1)
		return
	}
	fmt.Print(serverCLITf("server_cli.maintenance.email_test_sent", "recipient", to, "template", email.TemplateTest, "subject", subject))
}

func parseMaintenanceEmailArgs(args []string) (dryRun bool, to string, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--to":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return false, "", errors.New(serverCLIText("server_cli.maintenance.email_to_required"))
			}
			i++
			to = args[i]
		default:
			return false, "", errors.New(serverCLITf("server_cli.maintenance.unknown_email_option", "option", args[i]))
		}
	}
	return dryRun, to, nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func printMaintenanceEmailHelp() {
	printMaintenanceEmailHelpTo(os.Stdout)
}

func printMaintenanceEmailHelpTo(w io.Writer) {
	for _, line := range []string{
		serverCLIText("server_cli.maintenance.email_help_usage"),
		serverCLIText("server_cli.maintenance.email_help_description"),
	} {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return
		}
	}
}
