package apiserver

import (
	"net/http"
	"strings"
)

// handleSandbox routes sandbox requests. The sandbox is a local,
// production-like WordPress test site used to practice backup and restore
// drills: you can start it, take a backup, intentionally break it, and
// restore it.
//
//	GET  /api/v1/sandbox/status
//	POST /api/v1/sandbox/start
//	POST /api/v1/sandbox/break
//	POST /api/v1/sandbox/stop
func (s *APIServer) handleSandbox(w http.ResponseWriter, r *http.Request) {
	if s.Sandbox == nil {
		apiErr(w, http.StatusServiceUnavailable, "sandbox is not configured")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/sandbox")
	path = strings.TrimSuffix(path, "/")

	switch {
	case path == "" || path == "/status":
		if r.Method != "GET" {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		jsonResp(w, http.StatusOK, s.Sandbox.Status())

	case path == "/start":
		if r.Method != "POST" {
			apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		status, err := s.Sandbox.Start()
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
