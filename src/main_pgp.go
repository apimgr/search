package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"fmt"
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
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load config: %v\n", err)
		exitFunc(1)
		return nil
	}
	if !pgpAuthorized(cfg) {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " Not authorized.")
		fmt.Println("   Requires root, or SEARCH_TOKEN set to the current server.token.")
		exitFunc(1)
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
func pgpSensitiveConfirm(cfg *config.Config) (reasonText string, ok bool) {
	token := pgpReadLine("Re-enter operator token to confirm: ")
	if cfg.Server.Token == "" || token == "" {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " Operator token confirmation failed.")
		return "", false
	}
	expected := sha256.Sum256([]byte(cfg.Server.Token))
	got := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(expected[:], got[:]) != 1 {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " Operator token confirmation failed.")
		return "", false
	}
	reasonText = pgpReadLine("Reason for this action (recorded in audit.log): ")
	if reasonText == "" {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " A reason is required.")
		return "", false
	}
	return reasonText, true
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
		dm.Close()
		return nil, err
	}
	return dm, nil
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

// pgpMarkRotated updates last_rotated_at on the given row id.
func pgpMarkRotated(dm *database.DatabaseManager, id int64) error {
	db := dm.ServerDB()
	table := database.ServerTableName(db, "pgp_keypair")
	_, err := db.Exec(context.Background(),
		fmt.Sprintf("UPDATE %s SET last_rotated_at = ? WHERE id = ?", table),
		time.Now().UTC().Format(time.RFC3339), id)
	return err
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

// pgpKeyserversStatePath returns the path to the per-keyserver publish state
// file described in AI.md PART 11 "Backup Integration".
func pgpKeyserversStatePath(configDir string) string {
	return filepath.Join(configDir, security.PGPDirName, security.PGPKeyserversStateFilename)
}

// pgpWriteKeyserversState persists which keyservers a key has already been
// published to, so a later restore does not double-submit.
func pgpWriteKeyserversState(configDir string, results []security.KeyserverPublishResult) error {
	state := map[string]string{}
	path := pgpKeyserversStatePath(configDir)
	if existing, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(existing, &state)
	}
	for _, r := range results {
		if r.Published {
			state[r.Keyserver] = time.Now().UTC().Format(time.RFC3339)
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// pgpPublish publishes the current public key to every configured keyserver,
// retrying failures with exponential backoff, and reports/records the
// outcome. Shared by "generate", "rotate" (auto-publish) and "publish".
func pgpPublish(cfg *config.Config, dm *database.DatabaseManager, id int64) {
	if len(cfg.Server.Web.Security.Keyservers) == 0 {
		fmt.Println(display.Emoji("⚠️", "[WARN]") + " No keyservers configured (web.security.keyservers) — skipping publish.")
		return
	}
	pubArmor, err := security.LoadPublicKey(config.GetConfigDir())
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load public key: %v\n", err)
		return
	}
	fingerprint, _ := security.KeyFingerprint(pubArmor)

	client := &http.Client{Timeout: 30 * time.Second}
	pending := cfg.Server.Web.Security.Keyservers
	var results []security.KeyserverPublishResult
	backoff := time.Second
	for attempt := 0; attempt < 3 && len(pending) > 0; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}
		attemptResults := security.PublishToKeyservers(context.Background(), client, pubArmor, pending)
		var retry []string
		for _, r := range attemptResults {
			if r.Published {
				results = append(results, r)
			} else {
				retry = append(retry, r.Keyserver)
			}
		}
		pending = retry
	}
	for _, ks := range pending {
		results = append(results, security.KeyserverPublishResult{Keyserver: ks, Published: false, Err: fmt.Errorf("exhausted retries")})
	}

	success := len(pending) == 0
	for _, r := range results {
		if r.Published {
			fmt.Printf(display.Emoji("✅", "[OK]")+" Published to %s\n", r.Keyserver)
		} else {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to publish to %s: %v\n", r.Keyserver, r.Err)
		}
	}

	if dm != nil && id != 0 {
		if err := pgpMarkPublished(dm, id, results); err != nil {
			fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to record publish state in database: %v\n", err)
		}
	}
	if err := pgpWriteKeyserversState(config.GetConfigDir(), results); err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to write keyservers.state: %v\n", err)
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
		return fmt.Errorf("rate limited: last export was %s ago, wait %s", elapsed.Round(time.Second), (time.Hour - elapsed).Round(time.Second))
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
	fmt.Println(display.Emoji("🔐", "[PGP]") + " Generate PGP Keypair")
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}

	if _, err := security.LoadPublicKey(config.GetConfigDir()); err == nil {
		fmt.Println(display.Emoji("⚠️", "[WARN]") + " A keypair already exists. Use 'pgp rotate' to replace it.")
		exitFunc(1)
		return
	}

	kp, err := security.GenerateKeypair(cfg.Server.Title, cfg.Server.Web.Security.Contact, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to generate keypair: %v\n", err)
		exitFunc(1)
		return
	}
	configDir := config.GetConfigDir()
	if err := security.SavePublicKey(configDir, kp.PublicKeyArmor); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to save public key: %v\n", err)
		exitFunc(1)
		return
	}
	if err := security.SaveEncryptedPrivateKey(configDir, kp.EncryptedPrivateKey); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to save private key: %v\n", err)
		exitFunc(1)
		return
	}

	dm, err := pgpOpenDB()
	var id int64
	if err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to open database, keypair metadata not recorded: %v\n", err)
	} else {
		defer dm.Close()
		if err := pgpInsertKeypairRow(dm, kp.Fingerprint, kp.ExpiresAt); err != nil {
			fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to record keypair metadata: %v\n", err)
		} else {
			id, _, _ = pgpCurrentKeypairRow(dm)
		}
	}

	pgpAuditLogger().LogPGPKeyGenerated(pgpActor(), pgpAuditIP, kp.Fingerprint)

	fmt.Println(display.Emoji("✅", "[OK]") + " Keypair generated")
	fmt.Println("   Fingerprint: " + security.FormatFingerprint(kp.Fingerprint))
	fmt.Println("   Expires:     " + kp.ExpiresAt.Format("2006-01-02"))
	fmt.Println("   Public key:  " + filepath.Join(configDir, security.PGPDirName, security.PGPPublicKeyFilename))
	fmt.Println()

	pgpPublish(cfg, dm, id)
}

// runPGPRotate implements --maintenance pgp rotate.
func runPGPRotate() {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " Rotate PGP Keypair")
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	configDir := config.GetConfigDir()

	oldPubArmor, err := security.LoadPublicKey(configDir)
	if err != nil {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " No existing keypair to rotate. Use 'pgp generate' first.")
		exitFunc(1)
		return
	}
	oldFingerprint, _ := security.KeyFingerprint(oldPubArmor)

	// Unlock the outgoing private key now, before it is archived below, so we
	// can sign the incoming pubkey with it (AI.md PART 11 "Rotate": "signs
	// the new pubkey with the old key" — establishes chain of custody for
	// recipients verifying the rotation).
	oldPrivEncrypted, err := security.LoadEncryptedPrivateKey(configDir)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load previous private key: %v\n", err)
		exitFunc(1)
		return
	}
	oldKey, err := security.UnlockPrivateKey(oldPrivEncrypted, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to unlock previous private key: %v\n", err)
		exitFunc(1)
		return
	}

	kp, err := security.GenerateKeypair(cfg.Server.Title, cfg.Server.Web.Security.Contact, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to generate new keypair: %v\n", err)
		exitFunc(1)
		return
	}

	// Preserve the outgoing keypair for AI.md PART 11's 30-day grace period
	// so in-flight reports encrypted to it remain decryptable.
	if err := os.Rename(filepath.Join(configDir, security.PGPDirName, security.PGPPublicKeyFilename),
		filepath.Join(configDir, security.PGPDirName, "pgp.pub.old.asc")); err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to archive previous public key: %v\n", err)
	}
	if err := os.Rename(filepath.Join(configDir, security.PGPDirName, security.PGPEncryptedPrivateKeyFilename),
		filepath.Join(configDir, security.PGPDirName, "pgp.priv.old.asc.enc")); err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to archive previous private key: %v\n", err)
	}
	graceExpiry := time.Now().Add(security.PGPOldKeyGracePeriod).UTC().Format(time.RFC3339)
	_ = os.WriteFile(filepath.Join(configDir, security.PGPDirName, "pgp.old.expires"), []byte(graceExpiry), 0o600)

	if err := security.SavePublicKey(configDir, kp.PublicKeyArmor); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to save new public key: %v\n", err)
		exitFunc(1)
		return
	}
	if err := security.SaveEncryptedPrivateKey(configDir, kp.EncryptedPrivateKey); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to save new private key: %v\n", err)
		exitFunc(1)
		return
	}

	rotationSig, err := security.SignDetached(oldKey, []byte(kp.PublicKeyArmor))
	if err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to sign new pubkey with previous key: %v\n", err)
	} else if err := os.WriteFile(filepath.Join(configDir, security.PGPDirName, "pgp.rotation.sig.asc"), []byte(rotationSig), 0o644); err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to write rotation signature: %v\n", err)
	}

	dm, err := pgpOpenDB()
	var id int64
	if err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to open database, keypair metadata not recorded: %v\n", err)
	} else {
		defer dm.Close()
		if err := pgpInsertKeypairRow(dm, kp.Fingerprint, kp.ExpiresAt); err != nil {
			fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to record keypair metadata: %v\n", err)
		} else if id, _, err = pgpCurrentKeypairRow(dm); err == nil {
			_ = pgpMarkRotated(dm, id)
		}
	}

	pgpAuditLogger().LogPGPKeyRotated(pgpActor(), pgpAuditIP, oldFingerprint, kp.Fingerprint)

	fmt.Println(display.Emoji("✅", "[OK]") + " Keypair rotated")
	fmt.Println("   Old fingerprint: " + security.FormatFingerprint(oldFingerprint) + " (valid 30 more days)")
	fmt.Println("   New fingerprint: " + security.FormatFingerprint(kp.Fingerprint))
	fmt.Println()

	pgpPublish(cfg, dm, id)
}

// runPGPPublish implements --maintenance pgp publish.
func runPGPPublish() {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " Publish PGP Key to Keyservers")
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}

	dm, err := pgpOpenDB()
	var id int64
	if err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to open database: %v\n", err)
	} else {
		defer dm.Close()
		id, _, _ = pgpCurrentKeypairRow(dm)
	}
	pgpPublish(cfg, dm, id)
}

// runPGPExportPublic implements --maintenance pgp export public [path].
func runPGPExportPublic(path string) {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " Export Public Key")
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	_ = cfg

	armor, err := security.LoadPublicKey(config.GetConfigDir())
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load public key: %v\n", err)
		exitFunc(1)
		return
	}
	if path == "" {
		fmt.Println(armor)
		return
	}
	if err := os.WriteFile(path, []byte(armor), 0o644); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to write %s: %v\n", path, err)
		exitFunc(1)
		return
	}
	fmt.Println(display.Emoji("✅", "[OK]") + " Public key written to " + path)
}

// runPGPExportPrivate implements --maintenance pgp export private <path>.
func runPGPExportPrivate(path string) {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " Export Private Key")
	fmt.Println()
	if path == "" {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " A destination path is required.")
		fmt.Println("Usage: search --maintenance pgp export private <path>")
		exitFunc(1)
		return
	}

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	configDir := config.GetConfigDir()

	if err := pgpCheckExportRateLimit(configDir); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" %v\n", err)
		exitFunc(1)
		return
	}

	fmt.Println(display.Emoji("⚠️", "[WARN]") + " This exposes the raw, decrypted security private key.")
	reasonText, ok := pgpSensitiveConfirm(cfg)
	if !ok {
		exitFunc(1)
		return
	}

	privArmor, err := security.LoadEncryptedPrivateKey(configDir)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load private key: %v\n", err)
		exitFunc(1)
		return
	}
	unlocked, err := security.UnlockPrivateKey(privArmor, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to unlock private key: %v\n", err)
		exitFunc(1)
		return
	}
	decrypted, err := unlocked.Armor()
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to armor private key: %v\n", err)
		exitFunc(1)
		return
	}
	if err := os.WriteFile(path, []byte(decrypted), 0o600); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to write %s: %v\n", path, err)
		exitFunc(1)
		return
	}
	if err := pgpRecordExport(configDir); err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to record export rate-limit marker: %v\n", err)
	}

	pgpAuditLogger().LogPGPPrivateKeyExport(pgpActor(), pgpAuditIP, reasonText)

	fmt.Println(display.Emoji("✅", "[OK]") + " Private key exported to " + path + " (mode 0600)")
}

// runPGPImport implements --maintenance pgp import <file>.
func runPGPImport(file string) {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " Import PGP Key")
	fmt.Println()
	if file == "" {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " A source file is required.")
		fmt.Println("Usage: search --maintenance pgp import <file>")
		exitFunc(1)
		return
	}

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}

	reasonText, ok := pgpSensitiveConfirm(cfg)
	if !ok {
		exitFunc(1)
		return
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to read %s: %v\n", file, err)
		exitFunc(1)
		return
	}

	kp, err := security.ImportPrivateKey(string(raw), cfg.Server.Title, cfg.Server.Web.Security.Contact, cfg.Server.Security.InstallationSecret)
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to import key: %v\n", err)
		exitFunc(1)
		return
	}

	identityMismatch := !security.IdentityMatches(kp.Key, cfg.Server.Title, cfg.Server.Web.Security.Contact)
	if identityMismatch {
		fmt.Println(display.Emoji("⚠️", "[WARN]") + " Imported key identity does not match this project's expected identity.")
		confirm := pgpReadLine("Import anyway? (yes/no): ")
		if confirm != "yes" {
			fmt.Println("Cancelled.")
			return
		}
	}

	configDir := config.GetConfigDir()
	if err := security.SavePublicKey(configDir, kp.PublicKeyArmor); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to save public key: %v\n", err)
		exitFunc(1)
		return
	}
	if err := security.SaveEncryptedPrivateKey(configDir, kp.EncryptedPrivateKey); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to save private key: %v\n", err)
		exitFunc(1)
		return
	}

	dm, err := pgpOpenDB()
	if err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to open database, keypair metadata not recorded: %v\n", err)
	} else {
		defer dm.Close()
		if err := pgpInsertKeypairRow(dm, kp.Fingerprint, kp.ExpiresAt); err != nil {
			fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to record keypair metadata: %v\n", err)
		}
	}

	pgpAuditLogger().LogPGPPrivateKeyImport(pgpActor(), pgpAuditIP, reasonText, identityMismatch)

	fmt.Println(display.Emoji("✅", "[OK]") + " Private key imported")
	fmt.Println("   Fingerprint: " + security.FormatFingerprint(kp.Fingerprint))
}

// runPGPDelete implements --maintenance pgp delete.
func runPGPDelete() {
	fmt.Println(display.Emoji("🔐", "[PGP]") + " Delete PGP Keypair")
	fmt.Println()

	cfg := pgpRequireAuthorized()
	if cfg == nil {
		return
	}
	configDir := config.GetConfigDir()

	pubArmor, err := security.LoadPublicKey(configDir)
	if err != nil {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " No keypair to delete.")
		exitFunc(1)
		return
	}
	fingerprint, _ := security.KeyFingerprint(pubArmor)

	fmt.Println(display.Emoji("⚠️", "[WARN]") + " In-flight encrypted security reports will become un-decryptable.")
	reasonText, ok := pgpSensitiveConfirm(cfg)
	if !ok {
		exitFunc(1)
		return
	}
	confirm := pgpReadLine("Type 'DELETE' to confirm: ")
	if confirm != "DELETE" {
		fmt.Println("Cancelled.")
		return
	}

	if err := security.DeleteKeypairFiles(configDir); err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to delete keypair files: %v\n", err)
		exitFunc(1)
		return
	}

	cfg.Server.Web.Security.PublishPGPKey = false
	if err := cfg.Save(config.GetConfigPath()); err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to update config: %v\n", err)
	}

	dm, err := pgpOpenDB()
	if err != nil {
		fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to open database: %v\n", err)
	} else {
		defer dm.Close()
		if id, _, err := pgpCurrentKeypairRow(dm); err == nil && id != 0 {
			_ = pgpMarkRevoked(dm, id)
		}
	}

	pgpAuditLogger().LogPGPPrivateKeyDelete(pgpActor(), pgpAuditIP, fingerprint, reasonText)

	fmt.Println(display.Emoji("✅", "[OK]") + " Keypair deleted")
	fmt.Println("   Encryption: line removed from security.txt on next request.")
}
