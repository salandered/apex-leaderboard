package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

const readinessTimeout = time.Second

type healthChecker interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	Storage healthChecker
}

type HealthResp struct {
	Status     string `json:"status"`
	Dependency string `json:"dependency,omitempty"`
}

func (h *HealthHandler) HandleLive(w http.ResponseWriter, req *http.Request) {
	writeJSONToResponse(req.Context(), w, http.StatusOK, HealthResp{Status: "ok"})
}

func (h *HealthHandler) HandleReady(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), readinessTimeout)
	defer cancel()

	if err := h.Storage.Ping(ctx); err != nil {
		slog.WarnContext(ctx, "readiness check failed", "dependency", "redis", "error", err)
		writeJSONToResponse(ctx, w, http.StatusServiceUnavailable, HealthResp{
			Status:     "unavailable",
			Dependency: "redis",
		})
		return
	}
	writeJSONToResponse(ctx, w, http.StatusOK, HealthResp{Status: "ok"})
}
