package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *Database {
	t.Helper()
	dbPath := "./test_db_" + t.Name() + ".sqlite"

	// Remove existing test db
	os.Remove(dbPath)

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		os.Remove(dbPath)
	})

	return db
}

func TestNew(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		t.Fatal("expected non-nil database")
	}
}

func TestCreateAndGetSite(t *testing.T) {
	db := setupTestDB(t)

	site := &Site{
		ID:                 "site-1",
		Name:               "Test Site",
		WPSSHHost:          "example.com",
		WPSSHPort:          22,
		WPSSHUser:          "ubuntu",
		WPRoot:             "/var/www/html",
		DBHost:             "localhost",
		DBUser:             "wpuser",
		DBPassword:         "wppass",
		DBName:             "wordpress",
		ResticRepository:   "s3:bucket/backups",
		ResticPasswordFile: "/path/to/password",
		BackupDir:          "./backups",
		RetentionFlags:     "--keep-daily 7",
		HealthcheckURL:     "http://example.com",
		StagingEnabled:     true,
	}

	err := db.CreateSite(site)
	if err != nil {
		t.Fatalf("failed to create site: %v", err)
	}

	retrieved, err := db.GetSite("site-1")
	if err != nil {
		t.Fatalf("failed to get site: %v", err)
	}

	if retrieved.Name != "Test Site" {
		t.Errorf("expected name 'Test Site', got '%s'", retrieved.Name)
	}
	if retrieved.WPSSHHost != "example.com" {
		t.Errorf("expected host 'example.com', got '%s'", retrieved.WPSSHHost)
	}
}

func TestListSites(t *testing.T) {
	db := setupTestDB(t)

	site1 := &Site{ID: "site-1", Name: "Site 1", WPSSHHost: "host1.com", WPSSHUser: "user1", WPRoot: "/var/www/html1"}
	site2 := &Site{ID: "site-2", Name: "Site 2", WPSSHHost: "host2.com", WPSSHUser: "user2", WPRoot: "/var/www/html2"}

	db.CreateSite(site1)
	db.CreateSite(site2)

	sites, err := db.ListSites()
	if err != nil {
		t.Fatalf("failed to list sites: %v", err)
	}

	if len(sites) != 2 {
		t.Errorf("expected 2 sites, got %d", len(sites))
	}
}

func TestCreateAndGetUser(t *testing.T) {
	db := setupTestDB(t)

	user := &DBUser{
		ID:          "user-1",
		Username:    "admin",
		DisplayName: "Admin",
		Icon:        "shield",
		Password:    "hashed_password",
		Role:        "admin",
		ForcePass:   true,
	}

	err := db.CreateUser(user)
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	retrieved, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("failed to get user by username: %v", err)
	}

	if retrieved.Username != "admin" {
		t.Errorf("expected username 'admin', got '%s'", retrieved.Username)
	}
	if retrieved.DisplayName != "Admin" {
		t.Errorf("expected display name 'Admin', got '%s'", retrieved.DisplayName)
	}
	if retrieved.Icon != "shield" {
		t.Errorf("expected icon 'shield', got '%s'", retrieved.Icon)
	}
	if !retrieved.ForcePass {
		t.Error("expected force_pass to be true")
	}
}

func TestCreateAndGetToken(t *testing.T) {
	db := setupTestDB(t)

	token := &DBToken{
		ID:      "token-1",
		UserID:  "user-1",
		Token:   "test-token-value",
		Name:    "Test Token",
		Revoked: false,
	}

	err := db.CreateToken(token)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	retrieved, err := db.GetTokenByValue("test-token-value")
	if err != nil {
		t.Fatalf("failed to get token by value: %v", err)
	}

	if retrieved.Token != "test-token-value" {
		t.Errorf("expected token 'test-token-value', got '%s'", retrieved.Token)
	}
}

func TestListTokens(t *testing.T) {
	db := setupTestDB(t)

	token1 := &DBToken{ID: "token-1", UserID: "user-1", Token: "token1", Name: "Token 1", Revoked: false}
	token2 := &DBToken{ID: "token-2", UserID: "user-1", Token: "token2", Name: "Token 2", Revoked: false}

	db.CreateToken(token1)
	db.CreateToken(token2)

	tokens, err := db.ListTokens()
	if err != nil {
		t.Fatalf("failed to list tokens: %v", err)
	}

	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(tokens))
	}
}

func TestRevokeToken(t *testing.T) {
	db := setupTestDB(t)

	token := &DBToken{ID: "token-1", UserID: "user-1", Token: "revoked-token", Name: "Revoked Token", Revoked: false}
	db.CreateToken(token)

	err := db.RevokeToken("token-1")
	if err != nil {
		t.Fatalf("failed to revoke token: %v", err)
	}

	_, err = db.GetTokenByValue("revoked-token")
	if err == nil {
		t.Error("expected error for revoked token, got nil")
	}
}

func TestSiteSensitiveFieldsEncryptedAtRest(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "test-encryption-key")
	db := setupTestDB(t)

	site := &Site{
		ID:                 "site-encrypted-1",
		Name:               "Encrypted Site",
		WPSSHHost:          "example.com",
		WPSSHPort:          22,
		WPSSHUser:          "ubuntu",
		WPRoot:             "/var/www/html",
		DBHost:             "localhost",
		DBUser:             "wpuser",
		DBPassword:         "SuperSecret123!",
		DBName:             "wordpress",
		ResticPasswordFile: "/run/secrets/restic_password",
	}

	if err := db.CreateSite(site); err != nil {
		t.Fatalf("failed to create site: %v", err)
	}

	var rawDBPassword string
	var rawResticPasswordFile string
	err := db.QueryRow("SELECT db_password, restic_password_file FROM sites WHERE id = ?", site.ID).Scan(&rawDBPassword, &rawResticPasswordFile)
	if err != nil {
		t.Fatalf("failed to query raw site data: %v", err)
	}

	if rawDBPassword == site.DBPassword {
		t.Fatalf("expected db_password to be encrypted at rest")
	}
	if !strings.HasPrefix(rawDBPassword, encryptedV1Prefix) {
		t.Fatalf("expected encrypted db_password prefix, got: %s", rawDBPassword)
	}

	if rawResticPasswordFile == site.ResticPasswordFile {
		t.Fatalf("expected restic_password_file to be encrypted at rest")
	}
	if !strings.HasPrefix(rawResticPasswordFile, encryptedV1Prefix) {
		t.Fatalf("expected encrypted restic_password_file prefix, got: %s", rawResticPasswordFile)
	}

	retrieved, err := db.GetSite(site.ID)
	if err != nil {
		t.Fatalf("failed to get site: %v", err)
	}
	if retrieved.DBPassword != "SuperSecret123!" {
		t.Fatalf("expected decrypted db_password, got %s", retrieved.DBPassword)
	}
	if retrieved.ResticPasswordFile != "/run/secrets/restic_password" {
		t.Fatalf("expected decrypted restic_password_file, got %s", retrieved.ResticPasswordFile)
	}
}

func TestSiteSensitiveFieldsPlaintextBackwardCompatibility(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "test-encryption-key")
	db := setupTestDB(t)

	_, err := db.Exec(`
		INSERT INTO sites (
			id, name, wp_ssh_host, wp_ssh_port, wp_ssh_user, wp_ssh_key, wp_root,
			db_host, db_user, db_password, db_name,
			restic_repository, restic_password_file, backup_dir, retention_flags, healthcheck_url,
			staging_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"site-legacy-1", "Legacy Site", "legacy.example.com", 22, "ubuntu", "", "/var/www/html",
		"localhost", "wpuser", "legacy-plain-pass", "wordpress",
		"", "/legacy/path", "", "", "",
		0,
	)
	if err != nil {
		t.Fatalf("failed to insert legacy plaintext site: %v", err)
	}

	retrieved, err := db.GetSite("site-legacy-1")
	if err != nil {
		t.Fatalf("failed to get legacy site: %v", err)
	}

	if retrieved.DBPassword != "legacy-plain-pass" {
		t.Fatalf("expected plaintext-compatible db_password, got %s", retrieved.DBPassword)
	}
	if retrieved.ResticPasswordFile != "/legacy/path" {
		t.Fatalf("expected plaintext-compatible restic_password_file, got %s", retrieved.ResticPasswordFile)
	}
}

func TestMigrateLegacyPlaintextSiteSecretsEncryptsAtRest(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "test-encryption-key")
	db := setupTestDB(t)

	_, err := db.Exec(`
		INSERT INTO sites (
			id, name, wp_ssh_host, wp_ssh_port, wp_ssh_user, wp_ssh_key, wp_root,
			db_host, db_user, db_password, db_name,
			restic_repository, restic_password_file, backup_dir, retention_flags, healthcheck_url,
			staging_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"site-legacy-2", "Legacy Site 2", "legacy2.example.com", 22, "ubuntu", "", "/var/www/html",
		"localhost", "wpuser", "legacy-pass-2", "wordpress",
		"", "/legacy/path/2", "", "", "",
		0,
	)
	if err != nil {
		t.Fatalf("failed to insert legacy plaintext site: %v", err)
	}

	if err := db.migrateLegacyPlaintextSiteSecrets(); err != nil {
		t.Fatalf("failed to migrate legacy plaintext values: %v", err)
	}

	var rawDBPassword string
	var rawResticPasswordFile string
	err = db.QueryRow("SELECT db_password, restic_password_file FROM sites WHERE id = ?", "site-legacy-2").Scan(&rawDBPassword, &rawResticPasswordFile)
	if err != nil {
		t.Fatalf("failed to query migrated site data: %v", err)
	}

	if !strings.HasPrefix(rawDBPassword, encryptedV1Prefix) {
		t.Fatalf("expected migrated db_password to be encrypted, got: %s", rawDBPassword)
	}
	if !strings.HasPrefix(rawResticPasswordFile, encryptedV1Prefix) {
		t.Fatalf("expected migrated restic_password_file to be encrypted, got: %s", rawResticPasswordFile)
	}
}

func TestNewMigratesLegacyUserProfileFields(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy-users.sqlite")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open legacy database: %v", err)
	}

	_, err = legacyDB.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			force_pass INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		legacyDB.Close()
		t.Fatalf("failed to create legacy users table: %v", err)
	}

	_, err = legacyDB.Exec(`INSERT INTO users (id, username, password_hash, role, force_pass) VALUES (?, ?, ?, ?, ?)`, "user-1", "internal", "hash", "admin", 0)
	if err != nil {
		legacyDB.Close()
		t.Fatalf("failed to insert legacy user: %v", err)
	}

	if err := legacyDB.Close(); err != nil {
		t.Fatalf("failed to close legacy database: %v", err)
	}

	migrated, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen migrated database: %v", err)
	}
	defer migrated.Close()

	user, err := migrated.GetUserByUsername("internal")
	if err != nil {
		t.Fatalf("failed to get migrated user: %v", err)
	}

	if user.DisplayName != "internal" {
		t.Fatalf("expected display name to default to username, got %q", user.DisplayName)
	}
	if user.Icon != "user" {
		t.Fatalf("expected icon to default to user, got %q", user.Icon)
	}
}

func TestNewMigratesJobsProgressPercentColumn(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy-jobs.sqlite")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open legacy database: %v", err)
	}

	_, err = legacyDB.Exec(`
		CREATE TABLE jobs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			site_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			progress TEXT NOT NULL DEFAULT '',
			result TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		legacyDB.Close()
		t.Fatalf("failed to create legacy jobs table: %v", err)
	}

	if err := legacyDB.Close(); err != nil {
		t.Fatalf("failed to close legacy database: %v", err)
	}

	migrated, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen migrated database: %v", err)
	}
	defer migrated.Close()

	rows, err := migrated.Query("PRAGMA table_info(jobs)")
	if err != nil {
		t.Fatalf("failed to inspect jobs schema: %v", err)
	}
	defer rows.Close()

	hasProgressPercent := false
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("failed scanning jobs schema row: %v", err)
		}
		if name == "progress_percent" {
			hasProgressPercent = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed iterating jobs schema rows: %v", err)
	}

	if !hasProgressPercent {
		t.Fatalf("expected jobs table to include progress_percent column after migration")
	}
}
