package restore

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestSecureArchivePath(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name    string
		entry   string
		wantRel string
		wantErr bool
	}{
		{name: "root entry", entry: ".", wantRel: "."},
		{name: "nested file", entry: "wp/index.php", wantRel: "wp/index.php"},
		{name: "parent traversal", entry: "../escape.txt", wantErr: true},
		{name: "nested parent traversal", entry: "wp/../../escape.txt", wantErr: true},
		{name: "absolute path", entry: "/etc/passwd", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := secureArchivePath(root, tt.entry)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("secureArchivePath(%q) = %q, want error", tt.entry, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("secureArchivePath(%q) error = %v", tt.entry, err)
			}
			rel, err := filepath.Rel(root, got)
			if err != nil {
				t.Fatalf("filepath.Rel: %v", err)
			}
			if filepath.ToSlash(rel) != tt.wantRel {
				t.Fatalf("secureArchivePath(%q) rel = %q, want %q", tt.entry, rel, tt.wantRel)
			}
		})
	}
}

func TestRestoreBackupRejectsTraversal(t *testing.T) {
	backupDir := t.TempDir()
	timestamp := "20240101-000000"
	archiveDir := filepath.Join(backupDir, timestamp)
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatalf("failed to create archive dir: %v", err)
	}

	archivePath := filepath.Join(archiveDir, "backup.tar.gz")
	writeArchive(t, archivePath, map[string]string{
		"manifest.json": `{"timestamp":"` + timestamp + `"}`,
		"../escape.txt": "must not be written",
	})

	targetDir := filepath.Join(t.TempDir(), "target")
	rm, err := NewRestoreManager(backupDir, filepath.Join(t.TempDir(), "restore"))
	if err != nil {
		t.Fatalf("NewRestoreManager: %v", err)
	}

	if err := rm.RestoreBackup(timestamp, targetDir); err == nil {
		t.Fatal("RestoreBackup() = nil, want traversal error")
	}

	escapedPath := filepath.Join(filepath.Dir(targetDir), "escape.txt")
	if _, err := os.Stat(escapedPath); !os.IsNotExist(err) {
		t.Fatalf("traversal entry was written outside target: %s", escapedPath)
	}
}

func TestRestoreBackupExtractsRegularFiles(t *testing.T) {
	backupDir := t.TempDir()
	timestamp := "20240101-000001"
	archiveDir := filepath.Join(backupDir, timestamp)
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatalf("failed to create archive dir: %v", err)
	}

	writeArchive(t, filepath.Join(archiveDir, "backup.tar.gz"), map[string]string{
		"wp/index.php": "<?php // hello",
	})

	targetDir := filepath.Join(t.TempDir(), "target")
	rm, err := NewRestoreManager(backupDir, filepath.Join(t.TempDir(), "restore"))
	if err != nil {
		t.Fatalf("NewRestoreManager: %v", err)
	}

	if err := rm.RestoreBackup(timestamp, targetDir); err != nil {
		t.Fatalf("RestoreBackup() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(targetDir, "wp", "index.php"))
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(data) != "<?php // hello" {
		t.Fatalf("restored content = %q", string(data))
	}
}

func writeArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive: %v", err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("failed to write tar header %q: %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("failed to write tar content %q: %v", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
}
