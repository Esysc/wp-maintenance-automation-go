package apiserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
)

func TestHandleRehearsalSiteProxiesThroughPreviewPath(t *testing.T) {
	var gotPath, gotHost string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHost = r.Host
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>rehearsal preview</html>"))
	}))
	defer target.Close()

	s := setupTestAPIServer(t)
	job := &db.Job{
		ID:              "rehearsal-preview-1",
		Type:            "rehearsal",
		SiteID:          "site-1",
		Status:          "completed",
		Result:          `{"health_url":"` + target.URL + `","preview_url":"http://app.test/rehearsal-site/rehearsal-preview-1"}`,
		ProgressPercent: 100,
	}
	if err := s.Database.CreateJob(job); err != nil {
		t.Fatalf("failed to create job: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/rehearsal-site/rehearsal-preview-1/some/page?x=1", nil)
	req.Host = "app.test"
	rr := httptest.NewRecorder()
	s.handleRehearsalSite(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if gotPath != "/some/page" {
		t.Fatalf("proxied path = %q, want %q", gotPath, "/some/page")
	}
	if wantHost := strings.TrimPrefix(target.URL, "http://"); gotHost != wantHost {
		t.Fatalf("proxied host = %q, want %q", gotHost, wantHost)
	}
	if rr.Header().Get("X-Frame-Options") != "" {
		t.Fatalf("X-Frame-Options was not stripped: %q", rr.Header().Get("X-Frame-Options"))
	}
	if csp := rr.Header().Get("Content-Security-Policy"); strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("unsafe frame-ancestors was not replaced: %q", csp)
	}
	for _, directive := range []string{"frame-ancestors 'self'", "form-action 'none'", "connect-src 'none'", "sandbox allow-scripts"} {
		if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, directive) {
			t.Fatalf("CSP %q missing %q", csp, directive)
		}
	}
}

func TestHandleRehearsalSiteRedirectsToTrailingSlash(t *testing.T) {
	s := setupTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/rehearsal-site/job-1", nil)
	rr := httptest.NewRecorder()
	s.handleRehearsalSite(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusTemporaryRedirect)
	}
	if got := rr.Header().Get("Location"); got != "/rehearsal-site/job-1/" {
		t.Fatalf("Location = %q, want %q", got, "/rehearsal-site/job-1/")
	}
}

func TestHandleRehearsalSiteReturnsNotFoundForMissingJob(t *testing.T) {
	s := setupTestAPIServer(t)

	req := httptest.NewRequest(http.MethodGet, "/rehearsal-site/missing/", nil)
	rr := httptest.NewRecorder()
	s.handleRehearsalSite(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestRewriteRehearsalOrigins(t *testing.T) {
	origins := []string{"https://192.168.1.10:32773", "http://192.168.1.10:32774"}
	input := `<a href="https://192.168.1.10:32773/wp-admin/">admin</a>` +
		`<img src="http://192.168.1.10:32774/wp-content/a.png">` +
		`<script src="//192.168.1.10:32773/wp-includes/js/a.js"></script>`
	got := rewriteRehearsalOrigins(input, origins, "/rehearsal-site/job-1")
	want := `<a href="/rehearsal-site/job-1/wp-admin/">admin</a>` +
		`<img src="/rehearsal-site/job-1/wp-content/a.png">` +
		`<script src="/rehearsal-site/job-1/wp-includes/js/a.js"></script>`
	if got != want {
		t.Fatalf("rewriteRehearsalOrigins() = %q, want %q", got, want)
	}
}
