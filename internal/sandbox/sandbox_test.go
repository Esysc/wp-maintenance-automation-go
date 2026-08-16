package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWpArgsRetry_RetriesUntilSuccess(t *testing.T) {
	manager := &Manager{}
	attempts := 0
	manager.wpRunner = func(args ...string) (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("transient wp-cli failure")
		}
		return "ok", nil
	}

	start := time.Now()
	out, err := manager.wpArgsRetry("option", "update", "siteurl", "https://example.test")
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected output %q, got %q", "ok", out)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if elapsed := time.Since(start); elapsed < 1400*time.Millisecond {
		t.Fatalf("expected retry backoff to take noticeable time, got %s", elapsed)
	}
}

func TestWpArgsRetry_ReturnsLastError(t *testing.T) {
	manager := &Manager{}
	attempts := 0
	manager.wpRunner = func(args ...string) (string, error) {
		attempts++
		return "", errors.New("persistent wp-cli failure")
	}

	out, err := manager.wpArgsRetry("rewrite", "structure", "/%postname%/")
	if err == nil {
		t.Fatal("expected error after retries, got nil")
	}
	if out != "" {
		t.Fatalf("expected empty output on failure, got %q", out)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
	if !strings.Contains(err.Error(), "persistent wp-cli failure") {
		t.Fatalf("expected last error to be returned, got %v", err)
	}
}

func TestMustRead_ReturnsEmptyStringWhenMissing(t *testing.T) {
	if got := mustRead(filepath.Join(t.TempDir(), "missing.txt")); got != "" {
		t.Fatalf("expected empty string for missing file, got %q", got)
	}
}

func TestSaveState_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	manager := &Manager{DataDir: dir}
	state := &State{SiteID: "site-1", Broken: true, LastAction: "broken", HealthURL: "https://sandbox.test"}

	if err := manager.saveState(state); err != nil {
		t.Fatalf("saveState returned error: %v", err)
	}

	loaded := manager.loadState()
	if loaded == nil {
		t.Fatal("expected state to load back, got nil")
	}
	if loaded.SiteID != state.SiteID || loaded.LastAction != state.LastAction || !loaded.Broken {
		t.Fatalf("loaded state mismatch: %#v", loaded)
	}
}

func TestSaveState_ReturnsErrorWhenDataDirIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatalf("failed to create sentinel file: %v", err)
	}

	manager := &Manager{DataDir: file}
	if err := manager.saveState(&State{SiteID: "site-1"}); err == nil {
		t.Fatal("expected saveState to fail when DataDir is a file, got nil")
	}
}
