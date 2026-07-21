package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/ledger"
	"github.com/salandered/apex/storage"
)

const (
	afterQuery = "after"

	defaultEventLimit int64 = 50
	maxEventLimit     int64 = 100
)

type EventHandler struct {
	Storage storage.EventRepo
}

type ScoreEvent struct {
	EventId   string  `json:"event_id"`
	Type      string  `json:"type"`
	PlayerId  string  `json:"player_id"`
	BoardId   string  `json:"board_id"`
	Amount    float64 `json:"amount"`
	RequestId string  `json:"request_id"`
	CreatedAt string  `json:"created_at"`
}

type ListEventsResp struct {
	Events    []ScoreEvent `json:"events"`
	NextAfter string       `json:"next_after"`
}

func (h *EventHandler) HandleListEvents(w http.ResponseWriter, req *http.Request) {
	after := req.URL.Query().Get(afterQuery)
	if after == "" {
		writeErrorToResponse(w, fmt.Errorf("%s is required", afterQuery), http.StatusBadRequest)
		return
	}
	if err := validateEventID(after); err != nil {
		writeErrorToResponse(w, fmt.Errorf("invalid %s: %v", afterQuery, err), http.StatusBadRequest)
		return
	}

	limit, err := parseIntQuery(req, limitQuery, defaultEventLimit, 1, maxEventLimit)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	events, err := h.Storage.ListEventsAfter(req.Context(), after, limit)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	response := ListEventsResp{
		Events:    make([]ScoreEvent, 0, len(events)),
		NextAfter: after,
	}
	for _, event := range events {
		response.Events = append(response.Events, scoreEventFromLedger(event))
	}
	if len(events) > 0 {
		response.NextAfter = events[len(events)-1].ID
	}

	writeJSONToResponse(w, http.StatusOK, response)
}

func scoreEventFromLedger(event ledger.Event) ScoreEvent {
	return ScoreEvent{
		EventId:   event.ID,
		Type:      string(event.Type),
		PlayerId:  event.PlayerID,
		BoardId:   event.BoardID,
		Amount:    event.Amount,
		RequestId: event.RequestID,
		CreatedAt: apextime.Format(event.CreatedAt),
	}
}

func validateEventID(id string) error {
	milliseconds, sequence, ok := strings.Cut(id, "-")
	if !ok || milliseconds == "" || sequence == "" {
		return fmt.Errorf("want <milliseconds>-<sequence>")
	}
	if _, err := strconv.ParseUint(milliseconds, 10, 64); err != nil {
		return fmt.Errorf("invalid milliseconds")
	}
	if _, err := strconv.ParseUint(sequence, 10, 64); err != nil {
		return fmt.Errorf("invalid sequence")
	}
	return nil
}
