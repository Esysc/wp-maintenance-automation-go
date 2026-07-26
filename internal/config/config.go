package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	WPSSHHost          string        `json:"wp_ssh_host"`
	WPSSHPort          int           `json:"wp_ssh_port"`
	WPSSHUser          string        `json:"wp_ssh_user"`
	WPRoot             string        `json:"wp_root"`
	ResticRepository   string        `json:"restic_repository"`
	ResticPasswordFile string        `json:"restic_password_file"`
	BackupDir          string        `json:"backup_dir"`
	RetentionFlags     string        `json:"retention_flags"`
	HealthcheckURL     string        `json:"healthcheck_url"`
	HealthcheckRetry   int           `json:"healthcheck_retry"`
	StagingEnabled     bool          `json:"staging_enabled"`
	StagingHost        string        `json:"staging_host"`
	StagingPort        int           `json:"staging_port"`
	StagingUser        string        `json:"staging_user"`
	StagingRoot        string        `json:"staging_root"`
	AdminUsername      string        `json:"admin_username"`
	AdminPassword      string        `json:"admin_password"`
	SecretKey          string        `json:"secret_key"`
	APITokenDuration   time.Duration `json:"api_token_duration"`
}

const (
	DefaultPort      = 22
	DefaultRetry     = 5
	DefaultDuration  = 24 * time.Hour
	DefaultBackupDir = "./backup_artifacts"
)

var GlobalConfig *Config

// LoadConfig loads configuration from file
func LoadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewDefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	GlobalConfig = &config
	return &config, nil
}

// SaveConfig saves configuration to file
func SaveConfig(config *Config, configPath string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	return os.WriteFile(configPath, data, 0644)
}

// NewDefaultConfig creates a default configuration
func NewDefaultConfig() *Config {
	return &Config{
		WPSSHHost:          "example.com",
		WPSSHPort:          DefaultPort,
		WPSSHUser:          "ubuntu",
		WPRoot:             "/var/www/html",
		ResticRepository:   "/path/to/restic-repo",
		ResticPasswordFile: "$HOME/.config/restic/wp_repo_password",
		BackupDir:          DefaultBackupDir,
		RetentionFlags:     "--keep-daily 14 --keep-weekly 8 --keep-monthly 12",
		HealthcheckURL:     "",
		HealthcheckRetry:   DefaultRetry,
		StagingEnabled:     false,
		StagingHost:        "staging.example.com",
		StagingPort:        DefaultPort,
		StagingUser:        "ubuntu",
		StagingRoot:        "/var/www/html",
		AdminUsername:      "admin",
		AdminPassword:      "changeme",
		SecretKey:          "change-this-secret-key-in-production",
		APITokenDuration:   DefaultDuration,
	}
}

// Validate checks if required configuration is set
func (c *Config) Validate() error {
	if c.WPSSHHost == "" {
		return fmt.Errorf("WP_SSH_HOST is required")
	}
	if c.WPSSHUser == "" {
		return fmt.Errorf("WP_SSH_USER is required")
	}
	if c.WPRoot == "" {
		return fmt.Errorf("WP_ROOT is required")
	}
	if c.ResticRepository == "" {
		return fmt.Errorf("RESTIC_REPOSITORY is required")
	}
	if c.SecretKey == "" {
		return fmt.Errorf("SECRET_KEY is required")
	}
	return nil
}

// GetSSHOptions returns SSH options for remote connections
func (c *Config) GetSSHOptions() map[string]string {
	options := map[string]string{
		"BatchMode":             "yes",
		"StrictHostKeyChecking": "accept-new",
		"ConnectTimeout":        "15",
	}

	if c.WPSSHPort != DefaultPort {
		options["Port"] = fmt.Sprintf("%d", c.WPSSHPort)
	}

	return options
}
