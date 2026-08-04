package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/salandered/apex/storage"
)

type ViewHandler struct {
	Storage storage.ActivityRepo
}

const (
	dateQuery string = "date"

	defaultActivityLimit int64 = 10  // most-active-players widget size
	maxActivityLimit     int64 = 100 // cap on a single request
)

type activityEntry struct {
	PlayerId string `json:"player_id"`
	Count    int64  `json:"count"`
}

type ListDailyActivityResp struct {
	Date    string          `json:"date"`
	Entries []activityEntry `json:"entries"`
}

func (h *ViewHandler) HandleListDailyActivity(w http.ResponseWriter, req *http.Request) {
	date := req.URL.Query().Get(dateQuery)
	if date == "" {
		writeErrorToResponse(req.Context(), w, fmt.Errorf("%s is required", dateQuery), http.StatusBadRequest)
		return
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		writeErrorToResponse(req.Context(), w, fmt.Errorf("invalid %s, want YYYY-MM-DD: %w", dateQuery, err), http.StatusBadRequest)
		return
	}

	limit, err := parseIntQuery(req, limitQuery, defaultActivityLimit, 1, maxActivityLimit)
	if err != nil {
		writeErrorToResponse(req.Context(), w, err, http.StatusBadRequest)
		return
	}

	entries, err := h.Storage.ListDailyActivity(req.Context(), date, limit)
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	response := ListDailyActivityResp{
		Date:    date,
		Entries: make([]activityEntry, 0, len(entries)),
	}
	for _, e := range entries {
		response.Entries = append(response.Entries, activityEntry{PlayerId: e.PlayerID, Count: e.Count})
	}
	writeJSONToResponse(req.Context(), w, http.StatusOK, response)
}
