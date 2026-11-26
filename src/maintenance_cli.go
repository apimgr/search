package main

// Operator maintenance subcommands that are part of the frozen PART 8 command
// set but are not backup/restore/update/mode/setup/pgp:
//
//	--maintenance secret rotate <name>     AI.md PART 11 "Secret Rotation"
//	--maintenance token list|revoke <p>    AI.md PART 11 "Operator revocation"
//	--maintenance data export|delete       AI.md PART 12 GDPR/CCPA
//	--maintenance compliance report        AI.md PART 12 "Compliance Routes"
//
// Every action here is authorized exactly like the pgp actions: server.token
// OR root (AI.md PART 5 "Sensitive Operations"). Secret rotation additionally
// runs the typed-confirmation flow.

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/display"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/owner"
	"github.com/apimgr/search/src/security"
	"github.com/apimgr/search/src/server"
	"github.com/google/uuid"
)

// installationSecretGrace is how long the pre-rotation installation_secret
// stays valid for in-flight {security_id} URLs (AI.md PART 11: 7 days).
const installationSecretGrace = 7 * 24 * time.Hour

// encryptionKeyGrace is how long the pre-rotation encryption_key is retained
// so any at-rest data not yet re-encrypted still decrypts (AI.md PART 11: 30 days).
const encryptionKeyGrace = 30 * 24 * time.Hour

func maintenanceAuditActor() logging.AuditActor {
	return logging.AuditActor{Type: "operator", Username: pgpActor(), IP: pgpAuditIP}
}

// runMaintenanceSecret implements `--maintenance secret rotate <name>`.
// Only installation_secret and encryption_key support manual rotation;
// cookie_signing_key and csrf_token_secret are auto-rotated and rejected
// (AI.md PART 11 "Secret Rotation").
func runMaintenanceSecret(action, name string) {
	switch action {
	case "rotate":
		// handled below
	case "help", "--help", "":
		printMaintenanceSecretHelp()
		return
	default:
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.unknown_secret_action", "action", action))
		printMaintenanceSecretHelp()
		exitFunc(1)
		return
	}

	switch name {
	case database.SecretInstallation, "encryption_key":
		// rotatable
	case database.SecretCookieSigning, database.SecretCSRFToken:
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.secret_auto_rotated", "name", name))
		exitFunc(1)
		return
	default:
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.unknown_secret", "name", name))
		fmt.Println(serverCLIText("server_cli.maintenance.rotatable_secrets"))
		exitFunc(1)
		return
	}

	cfg := pgpRequireAuthorized()
	reasonText, ok := pgpSensitiveConfirm(cfg)
	if !ok {
		exitFunc(1)
		return
	}

	var err error
	if name == database.SecretInstallation {
		err = rotateInstallationSecret(cfg, reasonText)
	} else {
		err = rotateEncryptionKey(cfg, reasonText)
	}
	if err != nil {
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.rotation_failed", "error", err))
		exitFunc(1)
		return
	}
	fmt.Println(display.Emoji("✅", "[OK]")+" "+serverCLITf("server_cli.maintenance.secret_rotated", "name", name))
}

// printMaintenanceSecretHelp documents the secret subcommand.
func printMaintenanceSecretHelp() {
	fmt.Println(serverCLIText("server_cli.maintenance.secret_rotation_heading"))
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.help_rotate_installation_secret"))
	fmt.Println(serverCLIText("server_cli.maintenance.help_rotate_encryption_key"))
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.help_auto_rotated_secrets"))
	fmt.Println(serverCLIText("server_cli.maintenance.help_auto_rotated_secrets_continued"))
}

// rotateInstallationSecret generates a new installation_secret, re-encrypts the
// PGP private key under it, keeps the previous value for the 7-day grace window
// and records security.installation_secret_rotated in audit.log.
func rotateInstallationSecret(cfg *config.Config, reasonText string) error {
	oldSecret := cfg.Server.Security.InstallationSecret
	newSecret := config.GenerateBase64Secret()

	// Re-encrypt the at-rest PGP private key so it stays readable under the
	// new secret. A missing keypair is not an error — rotation still proceeds.
	if privArmor, loadErr := security.LoadEncryptedPrivateKey(config.GetConfigDir()); loadErr == nil && privArmor != "" {
		relocked, relockErr := security.RelockPrivateKey(privArmor, oldSecret, newSecret)
		if relockErr != nil {
			return fmt.Errorf("re-encrypt pgp private key: %w", relockErr)
		}
		if saveErr := security.SaveEncryptedPrivateKey(config.GetConfigDir(), relocked); saveErr != nil {
			return fmt.Errorf("write re-encrypted pgp private key: %w", saveErr)
		}
	}

	// Mirror the rotation into app_secrets so the DB-side grace window matches
	// the config-side one. A database that cannot be opened is not fatal here:
	// the live secret is the server.yml value.
	if dm, dbErr := pgpOpenDB(); dbErr == nil {
		defer closeutil.Close(dm)
		if _, rotErr := database.RotateSecret(context.Background(), dm.ServerDB(), database.SecretInstallation); rotErr != nil {
			return fmt.Errorf("rotate app_secrets row: %w", rotErr)
		}
	}

	cfg.Server.Security.InstallationSecretPrevious = oldSecret
	cfg.Server.Security.InstallationSecretPreviousExpires = time.Now().UTC().Add(installationSecretGrace).Format(time.RFC3339)
	cfg.Server.Security.InstallationSecret = newSecret
	if err := cfg.Save(config.GetConfigPath()); err != nil {
		return err
	}

	pgpAuditLogger().LogEvent(
		logging.AuditActionInstallationSecretRotated,
		logging.AuditCategorySecurity,
		logging.AuditSeverityCritical,
		maintenanceAuditActor(),
		&logging.AuditTarget{Type: "secret", Name: database.SecretInstallation},
		"success",
		map[string]interface{}{"grace_expires": cfg.Server.Security.InstallationSecretPreviousExpires},
		reasonText,
	)
	fmt.Println("   " + serverCLIText("server_cli.maintenance.previous_installation_secret_grace"))
	return nil
}

// rotateEncryptionKey rotates the canonical at-rest AES key and eagerly
// migrates every dependent ciphertext before recording the audit event.
func rotateEncryptionKey(cfg *config.Config, reasonText string) error {
	ctx := context.Background()
	oldKey := cfg.Server.Security.EncryptionKey
	if oldKey == "" {
		return fmt.Errorf("current encryption key is unavailable")
	}
	dm, err := pgpOpenDB()
	if err != nil { return err }
	defer closeutil.Close(dm)
	db := dm.ServerDB()
	if err := cfg.ImportLegacyAppSecrets(ctx, db); err != nil { return fmt.Errorf("import legacy secrets: %w", err) }
	prefix := ""
	if db.Driver() == "libsql" { prefix = "srv_" }
	previousKey, previousUntil, err := security.EnsurePreviousEncryptionKey(ctx, db, oldKey, cfg.Server.Security.EncryptionKeyPrevious, cfg.Server.Security.EncryptionKeyPreviousExpires)
	if err != nil { return fmt.Errorf("load previous encryption key: %w", err) }
	if _, err := owner.ReencryptTokenCopies(ctx, db, prefix, previousKey, oldKey); err != nil { return fmt.Errorf("finish prior owner-token rotation: %w", err) }
	if err := pgpMigrateLegacyReports(cfg, dm); err != nil { return fmt.Errorf("migrate legacy security reports: %w", err) }
	if _, err := security.ReencryptAESReportData(ctx, db, oldKey, previousKey); err != nil { return fmt.Errorf("finish prior security-report rotation: %w", err) }
	if _, err := security.ReencryptQRShortlinks(ctx, db, oldKey, previousKey); err != nil { return fmt.Errorf("finish prior QR short-link rotation: %w", err) }
	changed, err := cfg.ReencryptDNSCredentials(oldKey, previousKey)
	if err != nil { return fmt.Errorf("finish prior DNS credential rotation: %w", err) }
	if changed { if err := cfg.Save(config.GetConfigPath()); err != nil { return fmt.Errorf("save migrated DNS credentials: %w", err) } }
	if previousKey != "" {
		if previousUntil.After(time.Now().UTC()) {
			return fmt.Errorf("a previous encryption key is still within its 30-day grace period")
		}
		if err := database.DeleteAppSecretValue(ctx, db, database.SecretEncryptionKeyPrevious); err != nil { return err }
		if err := database.DeleteAppSecretValue(ctx, db, database.SecretEncryptionKeyPreviousUntil); err != nil { return err }
	}
	newKey := config.GenerateBase64Secret()
	expires := time.Now().UTC().Add(encryptionKeyGrace)
	if err := security.BeginEncryptionKeyRotation(ctx, db, oldKey, newKey, expires, reasonText); err != nil { return fmt.Errorf("stage encryption key rotation: %w", err) }
	cfg.Server.Security.EncryptionKey = newKey
	cfg.Server.Security.EncryptionKeyPrevious = ""
	cfg.Server.Security.EncryptionKeyPreviousExpires = ""
	if err := cfg.Save(config.GetConfigPath()); err != nil {
		return fmt.Errorf("persist new encryption key; rotation will resume at next startup: %w", err)
	}
	if err := pgpMigrateLegacyReports(cfg, dm); err != nil { return fmt.Errorf("migrate legacy security reports: %w", err) }
	tokenCount, err := owner.ReencryptTokenCopies(ctx, db, prefix, oldKey, newKey)
	if err != nil { return fmt.Errorf("re-encrypt owner and linked API tokens: %w", err) }
	reportCount, err := security.ReencryptAESReportData(ctx, db, newKey, oldKey)
	if err != nil { return fmt.Errorf("re-encrypt security reports: %w", err) }
	shortlinkCount, err := security.ReencryptQRShortlinks(ctx, db, newKey, oldKey)
	if err != nil { return fmt.Errorf("re-encrypt QR short links: %w", err) }
	dnsChanged, err := cfg.ReencryptDNSCredentials(newKey, oldKey)
	if err != nil { return fmt.Errorf("re-encrypt DNS credentials: %w", err) }
	if dnsChanged { if err := cfg.Save(config.GetConfigPath()); err != nil { return fmt.Errorf("save re-encrypted DNS credentials: %w", err) } }

	pgpAuditLogger().LogEvent(
		logging.AuditActionEncryptionKeyRotated,
		logging.AuditCategorySecurity,
		logging.AuditSeverityCritical,
		maintenanceAuditActor(),
		&logging.AuditTarget{Type: "secret", Name: "encryption_key"},
		"success",
		map[string]interface{}{
			"tokens_reencrypted": tokenCount,
			"reports_reencrypted": reportCount,
			"shortlinks_reencrypted": shortlinkCount,
			"grace_expires":       expires.Format(time.RFC3339),
		},
		reasonText,
	)
	if err := security.CompleteEncryptionKeyRotation(ctx, db); err != nil { return fmt.Errorf("finish encryption-key recovery marker: %w", err) }
	fmt.Println("   "+serverCLITf("server_cli.maintenance.encryption_rotation_summary", "tokens", tokenCount, "reports", reportCount, "shortlinks", shortlinkCount))
	fmt.Println("   " + serverCLIText("server_cli.maintenance.previous_encryption_key_grace"))
	return nil
}

// runMaintenanceToken implements `--maintenance token list|revoke <prefix>`
// per AI.md PART 11 "Operator revocation". Token revocation is operator-only
// via the CLI — there is no API route for it.
func runMaintenanceToken(action, prefix string) {
	switch action {
	case "list", "revoke":
		// handled below
	case "help", "--help", "":
		printMaintenanceTokenHelp()
		return
	default:
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.unknown_token_action", "action", action))
		printMaintenanceTokenHelp()
		exitFunc(1)
		return
	}

	pgpRequireAuthorized()

	dm, err := pgpOpenDB()
	if err != nil {
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.failed_open_database", "error", err))
		exitFunc(1)
		return
	}
	defer closeutil.Close(dm)

	db := dm.ServerDB()
	table := database.ServerTableName(db, "api_tokens")
	ctx := context.Background()

	if action == "list" {
		rows, err := db.Query(ctx, fmt.Sprintf(
			"SELECT token_prefix, COALESCE(owner_id, ''), kind, name, created_at, expires_at, revoked_at FROM %s WHERE owner_id IS NOT NULL ORDER BY created_at", table))
		if err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.failed_list_tokens", "error", err))
			exitFunc(1)
			return
		}
		defer closeutil.Close(rows)

		found := 0
		for rows.Next() {
			var tokenPrefix, ownerID, kind, name string
			var createdAt, expiresAt, revokedAt sql.NullInt64
			if err := rows.Scan(&tokenPrefix, &ownerID, &kind, &name, &createdAt, &expiresAt, &revokedAt); err != nil {
				fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.failed_read_token", "error", err))
				exitFunc(1)
				return
			}
			if found == 0 {
				fmt.Println(serverCLIText("server_cli.maintenance.active_tokens_heading"))
			}
			found++
			status := "active"
			if revokedAt.Valid {
				status = "revoked"
			} else if expiresAt.Valid && expiresAt.Int64 <= time.Now().Unix() {
				status = "expired"
			}
			fmt.Print(serverCLITf("server_cli.maintenance.token_list_row", "prefix", tokenPrefix, "kind", kind, "name", name, "owner", ownerID, "status", serverCLIText("server_cli.maintenance.token_status_"+status)))
			if createdAt.Valid {
				fmt.Print(serverCLITf("server_cli.maintenance.token_created", "date", time.Unix(createdAt.Int64, 0).UTC().Format(time.RFC3339)))
			}
			fmt.Println()
		}
		if err := rows.Err(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.failed_read_tokens", "error", err))
			exitFunc(1)
			return
		}
		if found == 0 {
			fmt.Println(serverCLIText("server_cli.maintenance.no_active_tokens"))
		}
		return
	}

	if prefix == "" {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.maintenance.token_prefix_required"))
		fmt.Println(serverCLITf("server_cli.maintenance.token_revoke_usage", "project", config.ProjectName))
		exitFunc(1)
		return
	}

	result, err := db.Exec(ctx,
		fmt.Sprintf("UPDATE %s SET revoked_at = ?, revoked_reason = 'operator_action' WHERE token_prefix = ? AND owner_id IS NOT NULL AND revoked_at IS NULL", table), time.Now().Unix(), prefix)
	if err != nil {
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.failed_revoke_token", "error", err))
		exitFunc(1)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.no_matching_active_token", "prefix", prefix))
		exitFunc(1)
		return
	}
	if affected > 0 {
		if _, err := db.Exec(ctx, "INSERT INTO "+database.ServerTableName(db, "audit_log")+" (id, timestamp, actor_type, action, resource_type, resource_id, details) VALUES (?, ?, 'operator', 'token.revoked', 'api_token', ?, ?)", uuid.NewString(), time.Now().Unix(), prefix, `{"reason":"operator_action"}`); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.token_revoked_audit_failed", "error", err))
			exitFunc(1)
			return
		}
	}

	pgpAuditLogger().LogTokenRevoke(pgpActor(), pgpAuditIP, prefix)
	fmt.Println(display.Emoji("✅", "[OK]")+" "+serverCLITf("server_cli.maintenance.tokens_revoked", "count", affected, "prefix", prefix))
}

// printMaintenanceTokenHelp documents the token subcommand.
func printMaintenanceTokenHelp() {
	fmt.Println(serverCLIText("server_cli.maintenance.token_management_heading"))
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.help_token_list"))
	fmt.Println(serverCLIText("server_cli.maintenance.help_token_revoke"))
}

type personalDataOwner struct {
	ID         string     `json:"id"`
	CreatedAt  *time.Time `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

type personalDataToken struct {
	Prefix     string     `json:"prefix"`
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	CreatedAt  *time.Time `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	Status     string     `json:"status"`
}

type personalDataExport struct {
	Format    int                    `json:"format"`
	Owner     personalDataOwner      `json:"owner"`
	Tokens    []personalDataToken    `json:"tokens"`
	Resources map[string]interface{} `json:"resources"`
}

type dataActionFlags struct {
	ownerPrefix string
	output      string
	yes         bool
	help        bool
}

func maintenanceDataWritesStdout(args []string) bool {
	for i, arg := range args {
		if arg == "--output" && i+1 < len(args) && args[i+1] == "-" {
			return true
		}
		if arg == "--output=-" {
			return true
		}
	}
	return false
}

func runMaintenanceData(action string, args []string) {
	if action == "help" || action == "--help" || action == "-h" || action == "" {
		printMaintenanceDataHelp()
		return
	}
	if action != "export" && action != "delete" {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.unknown_data_action", "action", action))
		printMaintenanceDataHelp()
		exitFunc(2)
		return
	}

	flags, err := parseDataActionFlags(action, args)
	if errors.Is(err, flag.ErrHelp) || flags.help {
		printMaintenanceDataHelp()
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.error", "error", err))
		printMaintenanceDataHelp()
		exitFunc(2)
		return
	}
	if flags.ownerPrefix == "" {
		fmt.Fprintln(os.Stderr, serverCLIText("server_cli.maintenance.owner_prefix_required"))
		printMaintenanceDataHelp()
		exitFunc(2)
		return
	}
	if action == "export" && flags.yes || action == "delete" && flags.output != "" {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.data_option_unsupported", "action", action))
		printMaintenanceDataHelp()
		exitFunc(2)
		return
	}

	cfg := pgpRequireAuthorized()
	dm, err := pgpOpenDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.failed_open_database", "error", err))
		exitFunc(1)
		return
	}
	defer closeutil.Close(dm)

	ctx := context.Background()
	dataOwner, err := resolveDataOwner(ctx, dm.ServerDB(), flags.ownerPrefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.error", "error", err))
		exitFunc(66)
		return
	}
	if action == "export" {
		if err := exportPersonalData(ctx, cfg, dm, dataOwner, flags.output); err != nil {
			fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.export_failed", "error", err))
			exitFunc(1)
		}
		return
	}
	if err := deletePersonalData(ctx, dm, dataOwner, flags.yes); err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.deletion_failed", "error", err))
		exitFunc(1)
	}
}

func parseDataActionFlags(action string, args []string) (dataActionFlags, error) {
	var result dataActionFlags
	fs := flag.NewFlagSet("data "+action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&result.ownerPrefix, "owner", "", serverCLIText("server_cli.maintenance.flag_owner_help"))
	fs.BoolVar(&result.help, "help", false, serverCLIText("server_cli.maintenance.flag_data_help"))
	fs.BoolVar(&result.help, "h", false, serverCLIText("server_cli.maintenance.flag_data_help"))
	if action == "export" {
		fs.StringVar(&result.output, "output", "", serverCLIText("server_cli.maintenance.flag_output_help"))
	} else {
		fs.BoolVar(&result.yes, "yes", false, serverCLIText("server_cli.maintenance.flag_yes_help"))
	}
	if err := fs.Parse(args); err != nil {
		return result, err
	}
	if fs.NArg() != 0 {
		return result, errors.New(serverCLITf("server_cli.maintenance.unexpected_argument", "argument", fs.Arg(0)))
	}
	return result, nil
}

func printMaintenanceDataHelp() {
	fmt.Println(serverCLIText("server_cli.maintenance.data_subject_heading"))
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.help_data_export"))
	fmt.Println(serverCLIText("server_cli.maintenance.help_data_export_description"))
	fmt.Println(serverCLIText("server_cli.maintenance.help_data_delete"))
	fmt.Println(serverCLIText("server_cli.maintenance.help_data_delete_description"))
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.help_data_security_note"))
}

func resolveDataOwner(ctx context.Context, db *database.DB, prefix string) (personalDataOwner, error) {
	if strings.TrimSpace(prefix) == "" {
		return personalDataOwner{}, errors.New("owner token prefix is required")
	}
	tableOwners := database.ServerTableName(db, "owners")
	tableTokens := database.ServerTableName(db, "api_tokens")
	rows, err := db.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT o.id, o.created_at, o.last_seen_at
		FROM %s AS o
		JOIN %s AS t ON t.owner_id = o.id
		WHERE t.token_prefix = ?
	`, tableOwners, tableTokens), prefix)
	if err != nil {
		return personalDataOwner{}, fmt.Errorf("resolve owner prefix: %w", err)
	}
	defer closeutil.Close(rows)

	var resolved personalDataOwner
	found := false
	for rows.Next() {
		var createdAt, lastSeenAt sql.NullInt64
		var item personalDataOwner
		if err := rows.Scan(&item.ID, &createdAt, &lastSeenAt); err != nil {
			return personalDataOwner{}, fmt.Errorf("read owner record: %w", err)
		}
		if found && resolved.ID != item.ID {
			return personalDataOwner{}, errors.New("owner token prefix is ambiguous")
		}
		item.CreatedAt = unixTimeOrNil(createdAt)
		item.LastSeenAt = unixTimeOrNil(lastSeenAt)
		resolved = item
		found = true
	}
	if err := rows.Err(); err != nil {
		return personalDataOwner{}, fmt.Errorf("read owner records: %w", err)
	}
	if !found {
		return personalDataOwner{}, errors.New("no owner token matches the supplied prefix")
	}
	return resolved, nil
}

func loadOwnerTokens(ctx context.Context, db *database.DB, ownerID string) ([]personalDataToken, error) {
	table := database.ServerTableName(db, "api_tokens")
	rows, err := db.Query(ctx, fmt.Sprintf(`
		SELECT token_prefix, kind, name, created_at, last_used_at, expires_at, revoked_at
		FROM %s
		WHERE owner_id = ?
		ORDER BY kind, id
	`, table), ownerID)
	if err != nil {
		return nil, fmt.Errorf("read owner token metadata: %w", err)
	}
	defer closeutil.Close(rows)

	items := make([]personalDataToken, 0)
	now := time.Now().Unix()
	for rows.Next() {
		var item personalDataToken
		var createdAt, lastUsedAt, expiresAt, revokedAt sql.NullInt64
		if err := rows.Scan(&item.Prefix, &item.Kind, &item.Name, &createdAt, &lastUsedAt, &expiresAt, &revokedAt); err != nil {
			return nil, fmt.Errorf("read owner token metadata: %w", err)
		}
		item.CreatedAt = unixTimeOrNil(createdAt)
		item.LastUsedAt = unixTimeOrNil(lastUsedAt)
		item.ExpiresAt = unixTimeOrNil(expiresAt)
		item.Status = "active"
		switch {
		case revokedAt.Valid:
			item.Status = "revoked"
		case expiresAt.Valid && expiresAt.Int64 <= now:
			item.Status = "expired"
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read owner token metadata: %w", err)
	}
	return items, nil
}

func unixTimeOrNil(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	result := time.Unix(value.Int64, 0).UTC()
	return &result
}

func exportPersonalData(ctx context.Context, cfg *config.Config, dm *database.DatabaseManager, dataOwner personalDataOwner, output string) error {
	requestID := uuid.NewString()
	logComplianceEvent(logging.AuditActionDataExportRequested, dataOwner.ID, requestID, nil)

	tokens, err := loadOwnerTokens(ctx, dm.ServerDB(), dataOwner.ID)
	if err != nil {
		return err
	}
	alerts, err := alert.NewManagerWithDB(dm.ServerDB(), cfg, nil, nil).ListByOwner(ctx, dataOwner.ID)
	if err != nil {
		return err
	}
	results, err := loadOwnerAlertResults(ctx, dm.ServerDB(), dataOwner.ID)
	if err != nil {
		return err
	}
	doc := personalDataExport{
		Format: 1,
		Owner:  dataOwner,
		Tokens: tokens,
		Resources: map[string]interface{}{
			"search_alerts":        alerts,
			"search_alert_results": results,
		},
	}
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	path := output
	if path == "" {
		path = fmt.Sprintf("%s-export-%s-%s.json", config.ProjectName, dataOwner.ID, time.Now().UTC().Format("20060102"))
	}
	if path == "-" {
		if _, err := os.Stdout.Write(payload); err != nil {
			return fmt.Errorf("write export to stdout: %w", err)
		}
	} else {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("create export file: %w", err)
		}
		if _, err := file.Write(payload); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return fmt.Errorf("write export file: %w", err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return fmt.Errorf("sync export file: %w", err)
		}
		if err := file.Close(); err != nil {
			_ = os.Remove(path)
			return fmt.Errorf("close export file: %w", err)
		}
	}

	logComplianceEvent(logging.AuditActionDataExportCompleted, dataOwner.ID, requestID, map[string]interface{}{
		"file_size": len(payload),
		"resources": len(alerts) + len(results),
	})
	if path != "-" {
		fmt.Println(display.Emoji("✅", "[OK]")+" "+serverCLITf("server_cli.maintenance.data_export_written", "path", path))
	}
	return nil
}

func loadOwnerAlertResults(ctx context.Context, db *database.DB, ownerID string) ([]alert.AlertResult, error) {
	resultsTable := database.ServerTableName(db, "search_alert_results")
	alertsTable := database.ServerTableName(db, "search_alerts")
	rows, err := db.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.title, r.url, r.content, r.engine, r.published_at, r.first_seen_at,
		       r.notified_email_at, r.notified_webhook_at
		FROM %s AS r
		JOIN %s AS a ON a.id = r.alert_id
		WHERE a.owner_id = ?
		ORDER BY r.first_seen_at DESC
	`, resultsTable, alertsTable), ownerID)
	if err != nil {
		return nil, fmt.Errorf("read owner alert results: %w", err)
	}
	defer closeutil.Close(rows)

	items := make([]alert.AlertResult, 0)
	for rows.Next() {
		var item alert.AlertResult
		var publishedAt, notifiedEmail, notifiedWebhook sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.Title, &item.URL, &item.Content, &item.Engine,
			&publishedAt, &item.FirstSeenAt, &notifiedEmail, &notifiedWebhook,
		); err != nil {
			return nil, fmt.Errorf("read owner alert result: %w", err)
		}
		if publishedAt.Valid {
			item.PublishedAt = &publishedAt.Time
		}
		if notifiedEmail.Valid {
			item.NotifiedEmailAt = &notifiedEmail.Time
		}
		if notifiedWebhook.Valid {
			item.NotifiedWebhookAt = &notifiedWebhook.Time
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read owner alert results: %w", err)
	}
	return items, nil
}

type ownerDeletionCounts struct {
	deleted    int64
	anonymized int64
	tokens     int64
	alerts     int64
	results    int64
}

func deletePersonalData(ctx context.Context, dm *database.DatabaseManager, dataOwner personalDataOwner, skipPrompt bool) error {
	requestID := uuid.NewString()
	if err := logDataDeletionRequested(ctx, dm.ServerDB(), dataOwner.ID, requestID); err != nil {
		return fmt.Errorf("log deletion request: %w", err)
	}
	counts, err := countOwnerData(ctx, dm.ServerDB(), dataOwner.ID)
	if err != nil {
		return err
	}
	if !skipPrompt {
			fmt.Print(serverCLITf("server_cli.maintenance.delete_owner_confirm", "owner", dataOwner.ID, "tokens", counts.tokens, "resources", counts.alerts))
		var confirmation string
		if _, err := fmt.Scanln(&confirmation); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read owner deletion confirmation: %w", err)
		}
		if confirmation != dataOwner.ID {
			fmt.Println(serverCLIText("admin.status.cancelled"))
			return nil
		}
	}

	counts, err = deleteOwnerData(ctx, dm.ServerDB(), dataOwner.ID, requestID)
	if err != nil {
		return err
	}
	logComplianceEvent(logging.AuditActionDataDeletionCompleted, dataOwner.ID, requestID, map[string]interface{}{
		"deleted_count":     counts.deleted,
		"anonymized_count":  counts.anonymized,
		"tokens_deleted":    counts.tokens,
		"resources_deleted": counts.alerts + counts.results,
	})
	fmt.Println(display.Emoji("✅", "[OK]")+" "+serverCLITf("server_cli.maintenance.data_deleted", "deleted", counts.deleted, "anonymized", counts.anonymized))
	return nil
}

func logDataDeletionRequested(ctx context.Context, db *database.DB, ownerID, requestID string) error {
	audit := database.ServerTableName(db, "audit_log")
	details, err := json.Marshal(map[string]string{"request_id": requestID})
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (id, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, severity, category)
		VALUES (?, ?, 'operator', ?, ?, 'owner', ?, ?, 'info', 'compliance')
	`, audit), uuid.NewString(), time.Now().UTC().Unix(), pgpActor(), string(logging.AuditActionDataDeletionRequested), ownerID, string(details))
	if err == nil {
		logComplianceEvent(logging.AuditActionDataDeletionRequested, ownerID, requestID, nil)
	}
	return err
}

func countOwnerData(ctx context.Context, db *database.DB, ownerID string) (ownerDeletionCounts, error) {
	tokens := database.ServerTableName(db, "api_tokens")
	owners := database.ServerTableName(db, "owners")
	alerts := database.ServerTableName(db, "search_alerts")
	results := database.ServerTableName(db, "search_alert_results")
	var counts ownerDeletionCounts
	if err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE owner_id = ?", tokens), ownerID).Scan(&counts.tokens); err != nil {
		return counts, err
	}
	if err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE owner_id = ?", alerts), ownerID).Scan(&counts.alerts); err != nil {
		return counts, err
	}
	if err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE alert_id IN (SELECT id FROM %s WHERE owner_id = ?)", results, alerts), ownerID).Scan(&counts.results); err != nil {
		return counts, err
	}
	var ownerCount int
	if err := db.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ?", owners), ownerID).Scan(&ownerCount); err != nil {
		return counts, err
	}
	if ownerCount != 1 {
		return counts, errors.New("owner no longer exists")
	}
	counts.deleted = counts.tokens + counts.alerts + counts.results + 1
	return counts, nil
}

func deleteOwnerData(ctx context.Context, db *database.DB, ownerID, requestID string) (ownerDeletionCounts, error) {
	owners := database.ServerTableName(db, "owners")
	tokens := database.ServerTableName(db, "api_tokens")
	alerts := database.ServerTableName(db, "search_alerts")
	results := database.ServerTableName(db, "search_alert_results")
	audit := database.ServerTableName(db, "audit_log")
	counts := ownerDeletionCounts{}
	err := db.Write(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ?", owners), ownerID).Scan(&exists); err != nil {
			return err
		}
		if exists != 1 {
			return errors.New("owner no longer exists")
		}

		result, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE alert_id IN (SELECT id FROM %s WHERE owner_id = ?)", results, alerts), ownerID)
		if err != nil {
			return err
		}
		counts.results, err = result.RowsAffected()
		if err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE owner_id = ?", alerts), ownerID)
		if err != nil {
			return err
		}
		counts.alerts, err = result.RowsAffected()
		if err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE owner_id = ?", tokens), ownerID)
		if err != nil {
			return err
		}
		counts.tokens, err = result.RowsAffected()
		if err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = ?", owners), ownerID)
		if err != nil {
			return err
		}
		if deleted, err := result.RowsAffected(); err != nil {
			return err
		} else if deleted != 1 {
			return errors.New("owner no longer exists")
		}

		details, err := json.Marshal(map[string]interface{}{
			"owner_id":         ownerID,
			"request_id":       requestID,
			"deleted_count":    counts.tokens + counts.alerts + counts.results + 1,
			"anonymized_count": counts.anonymized,
		})
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO %s (id, timestamp, actor_type, actor_id, action, resource_type, resource_id, details, severity, category)
			VALUES (?, ?, 'operator', ?, ?, 'owner', ?, ?, 'info', 'compliance')
		`, audit), uuid.NewString(), time.Now().UTC().Unix(), pgpActor(), string(logging.AuditActionDataDeletionCompleted), ownerID, string(details))
		if err != nil {
			return err
		}
		counts.deleted = counts.tokens + counts.alerts + counts.results + 1
		return nil
	})
	return counts, err
}

func logComplianceEvent(action logging.AuditAction, ownerID, requestID string, extra map[string]interface{}) {
	details := map[string]interface{}{"owner_id": ownerID, "request_id": requestID}
	for key, value := range extra {
		details[key] = value
	}
	target := &logging.AuditTarget{Type: "owner", ID: ownerID}
	pgpAuditLogger().LogEventQuiet(
		action,
		logging.AuditCategoryCompliance,
		logging.AuditSeverityInfo,
		maintenanceAuditActor(),
		target,
		"success",
		details,
		"",
	)
}

// runMaintenanceCompliance implements `--maintenance compliance report`
// (AI.md PART 12: "Operators run {project_name} --maintenance compliance
// report for a compliance summary"). Compliance is configured entirely in
// server.yml, so the report is rendered from the loaded configuration.
func runMaintenanceCompliance(action string) {
	switch action {
	case "report":
		// handled below
	case "help", "--help", "":
		printMaintenanceComplianceHelp()
		return
	default:
		fmt.Println(display.Emoji("❌", "[ERROR]")+" "+serverCLITf("server_cli.maintenance.unknown_compliance_action", "action", action))
		printMaintenanceComplianceHelp()
		exitFunc(1)
		return
	}

	cfg := pgpRequireAuthorized()
	if err := printComplianceReport(cfg); err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.maintenance.compliance_report_failed", "error", err))
		exitFunc(1)
		return
	}

	pgpAuditLogger().LogEvent(
		logging.AuditActionComplianceReport,
		logging.AuditCategoryCompliance,
		logging.AuditSeverityInfo,
		maintenanceAuditActor(),
		&logging.AuditTarget{Type: "compliance_report", Name: "summary"},
		"success",
		map[string]interface{}{"compliance_enabled": cfg.Server.Compliance.AnyEnabled()},
		"",
	)
}

// printMaintenanceComplianceHelp documents the compliance subcommand.
func printMaintenanceComplianceHelp() {
	fmt.Println(serverCLIText("server_cli.maintenance.compliance_heading"))
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.help_compliance_report"))
}

// printComplianceReport renders the operator-facing compliance summary.
func printComplianceReport(cfg *config.Config) error {
	yesNo := func(b bool) string {
		if b {
			return serverCLIText("common.yes")
		}
		return serverCLIText("common.no")
	}

	dm, err := pgpOpenDB()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer closeutil.Close(dm)
	db := dm.ServerDB()
	ctx := context.Background()
	count := func(table string, where string) (int64, error) {
		query := "SELECT COUNT(*) FROM " + database.ServerTableName(db, table)
		if where != "" {
			query += " WHERE " + where
		}
		var value int64
		if err := db.QueryRow(ctx, query).Scan(&value); err != nil {
			return 0, fmt.Errorf("count %s: %w", table, err)
		}
		return value, nil
	}
	owners, err := count("owners", "")
	if err != nil {
		return err
	}
	tokens, err := count("api_tokens", "owner_id IS NOT NULL AND revoked_at IS NULL")
	if err != nil {
		return err
	}
	alerts, err := count("search_alerts", "")
	if err != nil {
		return err
	}
	results, err := count("search_alert_results", "")
	if err != nil {
		return err
	}
	openReports, err := count("security_reports", "closed_at IS NULL")
	if err != nil {
		return err
	}
	var lastBackup time.Time
	backups, err := backup.NewManager().List()
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	for _, item := range backups {
		if item.CreatedAt.After(lastBackup) {
			lastBackup = item.CreatedAt
		}
	}

	auditPath := filepath.Join(config.GetLogDir(), "audit.log")
	effectiveAuditKeep, err := effectiveAuditLogKeep(cfg)
	if err != nil {
		return fmt.Errorf("resolve audit log retention: %w", err)
	}
	auditEntries, integrityIssue, err := logging.VerifyAuditLog(auditPath)
	if err != nil {
		return fmt.Errorf("verify audit log: %w", err)
	}
	events, err := recentComplianceEvents(auditPath, time.Now().UTC().Add(-30*24*time.Hour))
	if err != nil {
		return fmt.Errorf("read compliance events: %w", err)
	}

	fmt.Println(serverCLIText("server_cli.maintenance.compliance_report_title"))
	fmt.Println(strings.Repeat("-", 40))
	enabledStandards := enabledComplianceStandards(cfg.Server.Compliance)
	if len(enabledStandards) == 0 {
		enabledStandards = []string{"none"}
	}
	fmt.Println(serverCLITf("server_cli.maintenance.report_enabled_standards", "standards", strings.Join(enabledStandards, ", ")))
	fmt.Println(serverCLITf("server_cli.maintenance.report_backup_encryption", "state", yesNo(cfg.Server.Backup.EncryptionPassword != "")))
	fmt.Println(serverCLITf("server_cli.maintenance.report_tls", "state", yesNo(cfg.Server.SSL.Enabled)))
	fmt.Println(serverCLITf("server_cli.maintenance.report_hsts", "state", yesNo(cfg.Server.Security.HSTS.Enabled)))
	fmt.Println(serverCLITf("server_cli.maintenance.report_audit_retention", "period", effectiveAuditKeep))
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.report_privacy_heading"))
	fmt.Println(serverCLITf("server_cli.maintenance.report_data_sold", "state", yesNo(cfg.Server.Privacy.Data.Sold)))
	fmt.Println(serverCLITf("server_cli.maintenance.report_data_stored", "state", yesNo(cfg.Server.Privacy.Data.StoredOnServer)))
	fmt.Println(serverCLITf("server_cli.maintenance.report_export_right", "state", yesNo(cfg.Server.Privacy.Retention.ExportAvailable)))
	fmt.Println(serverCLITf("server_cli.maintenance.report_deletion_right", "state", yesNo(cfg.Server.Privacy.Retention.DeletionAvailable)))
	if cfg.Server.Privacy.Retention.Period != "" {
		fmt.Println(serverCLITf("server_cli.maintenance.report_privacy_retention", "period", cfg.Server.Privacy.Retention.Period))
	}
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.report_data_counts"))
	fmt.Println(serverCLITf("server_cli.maintenance.report_owners", "count", owners))
	fmt.Println(serverCLITf("server_cli.maintenance.report_resources", "count", alerts+results))
	fmt.Println(serverCLITf("server_cli.maintenance.report_active_tokens", "count", tokens))
	fmt.Println(serverCLITf("server_cli.maintenance.report_open_security_reports", "count", openReports))
	lastBackupText := serverCLIText("server_cli.maintenance.none")
	if !lastBackup.IsZero() {
		lastBackupText = lastBackup.UTC().Format(time.RFC3339)
	}
	fmt.Println(serverCLITf("server_cli.maintenance.report_last_backup", "date", lastBackupText))
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.report_compliance_events"))
	if len(events) == 0 {
		fmt.Println("  " + serverCLIText("server_cli.maintenance.none"))
	} else {
		for _, event := range events {
			fmt.Println(serverCLITf("server_cli.maintenance.report_event", "name", event.Name, "count", event.Count))
		}
	}
	fmt.Println()
	if integrityIssue == nil {
		fmt.Println(serverCLITf("server_cli.maintenance.report_audit_intact", "count", auditEntries))
	} else if integrityIssue.Line > 0 {
		fmt.Println(serverCLITf("server_cli.maintenance.report_audit_broken", "file", filepath.Base(integrityIssue.File), "line", integrityIssue.Line))
	} else {
		fmt.Println(serverCLITf("server_cli.maintenance.report_audit_broken_reason", "file", filepath.Base(integrityIssue.File), "reason", integrityIssue.Reason))
	}
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.report_effective_retention"))
	for _, standard := range enabledComplianceStandards(cfg.Server.Compliance) {
		fmt.Println(serverCLITf("server_cli.maintenance.report_standard_retention", "standard", standard, "period", complianceRetention[standard]))
	}
	fmt.Println()
	fmt.Println(serverCLIText("server_cli.maintenance.data_subject_heading"))
	fmt.Println(serverCLITf("server_cli.maintenance.data_export_command", "project", config.ProjectName))
	fmt.Println(serverCLITf("server_cli.maintenance.data_delete_command", "project", config.ProjectName))
	return nil
}

func effectiveAuditLogKeep(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", errors.New("configuration is unavailable")
	}
	keep := cfg.Server.Logs.Audit.Keep
	if keep == "" {
		keep = cfg.Server.Logs.Defaults.Keep
	}
	policy, err := logging.AuditRetentionPolicyFromKeep(keep)
	if err != nil {
		return "", err
	}
	minimum := server.MinimumComplianceAuditRetention(cfg.Server.Compliance)
	if policy.MaxAge >= 0 && policy.MaxAge < minimum {
		policy.MaxAge = minimum
	}
	if policy.MaxAge < 0 {
		return "forever", nil
	}
	if policy.MaxAge == 0 {
		return "none", nil
	}
	return fmt.Sprintf("%dd", int(policy.MaxAge/(24*time.Hour))), nil
}

type complianceEventCount struct {
	Name  string
	Count int
}

var complianceRetention = map[string]string{
	"gdpr": "1 year", "ccpa": "1 year", "hipaa": "6 years", "soc2": "1 year",
	"pci_dss": "1 year", "iso27001": "3 years", "fedramp": "not specified by the template",
	"lgpd": "not specified by the template", "pipeda": "not specified by the template",
	"appi": "not specified by the template", "pdpa": "not specified by the template",
}

func enabledComplianceStandards(c config.ComplianceConfig) []string {
	all := []struct {
		name    string
		enabled bool
	}{
		{"gdpr", c.GDPR},
		{"ccpa", c.CCPA},
		{"hipaa", c.HIPAA},
		{"soc2", c.SOC2},
		{"pci_dss", c.PCIDSS},
		{"iso27001", c.ISO27001},
		{"fedramp", c.FedRAMP},
		{"lgpd", c.LGPD},
		{"pipeda", c.PIPEDA},
		{"appi", c.APPI},
		{"pdpa", c.PDPA},
	}
	var enabled []string
	for _, item := range all {
		if item.enabled {
			enabled = append(enabled, item.name)
		}
	}
	return enabled
}

func recentComplianceEvents(path string, since time.Time) ([]complianceEventCount, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type auditSummary struct {
		Time     time.Time `json:"time"`
		Event    string    `json:"event"`
		Category string    `json:"category"`
	}
	counts := make(map[string]int)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var entry auditSummary
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && entry.Time.After(since) && (entry.Category == "compliance" || strings.HasPrefix(entry.Event, "compliance.")) {
			counts[entry.Event]++
		}
	}
	scanErr := scanner.Err()
	closeErr := file.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close audit log after reading compliance events: %w", closeErr)
	}
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]complianceEventCount, 0, len(names))
	for _, name := range names {
		result = append(result, complianceEventCount{Name: name, Count: counts[name]})
	}
	return result, nil
}
