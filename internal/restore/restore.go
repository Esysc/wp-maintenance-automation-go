package restore

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RestoreManager struct {
	backupDir  string
	restoreDir string
}

func NewRestoreManager(backupDir, restoreDir string) (*RestoreManager, error) {
	if err := os.MkdirAll(restoreDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create restore directory: %w", err)
	}

	return &RestoreManager{
		backupDir:  backupDir,
		restoreDir: restoreDir,
	}, nil
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
	RSExclude string `json:"rs_exclude"`
	Retention string `json:"retention"`
	SSHHost   string `json:"ssh_host"`
	SSHUser   string `json:"ssh_user"`
	SSHPort   int    `json:"ssh_port"`
}

func (rm *RestoreManager) RestoreBackup(timestamp, targetDir string) error {
	backupPath := filepath.Join(rm.backupDir, timestamp, "backup.tar.gz")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup archive not found: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		targetPath, err := secureArchivePath(targetDir, header.Name)
		if err != nil {
			return fmt.Errorf("unsafe tar entry %q: %w", header.Name, err)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)&0o777); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			if err := outFile.Close(); err != nil {
				return fmt.Errorf("failed to close file: %w", err)
			}
		}
	}

	return nil
}

// secureArchivePath joins an archive entry name to root and rejects entries
// that would escape root (absolute paths or ".." traversal).
func secureArchivePath(root, name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(name))
	if cleaned == "." {
		return filepath.Clean(root), nil
	}
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("archive entry has an absolute path")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry escapes the destination")
	}

	target := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry escapes the destination")
	}
	return target, nil
}

func (rm *RestoreManager) GetBackupManifest(timestamp string) (*BackupManifest, error) {
	manifestPath := filepath.Join(rm.backupDir, timestamp, "manifest.json")
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

func (rm *RestoreManager) ApplyDBRestore(backupManifest *BackupManifest, remoteHost, remoteUser, remoteRoot, dbPassword string) error {
	return fmt.Errorf("DB restore to remote not yet implemented: %s", remoteHost)
}

func (rm *RestoreManager) ApplyFileRestore(backupManifest *BackupManifest, remoteHost, remoteUser, remoteRoot string, deleteRemoteFiles bool) error {
	return fmt.Errorf("File restore to remote not yet implemented: %s", remoteHost)
}

func (rm *RestoreManager) ApplyConfigRestore(backupManifest *BackupManifest, remoteHost, remoteUser, remoteRoot string) error {
	return fmt.Errorf("Config restore to remote not yet implemented: %s", remoteHost)
}

func (rm *RestoreManager) CreateRestoreReport(timestamp, targetDir string, success bool, message string) error {
	reportPath := filepath.Join(targetDir, fmt.Sprintf("restore_%s_report.txt", timestamp))
	status := "Failed"
	if success {
		status = "Success"
	}
	report := fmt.Sprintf("Restore Report\n%s\n\nStatus: %s\nMessage: %s\nTimestamp: %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		status, message, timestamp,
	)
	return os.WriteFile(reportPath, []byte(report), 0644)
}

func (rm *RestoreManager) ValidateRestore(targetDir string) error {
	requiredFiles := []string{"manifest.json", "backup.tar.gz"}
	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(targetDir, file)); os.IsNotExist(err) {
			return fmt.Errorf("required file not found: %s", file)
		}
	}
	return ValidateBackup(filepath.Join(targetDir, "backup.tar.gz"))
}

func (rm *RestoreManager) CleanupRestore(targetDir string) error {
	return os.RemoveAll(targetDir)
}

func ValidateBackup(archivePath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open backup archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		if header.Name == "manifest.json" {
			var manifest BackupManifest
			if err := json.NewDecoder(tarReader).Decode(&manifest); err != nil {
				return fmt.Errorf("invalid manifest in backup: %w", err)
			}
		}
	}

	return nil
}
