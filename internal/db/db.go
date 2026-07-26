package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Database struct {
	*sql.DB
}

func New(dbPath string) (*Database, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := initSchema(db); err != nil {
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	database := &Database{DB: db}
	if err := database.migrateUserProfileFields(); err != nil {
		return nil, fmt.Errorf("failed to migrate user profile fields: %w", err)
	}
	if err := database.migrateLegacyPlaintextSiteSecrets(); err != nil {
		return nil, fmt.Errorf("failed to migrate sensitive site fields: %w", err)
	}

	return database, nil
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		icon TEXT NOT NULL DEFAULT 'user',
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL,
		force_pass INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS api_tokens (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		token TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		expires_at DATETIME,
		last_used DATETIME,
		revoked INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS sites (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		wp_ssh_host TEXT NOT NULL,
		wp_ssh_port INTEGER NOT NULL DEFAULT 22,
		wp_ssh_user TEXT NOT NULL,
		wp_root TEXT NOT NULL,
		db_host TEXT,
		db_user TEXT,
		db_password TEXT,
		db_name TEXT,
		restic_repository TEXT,
		restic_password_file TEXT,
		backup_dir TEXT,
		retention_flags TEXT,
		healthcheck_url TEXT,
		staging_enabled INTEGER DEFAULT 0,
		staging_host TEXT,
		staging_port INTEGER DEFAULT 22,
		staging_user TEXT,
		staging_root TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS backups (
		id TEXT PRIMARY KEY,
		site_id TEXT NOT NULL,
		timestamp TEXT NOT NULL,
		host TEXT NOT NULL,
		wp_root TEXT NOT NULL,
		wp_version TEXT,
		db_name TEXT,
		db_host TEXT,
		db_version TEXT,
		dump_file TEXT,
		file_count INTEGER DEFAULT 0,
		rs_exclude TEXT,
		retention TEXT,
		ssh_host TEXT,
		ssh_user TEXT,
		ssh_port INTEGER,
		backup_size INTEGER DEFAULT 0,
		checksum TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(site_id) REFERENCES sites(id)
	);

	CREATE INDEX IF NOT EXISTS idx_backups_site_id ON backups(site_id);
	CREATE INDEX IF NOT EXISTS idx_backups_timestamp ON backups(timestamp);
	`

	_, err := db.Exec(schema)
	return err
}

func (db *Database) migrateUserProfileFields() error {
	rows, err := db.Query("PRAGMA table_info(users)")
	if err != nil {
		return err
	}
	defer rows.Close()

	hasDisplayName := false
	hasIcon := false

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}

		switch name {
		case "display_name":
			hasDisplayName = true
		case "icon":
			hasIcon = true
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if !hasDisplayName {
		if _, err := db.Exec("ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}

	if !hasIcon {
		if _, err := db.Exec("ALTER TABLE users ADD COLUMN icon TEXT NOT NULL DEFAULT 'user'"); err != nil {
			return err
		}
	}

	if _, err := db.Exec("UPDATE users SET display_name = username WHERE COALESCE(display_name, '') = ''"); err != nil {
		return err
	}

	if _, err := db.Exec("UPDATE users SET icon = 'user' WHERE COALESCE(icon, '') = ''"); err != nil {
		return err
	}

	return nil
}
