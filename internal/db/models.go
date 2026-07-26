package db

import (
	"database/sql"
	"fmt"
	"time"
)

type DBUser struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Icon        string    `json:"icon"`
	Password    string    `json:"password_hash"`
	Role        string    `json:"role"`
	ForcePass   bool      `json:"force_pass"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DBToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Token     string     `json:"token,omitempty"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	Revoked   bool       `json:"revoked"`
}

type Site struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	WPSSHHost          string `json:"wp_ssh_host"`
	WPSSHPort          int    `json:"wp_ssh_port"`
	WPSSHUser          string `json:"wp_ssh_user"`
	WPRoot             string `json:"wp_root"`
	DBHost             string `json:"db_host"`
	DBUser             string `json:"db_user"`
	DBPassword         string `json:"db_password"`
	DBName             string `json:"db_name"`
	ResticRepository   string `json:"restic_repository"`
	ResticPasswordFile string `json:"restic_password_file"`
	BackupDir          string `json:"backup_dir"`
	RetentionFlags     string `json:"retention_flags"`
	HealthcheckURL     string `json:"healthcheck_url"`
	StagingEnabled     bool   `json:"staging_enabled"`
	StagingHost        string `json:"staging_host"`
	StagingPort        int    `json:"staging_port"`
	StagingUser        string `json:"staging_user"`
	StagingRoot        string `json:"staging_root"`
}

func (db *Database) CreateSite(site *Site) error {
	toStore := *site
	if err := encryptSiteSensitiveFields(&toStore); err != nil {
		return err
	}

	query := `
		INSERT INTO sites (id, name, wp_ssh_host, wp_ssh_port, wp_ssh_user, wp_root, db_host, db_user, db_password, db_name, restic_repository, restic_password_file, backup_dir, retention_flags, healthcheck_url, staging_enabled, staging_host, staging_port, staging_user, staging_root)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query,
		toStore.ID, toStore.Name, toStore.WPSSHHost, toStore.WPSSHPort, toStore.WPSSHUser, toStore.WPRoot,
		toStore.DBHost, toStore.DBUser, toStore.DBPassword, toStore.DBName,
		toStore.ResticRepository, toStore.ResticPasswordFile, toStore.BackupDir, toStore.RetentionFlags, toStore.HealthcheckURL,
		boolToInt(toStore.StagingEnabled), toStore.StagingHost, toStore.StagingPort, toStore.StagingUser, toStore.StagingRoot,
	)
	return err
}

func (db *Database) GetSite(id string) (*Site, error) {
	query := `
		SELECT id, name, wp_ssh_host, wp_ssh_port, wp_ssh_user, wp_root, db_host, db_user, db_password, db_name,
		       restic_repository, restic_password_file, backup_dir, retention_flags, healthcheck_url,
		       staging_enabled, staging_host, staging_port, staging_user, staging_root
		FROM sites WHERE id = ?
	`
	site := &Site{}
	err := db.QueryRow(query, id).Scan(
		&site.ID, &site.Name, &site.WPSSHHost, &site.WPSSHPort, &site.WPSSHUser, &site.WPRoot,
		&site.DBHost, &site.DBUser, &site.DBPassword, &site.DBName,
		&site.ResticRepository, &site.ResticPasswordFile, &site.BackupDir, &site.RetentionFlags, &site.HealthcheckURL,
		&site.StagingEnabled, &site.StagingHost, &site.StagingPort, &site.StagingUser, &site.StagingRoot,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("site not found")
		}
		return nil, err
	}
	if err := decryptSiteSensitiveFields(site); err != nil {
		return nil, err
	}
	return site, nil
}

func (db *Database) ListSites() ([]*Site, error) {
	query := `
		SELECT id, name, wp_ssh_host, wp_ssh_port, wp_ssh_user, wp_root, db_host, db_user, db_password, db_name,
		       restic_repository, restic_password_file, backup_dir, retention_flags, healthcheck_url,
		       staging_enabled, staging_host, staging_port, staging_user, staging_root
		FROM sites ORDER BY created_at DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := []*Site{}
	for rows.Next() {
		site := &Site{}
		err := rows.Scan(
			&site.ID, &site.Name, &site.WPSSHHost, &site.WPSSHPort, &site.WPSSHUser, &site.WPRoot,
			&site.DBHost, &site.DBUser, &site.DBPassword, &site.DBName,
			&site.ResticRepository, &site.ResticPasswordFile, &site.BackupDir, &site.RetentionFlags, &site.HealthcheckURL,
			&site.StagingEnabled, &site.StagingHost, &site.StagingPort, &site.StagingUser, &site.StagingRoot,
		)
		if err != nil {
			return nil, err
		}
		if err := decryptSiteSensitiveFields(site); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	return sites, nil
}

func (db *Database) UpdateSite(site *Site) error {
	toStore := *site
	if err := encryptSiteSensitiveFields(&toStore); err != nil {
		return err
	}

	query := `
		UPDATE sites SET name=?, wp_ssh_host=?, wp_ssh_port=?, wp_ssh_user=?, wp_root=?,
		               db_host=?, db_user=?, db_password=?, db_name=?, restic_repository=?,
		               restic_password_file=?, backup_dir=?, retention_flags=?, healthcheck_url=?,
		               staging_enabled=?, staging_host=?, staging_port=?, staging_user=?, staging_root=?,
		               updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`
	_, err := db.Exec(query,
		toStore.Name, toStore.WPSSHHost, toStore.WPSSHPort, toStore.WPSSHUser, toStore.WPRoot,
		toStore.DBHost, toStore.DBUser, toStore.DBPassword, toStore.DBName,
		toStore.ResticRepository, toStore.ResticPasswordFile, toStore.BackupDir, toStore.RetentionFlags, toStore.HealthcheckURL,
		boolToInt(toStore.StagingEnabled), toStore.StagingHost, toStore.StagingPort, toStore.StagingUser, toStore.StagingRoot,
		toStore.ID,
	)
	return err
}

func (db *Database) DeleteSite(id string) error {
	_, err := db.Exec("DELETE FROM sites WHERE id=?", id)
	return err
}

func (db *Database) CreateBackup(backup *Backup) error {
	query := `
		INSERT INTO backups (id, site_id, timestamp, host, wp_root, wp_version, db_name, db_host, db_version, dump_file, file_count, rs_exclude, retention, ssh_host, ssh_user, ssh_port, backup_size, checksum, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query,
		backup.ID, backup.SiteID, backup.Timestamp, backup.Host, backup.WPRoot, backup.WPVersion,
		backup.DBName, backup.DBHost, backup.DBVersion, backup.DumpFile, backup.FileCount,
		backup.RSExclude, backup.Retention, backup.SSHHost, backup.SSHUser, backup.SSHPort,
		backup.BackupSize, backup.Checksum, time.Now().Format("2006-01-02 15:04:05"),
	)
	return err
}

func (db *Database) GetBackupsBySite(siteID string) ([]*Backup, error) {
	query := `
		SELECT id, site_id, timestamp, host, wp_root, wp_version, db_name, db_host, db_version, dump_file,
		       file_count, rs_exclude, retention, ssh_host, ssh_user, ssh_port, backup_size, checksum, created_at
		FROM backups WHERE site_id = ? ORDER BY timestamp DESC
	`
	rows, err := db.Query(query, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	backups := []*Backup{}
	for rows.Next() {
		backup := &Backup{}
		err := rows.Scan(
			&backup.ID, &backup.SiteID, &backup.Timestamp, &backup.Host, &backup.WPRoot, &backup.WPVersion,
			&backup.DBName, &backup.DBHost, &backup.DBVersion, &backup.DumpFile,
			&backup.FileCount, &backup.RSExclude, &backup.Retention, &backup.SSHHost, &backup.SSHUser, &backup.SSHPort,
			&backup.BackupSize, &backup.Checksum, &backup.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	return backups, nil
}

type Backup struct {
	ID         string `json:"id"`
	SiteID     string `json:"site_id"`
	Timestamp  string `json:"timestamp"`
	Host       string `json:"host"`
	WPRoot     string `json:"wp_root"`
	WPVersion  string `json:"wp_version"`
	DBName     string `json:"db_name"`
	DBHost     string `json:"db_host"`
	DBVersion  string `json:"db_version"`
	DumpFile   string `json:"dump_file"`
	FileCount  int    `json:"file_count"`
	RSExclude  string `json:"rs_exclude"`
	Retention  string `json:"retention"`
	SSHHost    string `json:"ssh_host"`
	SSHUser    string `json:"ssh_user"`
	SSHPort    int    `json:"ssh_port"`
	BackupSize int64  `json:"backup_size"`
	Checksum   string `json:"checksum"`
	CreatedAt  string `json:"created_at"`
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (db *Database) CreateUser(user *DBUser) error {
	query := `
		INSERT INTO users (id, username, display_name, icon, password_hash, role, force_pass, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`
	_, err := db.Exec(query, user.ID, user.Username, user.DisplayName, user.Icon, user.Password, user.Role, boolToInt(user.ForcePass))
	return err
}

func (db *Database) GetUserByUsername(username string) (*DBUser, error) {
	query := `
		SELECT id, username, display_name, icon, password_hash, role, force_pass, created_at, updated_at
		FROM users WHERE username = ?
	`
	user := &DBUser{}
	err := db.QueryRow(query, username).Scan(
		&user.ID, &user.Username, &user.DisplayName, &user.Icon, &user.Password, &user.Role, &user.ForcePass,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	return user, nil
}

func (db *Database) GetUserByID(id string) (*DBUser, error) {
	query := `
		SELECT id, username, display_name, icon, password_hash, role, force_pass, created_at, updated_at
		FROM users WHERE id = ?
	`
	user := &DBUser{}
	err := db.QueryRow(query, id).Scan(
		&user.ID, &user.Username, &user.DisplayName, &user.Icon, &user.Password, &user.Role, &user.ForcePass,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	return user, nil
}

func (db *Database) UpdateUser(user *DBUser) error {
	query := `
		UPDATE users SET username=?, display_name=?, icon=?, password_hash=?, role=?, force_pass=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`
	_, err := db.Exec(query, user.Username, user.DisplayName, user.Icon, user.Password, user.Role, boolToInt(user.ForcePass), user.ID)
	return err
}

func (db *Database) ListUsers() ([]DBUser, error) {
	query := `
		SELECT id, username, display_name, icon, password_hash, role, force_pass, created_at, updated_at
		FROM users ORDER BY created_at DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []DBUser{}
	for rows.Next() {
		user := DBUser{}
		err := rows.Scan(
			&user.ID, &user.Username, &user.DisplayName, &user.Icon, &user.Password, &user.Role, &user.ForcePass,
			&user.CreatedAt, &user.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (db *Database) CreateToken(token *DBToken) error {
	query := `
		INSERT INTO api_tokens (id, user_id, token, name, created_at, expires_at, last_used, revoked)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?, NULL, ?)
	`
	var expiresAt *string
	if token.ExpiresAt != nil {
		s := token.ExpiresAt.Format(time.RFC3339)
		expiresAt = &s
	}
	_, err := db.Exec(query, token.ID, token.UserID, token.Token, token.Name, expiresAt, boolToInt(token.Revoked))
	return err
}

func (db *Database) GetTokenByValue(tokenValue string) (*DBToken, error) {
	query := `
		SELECT id, user_id, token, name, created_at, expires_at, last_used, revoked
		FROM api_tokens WHERE token = ? AND revoked = 0
	`
	token := &DBToken{}
	var expiresAt, lastUsed sql.NullString
	err := db.QueryRow(query, tokenValue).Scan(
		&token.ID, &token.UserID, &token.Token, &token.Name,
		&token.CreatedAt, &expiresAt, &lastUsed, &token.Revoked,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token not found or revoked")
		}
		return nil, err
	}
	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		token.ExpiresAt = &t
	}
	return token, nil
}

func (db *Database) ListTokens() ([]*DBToken, error) {
	query := `
		SELECT id, user_id, token, name, created_at, expires_at, last_used, revoked
		FROM api_tokens ORDER BY created_at DESC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := []*DBToken{}
	for rows.Next() {
		token := &DBToken{}
		var expiresAt, lastUsed sql.NullString
		err := rows.Scan(
			&token.ID, &token.UserID, &token.Token, &token.Name,
			&token.CreatedAt, &expiresAt, &lastUsed, &token.Revoked,
		)
		if err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			t, _ := time.Parse(time.RFC3339, expiresAt.String)
			token.ExpiresAt = &t
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func (db *Database) RevokeToken(tokenID string) error {
	_, err := db.Exec("UPDATE api_tokens SET revoked=1 WHERE id=?", tokenID)
	return err
}
