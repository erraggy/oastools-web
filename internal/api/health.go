package api

import (
	"encoding/json"
	"net/http"
)

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	OASTools string `json:"oastools"`
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := HealthResponse{
		Status:   "healthy",
		Version:  h.version,
		OASTools: "1.33.0", // TODO: get from oastools package if exposed
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
