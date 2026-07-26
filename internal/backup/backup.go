package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BackupManifest struct {
	Timestamp  string    `json:"timestamp"`
	Host       string    `json:"host"`
	WPRoot     string    `json:"wp_root"`
	WPVersion  string    `json:"wp_version"`
	DBName     string    `json:"db_name"`
	DBHost     string    `json:"db_host"`
	DBVersion  string    `json:"db_version"`
	DumpFile   string    `json:"dump_file"`
	FileCount  int       `json:"file_count"`
	RSExclude  string    `json:"rs_exclude"`
	Retention  string    `json:"retention"`
	SSHHost    string    `json:"ssh_host"`
	SSHUser    string    `json:"ssh_user"`
	SSHPort    int       `json:"ssh_port"`
	BackupSize int64     `json:"backup_size"`
	Checksum   string    `json:"checksum"`
	CreatedAt  time.Time `json:"created_at"`
}

type BackupManager struct {
	manifestFile string
	backupDir    string
}

func NewBackupManager(backupDir string) (*BackupManager, error) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	return &BackupManager{
		manifestFile: backupDir + "/manifest.json",
		backupDir:    backupDir,
	}, nil
}

// CreateBackup creates a new backup
func (bm *BackupManager) CreateBackup(manifest BackupManifest) error {
	// Create timestamped directory
	timestamp := manifest.Timestamp
	if timestamp == "" {
		timestamp = time.Now().Format("20060102_150405")
	}
	backupDir := filepath.Join(bm.backupDir, timestamp)

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Write manifest
	if err := bm.writeManifest(manifest, backupDir); err != nil {
		return err
	}

	return nil
}

// writeManifest writes the backup manifest
func (bm *BackupManager) writeManifest(manifest BackupManifest, dir string) error {
	manifestFile := filepath.Join(dir, "manifest.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}
	return os.WriteFile(manifestFile, data, 0644)
}

// ListBackups lists all available backups
func (bm *BackupManager) ListBackups() ([]BackupManifest, error) {
	var backups []BackupManifest

	entries, err := os.ReadDir(bm.backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(bm.backupDir, entry.Name(), "manifest.json")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var manifest BackupManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}

		backups = append(backups, manifest)
	}

	// Sort by timestamp descending
	for i := 0; i < len(backups)-1; i++ {
		for j := i + 1; j < len(backups); j++ {
			if backups[i].Timestamp < backups[j].Timestamp {
				backups[i], backups[j] = backups[j], backups[i]
			}
		}
	}

	return backups, nil
}

// GetBackup retrieves a specific backup by timestamp
func (bm *BackupManager) GetBackup(timestamp string) (*BackupManifest, error) {
	manifestPath := filepath.Join(bm.backupDir, timestamp, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return &manifest, nil
}

// DeleteBackup removes a backup
func (bm *BackupManager) DeleteBackup(timestamp string) error {
	backupDir := filepath.Join(bm.backupDir, timestamp)
	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}
	return nil
}

// CleanupOldBackups removes backups older than specified days
func (bm *BackupManager) CleanupOldBackups(days int) error {
	now := time.Now()
	cutoff := now.AddDate(0, 0, -days)

	entries, err := os.ReadDir(bm.backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			backupDir := filepath.Join(bm.backupDir, entry.Name())
			if err := os.RemoveAll(backupDir); err != nil {
				return fmt.Errorf("failed to remove backup %s: %w", entry.Name(), err)
			}
		}
	}

	return nil
}

// ParseRetentionFlags parses retention flags string
func ParseRetentionFlags(flags string) []string {
	if flags == "" {
		return []string{}
	}
	return strings.Split(flags, " ")
}

// GetBackupSize returns the size of a backup in bytes
func (bm *BackupManager) GetBackupSize(timestamp string) (int64, error) {
	backupDir := filepath.Join(bm.backupDir, timestamp)
	var size int64

	err := filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		return 0, err
	}

	return size, nil
}
