package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestScanFindsCommonWordPressIssues(t *testing.T) {
	mux := http.NewServeMux()
	handler := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodTrace {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Server", "Apache/2.4.69")
		w.Header().Set("X-Powered-By", "PHP/8.2.35")
		_, _ = w.Write([]byte(`<html><head><meta name="generator" content="WordPress 6.1">` +
			`<script src="http://cdn.example.test/app.js"></script></head>` +
			`<body>Warning: Undefined array key "x" in /var/www/html/wp-config.php</body></html>`))
	})
	mux.HandleFunc("/wp-login.php", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "wordpress_logged_in_test", Value: "abc", Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "wordpress_test_cookie", Value: "WP+Cookie+check", Path: "/"})
		_, _ = w.Write([]byte("Log In"))
	})
	mux.HandleFunc("/wp-admin/install.php", handler("Install WordPress"))
	mux.HandleFunc("/readme.html", handler("<h1>WordPress</h1>"))
	mux.HandleFunc("/license.txt", handler("WordPress license"))
	mux.HandleFunc("/wp-config.php.bak", handler("<?php define('DB_NAME', 'wp'); define('DB_PASSWORD', 'secret');"))
	mux.HandleFunc("/.env", handler("DB_PASSWORD=secret\nAPP_KEY=abc\n"))
	mux.HandleFunc("/.git/config", handler("[core]\n\trepositoryformatversion = 0\n"))
	mux.HandleFunc("/wp-content/debug.log", handler("PHP Fatal error: something failed"))
	mux.HandleFunc("/wp-content/uploads/", handler("<title>Index of /wp-content/uploads</title>"))
	mux.HandleFunc("/xmlrpc.php", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("XML-RPC server accepts POST requests only."))
	})
	mux.HandleFunc("/wp-json/wp/v2/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"admin","slug":"admin"}]`))
	})
	mux.HandleFunc("/wp-json/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Test"}`))
	})
	mux.HandleFunc("/.well-known/security.txt", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	report, err := Scan(context.Background(), server.URL, Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	want := map[string]Severity{
		"wp-config-backup":  SeverityCritical,
		"git-config":        SeverityCritical,
		"env-file":          SeverityHigh,
		"debug-output":      SeverityHigh,
		"debug-log":         SeverityHigh,
		"installer":         SeverityHigh,
		"hsts":              SeverityMedium,
		"csp":               SeverityMedium,
		"xmlrpc":            SeverityMedium,
		"user-enumeration":  SeverityMedium,
		"directory-listing": SeverityMedium,
		"mixed-content":     SeverityMedium,
		"cookie-secure":     SeverityMedium,
		"cookie-httponly":   SeverityMedium,
		"trace":             SeverityMedium,
		"generator":         SeverityLow,
		"readme":            SeverityLow,
		"server-banner":     SeverityLow,
		"powered-by":        SeverityLow,
		"security-txt":      SeverityInfo,
		"login":             SeverityInfo,
		"rest-api":          SeverityInfo,
	}
	found := map[string]Severity{}
	findingsByID := map[string]Finding{}
	for _, finding := range report.Findings {
		found[finding.ID] = finding.Severity
		findingsByID[finding.ID] = finding
	}
	for id, severity := range want {
		if got, ok := found[id]; !ok {
			t.Errorf("expected finding %q, got none", id)
		} else if got != severity {
			t.Errorf("finding %q severity = %q, want %q", id, got, severity)
		}
	}
	if report.Counts[string(SeverityCritical)] != 2 {
		t.Errorf("critical count = %d, want 2", report.Counts[string(SeverityCritical)])
	}
	if !strings.Contains(findingsByID["installer"].Action, "install.php") {
		t.Errorf("installer action = %q", findingsByID["installer"].Action)
	}
	if report.Summary() == "" {
		t.Fatal("Summary() is empty")
	}
}

func TestScanCleanSiteHasNoFindings(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodTrace {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=()")
		w.Header().Del("Server")
		w.Header().Del("X-Powered-By")
		_, _ = w.Write([]byte(`<html><head><title>Clean</title></head><body>Hello</body></html>`))
	})
	mux.HandleFunc("/.well-known/security.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Contact: security@example.test\n"))
	})

	server := httptest.NewTLSServer(mux)
	defer server.Close()

	report, err := Scan(context.Background(), server.URL, Options{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("expected no findings, got %d: %s", len(report.Findings), report.Summary())
	}
}

func TestScanRejectsInvalidTarget(t *testing.T) {
	if _, err := Scan(context.Background(), "not-a-url", Options{}); err == nil {
		t.Fatal("Scan() error = nil, want error")
	}
}

func TestReportSummary(t *testing.T) {
	report := &Report{
		Findings: []Finding{
			{ID: "a", Severity: SeverityHigh},
			{ID: "b", Severity: SeverityHigh},
			{ID: "c", Severity: SeverityInfo},
		},
		Counts: map[string]int{"high": 2, "info": 1},
	}
	if got := report.Summary(); got != "2 high, 1 info" {
		t.Fatalf("Summary() = %q", got)
	}
}

func TestScannerURLResolution(t *testing.T) {
	base := mustParse(t, "https://example.test:8443/")
	s := &scanner{base: base}
	if got := s.urlFor("/wp-admin/"); got != "https://example.test:8443/wp-admin/" {
		t.Fatalf("urlFor() = %q", got)
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
