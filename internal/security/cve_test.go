package security

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLookupWPScanFiltersByVersionAndCaches(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Token test-token" {
			t.Errorf("missing WPScan authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/wordpresses/6.4.2":
			_, _ = w.Write([]byte(`{"6.4.2":{"vulnerabilities":[{"id":"core-1","title":"Core XSS","fixed_in":"6.4.3","cvss":{"score":6.1},"references":{"cve":["CVE-2024-0001"]}}]}}`))
		case "/plugins/foo":
			_, _ = w.Write([]byte(`{"foo":{"vulnerabilities":[` +
				`{"id":"plugin-1","title":"Remote code execution","fixed_in":"1.2.0","cvss":{"score":9.8,"vector":"CVSS:3.1/AV:N"},"references":{"cve":["CVE-2024-0002"],"url":["https://example.test/advisory"]}},` +
				`{"id":"plugin-2","title":"Already fixed","fixed_in":"0.9.0","cvss":{"score":5.0}}` +
				`]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	components := []Component{
		{Type: "core", Slug: "wordpress", Name: "WordPress", Version: "6.4.2", Active: true},
		{Type: "plugin", Slug: "foo", Name: "Foo", Version: "1.0.0", Active: true},
		{Type: "plugin", Slug: "inactive", Name: "Inactive", Version: "1.0", Active: false},
	}
	cache := NewMemoryCache(time.Hour)
	opts := VulnerabilityOptions{
		WPScanToken:   "test-token",
		WPScanBaseURL: server.URL,
		Cache:         cache,
		HTTPClient:    server.Client(),
	}

	findings, err := lookupWPScan(context.Background(), components, opts)
	if err != nil {
		t.Fatalf("lookupWPScan() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2 (core + active plugin)", requests)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %d, want 2: %#v", len(findings), findings)
	}
	byID := map[string]Finding{}
	for _, finding := range findings {
		byID[finding.ID] = finding
	}
	if byID["wpscan-plugin-1"].Severity != SeverityCritical {
		t.Errorf("plugin severity = %q, want critical", byID["wpscan-plugin-1"].Severity)
	}
	if !strings.Contains(byID["wpscan-plugin-1"].Recommendation, "1.2.0") {
		t.Errorf("recommendation = %q", byID["wpscan-plugin-1"].Recommendation)
	}
	if byID["wpscan-core-1"].Severity != SeverityMedium {
		t.Errorf("core severity = %q, want medium", byID["wpscan-core-1"].Severity)
	}
	if _, ok := byID["wpscan-plugin-2"]; ok {
		t.Error("already-fixed vulnerability was reported")
	}

	// A second lookup must use the cache and not call the API again.
	if _, err := lookupWPScan(context.Background(), components, opts); err != nil {
		t.Fatalf("second lookupWPScan() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests after cache hit = %d, want 2", requests)
	}
}

func TestLookupWPScanRateLimitAddsIncompleteFinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	findings, err := lookupWPScan(context.Background(), []Component{
		{Type: "plugin", Slug: "foo", Name: "Foo", Version: "1.0", Active: true},
		{Type: "plugin", Slug: "bar", Name: "Bar", Version: "1.0", Active: true},
	}, VulnerabilityOptions{
		WPScanToken:   "test-token",
		WPScanBaseURL: server.URL,
		Cache:         NewMemoryCache(time.Hour),
		HTTPClient:    server.Client(),
	})
	if err != nil {
		t.Fatalf("lookupWPScan() error = %v", err)
	}
	found := false
	for _, finding := range findings {
		if finding.ID == "cve-lookup-incomplete" {
			found = true
			if !strings.Contains(finding.Evidence, "Foo") || !strings.Contains(finding.Evidence, "Bar") {
				t.Errorf("incomplete evidence = %q", finding.Evidence)
			}
		}
	}
	if !found {
		t.Fatalf("expected cve-lookup-incomplete finding, got %#v", findings)
	}
}

func TestLookupWPScanWithoutToken(t *testing.T) {
	findings, err := lookupWPScan(context.Background(), nil, VulnerabilityOptions{})
	if err != nil {
		t.Fatalf("lookupWPScan() error = %v", err)
	}
	if len(findings) != 1 || findings[0].ID != "cve-lookup-not-configured" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestFindComposerPackages(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "wp-content", "plugins", "foo", "vendor", "composer", "installed.json")
	if err := os.MkdirAll(filepath.Dir(installed), 0755); err != nil {
		t.Fatal(err)
	}
	data := `{"packages":[{"name":"vendor/pkg","version":"v1.2.3","type":"library"},{"name":"vendor/meta","version":"1.0.0","type":"metapackage"}]}`
	if err := os.WriteFile(installed, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	packages := findComposerPackages(dir)
	if len(packages) != 1 {
		t.Fatalf("packages = %#v, want 1", packages)
	}
	if packages[0].Name != "vendor/pkg" || packages[0].Version != "1.2.3" {
		t.Fatalf("package = %#v", packages[0])
	}
}

func TestComposerPackagesFromContents(t *testing.T) {
	contents := []string{
		`{"packages":[{"name":"vendor/pkg","version":"1.2.3","type":"library"},{"name":"vendor/meta","version":"1.0.0","type":"metapackage"},{"name":"vendor/dev","version":"dev-main","type":"library"}]}`,
	}
	packages := ComposerPackages(contents)
	if len(packages) != 1 {
		t.Fatalf("packages = %#v, want 1", packages)
	}
	if packages[0].Name != "vendor/pkg" || packages[0].Version != "1.2.3" {
		t.Fatalf("package = %#v", packages[0])
	}
}

func TestLookupOSVFindsComposerVulnerability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/query" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var request osvQuery
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Package.Ecosystem != "Packagist" {
			t.Errorf("unexpected query: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"vulns":[{"id":"OSV-2026-1","summary":"Example dependency issue","database_specific":{"severity":"HIGH"},"aliases":["CVE-2026-9999"],"affected":[{"ranges":[{"events":[{"fixed":"2.0.0"}]}]}],"references":[{"url":"https://osv.dev/OSV-2026-1"}]}]}`))
	}))
	defer server.Close()

	findings, err := lookupOSV(context.Background(), []Component{
		{Type: "composer", Slug: "vendor/pkg", Name: "vendor/pkg", Version: "1.5.0", Active: true},
	}, VulnerabilityOptions{
		OSVBaseURL: server.URL,
		Cache:      NewMemoryCache(time.Hour),
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("lookupOSV() error = %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %#v", findings)
	}
	if findings[0].Severity != SeverityHigh {
		t.Errorf("severity = %q, want high", findings[0].Severity)
	}
	if !strings.Contains(findings[0].Recommendation, "2.0.0") {
		t.Errorf("recommendation = %q", findings[0].Recommendation)
	}
	if findings[0].Reference != "https://osv.dev/OSV-2026-1" {
		t.Errorf("reference = %q", findings[0].Reference)
	}
}

func TestSelectFixedVersion(t *testing.T) {
	cases := []struct {
		installed  string
		candidates []string
		want       string
	}{
		{"7.0.0", []string{"6.5.8", "7.12.1", "7.14.2"}, "7.12.1"},
		{"1.0.0", []string{"2.0.0"}, "2.0.0"},
		{"2.0.0", []string{"1.0.0"}, ""},
	}
	for _, c := range cases {
		if got := selectFixedVersion(c.installed, c.candidates); got != c.want {
			t.Errorf("selectFixedVersion(%q, %v) = %q, want %q", c.installed, c.candidates, got, c.want)
		}
	}
}

func TestLookupPHPEOL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/php.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"cycle":"8.1","eol":"2025-12-31"},{"cycle":"8.3","eol":"2027-12-31"}]`))
	}))
	defer server.Close()

	findings, err := lookupPHPEOL(context.Background(), Component{Type: "php", Version: "8.1.27"}, VulnerabilityOptions{
		EndOfLifeBaseURL: server.URL,
		Cache:            NewMemoryCache(time.Hour),
		HTTPClient:       server.Client(),
	})
	if err != nil {
		t.Fatalf("lookupPHPEOL() error = %v", err)
	}
	if len(findings) != 1 || findings[0].ID != "php-eol" {
		t.Fatalf("findings = %#v", findings)
	}
}

func TestVersionAffected(t *testing.T) {
	tests := []struct {
		name       string
		installed  string
		introduced string
		fixed      string
		want       bool
	}{
		{"older than fix", "1.0.0", "", "1.2.0", true},
		{"equal to fix", "1.2.0", "", "1.2.0", false},
		{"newer than fix", "1.3.0", "", "1.2.0", false},
		{"before introduction", "0.8.0", "0.9.0", "1.2.0", false},
		{"no fixed version", "1.0.0", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionAffected(tt.installed, tt.introduced, tt.fixed); got != tt.want {
				t.Fatalf("versionAffected() = %v, want %v", got, tt.want)
			}
		})
	}
}
