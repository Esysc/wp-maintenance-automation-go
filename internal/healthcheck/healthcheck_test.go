package healthcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewChecker(t *testing.T) {
	hc := NewChecker()
	if hc == nil {
		t.Fatal("expected non-nil checker")
	}
}

func TestQuickCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	hc := NewChecker()
	passed, err := hc.QuickCheck(server.URL)
	if err != nil {
		t.Fatalf("quick check failed: %v", err)
	}

	if !passed {
		t.Error("expected check to pass")
	}
}

func TestQuickCheckFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	hc := NewChecker()
	passed, err := hc.QuickCheck(server.URL)
	if err != nil {
		t.Fatalf("quick check should not error on unexpected status: %v", err)
	}

	if passed {
		t.Error("expected check to fail for 500 response")
	}
}

func TestCheckWithExpectedCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewChecker()
	opts := NewDefaultOptions(server.URL)
	opts.ExpectedCode = 200
	opts.MaxRetries = 3
	result, err := hc.Check(opts)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}

	if result["passed"].(bool) != true {
		t.Error("expected check to pass")
	}

	if result["max_attempts"].(int) != 3 {
		t.Errorf("expected 3 max attempts, got %d", result["max_attempts"])
	}
}

func TestNewDefaultOptions(t *testing.T) {
	opts := NewDefaultOptions("https://example.com")
	if opts.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got '%s'", opts.URL)
	}
	if opts.ExpectedCode != 200 {
		t.Errorf("expected expected code 200, got %d", opts.ExpectedCode)
	}
	if opts.MaxRetries != 5 {
		t.Errorf("expected 5 max retries, got %d", opts.MaxRetries)
	}
}

func TestURLRequired(t *testing.T) {
	hc := NewChecker()
	_, err := hc.QuickCheck("")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestInsecureSkipVerify(t *testing.T) {
	hc := NewChecker()
	opts := NewDefaultOptions("https://localhost:9999")
	opts.Insecure = true
	opts.MaxRetries = 1

	_, err := hc.Check(opts)
	if err == nil {
		t.Log("expected connection error (localhost:9999 not running)")
	}
}

func TestCheckResultFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewChecker()
	opts := NewDefaultOptions(server.URL)
	result, err := hc.Check(opts)
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}

	if result["status_code"].(int) != 200 {
		t.Errorf("expected status 200, got %d", result["status_code"])
	}

	if result["response_time_ms"].(int64) == 0 && result["passed"].(bool) {
		t.Log("response time was 0ms (fast localhost)")
	}
}
