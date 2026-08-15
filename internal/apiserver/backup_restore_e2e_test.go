package apiserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/auth"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/healthcheck"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/restic"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/ssh"
)

// TestBackupMessUpRestoreRoundTrip is an end-to-end disaster-recovery drill.
//
// It builds a fake production WordPress site (files + database) served through
// a fake SSH client, runs a real backup job through the API and job worker
// (including a real restic repository), deliberately destroys the site and its
// database, then restores from the backup and verifies the site is bit-for-bit
// identical to the original state.
func TestBackupMessUpRestoreRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		t.Skip("restic binary not available; skipping backup/restore round-trip test")
	}

	fake := newFakeSSHClient(t)
	if err := fake.seedFakeProdSite(); err != nil {
		t.Fatalf("failed to seed fake prod site: %v", err)
	}

	original := captureFakeProdState(t, fake)
	if !fakeProdSiteHealthy(fake) {
		t.Fatal("seeded fake prod site should be healthy")
	}

	s := setupRoundTripServer(t, fake)
	defer s.Database.Close()

	token := loginTestAdmin(t, s)

	resticRepo := filepath.Join(t.TempDir(), "repo")
	resticPassFile := filepath.Join(t.TempDir(), "restic_password")
	if err := os.WriteFile(resticPassFile, []byte("test-restic-password"), 0600); err != nil {
		t.Fatalf("failed to write restic password file: %v", err)
	}
	if err := os.MkdirAll(resticRepo, 0700); err != nil {
		t.Fatalf("failed to create restic repo dir: %v", err)
	}
	resticClient, err := restic.NewClient(resticRepo, resticPassFile)
	if err != nil {
		t.Fatalf("failed to create restic client: %v", err)
	}
	if err := resticClient.Init(); err != nil {
		t.Fatalf("failed to init restic repo: %v", err)
	}
	s.Restic = resticClient

	siteID := createFakeSite(t, s, token, fake, resticRepo, resticPassFile)

	// 1) Take a backup.
	backupJobID := enqueueJobViaAPI(t, s, token, "/api/v1/backup", map[string]interface{}{"site_id": siteID})
	backupJob := waitForJob(t, s, token, backupJobID)
	if backupJob.Status != "completed" {
		t.Fatalf("backup job failed: status=%s error=%s", backupJob.Status, backupJob.Error)
	}

	backups, err := s.Database.GetBackupsBySite(siteID)
	if err != nil {
		t.Fatalf("failed to list backups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected exactly 1 backup record, got %d", len(backups))
	}
	snapshotID := backups[0].SnapshotID
	if snapshotID == "" {
		t.Fatal("expected a restic snapshot_id on the backup record")
	}

	snapshots, err := resticClient.Snapshots()
	if err != nil {
		t.Fatalf("failed to list restic snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 restic snapshot, got %d", len(snapshots))
	}

	// 2) Deliberately destroy the site and its database.
	messUpFakeProdSite(fake)
	if fakeProdSiteHealthy(fake) {
		t.Fatal("fake prod site should be broken after the intentional mess-up")
	}
	// The backup itself must survive the disaster.
	if _, err := resticClient.Snapshots(); err != nil {
		t.Fatalf("backup was lost: %v", err)
	}

	// 3) Restore from the backup.
	restoreJobID := enqueueJobViaAPI(t, s, token, "/api/v1/restore", map[string]interface{}{
		"site_id":     siteID,
		"snapshot_id": snapshotID,
		"apply_db":    true,
		"apply_files": true,
		"confirm":     true,
	})
	restoreJob := waitForJob(t, s, token, restoreJobID)
	if restoreJob.Status != "completed" {
		t.Fatalf("restore job failed: status=%s error=%s", restoreJob.Status, restoreJob.Error)
	}

	// 4) Everything must be exactly like before.
	restored := captureFakeProdState(t, fake)
	if !reflect.DeepEqual(original, restored) {
		t.Fatalf("fake prod site does not match the original state after restore\nwanted: %v\ngot:    %v", original, restored)
	}
	if !fakeProdSiteHealthy(fake) {
		t.Fatal("fake prod site should be healthy again after restore")
	}
}

// TestRestoreDryRunRejectedWithoutConfirm checks the safety gate: a restore
// without confirm=true must not enqueue a job.
func TestRestoreDryRunRejectedWithoutConfirm(t *testing.T) {
	fake := newFakeSSHClient(t)
	if err := fake.seedFakeProdSite(); err != nil {
		t.Fatalf("failed to seed fake prod site: %v", err)
	}

	s := setupRoundTripServer(t, fake)
	defer s.Database.Close()

	token := loginTestAdmin(t, s)
	resticRepo := filepath.Join(t.TempDir(), "repo")
	resticPassFile := filepath.Join(t.TempDir(), "restic_password")
	os.WriteFile(resticPassFile, []byte("test-restic-password"), 0600)
	os.MkdirAll(resticRepo, 0700)
	resticClient, _ := restic.NewClient(resticRepo, resticPassFile)
	if err := resticClient.Init(); err != nil {
		t.Fatalf("failed to init restic repo: %v", err)
	}
	s.Restic = resticClient

	siteID := createFakeSite(t, s, token, fake, resticRepo, resticPassFile)

	body, _ := json.Marshal(map[string]interface{}{
		"site_id":     siteID,
		"snapshot_id": "does-not-exist",
		"apply_db":    true,
		"apply_files": true,
		"confirm":     false,
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	s.handleRestore(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected dry-run response 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Status  string `json:"status"`
			Warning string `json:"warning"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse dry-run response: %v", err)
	}
	if resp.Data.Status != "pending" || !strings.Contains(resp.Data.Warning, "confirm=true") {
		t.Fatalf("expected dry-run warning, got %+v", resp.Data)
	}

	// No job should have been enqueued.
	jobs, err := s.Database.ListJobsBySite(siteID)
	if err != nil {
		t.Fatalf("failed to list jobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected no jobs enqueued for a dry-run restore, got %d", len(jobs))
	}
}

// setupRoundTripServer builds an APIServer wired to a fake SSH client and a
// started job worker, backed by a sqlite test database.
func setupRoundTripServer(t *testing.T, fake *fakeSSHClient) *APIServer {
	t.Helper()

	dir := t.TempDir()
	database, err := db.New(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}

	s := &APIServer{
		Auth:         auth.NewAuthManager(database, "test-secret-key"),
		Database:     database,
		Checker:      healthcheck.NewChecker(),
		NewSSHClient: func(opts *ssh.SSHOptions) ssh.Client { return fake },
		jobQueue:     make(chan *jobTuple, jobQueueSize),
	}
	s.startWorker()

	t.Cleanup(func() {
		s.workerOnce.Do(func() {}) // no-op; keep cleanup simple
	})

	return s
}

func createFakeSite(t *testing.T, s *APIServer, token string, fake *fakeSSHClient, resticRepo, resticPassFile string) string {
	t.Helper()

	body, _ := json.Marshal(map[string]interface{}{
		"name":                 "Fake Prod Site",
		"wp_ssh_host":          "fake-prod.example.com",
		"wp_ssh_port":          22,
		"wp_ssh_user":          "deploy",
		"wp_root":              fake.siteRoot,
		"db_host":              fake.dbHost,
		"db_user":              fake.dbUser,
		"db_password":          fake.dbPassword,
		"db_name":              fake.dbName,
		"restic_repository":    resticRepo,
		"restic_password_file": resticPassFile,
		"backup_dir":           t.TempDir(),
		"healthcheck_url":      "http://fake-prod.example.com",
		"staging_enabled":      false,
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/sites", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	s.handleSites(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("failed to create site: %d %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse create site response: %v", err)
	}
	siteID, _ := resp.Data["id"].(string)
	if siteID == "" {
		t.Fatal("create site response missing id")
	}
	return siteID
}

func enqueueJobViaAPI(t *testing.T, s *APIServer, token, endpoint string, payload map[string]interface{}) string {
	t.Helper()

	body, _ := json.Marshal(payload)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	var handler http.HandlerFunc
	switch endpoint {
	case "/api/v1/backup":
		handler = s.handleBackup
	case "/api/v1/restore":
		handler = s.handleRestore
	default:
		t.Fatalf("unsupported endpoint %q", endpoint)
	}
	handler(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("enqueue %s failed: %d %s", endpoint, rr.Code, rr.Body.String())
	}

	var resp struct {
		Data struct {
			JobID string `json:"job_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse enqueue response: %v", err)
	}
	if resp.Data.JobID == "" {
		t.Fatalf("enqueue %s response missing job_id", endpoint)
	}
	return resp.Data.JobID
}

func waitForJob(t *testing.T, s *APIServer, token, jobID string) *db.Job {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		job, err := s.Database.GetJob(jobID)
		if err != nil {
			t.Fatalf("failed to get job %s: %v", jobID, err)
		}
		switch job.Status {
		case "completed", "failed", "cancelled":
			return job
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for job %s (last status: %s)", jobID, func() string {
		job, err := s.Database.GetJob(jobID)
		if err != nil {
			return "unknown"
		}
		return job.Status
	}())
	return nil
}

// fakeProdSiteHealthy reports whether the fake prod site would serve requests
// and whether its database is intact.
func fakeProdSiteHealthy(fake *fakeSSHClient) bool {
	index, err := os.ReadFile(filepath.Join(fake.siteDir, "index.php"))
	if err != nil || !strings.Contains(string(index), "FAKE PROD INDEX") {
		return false
	}
	version, err := os.ReadFile(filepath.Join(fake.siteDir, "wp-includes", "version.php"))
	if err != nil || !strings.Contains(string(version), "$wp_version") {
		return false
	}
	uploads, err := os.ReadFile(filepath.Join(fake.siteDir, "wp-content", "uploads", "2026", "08", "photo.jpg"))
	if err != nil || len(uploads) == 0 {
		return false
	}
	dbData, err := os.ReadFile(fake.dbFile)
	if err != nil {
		return false
	}
	return strings.Contains(string(dbData), "-- Fake prod database dump") &&
		strings.Contains(string(dbData), "INSERT INTO wp_posts")
}

// messUpFakeProdSite destroys the fake site: it removes core files, corrupts
// the front page, deletes uploads and corrupts the database so the site stops
// working.
func messUpFakeProdSite(fake *fakeSSHClient) {
	os.RemoveAll(filepath.Join(fake.siteDir, "wp-includes"))
	os.RemoveAll(filepath.Join(fake.siteDir, "wp-content", "uploads"))
	os.WriteFile(filepath.Join(fake.siteDir, "index.php"), []byte("<?php /* CORRUPTED BY TEST */ die('broken'); ?>"), 0644)
	os.WriteFile(fake.dbFile, []byte("CORRUPTED DATABASE -- this is not a valid SQL dump at all"), 0644)
}

// prodState is a content fingerprint (relPath -> sha256) of the site files
// plus the database dump.
type prodState struct {
	files map[string]string
	db    string
}

func captureFakeProdState(t *testing.T, fake *fakeSSHClient) prodState {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(fake.siteDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(fake.siteDir, path)
		sum, err := sha256File(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = sum
		return nil
	})
	if err != nil {
		t.Fatalf("failed to fingerprint site files: %v", err)
	}

	dbSum, err := sha256File(fake.dbFile)
	if err != nil {
		t.Fatalf("failed to fingerprint database dump: %v", err)
	}

	return prodState{files: files, db: dbSum}
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
