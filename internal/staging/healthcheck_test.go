package staging

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompareStagingPagesIgnoresHostDifferences(t *testing.T) {
	prod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><head><title>Example Site</title></head><body><h1>Welcome</h1><a href="https://prod.example.com/about">About</a></body></html>`)
	}))
	defer prod.Close()

	rehearsal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><head><title>Example Site</title></head><body><h1>Welcome</h1><a href="https://localhost:8443/about">About</a></body></html>`)
	}))
	defer rehearsal.Close()

	result, err := CompareStagingPages(prod.URL, rehearsal.URL, &HealthcheckOptions{
		Timeout:    2 * time.Second,
		MaxRetries: 1,
		Delay:      0,
	})
	if err != nil {
		t.Fatalf("expected comparison to succeed, got error: %v", err)
	}
	if !result.Passed {
		t.Fatalf("expected comparison to pass, got %+v", result)
	}
}

func TestCompareStagingPagesDetectsContentMismatch(t *testing.T) {
	prod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><head><title>Example Site</title></head><body><h1>Welcome</h1></body></html>`)
	}))
	defer prod.Close()

	rehearsal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><head><title>Example Site</title></head><body><h1>Different content</h1></body></html>`)
	}))
	defer rehearsal.Close()

	result, err := CompareStagingPages(prod.URL, rehearsal.URL, &HealthcheckOptions{
		Timeout:    2 * time.Second,
		MaxRetries: 1,
		Delay:      0,
	})
	if err != nil {
		t.Fatalf("comparison should return a result even on mismatch: %v", err)
	}
	if result.Passed {
		t.Fatalf("expected comparison to fail, got %+v", result)
	}
}
