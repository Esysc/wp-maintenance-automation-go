package apiserver

import (
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// handleSandbox routes sandbox requests. The sandbox is a local,
// production-like WordPress test site used to practice backup and restore
// drills: you can start it, take a backup, intentionally break it, and
// restore it.
//
//	GET    /api/v1/sandbox/status
//	POST   /api/v1/sandbox/start
//	POST   /api/v1/sandbox/break
//	POST   /api/v1/sandbox/stop
//	DELETE /api/v1/sandbox      (or /api/v1/sandbox/delete)
func (s *APIServer) handleSandbox(w http.ResponseWriter, r *http.Request) {
	if s.Sandbox == nil {
		apiErr(w, http.StatusServiceUnavailable, "sandbox is not configured")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/sandbox")
	path = strings.TrimSuffix(path, "/")

	switch {
	case path == "" && r.Method == "DELETE":
		if err := s.Sandbox.Delete(); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to delete sandbox: "+err.Error())
			return
		}
		jsonResp(w, http.StatusOK, map[string]interface{}{"message": "sandbox deleted"})

	case path == "/delete":
		if r.Method != "DELETE" && r.Method != "POST" {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := s.Sandbox.Delete(); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to delete sandbox: "+err.Error())
			return
		}
		jsonResp(w, http.StatusOK, map[string]interface{}{"message": "sandbox deleted"})

	case path == "" || path == "/status":
		if r.Method != "GET" {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		jsonResp(w, http.StatusOK, s.Sandbox.Status())

	case path == "/start/logs":
		if r.Method != "GET" {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		if err := s.Sandbox.StreamStartLogs(w, r); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to stream sandbox start logs: "+err.Error())
			return
		}

	case path == "/start":
		if r.Method != "POST" {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		status, err := s.Sandbox.Start(sandboxPublicBase(r))
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to start sandbox: "+err.Error())
			return
		}
		jsonResp(w, http.StatusOK, status)

	case path == "/break":
		if r.Method != "POST" {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		status, err := s.Sandbox.Break()
		if err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to break sandbox: "+err.Error())
			return
		}
		jsonResp(w, http.StatusOK, status)

	case path == "/stop":
		if r.Method != "POST" {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := s.Sandbox.Stop(); err != nil {
			apiErr(w, http.StatusInternalServerError, "failed to stop sandbox: "+err.Error())
			return
		}
		jsonResp(w, http.StatusOK, map[string]interface{}{"message": "sandbox stopped"})

	default:
		apiErr(w, http.StatusNotFound, "sandbox endpoint not found")
	}
}

// sandboxPublicBase derives the externally reachable base URL of the
// application from the request. The web server forwards the original Host and
// the TLS scheme (via X-Forwarded-*), which the sandbox uses to publish its
// site at a URL the browser can actually open.
func sandboxPublicBase(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host
}

// handleSandboxSite serves the sandbox WordPress site through the API at
// /sandbox-site/*. The browser reaches the site via the application's own
// host (no extra ports exposed), and the API proxies into the sandbox network
// to reach the sandbox web container. The /sandbox-site prefix is stripped
// because the sandbox web server serves everything from its document root.
func (s *APIServer) handleSandboxSite(w http.ResponseWriter, r *http.Request) {
	if s.Sandbox == nil {
		apiErr(w, http.StatusServiceUnavailable, "sandbox is not configured")
		return
	}
	target, err := s.Sandbox.SiteProxyTarget()
	if err != nil {
		apiErr(w, http.StatusNotFound, "sandbox site is not running")
		return
	}
	targetURL, err := url.Parse(target)
	if err != nil {
		apiErr(w, http.StatusInternalServerError, "invalid sandbox target")
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/sandbox-site")
	if rest == "" {
		rest = "/"
	}

	proxy := &httputil.ReverseProxy{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.URL.Path = rest
			req.URL.RawQuery = r.URL.RawQuery
			if h := r.Header.Get("X-Forwarded-Host"); h != "" {
				req.Host = h
			} else {
				req.Host = r.Host
			}
		},
	}
	proxy.ServeHTTP(w, r)
}
