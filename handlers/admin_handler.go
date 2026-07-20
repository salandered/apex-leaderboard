package handlers

import (
	"net/http"

	"github.com/salandered/apex/storage"
)

type AdminHandler struct {
	Storage storage.ProjectionAdmin
}

type ProjectionMismatchResp struct {
	BoardId       string  `json:"board_id"`
	PlayerId      string  `json:"player_id"`
	LiveScore     float64 `json:"live_score"`
	LivePresent   bool    `json:"live_present"`
	ReplayScore   float64 `json:"replay_score"`
	ReplayPresent bool    `json:"replay_present"`
}

type VerifyProjectionResp struct {
	Mismatches []ProjectionMismatchResp `json:"mismatches"`
}

func (h *AdminHandler) HandleRebuildProjection(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	err = h.Storage.RebuildProjection(req.Context(), boardId)
	if err != nil {
		writeStorageError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) HandleVerifyProjection(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	mismatches, err := h.Storage.VerifyProjection(req.Context(), boardId)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	response := VerifyProjectionResp{
		Mismatches: make([]ProjectionMismatchResp, 0, len(mismatches)),
	}
	for _, mismatch := range mismatches {
		response.Mismatches = append(response.Mismatches, ProjectionMismatchResp{
			BoardId:       mismatch.BoardID,
			PlayerId:      mismatch.PlayerID,
			LiveScore:     mismatch.LiveScore,
			LivePresent:   mismatch.LivePresent,
			ReplayScore:   mismatch.ReplayScore,
			ReplayPresent: mismatch.ReplayPresent,
		})
	}
	writeJSONToResponse(w, http.StatusOK, response)
}
