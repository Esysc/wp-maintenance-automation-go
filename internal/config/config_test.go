package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig()
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if cfg.WPSSHHost != "example.com" {
		t.Errorf("expected host 'example.com', got '%s'", cfg.WPSSHHost)
	}

	if cfg.BackupDir != "./backup_artifacts" {
		t.Errorf("expected backup dir './backup_artifacts', got '%s'", cfg.BackupDir)
	}

	if cfg.AdminUsername != "admin" {
		t.Errorf("expected admin username 'admin', got '%s'", cfg.AdminUsername)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.WPSSHHost = "myserver.com"
	cfg.WPSSHUser = "myuser"
	cfg.WPRoot = "/var/www/mysite"

	configPath := filepath.Join(t.TempDir(), "config.json")

	err := SaveConfig(cfg, configPath)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	loadedCfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loadedCfg.WPSSHHost != "myserver.com" {
		t.Errorf("expected host 'myserver.com', got '%s'", loadedCfg.WPSSHHost)
	}

	if loadedCfg.WPSSHUser != "myuser" {
		t.Errorf("expected user 'myuser', got '%s'", loadedCfg.WPSSHUser)
	}

	if loadedCfg.WPRoot != "/var/www/mysite" {
		t.Errorf("expected wp root '/var/www/mysite', got '%s'", loadedCfg.WPRoot)
	}
}

func TestLoadConfigNotFound(t *testing.T) {
	cfg, err := LoadConfig("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("expected no error for missing config, got: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected default config for missing file")
	}

	if GlobalConfig != cfg {
		t.Fatal("expected LoadConfig to publish the default config as GlobalConfig")
	}
}

func TestSaveConfigUsesOwnerOnlyPermissions(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")

	if err := SaveConfig(NewDefaultConfig(), configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("new config permissions = %o, want 0600", perm)
	}

	// Tighten an existing world-readable config as well.
	if err := os.Chmod(configPath, 0644); err != nil {
		t.Fatalf("failed to chmod config: %v", err)
	}
	if err := SaveConfig(NewDefaultConfig(), configPath); err != nil {
		t.Fatalf("failed to re-save config: %v", err)
	}
	info, err = os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("existing config permissions = %o, want 0600", perm)
	}
}

func TestValidateConfig(t *testing.T) {
	cfg := NewDefaultConfig()

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("default config should validate: %v", err)
	}

	emptyCfg := &Config{}
	err = emptyCfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty config")
	}
}

func TestGetSSHOptions(t *testing.T) {
	cfg := NewDefaultConfig()

	opts := cfg.GetSSHOptions()
	if opts == nil {
		t.Fatal("expected non-nil SSH options")
	}

	if opts["BatchMode"] != "yes" {
		t.Errorf("expected BatchMode 'yes', got '%s'", opts["BatchMode"])
	}

	cfg.WPSSHPort = 2222
	opts = cfg.GetSSHOptions()
	if opts["Port"] != "2222" {
		t.Errorf("expected Port '2222', got '%s'", opts["Port"])
	}
}

func TestGlobalConfig(t *testing.T) {
	GlobalConfig = NewDefaultConfig()
	if GlobalConfig == nil {
		t.Fatal("expected non-nil global config")
	}

	if GlobalConfig.ResticRepository != "/path/to/restic-repo" {
		t.Errorf("expected restic repo '/path/to/restic-repo', got '%s'", GlobalConfig.ResticRepository)
	}
}
