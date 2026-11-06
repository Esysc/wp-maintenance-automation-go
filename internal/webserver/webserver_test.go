package webserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAPISnapshotsPreservesIDAndMethod(t *testing.T) {
	var gotMethod, gotPath string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer api.Close()

	s := &WebServer{
		apiBaseURL: api.URL,
		client:     api.Client(),
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/snapshots/abc123", nil)
	rr := httptest.NewRecorder()
	s.handleAPISnapshots(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handleAPISnapshots() status = %d, want %d", rr.Code, http.StatusOK)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("proxied method = %q, want %q", gotMethod, http.MethodDelete)
	}
	if gotPath != "/api/v1/snapshots/abc123" {
		t.Fatalf("proxied path = %q, want %q", gotPath, "/api/v1/snapshots/abc123")
	}
}

func TestHandleAPISnapshotsMapsCollectionAndLegacyPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "v1 collection", path: "/api/v1/snapshots"},
		{name: "legacy collection", path: "/api/snapshots"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"success":true}`))
			}))
			defer api.Close()

			s := &WebServer{
				apiBaseURL: api.URL,
				client:     api.Client(),
			}

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()
			s.handleAPISnapshots(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("handleAPISnapshots() status = %d, want %d", rr.Code, http.StatusOK)
			}
			if gotPath != "/api/v1/snapshots" {
				t.Fatalf("proxied path = %q, want %q", gotPath, "/api/v1/snapshots")
			}
		})
	}
}
