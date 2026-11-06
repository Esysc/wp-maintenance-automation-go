package apiserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/version"
)

func TestHandleHealthReportsEmbeddedVersion(t *testing.T) {
	s := setupTestAPIServer(t)
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	rr := httptest.NewRecorder()

	s.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handleHealth() status = %d, want %d", rr.Code, http.StatusOK)
	}

	var body struct {
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Data.Version != version.Version {
		t.Fatalf("health version = %q, want %q", body.Data.Version, version.Version)
	}
}

func TestEnsureEncryptionKey(t *testing.T) {
	t.Run("explicit key wins", func(t *testing.T) {
		t.Setenv("DATA_ENCRYPTION_KEY", "explicit-key")
		t.Setenv("SECRET_KEY", "")
		dir := t.TempDir()

		if err := ensureEncryptionKey(dir); err != nil {
			t.Fatalf("ensureEncryptionKey() error = %v", err)
		}
		if got := os.Getenv("DATA_ENCRYPTION_KEY"); got != "explicit-key" {
			t.Fatalf("DATA_ENCRYPTION_KEY = %q, want explicit-key", got)
		}
		if _, err := os.Stat(filepath.Join(dir, "encryption.key")); !os.IsNotExist(err) {
			t.Fatalf("expected no generated key file, stat error = %v", err)
		}
	})

	t.Run("generates and persists a key", func(t *testing.T) {
		t.Setenv("DATA_ENCRYPTION_KEY", "")
		t.Setenv("SECRET_KEY", "")
		dir := t.TempDir()

		if err := ensureEncryptionKey(dir); err != nil {
			t.Fatalf("ensureEncryptionKey() error = %v", err)
		}
		first := os.Getenv("DATA_ENCRYPTION_KEY")
		if first == "" {
			t.Fatal("expected a generated encryption key")
		}

		keyPath := filepath.Join(dir, "encryption.key")
		info, err := os.Stat(keyPath)
		if err != nil {
			t.Fatalf("failed to stat generated key: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Fatalf("generated key permissions = %o, want 0600", perm)
		}

		// Simulate a restart: clear the environment and load the key again.
		if err := os.Setenv("DATA_ENCRYPTION_KEY", ""); err != nil {
			t.Fatalf("failed to clear encryption key: %v", err)
		}
		if err := ensureEncryptionKey(dir); err != nil {
			t.Fatalf("ensureEncryptionKey() on restart error = %v", err)
		}
		if got := os.Getenv("DATA_ENCRYPTION_KEY"); got != first {
			t.Fatalf("restarted key = %q, want %q", got, first)
		}
	})
}
