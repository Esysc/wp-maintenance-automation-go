package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchHostAgent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Host{Hostname: "agent-host", Available: true})
	}))
	defer ts.Close()

	h, err := fetchHostAgent(ts.URL)
	if err != nil {
		t.Fatalf("fetchHostAgent: %v", err)
	}
	if h.Hostname != "agent-host" || !h.Available {
		t.Errorf("unexpected host: %+v", h)
	}
}

func TestFetchHostAgentErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	if _, err := fetchHostAgent(ts.URL); err == nil {
		t.Error("expected error for non-200 response")
	}

	if _, err := fetchHostAgent("http://127.0.0.1:1"); err == nil {
		t.Error("expected error for unreachable agent")
	}
}

func TestHostMetricsUsesAgent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Host{Hostname: "agent-host", Available: true})
	}))
	defer ts.Close()

	t.Setenv("HOST_AGENT_URL", ts.URL)
	h := HostMetrics()
	if h.Hostname != "agent-host" {
		t.Errorf("expected agent hostname, got %q", h.Hostname)
	}
}
