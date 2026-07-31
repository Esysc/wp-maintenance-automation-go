package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/metrics"
)

// host-agent is a tiny daemon that exposes the machine's resource metrics
// over HTTP. It runs on the physical host (Linux or macOS) so the app can
// report the real host's metrics even when Docker runs inside a VM (e.g.
// Colima, Docker Desktop). The API server uses it when HOST_AGENT_URL is set.
func main() {
	addr := flag.String("addr", "127.0.0.1:9100", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metrics.CollectHost())
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	log.Printf("host-agent listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
