package main

import (
	"fmt"
	"os"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/email"
)

// runEmail dispatches `search email <subcommand>` per AI.md PART 17
// ("Email Template Configuration" — `{project_name} email test` validates
// SMTP actually works before enabling email features).
func runEmail(args []string) {
	if len(args) == 0 {
		printEmailHelp()
		os.Exit(1)
	}

	switch args[0] {
	case "test":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: search email test <address>")
			os.Exit(1)
		}
		runEmailTest(args[1])
	case "--help", "-h", "help":
		printEmailHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown email subcommand: %s\n\n", args[0])
		printEmailHelp()
		os.Exit(1)
	}
}

// printEmailHelp prints the email subcommand usage (AI.md PART 17).
func printEmailHelp() {
	fmt.Println("Usage: search email <subcommand>")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  test <address>  Send a test email to verify SMTP configuration")
}

// runEmailTest builds a mailer from server.yml (same SMTP config resolution
// as the running server — see src/server/server.go NewServer) and sends a
// [TEST]-prefixed sample email, per AI.md PART 17 "Send Test Email":
// "Requires SMTP, uses sample data, subject prefixed [TEST]".
func runEmailTest(to string) {
	cfg, err := config.Load(config.GetConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load config: %v\n", err)
		os.Exit(1)
	}

	mailer, ok := email.NewMailerFromConfig(cfg)
	if !ok {
		fmt.Fprintln(os.Stderr, "Error: SMTP is not configured — set server.notifications.email.smtp.host in server.yml")
		os.Exit(1)
	}

	if err := mailer.SendTest(to); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to send test email: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Test email sent to %s\n", to)
}
