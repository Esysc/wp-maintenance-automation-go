package apiserver

import (
	"log"
	"net/http"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/metrics"
)

func (s *APIServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	containers, composeManaged, err := metrics.Containers()
	containersAvailable := err == nil
	if err != nil {
		log.Printf("metrics: docker containers unavailable: %v", err)
		containers = []metrics.Container{}
	}

	jsonResp(w, http.StatusOK, map[string]interface{}{
		"host":                 metrics.HostMetrics(),
		"containers":           containers,
		"containers_available": containersAvailable,
		"compose_managed":      composeManaged,
	})
}
