package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewBackupManager(t *testing.T) {
	dir := t.TempDir()
	bm, err := NewBackupManager(dir)
	if err != nil {
		t.Fatalf("failed to create backup manager: %v", err)
	}

	if bm == nil {
		t.Fatal("expected non-nil backup manager")
	}
}

func TestCreateAndListBackups(t *testing.T) {
	dir := t.TempDir()
	bm, _ := NewBackupManager(dir)

	manifest := BackupManifest{
		Timestamp: time.Now().Format("20060102_150405"),
		Host:      "example.com",
		WPRoot:    "/var/www/html",
		WPVersion: "6.6.2",
		DBName:    "wordpress",
		FileCount: 1234,
	}

	err := bm.CreateBackup(manifest)
	if err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}

	backups, err := bm.ListBackups()
	if err != nil {
		t.Fatalf("failed to list backups: %v", err)
	}

	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}

	if backups[0].Host != "example.com" {
		t.Errorf("expected host 'example.com', got '%s'", backups[0].Host)
	}
}

func TestGetBackup(t *testing.T) {
	dir := t.TempDir()
	bm, _ := NewBackupManager(dir)

	manifest := BackupManifest{
		Timestamp: "20260723_120000",
		Host:      "test.com",
		WPRoot:    "/var/www/test",
	}

	bm.CreateBackup(manifest)

	found, err := bm.GetBackup("20260723_120000")
	if err != nil {
		t.Fatalf("failed to get backup: %v", err)
	}

	if found.Host != "test.com" {
		t.Errorf("expected host 'test.com', got '%s'", found.Host)
	}
}

func TestGetBackupNotFound(t *testing.T) {
	dir := t.TempDir()
	bm, _ := NewBackupManager(dir)

	_, err := bm.GetBackup("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestDeleteBackup(t *testing.T) {
	dir := t.TempDir()
	bm, _ := NewBackupManager(dir)

	ts := "20260723_120000"
	manifest := BackupManifest{
		Timestamp: ts,
		Host:      "test.com",
	}

	bm.CreateBackup(manifest)

	err := bm.DeleteBackup(ts)
	if err != nil {
		t.Fatalf("failed to delete backup: %v", err)
	}

	backups, _ := bm.ListBackups()
	if len(backups) != 0 {
		t.Errorf("expected 0 backups after delete, got %d", len(backups))
	}
}

func TestCleanupOldBackups(t *testing.T) {
	dir := t.TempDir()
	bm, _ := NewBackupManager(dir)

	oldTime := time.Now().AddDate(0, 0, -31)
	oldDir := filepath.Join(dir, "old_backup")
	os.MkdirAll(oldDir, 0755)
	os.WriteFile(filepath.Join(oldDir, "manifest.json"), []byte(`{"timestamp":"old_backup"}`), 0644)
	os.Chtimes(oldDir, oldTime, oldTime)

	newDir := filepath.Join(dir, "new_backup")
	os.MkdirAll(newDir, 0755)
	os.WriteFile(filepath.Join(newDir, "manifest.json"), []byte(`{"timestamp":"new_backup"}`), 0644)

	err := bm.CleanupOldBackups(30)
	if err != nil {
		t.Fatalf("failed to cleanup old backups: %v", err)
	}

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("expected old backup to be removed")
	}

	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		t.Error("expected new backup to remain")
	}
}

func TestParseRetentionFlags(t *testing.T) {
	flags := ParseRetentionFlags("--keep-daily 14 --keep-weekly 8")
	if len(flags) != 4 {
		t.Errorf("expected 4 flags, got %d", len(flags))
	}

	empty := ParseRetentionFlags("")
	if len(empty) != 0 {
		t.Errorf("expected 0 flags for empty string, got %d", len(empty))
	}
}

func TestBackupManifestDefaultValues(t *testing.T) {
	m := BackupManifest{}
	if m.DBName != "" {
		t.Errorf("expected empty DBName, got '%s'", m.DBName)
	}
}
