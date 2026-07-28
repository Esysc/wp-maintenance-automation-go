package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

type Database struct {
	*sql.DB
}

func New(connStr string) (*Database, error) {
	db, err := sql.Open("postgres", connStr)
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
	if err := database.migrateSiteSSHKey(); err != nil {
		return nil, fmt.Errorf("failed to migrate site ssh key: %w", err)
	}
	if err := database.migrateDropStagingHostFields(); err != nil {
		return nil, fmt.Errorf("failed to migrate staging fields: %w", err)
	}
	if err := database.migrateJobsProgressPercent(); err != nil {
		return nil, fmt.Errorf("failed to migrate jobs progress percent: %w", err)
	}
	if err := database.migrateBackupsSnapshotID(); err != nil {
		return nil, fmt.Errorf("failed to migrate backups snapshot_id: %w", err)
	}

	return database, nil
}

func pgPlaceholders(query string) string {
	var buf strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			buf.WriteString(fmt.Sprintf("$%d", n))
		} else {
			buf.WriteByte(query[i])
		}
	}
	return buf.String()
}

func (db *Database) Exec(query string, args ...interface{}) (sql.Result, error) {
	return db.DB.Exec(pgPlaceholders(query), args...)
}

func (db *Database) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.DB.Query(pgPlaceholders(query), args...)
}

func (db *Database) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.DB.QueryRow(pgPlaceholders(query), args...)
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
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS api_tokens (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		token TEXT UNIQUE NOT NULL,
		name TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP,
		last_used TIMESTAMP,
		revoked INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY(user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS sites (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		wp_ssh_host TEXT NOT NULL,
		wp_ssh_port INTEGER NOT NULL DEFAULT 22,
		wp_ssh_user TEXT NOT NULL,
		wp_ssh_key TEXT,
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
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
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
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(site_id) REFERENCES sites(id)
	);

	CREATE INDEX IF NOT EXISTS idx_backups_site_id ON backups(site_id);
	CREATE INDEX IF NOT EXISTS idx_backups_timestamp ON backups(timestamp);

	CREATE TABLE IF NOT EXISTS jobs (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		site_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued',
		progress TEXT NOT NULL DEFAULT '',
		progress_percent INTEGER NOT NULL DEFAULT 0,
		result TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_jobs_site_id ON jobs(site_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	`

	_, err := db.Exec(schema)
	return err
}

func (db *Database) hasColumn(table, column string) (bool, error) {
	query := `SELECT column_name FROM information_schema.columns WHERE table_name = $1 AND column_name = $2`
	rows, err := db.DB.Query(query, table, column)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

func (db *Database) migrateUserProfileFields() error {
	for _, m := range []struct {
		col string
		def string
	}{
		{"display_name", "TEXT NOT NULL DEFAULT ''"},
		{"icon", "TEXT NOT NULL DEFAULT 'user'"},
	} {
		ok, err := db.hasColumn("users", m.col)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := db.DB.Exec("ALTER TABLE users ADD COLUMN " + m.col + " " + m.def); err != nil {
				return err
			}
		}
	}

	if _, err := db.DB.Exec("UPDATE users SET display_name = username WHERE COALESCE(display_name, '') = ''"); err != nil {
		return err
	}
	if _, err := db.DB.Exec("UPDATE users SET icon = 'user' WHERE COALESCE(icon, '') = ''"); err != nil {
		return err
	}

	return nil
}

func (db *Database) migrateSiteSSHKey() error {
	ok, err := db.hasColumn("sites", "wp_ssh_key")
	if err != nil {
		return err
	}
	if !ok {
		_, err = db.DB.Exec("ALTER TABLE sites ADD COLUMN wp_ssh_key TEXT")
	}
	return err
}

func (db *Database) migrateDropStagingHostFields() error {
	rows, err := db.DB.Query(`SELECT column_name FROM information_schema.columns WHERE table_name = 'sites'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		columns[name] = true
	}

	for _, col := range []string{"staging_host", "staging_port", "staging_user", "staging_root"} {
		if columns[col] {
			if _, err := db.DB.Exec("ALTER TABLE sites DROP COLUMN " + col); err != nil {
				return err
			}
		}
	}
	return nil
}

func (db *Database) migrateBackupsSnapshotID() error {
	ok, err := db.hasColumn("backups", "snapshot_id")
	if err != nil {
		return err
	}
	if !ok {
		if _, err := db.DB.Exec("ALTER TABLE backups ADD COLUMN snapshot_id TEXT"); err != nil {
			return err
		}
	}
	return nil
}

func (db *Database) migrateJobsProgressPercent() error {
	ok, err := db.hasColumn("jobs", "progress_percent")
	if err != nil {
		return err
	}
	if !ok {
		_, err = db.DB.Exec("ALTER TABLE jobs ADD COLUMN progress_percent INTEGER NOT NULL DEFAULT 0")
	}
	return err
}
