package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/display"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/security"
)

// pgpActor returns the identifier to attribute PGP maintenance actions to in
// the audit log. There is no session/user system for the CLI, so the OS user
// running the command is the best available identity.
func pgpActor() string {
	u, err := user.Current()
	if err != nil || u.Username == "" {
		return "unknown"
	}
	return u.Username
}

// pgpAuditIP is the placeholder "IP" recorded for CLI-originated audit
// entries — there is no network request to read a real address from.
const pgpAuditIP = "cli-local"

// pgpAuthorized implements the "server.token OR root" gate required by
// AI.md PART 5 "Sensitive Operations" for every --maintenance pgp <action>.
// A caller running as root is always authorized. Otherwise the SEARCH_TOKEN
// environment variable must hold the current operator bearer token.
func pgpAuthorized(cfg *config.Config) bool {
	if config.IsPrivileged() {
		return true
	}
	if cfg.Server.Token == "" {
		return false
	}
	presented := os.Getenv("SEARCH_TOKEN")
	if presented == "" {
		return false
	}
	expected := sha256.Sum256([]byte(cfg.Server.Token))
	got := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(expected[:], got[:]) == 1
}

// pgpRequireAuthorized loads config and enforces pgpAuthorized, printing the
// standard error and exiting when authorization fails.
func pgpRequireAuthorized() *config.Config {
	cfg, err := config.Initialize()
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_load_config", "error", err))
		exitFunc(78)
		return nil
	}
	if !pgpAuthorized(cfg) {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.not_authorized"))
		fmt.Fprintln(os.Stderr, "   "+serverCLIText("server_cli.pgp.requires_operator_auth"))
		exitFunc(77)
		return nil
	}
	return cfg
}

// pgpReadLine prompts and reads one line of free-form text from stdin
// (fmt.Scanln stops at the first space, which reason text needs to allow).
func pgpReadLine(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// pgpSensitiveConfirm implements the extra "typed confirmation" gate AI.md
// PART 5 requires on top of pgpAuthorized for export private / import /
// delete: re-prompt for the operator token and collect the reason text the
// operator must supply for the audit entry.
func pgpSensitiveConfirm(cfg *config.Config) (reasonText string, exitCode int) {
	token := pgpReadLine(serverCLIText("server_cli.pgp.reenter_operator_token"))
	if cfg.Server.Token == "" || token == "" {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.token_confirmation_failed"))
		return "", 77
	}
	expected := sha256.Sum256([]byte(cfg.Server.Token))
	got := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(expected[:], got[:]) != 1 {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.token_confirmation_failed"))
		return "", 77
	}
	reasonText = pgpReadLine(serverCLIText("server_cli.pgp.reason_prompt"))
	if reasonText == "" {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.reason_required"))
		return "", 65
	}
	return reasonText, 0
}

// pgpAuditLogger opens the audit logger used by every pgp subcommand.
func pgpAuditLogger() *logging.AuditLogger {
	return logging.NewManager(config.GetLogDir()).Audit()
}

// pgpOpenDB opens (and schema-initializes) the server database so PGP
// keypair metadata (AI.md PART 11 "Keypair properties stored in DB") can be
// persisted. Callers must Close() the returned manager.
func pgpOpenDB() (*database.DatabaseManager, error) {
	dbCfg := &database.Config{
		Driver:   "sqlite",
		DataDir:  config.GetDatabaseDir(),
		MaxOpen:  10,
		MaxIdle:  5,
		Lifetime: 300,
	}
	dm, err := database.NewDatabaseManager(dbCfg)
	if err != nil {
		return nil, err
	}
	if err := database.InitSchema(context.Background(), dm); err != nil {
		return nil, errors.Join(err, dm.Close())
	}
	return dm, nil
}

func pgpMigrateLegacyReports(cfg *config.Config, dm *database.DatabaseManager) error {
	if cfg == nil || dm == nil || dm.ServerDB() == nil {
		return fmt.Errorf("security report migration requires initialized config and database")
	}
	previousEncryptionKey, _, err := security.EnsurePreviousEncryptionKey(context.Background(), dm.ServerDB(), cfg.Server.Security.EncryptionKey, cfg.Server.Security.EncryptionKeyPrevious, cfg.Server.Security.EncryptionKeyPreviousExpires)
	if err != nil {
		return fmt.Errorf("load previous encryption key: %w", err)
	}
	var publicKey, encryptedPrivateKey string
	if value, err := security.LoadPublicKey(config.GetConfigDir()); err == nil {
		publicKey = value
	}
	if value, err := security.LoadEncryptedPrivateKey(config.GetConfigDir()); err == nil {
		encryptedPrivateKey = value
	}
	if _, err := security.MigrateLegacyReportPlaintext(context.Background(), dm.ServerDB(), cfg.Server.Security.EncryptionKey, previousEncryptionKey, publicKey, encryptedPrivateKey, cfg.Server.Security.InstallationSecret); err != nil {
		return err
	}
	_, err = security.MigrateSecurityThanks(context.Background(), dm.ServerDB())
	return err
}

// pgpInsertKeypairRow records a freshly generated (or rotated) keypair in
// {prefix}pgp_keypair per AI.md PART 11.
func pgpInsertKeypairRow(dm *database.DatabaseManager, fingerprint string, expiresAt time.Time) error {
	db := dm.ServerDB()
	table := database.ServerTableName(db, "pgp_keypair")
	_, err := db.Exec(context.Background(),
		fmt.Sprintf("INSERT INTO %s (fingerprint, expires_at) VALUES (?, ?)", table),
		fingerprint, expiresAt.UTC().Format(time.RFC3339))
	return err
}

// pgpCurrentKeypairRow returns the fingerprint of the most recent, non-revoked
// keypair row, or "" if none exists.
func pgpCurrentKeypairRow(dm *database.DatabaseManager) (id int64, fingerprint string, err error) {
	db := dm.ServerDB()
	table := database.ServerTableName(db, "pgp_keypair")
	row := db.QueryRow(context.Background(),
		fmt.Sprintf("SELECT id, fingerprint FROM %s WHERE revoked = 0 ORDER BY id DESC LIMIT 1", table))
	if err := row.Scan(&id, &fingerprint); err != nil {
		if err == sql.ErrNoRows {
			return 0, "", nil
		}
		return 0, "", err
	}
	return id, fingerprint, nil
}

// pgpMarkPublished records the keyservers a key has been published to.
func pgpMarkPublished(dm *database.DatabaseManager, id int64, results []security.KeyserverPublishResult) error {
	published := make(map[string]string, len(results))
	for _, r := range results {
		if r.Published {
			published[r.Keyserver] = time.Now().UTC().Format(time.RFC3339)
		}
	}
	data, err := json.Marshal(published)
	if err != nil {
		return err
	}
	db := dm.ServerDB()
	table := database.ServerTableName(db, "pgp_keypair")
	_, err = db.Exec(context.Background(),
		fmt.Sprintf("UPDATE %s SET keyservers_published = ? WHERE id = ?", table),
		string(data), id)
	return err
}

// pgpMarkRevoked flags the row for a deleted keypair as revoked.
func pgpMarkRevoked(dm *database.DatabaseManager, id int64) error {
	db := dm.ServerDB()
	table := database.ServerTableName(db, "pgp_keypair")
	_, err := db.Exec(context.Background(),
		fmt.Sprintf("UPDATE %s SET revoked = 1 WHERE id = ?", table), id)
	return err
}

// pgpPublish publishes the current public key to every configured keyserver,
// retrying failures with exponential backoff, and reports/records the
// outcome. Shared by "generate", "rotate" (auto-publish) and "publish".
func pgpPublish(cfg *config.Config, dm *database.DatabaseManager, id int64) {
	if len(cfg.Server.Web.Security.Keyservers) == 0 {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLIText("server_cli.pgp.no_keyservers"))
		return
	}
	pubArmor, err := security.LoadPublicKey(config.GetConfigDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_load_public_key", "error", err))
		return
	}
	fingerprint, _ := security.KeyFingerprint(pubArmor)

	client := &http.Client{Timeout: 30 * time.Second}
	results, err := security.PublishKeyserversOnce(context.Background(), client, config.GetConfigDir(), pubArmor, cfg.Server.Web.Security.Keyservers, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.failed_persist_retry_state", "error", err))
	}
	success := true
	for _, r := range results {
		if r.Published {
			fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLITf("server_cli.pgp.published_to", "keyserver", r.Keyserver))
		} else {
			success = false
			fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_publish_to", "keyserver", r.Keyserver, "error", r.Err))
		}
	}

	if dm != nil && id != 0 {
		if err := pgpMarkPublished(dm, id, results); err != nil {
			fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.failed_record_publish_state", "error", err))
		}
	}
	pgpAuditLogger().LogPGPKeyPublished(pgpActor(), pgpAuditIP, fingerprint, cfg.Server.Web.Security.Keyservers, success)
}

// pgpExportRateLimitPath is the marker file used to enforce "1 per hour per
// operator" on --maintenance pgp export private (AI.md PART 11).
func pgpExportRateLimitPath(configDir string) string {
	return filepath.Join(configDir, security.PGPDirName, ".export_private_last")
}

// pgpCheckExportRateLimit returns an error if a private-key export happened
// within the last hour.
func pgpCheckExportRateLimit(configDir string) error {
	path := pgpExportRateLimitPath(configDir)
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if elapsed := time.Since(info.ModTime()); elapsed < time.Hour {
		return errors.New(serverCLITf("server_cli.pgp.export_rate_limited", "elapsed", elapsed.Round(time.Second), "wait", (time.Hour - elapsed).Round(time.Second)))
	}
	return nil
}

// pgpRecordExport stamps the rate-limit marker after a successful export.
func pgpRecordExport(configDir string) error {
	path := pgpExportRateLimitPath(configDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0o600)
}

// runPGPGenerate implements --maintenance pgp generate.
func runPGPGenerate() {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " " + serverCLIText("server_cli.pgp.generate_heading"))
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}

	if _, err := security.LoadPublicKey(config.GetConfigDir()); err == nil {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLIText("server_cli.pgp.keypair_exists"))
		exitFunc(1)
		return
	}

	kp, err := security.GenerateKeypair(cfg.Server.Title, cfg.Server.Web.Security.Contact, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_generate", "error", err))
		exitFunc(1)
		return
	}
	configDir := config.GetConfigDir()
	if err := security.SavePublicKey(configDir, kp.PublicKeyArmor); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_save_public_key", "error", err))
		exitFunc(1)
		return
	}
	if err := security.SaveEncryptedPrivateKey(configDir, kp.EncryptedPrivateKey); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_save_private_key", "error", err))
		exitFunc(1)
		return
	}

	dm, err := pgpOpenDB()
	var id int64
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.metadata_database_unavailable", "error", err))
	} else {
		defer func() {
			if err := dm.Close(); err != nil {
				slog.Warn("failed to close PGP metadata database", "error", err)
			}
		}()
		if err := pgpInsertKeypairRow(dm, kp.Fingerprint, kp.ExpiresAt); err != nil {
			fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.failed_record_keypair", "error", err))
		} else {
			id, _, _ = pgpCurrentKeypairRow(dm)
		}
	}

	pgpAuditLogger().LogPGPKeyGenerated(pgpActor(), pgpAuditIP, kp.Fingerprint)

	fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.pgp.keypair_generated"))
	fmt.Println("   " + serverCLITf("server_cli.pgp.fingerprint", "fingerprint", security.FormatFingerprint(kp.Fingerprint)))
	fmt.Println("   " + serverCLITf("server_cli.pgp.expires", "date", kp.ExpiresAt.Format("2006-01-02")))
	fmt.Println("   " + serverCLITf("server_cli.pgp.public_key_path", "path", filepath.Join(configDir, security.PGPDirName, security.PGPPublicKeyFilename)))
	fmt.Println()

	pgpPublish(cfg, dm, id)
}

// runPGPRotate implements --maintenance pgp rotate.
func runPGPRotate() {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " " + serverCLIText("server_cli.pgp.rotate_heading"))
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	configDir := config.GetConfigDir()
	reportLock, err := security.LockReportMutation(configDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_lock_reports", "error", err))
		exitFunc(1)
		return
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			if err := reportLock.Unlock(); err != nil {
				slog.Error("failed to unlock security reports after PGP rotation", "error", err)
			}
		}
	}()
	if err := security.CheckPGPRotationFiles(configDir); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.cannot_start_rotation", "error", err))
		exitFunc(1)
		return
	}

	oldPubArmor, err := security.LoadPublicKey(configDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.no_keypair_to_rotate"))
		exitFunc(1)
		return
	}
	oldFingerprint, err := security.KeyFingerprint(oldPubArmor)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_fingerprint_previous", "error", err))
		exitFunc(1)
		return
	}

	// Unlock the outgoing private key now, before it is archived below, so we
	// can sign the incoming pubkey with it (AI.md PART 11 "Rotate": "signs
	// the new pubkey with the old key" — establishes chain of custody for
	// recipients verifying the rotation).
	oldPrivEncrypted, err := security.LoadEncryptedPrivateKey(configDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_load_previous_private", "error", err))
		exitFunc(1)
		return
	}
	oldKey, err := security.UnlockPrivateKey(oldPrivEncrypted, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_unlock_previous_private", "error", err))
		exitFunc(1)
		return
	}

	kp, err := security.GenerateKeypair(cfg.Server.Title, cfg.Server.Web.Security.Contact, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_generate_rotation_keypair", "error", err))
		exitFunc(1)
		return
	}
	rotationSig, err := security.SignDetached(oldKey, []byte(kp.PublicKeyArmor))
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_sign_new_key", "error", err))
		exitFunc(1)
		return
	}
	if err := security.StagePGPRotation(configDir, kp, rotationSig, time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_stage_keypair", "error", err))
		exitFunc(1)
		return
	}

	dm, err := pgpOpenDB()
	if err != nil {
		cleanupErr := security.DiscardPGPRotation(configDir)
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_open_database", "error", errors.Join(err, cleanupErr)))
		exitFunc(1)
		return
	}
	defer func() {
		if err := dm.Close(); err != nil {
			slog.Warn("failed to close PGP metadata database", "error", err)
		}
	}()
	if err := pgpMigrateLegacyReports(cfg, dm); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.legacy_report_migration_failed", "error", err))
		exitFunc(1)
		return
	}
	serverDB := dm.ServerDB()
	keyTable := database.ServerTableName(serverDB, "pgp_keypair")
	rotatedAt := time.Now().UTC()
	err = serverDB.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := security.ReencryptPGPReportBodiesTx(context.Background(), tx, serverDB, oldKey, kp.PublicKeyArmor); err != nil {
			return err
		}
		_, err := database.ExecContext(context.Background(), tx,
			fmt.Sprintf("INSERT INTO %s (fingerprint, created_at, expires_at, last_rotated_at, revoked) VALUES (?, ?, ?, ?, 0)", keyTable),
			kp.Fingerprint, kp.CreatedAt.UTC().Format(time.RFC3339), kp.ExpiresAt.UTC().Format(time.RFC3339), rotatedAt.Format(time.RFC3339))
		return err
	})
	if err != nil {
		cleanupErr := security.DiscardPGPRotation(configDir)
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.rotation_transaction_failed", "error", errors.Join(err, cleanupErr)))
		exitFunc(1)
		return
	}
	if err := security.FinalizePGPRotation(configDir, kp.Fingerprint); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.rotation_promotion_failed", "error", err))
		exitFunc(1)
		return
	}
	id, fingerprint, err := pgpCurrentKeypairRow(dm)
	if err != nil || fingerprint != kp.Fingerprint {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.rotation_metadata_failed", "error", err))
		exitFunc(1)
		return
	}
	if err := reportLock.Unlock(); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.report_unlock_failed", "error", err))
		exitFunc(1)
		return
	}
	lockHeld = false

	pgpAuditLogger().LogPGPKeyRotated(pgpActor(), pgpAuditIP, oldFingerprint, kp.Fingerprint)

	fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.pgp.keypair_rotated"))
	fmt.Println("   " + serverCLITf("server_cli.pgp.old_fingerprint_grace", "fingerprint", security.FormatFingerprint(oldFingerprint)))
	fmt.Println("   " + serverCLITf("server_cli.pgp.fingerprint", "fingerprint", security.FormatFingerprint(kp.Fingerprint)))
	fmt.Println()

	pgpPublish(cfg, dm, id)
}

// runPGPPublish implements --maintenance pgp publish.
func runPGPPublish() {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " " + serverCLIText("server_cli.pgp.publish_heading"))
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}

	dm, err := pgpOpenDB()
	var id int64
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.failed_open_database", "error", err))
	} else {
		defer func() {
			if err := dm.Close(); err != nil {
				slog.Warn("failed to close PGP metadata database", "error", err)
			}
		}()
		id, _, _ = pgpCurrentKeypairRow(dm)
	}
	pgpPublish(cfg, dm, id)
}

// runPGPExportPublic implements --maintenance pgp export public [path].
func runPGPExportPublic(path string) {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " " + serverCLIText("server_cli.pgp.export_public_heading"))
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	_ = cfg

	armor, err := security.LoadPublicKey(config.GetConfigDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_load_public_key", "error", err))
		exitFunc(1)
		return
	}
	if path == "" {
		fmt.Println(armor)
		return
	}
	if err := os.WriteFile(path, []byte(armor), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_write_file", "path", path, "error", err))
		exitFunc(1)
		return
	}
	fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLITf("server_cli.pgp.public_key_written", "path", path))
}

// runPGPExportPrivate implements --maintenance pgp export private <path>.
func runPGPExportPrivate(path string) {
	if path == "" {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.destination_required"))
		fmt.Fprintln(os.Stderr, serverCLIText("server_cli.pgp.usage_export_private"))
		exitFunc(2)
		return
	}
	fmt.Println(display.Emoji("🔐", "[PGP]") + " " + serverCLIText("server_cli.pgp.export_private_heading"))
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	configDir := config.GetConfigDir()

	if err := pgpCheckExportRateLimit(configDir); err != nil {
		fmt.Fprintf(os.Stderr, display.Emoji("❌", "[ERROR]")+" %v\n", err)
		exitFunc(1)
		return
	}

	fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLIText("server_cli.pgp.private_key_warning"))
	reasonText, exitCode := pgpSensitiveConfirm(cfg)
	if exitCode != 0 {
		exitFunc(exitCode)
		return
	}

	privArmor, err := security.LoadEncryptedPrivateKey(configDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_load_private_key", "error", err))
		exitFunc(1)
		return
	}
	unlocked, err := security.UnlockPrivateKey(privArmor, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_unlock_private_key", "error", err))
		exitFunc(1)
		return
	}
	decrypted, err := unlocked.Armor()
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_armor_private_key", "error", err))
		exitFunc(1)
		return
	}
	if err := os.WriteFile(path, []byte(decrypted), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_write_file", "path", path, "error", err))
		exitFunc(1)
		return
	}
	if err := pgpRecordExport(configDir); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.failed_record_export_marker", "error", err))
	}

	pgpAuditLogger().LogPGPPrivateKeyExport(pgpActor(), pgpAuditIP, reasonText)

	fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLITf("server_cli.pgp.private_key_exported", "path", path))
}

// runPGPImport implements --maintenance pgp import <file>.
func runPGPImport(file string) {
	if file == "" {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.source_required"))
		fmt.Fprintln(os.Stderr, serverCLIText("server_cli.pgp.usage_import"))
		exitFunc(2)
		return
	}
	fmt.Println(display.Emoji("🔐", "[PGP]") + " " + serverCLIText("server_cli.pgp.import_heading"))
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}

	reasonText, exitCode := pgpSensitiveConfirm(cfg)
	if exitCode != 0 {
		exitFunc(exitCode)
		return
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_read_file", "path", file, "error", err))
		exitFunc(1)
		return
	}

	kp, err := security.ImportPrivateKey(string(raw), cfg.Server.Title, cfg.Server.Web.Security.Contact, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_import_key", "error", err))
		exitFunc(1)
		return
	}

	identityMismatch := !security.IdentityMatches(kp.Key, cfg.Server.Title, cfg.Server.Web.Security.Contact)
	if identityMismatch {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLIText("server_cli.pgp.identity_mismatch"))
		confirm := pgpReadLine(serverCLIText("server_cli.pgp.import_anyway_prompt"))
		if confirm != "yes" {
			fmt.Println(serverCLIText("admin.status.cancelled"))
			return
		}
	}

	configDir := config.GetConfigDir()
	if err := security.SavePublicKey(configDir, kp.PublicKeyArmor); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_save_public_key", "error", err))
		exitFunc(1)
		return
	}
	if err := security.SaveEncryptedPrivateKey(configDir, kp.EncryptedPrivateKey); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_save_private_key", "error", err))
		exitFunc(1)
		return
	}

	dm, err := pgpOpenDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.metadata_database_unavailable", "error", err))
	} else {
		defer func() {
			if err := dm.Close(); err != nil {
				slog.Warn("failed to close PGP metadata database", "error", err)
			}
		}()
		if err := pgpInsertKeypairRow(dm, kp.Fingerprint, kp.ExpiresAt); err != nil {
			fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.failed_record_keypair", "error", err))
		}
	}

	pgpAuditLogger().LogPGPPrivateKeyImport(pgpActor(), pgpAuditIP, reasonText, identityMismatch)

	fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.pgp.private_key_imported"))
	fmt.Println("   " + serverCLITf("server_cli.pgp.fingerprint", "fingerprint", security.FormatFingerprint(kp.Fingerprint)))
}

// runPGPDelete implements --maintenance pgp delete.
func runPGPDelete() {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " " + serverCLIText("server_cli.pgp.delete_heading"))
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	configDir := config.GetConfigDir()

	pubArmor, err := security.LoadPublicKey(configDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.no_keypair_to_delete"))
		exitFunc(1)
		return
	}
	fingerprint, _ := security.KeyFingerprint(pubArmor)

	fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLIText("server_cli.pgp.delete_reports_warning"))
	reasonText, exitCode := pgpSensitiveConfirm(cfg)
	if exitCode != 0 {
		exitFunc(exitCode)
		return
	}
	confirm := pgpReadLine(serverCLIText("server_cli.pgp.delete_confirmation_prompt"))
	if confirm != "DELETE" {
		fmt.Println(serverCLIText("admin.status.cancelled"))
		return
	}

	if err := security.DeleteKeypairFiles(configDir); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.failed_delete_keypair", "error", err))
		exitFunc(1)
		return
	}

	cfg.Server.Web.Security.PublishPGPKey = false
	if err := cfg.Save(config.GetConfigPath()); err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.failed_update_config", "error", err))
	}

	dm, err := pgpOpenDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, display.Emoji("⚠️", "[WARN]") + " " + serverCLITf("server_cli.pgp.failed_open_database", "error", err))
	} else {
		defer func() {
			if err := dm.Close(); err != nil {
				slog.Warn("failed to close PGP metadata database", "error", err)
			}
		}()
		if id, _, err := pgpCurrentKeypairRow(dm); err == nil && id != 0 {
			_ = pgpMarkRevoked(dm, id)
		}
	}

	pgpAuditLogger().LogPGPPrivateKeyDelete(pgpActor(), pgpAuditIP, fingerprint, reasonText)

	fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.pgp.keypair_deleted"))
	fmt.Println("   " + serverCLIText("server_cli.pgp.security_txt_removed_next_request"))
}
