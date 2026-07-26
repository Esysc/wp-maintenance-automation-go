package models

import "time"

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ForcePass bool      `json:"force_pass"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserCreateRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type UserUpdateRequest struct {
	Password string `json:"password"`
	Role     string `json:"role"`
}

type UserLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type UserLoginResponse struct {
	Token     string `json:"token"`
	User      *User  `json:"user"`
	ForcePass bool   `json:"force_pass"`
}

type APIToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Token     string     `json:"token,omitempty"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	Revoked   bool       `json:"revoked"`
}

type TokenCreateRequest struct {
	UserID   string `json:"user_id"`
	Name     string `json:"name"`
	Duration string `json:"duration"`
}

type BackupManifest struct {
	Timestamp string `json:"timestamp"`
	Host      string `json:"host"`
	WPRoot    string `json:"wp_root"`
	WPVersion string `json:"wp_version"`
	DBName    string `json:"db_name"`
	DBHost    string `json:"db_host"`
	DBVersion string `json:"db_version"`
	DumpFile  string `json:"dump_file"`
	FileCount int    `json:"file_count"`
	SSHHost   string `json:"ssh_host"`
	SSHUser   string `json:"ssh_user"`
	SSHPort   int    `json:"ssh_port"`
}

type BackupCreateRequest struct {
	Tags []string `json:"tags"`
}

type RestoreCreateRequest struct {
	SnapshotID     string `json:"snapshot_id"`
	ApplyDB        bool   `json:"apply_db"`
	ApplyFiles     bool   `json:"apply_files"`
	ApplyConfigs   bool   `json:"apply_configs"`
	DeleteRemote   bool   `json:"delete_remote"`
	ConfirmRestore bool   `json:"confirm_restore"`
}

type UpgradeCreateRequest struct {
	AutoRollback    bool   `json:"auto_rollback"`
	ForceUpgrade    bool   `json:"force_upgrade"`
	SkipStaging     bool   `json:"skip_staging"`
	HealthcheckURL  string `json:"healthcheck_url"`
	HealthcheckCode int    `json:"healthcheck_code"`
}

type HealthcheckRequest struct {
	URL          string `json:"url"`
	ExpectedCode int    `json:"expected_code"`
	MaxRetries   int    `json:"max_retries"`
	Insecure     bool   `json:"insecure"`
}

type HealthcheckResponse struct {
	URL            string `json:"url"`
	StatusCode     int    `json:"status_code"`
	ExpectedCode   int    `json:"expected_code"`
	Passed         bool   `json:"passed"`
	Attempt        int    `json:"attempt"`
	MaxAttempts    int    `json:"max_attempts"`
	ResponseTimeMs int64  `json:"response_time_ms"`
	ErrorMessage   string `json:"error_message,omitempty"`
}

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type UpgradeReport struct {
	Timestamp      string `json:"timestamp"`
	Host           string `json:"host"`
	WPRoot         string `json:"wp_root"`
	Snapshot       string `json:"snapshot"`
	HealthcheckURL string `json:"healthcheck_url"`
	Steps          []Step `json:"steps"`
	FinalStatus    string `json:"final_status"`
	FinalReason    string `json:"final_reason"`
}

type Step struct {
	Phase     string    `json:"phase"`
	Success   bool      `json:"success"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}
