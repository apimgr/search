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
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/display"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/security"
	"github.com/google/uuid"
)

// installationSecretGrace is how long the pre-rotation installation_secret
// stays valid for in-flight {security_id} URLs (AI.md PART 11: 7 days).
const installationSecretGrace = 7 * 24 * time.Hour

// encryptionKeyGrace is how long the pre-rotation encryption_key is retained
// so any at-rest data not yet re-encrypted still decrypts (AI.md PART 11: 30 days).
const encryptionKeyGrace = 30 * 24 * time.Hour

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
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown secret action: %s\n", action)
		printMaintenanceSecretHelp()
		exitFunc(1)
		return
	}

	switch name {
	case database.SecretInstallation, "encryption_key":
		// rotatable
	case database.SecretCookieSigning, database.SecretCSRFToken:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" %s is auto-rotated and cannot be rotated manually.\n", name)
		exitFunc(1)
		return
	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown secret: %s\n", name)
		fmt.Println("Rotatable secrets: installation_secret, encryption_key")
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
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Rotation failed: %v\n", err)
		exitFunc(1)
		return
	}
	fmt.Printf(display.Emoji("✅", "[OK]")+" %s rotated.\n", name)
}

// printMaintenanceSecretHelp documents the secret subcommand.
func printMaintenanceSecretHelp() {
	fmt.Println("Secret rotation:")
	fmt.Println()
	fmt.Println("  rotate installation_secret   Rotate the HMAC root secret (7-day grace)")
	fmt.Println("  rotate encryption_key        Rotate the AES-256-GCM at-rest key (30-day grace)")
	fmt.Println()
	fmt.Println("cookie_signing_key and csrf_token_secret are auto-rotated and cannot")
	fmt.Println("be rotated manually.")
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
	cfg.Server.Security.InstallationSecretPreviousExpires =
		time.Now().UTC().Add(installationSecretGrace).Format(time.RFC3339)
	cfg.Server.Security.InstallationSecret = newSecret
	if err := cfg.Save(config.GetConfigPath()); err != nil {
		return err
	}

	pgpAuditLogger().LogEvent(
		logging.AuditActionInstallationSecretRotated,
		logging.AuditCategorySecurity,
		logging.AuditSeverityCritical,
		logging.AuditActor{Type: "admin", Username: pgpActor(), IP: pgpAuditIP},
		&logging.AuditTarget{Type: "secret", Name: database.SecretInstallation},
		"success",
		map[string]interface{}{"grace_expires": cfg.Server.Security.InstallationSecretPreviousExpires},
		reasonText,
	)
	fmt.Println("   Previous secret stays valid for 7 days (in-flight security_id URLs).")
	return nil
}

// rotateEncryptionKey generates a new AES-256-GCM key, re-encrypts every
// AES-encrypted security report body under it, keeps the previous key for the
// 30-day grace window and records security.encryption_key_rotated.
func rotateEncryptionKey(cfg *config.Config, reasonText string) error {
	oldKey := cfg.Server.Security.EncryptionKey
	newKey := config.GenerateBase64Secret()

	reencrypted, err := reencryptSecurityReports(oldKey, newKey)
	if err != nil {
		return err
	}

	cfg.Server.Security.EncryptionKeyPrevious = oldKey
	cfg.Server.Security.EncryptionKeyPreviousExpires =
		time.Now().UTC().Add(encryptionKeyGrace).Format(time.RFC3339)
	cfg.Server.Security.EncryptionKey = newKey
	if err := cfg.Save(config.GetConfigPath()); err != nil {
		return err
	}

	pgpAuditLogger().LogEvent(
		logging.AuditActionEncryptionKeyRotated,
		logging.AuditCategorySecurity,
		logging.AuditSeverityCritical,
		logging.AuditActor{Type: "admin", Username: pgpActor(), IP: pgpAuditIP},
		&logging.AuditTarget{Type: "secret", Name: "encryption_key"},
		"success",
		map[string]interface{}{
			"reports_reencrypted": reencrypted,
			"grace_expires":       cfg.Server.Security.EncryptionKeyPreviousExpires,
		},
		reasonText,
	)
	fmt.Printf("   Re-encrypted %d security report bodies.\n", reencrypted)
	fmt.Println("   Previous key stays valid for 30 days.")
	return nil
}

// reencryptSecurityReports rewrites every AES-256-GCM encrypted report body
// under newKey, returning how many rows were rewritten.
func reencryptSecurityReports(oldKey, newKey string) (int, error) {
	dm, err := pgpOpenDB()
	if err != nil {
		return 0, err
	}
	defer closeutil.Close(dm)

	db := dm.ServerDB()
	table := database.ServerTableName(db, "security_reports")
	ctx := context.Background()
	rows, err := db.Query(ctx,
		fmt.Sprintf("SELECT tracking_id, encrypted_body FROM %s WHERE encryption_method = ?", table),
		security.EncryptionMethodAESGCM)
	if err != nil {
		return 0, err
	}

	type pending struct {
		id   string
		body string
	}
	var updates []pending
	for rows.Next() {
		var id string
		var body []byte
		if err := rows.Scan(&id, &body); err != nil {
			return 0, errors.Join(err, rows.Close())
		}
		plaintext, decErr := security.DecryptAESGCM(oldKey, string(body))
		if decErr != nil {
			return 0, fmt.Errorf("decrypt report %s: %w", id, errors.Join(decErr, rows.Close()))
		}
		encoded, encErr := security.EncryptAESGCM(newKey, plaintext)
		if encErr != nil {
			return 0, fmt.Errorf("re-encrypt report %s: %w", id, errors.Join(encErr, rows.Close()))
		}
		updates = append(updates, pending{id: id, body: encoded})
	}
	if err := rows.Err(); err != nil {
		return 0, errors.Join(err, rows.Close())
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	for _, u := range updates {
		if _, err := db.Exec(ctx,
			fmt.Sprintf("UPDATE %s SET encrypted_body = ?, updated_at = CURRENT_TIMESTAMP WHERE tracking_id = ?", table),
			[]byte(u.body), u.id); err != nil {
			return 0, err
		}
	}
	return len(updates), nil
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
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown token action: %s\n", action)
		printMaintenanceTokenHelp()
		exitFunc(1)
		return
	}

	pgpRequireAuthorized()

	dm, err := pgpOpenDB()
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to open database: %v\n", err)
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
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to list tokens: %v\n", err)
			exitFunc(1)
			return
		}
		defer closeutil.Close(rows)

		found := 0
		for rows.Next() {
			var tokenPrefix, ownerID, kind, name string
			var createdAt, expiresAt, revokedAt sql.NullInt64
			if err := rows.Scan(&tokenPrefix, &ownerID, &kind, &name, &createdAt, &expiresAt, &revokedAt); err != nil {
				fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to read token row: %v\n", err)
				exitFunc(1)
				return
			}
			if found == 0 {
				fmt.Println("Active tokens:")
			}
			found++
			status := "active"
			if revokedAt.Valid {
				status = "revoked"
			} else if expiresAt.Valid && expiresAt.Int64 <= time.Now().Unix() {
				status = "expired"
			}
			fmt.Printf("  %-12s %-7s %-20s owner=%s status=%s", tokenPrefix, kind, name, ownerID, status)
			if createdAt.Valid {
				fmt.Printf(" created=%s", time.Unix(createdAt.Int64, 0).UTC().Format(time.RFC3339))
			}
			fmt.Println()
		}
		if err := rows.Err(); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to read tokens: %v\n", err)
			exitFunc(1)
			return
		}
		if found == 0 {
			fmt.Println("No active tokens.")
		}
		return
	}

	if prefix == "" {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " Please specify the token prefix to revoke")
		fmt.Println("Usage: " + config.ProjectName + " --maintenance token revoke <prefix>")
		exitFunc(1)
		return
	}

	result, err := db.Exec(ctx,
		fmt.Sprintf("UPDATE %s SET active = 0, revoked_at = ?, revoked_reason = 'operator_action' WHERE token_prefix = ? AND owner_id IS NOT NULL AND active = 1 AND revoked_at IS NULL", table), time.Now().Unix(), prefix)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to revoke token: %v\n", err)
		exitFunc(1)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" No active token with prefix %s\n", prefix)
		exitFunc(1)
		return
	}
	if affected > 0 {
		if _, err := db.Exec(ctx, "INSERT INTO "+database.ServerTableName(db, "audit_log")+" (id, timestamp, actor_type, action, resource_type, resource_id, details) VALUES (?, ?, 'operator', 'token.revoked', 'api_token', ?, ?)", uuid.NewString(), time.Now().Unix(), prefix, `{"reason":"operator_action"}`); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Token revoked but audit write failed: %v\n", err)
			exitFunc(1)
			return
		}
	}

	pgpAuditLogger().LogTokenRevoke(pgpActor(), pgpAuditIP, prefix)
	fmt.Printf(display.Emoji("✅", "[OK]")+" Revoked %d token(s) with prefix %s\n", affected, prefix)
}

// printMaintenanceTokenHelp documents the token subcommand.
func printMaintenanceTokenHelp() {
	fmt.Println("Token management:")
	fmt.Println()
	fmt.Println("  list               List active tokens (prefix + name)")
	fmt.Println("  revoke <prefix>    Revoke the token with the given prefix")
}

// personalDataExport is the JSON document produced by
// `--maintenance data export` (AI.md PART 12: GDPR data export / CCPA
// data disclosure). It never contains secrets, hashes, or encrypted material.
type personalDataExport struct {
	GeneratedAt string                   `json:"generated_at"`
	Subject     string                   `json:"subject,omitempty"`
	DataSold    bool                     `json:"data_sold"`
	Retention   string                   `json:"retention_period,omitempty"`
	Alerts      []map[string]interface{} `json:"search_alerts"`
	AuditEvents []map[string]interface{} `json:"audit_events"`
	BlockedIPs  []map[string]interface{} `json:"blocked_ips"`
}

// runMaintenanceData implements `--maintenance data export|delete [email]`
// per AI.md PART 12 (GDPR right of access / right to erasure, CCPA data
// disclosure / right to delete). With no email, the whole instance's personal
// data is covered; with an email, only that data subject's.
func runMaintenanceData(action, subject string) {
	switch action {
	case "export", "delete":
		// handled below
	case "help", "--help", "":
		printMaintenanceDataHelp()
		return
	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown data action: %s\n", action)
		printMaintenanceDataHelp()
		exitFunc(1)
		return
	}

	cfg := pgpRequireAuthorized()

	dm, err := pgpOpenDB()
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to open database: %v\n", err)
		exitFunc(1)
		return
	}
	defer closeutil.Close(dm)

	if action == "export" {
		if err := exportPersonalData(cfg, dm, subject); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Export failed: %v\n", err)
			exitFunc(1)
		}
		return
	}
	if err := deletePersonalData(dm, subject); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Deletion failed: %v\n", err)
		exitFunc(1)
	}
}

// printMaintenanceDataHelp documents the data subcommand.
func printMaintenanceDataHelp() {
	fmt.Println("Data subject requests (GDPR / CCPA):")
	fmt.Println()
	fmt.Println("  export [email]   Write every piece of personal data held to a JSON file")
	fmt.Println("  delete [email]   Erase personal data; audit entries are anonymized, not deleted")
	fmt.Println()
	fmt.Println("Omitting the email covers every data subject on this instance.")
}

// exportPersonalData writes the export document to
// {data_dir}/exports/data-export-{timestamp}.json and prints its path.
func exportPersonalData(cfg *config.Config, dm *database.DatabaseManager, subject string) error {
	ctx := context.Background()
	db := dm.ServerDB()

	doc := personalDataExport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Subject:     subject,
		DataSold:    cfg.Server.Privacy.Data.Sold,
		Retention:   cfg.Server.Privacy.Retention.Period,
		Alerts:      []map[string]interface{}{},
		AuditEvents: []map[string]interface{}{},
		BlockedIPs:  []map[string]interface{}{},
	}

	alertTable := database.ServerTableName(db, "search_alerts")
	alertQuery := fmt.Sprintf(`SELECT id, email, query, category, frequency, status,
		COALESCE(created_from_ip, ''), created_at FROM %s`, alertTable)
	var alertArgs []interface{}
	if subject != "" {
		alertQuery += " WHERE email = ?"
		alertArgs = append(alertArgs, subject)
	}
	if err := scanRowsInto(ctx, db, alertQuery, alertArgs,
		[]string{"id", "email", "query", "category", "frequency", "status", "created_from_ip", "created_at"},
		&doc.Alerts); err != nil {
		return err
	}

	auditTable := database.ServerTableName(db, "audit_log")
	auditQuery := fmt.Sprintf(`SELECT id, timestamp, action, COALESCE(actor_id, ''),
		COALESCE(ip_address, ''), COALESCE(user_agent, '') FROM %s`, auditTable)
	var auditArgs []interface{}
	if subject != "" {
		auditQuery += " WHERE actor_id = ?"
		auditArgs = append(auditArgs, subject)
	}
	if err := scanRowsInto(ctx, db, auditQuery, auditArgs,
		[]string{"id", "timestamp", "action", "actor_id", "ip_address", "user_agent"},
		&doc.AuditEvents); err != nil {
		return err
	}

	// Blocked IPs are only personal data when a specific subject is not named;
	// they are keyed by address, not by an identified person.
	if subject == "" {
		blockedTable := database.ServerTableName(db, "blocked_ips")
		blockedQuery := fmt.Sprintf(
			"SELECT ip_address, COALESCE(reason, ''), created_at FROM %s", blockedTable)
		if err := scanRowsInto(ctx, db, blockedQuery, nil,
			[]string{"ip_address", "reason", "created_at"}, &doc.BlockedIPs); err != nil {
			return err
		}
	}

	dir := filepath.Join(config.GetDataDir(), "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, fmt.Sprintf("data-export-%s.json", time.Now().UTC().Format("20060102-150405")))
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return err
	}

	pgpAuditLogger().LogEvent(
		logging.AuditActionDataExportCompleted,
		logging.AuditCategoryCompliance,
		logging.AuditSeverityInfo,
		logging.AuditActor{Type: "admin", Username: pgpActor(), IP: pgpAuditIP},
		&logging.AuditTarget{Type: "data_export", Name: subject},
		"success",
		map[string]interface{}{"file_size": len(payload), "alerts": len(doc.Alerts)},
		"",
	)

	fmt.Printf(display.Emoji("✅", "[OK]")+" Data export written: %s\n", path)
	fmt.Printf("   Search alerts: %d  Audit events: %d\n", len(doc.Alerts), len(doc.AuditEvents))
	return nil
}

// scanRowsInto runs a query whose selected columns are all text-compatible and
// appends one map per row, keyed by the supplied column names.
func scanRowsInto(ctx context.Context, db *database.DB, query string, args []interface{}, columns []string, out *[]map[string]interface{}) error {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer closeutil.Close(rows)

	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		targets := make([]interface{}, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			return err
		}
		record := make(map[string]interface{}, len(columns))
		for i, name := range columns {
			if values[i].Valid {
				record[name] = values[i].String
			} else {
				record[name] = nil
			}
		}
		*out = append(*out, record)
	}
	return rows.Err()
}

// deletePersonalData erases the data subject's records. Per AI.md PART 12
// ("Right to Erasure vs Retention Requirements") audit entries are anonymized
// rather than deleted so the audit trail survives with the PII removed.
func deletePersonalData(dm *database.DatabaseManager, subject string) error {
	target := subject
	if target == "" {
		target = "EVERY data subject on this instance"
	}
	fmt.Printf("Erase personal data for %s? This cannot be undone. (yes/no): ", target)
	var confirm string
	if _, err := fmt.Scanln(&confirm); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read personal data erasure confirmation: %w", err)
	}
	if confirm != "yes" {
		fmt.Println("Cancelled.")
		return nil
	}

	ctx := context.Background()
	db := dm.ServerDB()
	alertTable := database.ServerTableName(db, "search_alerts")
	resultTable := database.ServerTableName(db, "search_alert_results")
	auditTable := database.ServerTableName(db, "audit_log")
	rateTable := database.ServerTableName(db, "rate_limits")

	const redacted = "[REDACTED]"
	var deletedAlerts int64

	if subject == "" {
		if _, err := db.Exec(ctx, fmt.Sprintf("DELETE FROM %s", resultTable)); err != nil {
			return err
		}
		res, err := db.Exec(ctx, fmt.Sprintf("DELETE FROM %s", alertTable))
		if err != nil {
			return err
		}
		deletedAlerts, _ = res.RowsAffected()
		if _, err := db.Exec(ctx, fmt.Sprintf(
			"UPDATE %s SET ip_address = ?, user_agent = ?, actor_id = ?", auditTable),
			redacted, redacted, redacted); err != nil {
			return err
		}
		if _, err := db.Exec(ctx, fmt.Sprintf("DELETE FROM %s", rateTable)); err != nil {
			return err
		}
	} else {
		if _, err := db.Exec(ctx, fmt.Sprintf(
			"DELETE FROM %s WHERE alert_id IN (SELECT id FROM %s WHERE email = ?)", resultTable, alertTable),
			subject); err != nil {
			return err
		}
		res, err := db.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE email = ?", alertTable), subject)
		if err != nil {
			return err
		}
		deletedAlerts, _ = res.RowsAffected()
		if _, err := db.Exec(ctx, fmt.Sprintf(
			"UPDATE %s SET ip_address = ?, user_agent = ?, actor_id = ? WHERE actor_id = ?", auditTable),
			redacted, redacted, redacted, subject); err != nil {
			return err
		}
	}

	pgpAuditLogger().LogEvent(
		logging.AuditActionDataDeletionCompleted,
		logging.AuditCategoryCompliance,
		logging.AuditSeverityWarning,
		logging.AuditActor{Type: "admin", Username: pgpActor(), IP: pgpAuditIP},
		&logging.AuditTarget{Type: "data_subject", Name: subject},
		"success",
		map[string]interface{}{"alerts_deleted": deletedAlerts},
		"",
	)

	fmt.Printf(display.Emoji("✅", "[OK]")+" Deleted %d search alert(s); audit entries anonymized.\n", deletedAlerts)
	return nil
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
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown compliance action: %s\n", action)
		printMaintenanceComplianceHelp()
		exitFunc(1)
		return
	}

	cfg := pgpRequireAuthorized()
	printComplianceReport(cfg)

	pgpAuditLogger().LogEvent(
		logging.AuditActionComplianceReport,
		logging.AuditCategoryCompliance,
		logging.AuditSeverityInfo,
		logging.AuditActor{Type: "admin", Username: pgpActor(), IP: pgpAuditIP},
		&logging.AuditTarget{Type: "compliance_report", Name: "summary"},
		"success",
		map[string]interface{}{"compliance_enabled": cfg.Server.Compliance.Enabled},
		"",
	)
}

// printMaintenanceComplianceHelp documents the compliance subcommand.
func printMaintenanceComplianceHelp() {
	fmt.Println("Compliance:")
	fmt.Println()
	fmt.Println("  report   Print the compliance summary for this instance")
}

// printComplianceReport renders the operator-facing compliance summary.
func printComplianceReport(cfg *config.Config) {
	yesNo := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}

	fmt.Println("Compliance report")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Printf("  Compliance mode:        %s\n", yesNo(cfg.Server.Compliance.Enabled))
	fmt.Printf("  Backup encryption:      %s\n", yesNo(cfg.Server.Backup.Encryption.Enabled || cfg.Server.Compliance.Enabled))
	fmt.Printf("  TLS enabled:            %s\n", yesNo(cfg.Server.SSL.Enabled))
	fmt.Printf("  HSTS enabled:           %s\n", yesNo(cfg.Server.Security.HSTS.Enabled))
	fmt.Println()
	fmt.Println("Privacy policy (server.privacy):")
	fmt.Printf("  Data sold:              %s\n", yesNo(cfg.Server.Privacy.Data.Sold))
	fmt.Printf("  Stored on this server:  %s\n", yesNo(cfg.Server.Privacy.Data.StoredOnServer))
	fmt.Printf("  Export right offered:   %s\n", yesNo(cfg.Server.Privacy.Retention.ExportAvailable))
	fmt.Printf("  Deletion right offered: %s\n", yesNo(cfg.Server.Privacy.Retention.DeletionAvailable))
	if cfg.Server.Privacy.Retention.Period != "" {
		fmt.Printf("  Retention period:       %s\n", cfg.Server.Privacy.Retention.Period)
	}
	fmt.Println()
	fmt.Println("Data subject requests:")
	fmt.Println("  " + config.ProjectName + " --maintenance data export [email]")
	fmt.Println("  " + config.ProjectName + " --maintenance data delete [email]")
}
