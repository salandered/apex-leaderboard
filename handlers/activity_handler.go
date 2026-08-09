package handlers

import (
	"fmt"
	"net/http"

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
	Date     string          `json:"date"`
	Entries  []activityEntry `json:"entries"`
	Metadata limitMeta       `json:"metadata"`
}

func (h *ViewHandler) HandleListDailyActivity(w http.ResponseWriter, req *http.Request) {
	date := req.URL.Query().Get(dateQuery)
	if date == "" {
		writeRequestError(req.Context(), w, fmt.Errorf("%s is required", dateQuery))
		return
	}
	if _, err := parseDateQuery(req, dateQuery); err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	limit, err := parseIntQuery(req, limitQuery, defaultActivityLimit, 1, maxActivityLimit)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	entries, err := h.Storage.ListDailyActivity(req.Context(), date, limit)
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	response := ListDailyActivityResp{
		Date:     date,
		Entries:  make([]activityEntry, 0, len(entries)),
		Metadata: limitMeta{Limit: limit},
	}
	for _, e := range entries {
		response.Entries = append(response.Entries, activityEntry{PlayerId: e.PlayerID, Count: e.Count})
	}
	writeJSONToResponse(req.Context(), w, http.StatusOK, response)
}
