package handlers

import (
	"context"
	"net/http"
	"time"
)

const readinessTimeout = time.Second

type healthChecker interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	Storage healthChecker
	// Seeded reports whether one-time app init (the main board) is done.
	// nil means no seeding gate: readiness depends on the datastore only.
	Seeded func() bool
}

type HealthResp struct {
	Status     string `json:"status"`
	Dependency string `json:"dependency,omitempty"`
}

func (h *HealthHandler) HandleLive(w http.ResponseWriter, req *http.Request) {
	writeJSONToResponse(w, http.StatusOK, HealthResp{Status: "ok"})
}

func (h *HealthHandler) HandleReady(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), readinessTimeout)
	defer cancel()

	if err := h.Storage.Ping(ctx); err != nil {
		writeJSONToResponse(w, http.StatusServiceUnavailable, HealthResp{
			Status:     "unavailable",
			Dependency: "redis",
		})
		return
	}
	if h.Seeded != nil && !h.Seeded() {
		writeJSONToResponse(w, http.StatusServiceUnavailable, HealthResp{
			Status:     "unavailable",
			Dependency: "seed",
		})
		return
	}
	writeJSONToResponse(w, http.StatusOK, HealthResp{Status: "ok"})
}
