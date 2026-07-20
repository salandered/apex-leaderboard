package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/salandered/apex/player"
	"github.com/salandered/apex/storage"
)

type ScoreHandler struct {
	Storage storage.ScoreRepo
}

type PutScoreReq struct {
	PlayerScore float64 `json:"player_score"`
}

type IncrementScoreReq struct {
	Amount float64 `json:"amount"`
}

// one ledger entry in the API response
type HistoryEvent struct {
	Id        string  `json:"id"`
	Type      string  `json:"type"`
	Amount    float64 `json:"amount"`
	RequestId string  `json:"request_id"`
	CreatedAt string  `json:"created_at"`
}

type HistoryResp struct {
	PlayerId player.ID      `json:"player_id"`
	Events   []HistoryEvent `json:"events"`
}

// part of the ListScoresResp
type scoreEntry struct {
	PlayerId string  `json:"player_id"`
	Score    float64 `json:"score"`
	Rank     int64   `json:"rank"`
}

type ListScoresResp struct {
	Scores []scoreEntry `json:"scores"`
	Limit  int64        `json:"limit"`
	Offset int64        `json:"offset"`
	Total  int64        `json:"total"`
}

type RankResp struct {
	PlayerId player.ID `json:"player_id"`
	Rank     int64     `json:"rank"`
	Score    float64   `json:"score"`
	Total    int64     `json:"total"`
}

func (h *ScoreHandler) HandlePutScore(w http.ResponseWriter, req *http.Request) {
	playerId, boardId, err := parsePlayerBoardPathValues(w, req)
	// TODO: get rid of parsePlayerBoardPathValues and consider validating boardId here
	if err != nil {
		return
	}
	idempotencyKey, err := readIdempotencyKey(req)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	var data PutScoreReq
	err = json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	err = h.Storage.SetScore(
		req.Context(),
		playerId,
		boardId,
		data.PlayerScore,
		newRequestID(),
		idempotencyKey,
	)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ScoreHandler) HandleIncrementScore(w http.ResponseWriter, req *http.Request) {
	playerId, boardId, err := parsePlayerBoardPathValues(w, req)
	if err != nil {
		return
	}
	idempotencyKey, err := readIdempotencyKey(req)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	var data IncrementScoreReq
	err = json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	err = h.Storage.IncrementScore(
		req.Context(),
		playerId,
		boardId,
		data.Amount,
		newRequestID(),
		idempotencyKey,
	)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ScoreHandler) HandleGetRank(w http.ResponseWriter, req *http.Request) {
	playerId, boardId, err := parsePlayerBoardPathValues(w, req)
	if err != nil {
		return
	}

	standing, total, err := h.Storage.GetStanding(req.Context(), playerId, boardId)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	writeJSONToResponse(w, http.StatusOK, RankResp{
		PlayerId: playerId,
		Rank:     standing.Rank,
		Score:    standing.Score,
		Total:    total,
	})
}

func (h *ScoreHandler) HandleListScores(w http.ResponseWriter, req *http.Request) {
	limit, err := parseIntQuery(req, limitQuery, defaultListLimit, 1, maxListLimit)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	offset, err := parseIntQuery(req, offsetQuery, 0, 0, 0)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	scores, total, err := h.Storage.ListStandings(req.Context(), boardId, limit, offset)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	response := ListScoresResp{
		Scores: make([]scoreEntry, 0, len(scores)),
		Limit:  limit,
		Offset: offset,
		Total:  total,
	}
	for _, sc := range scores {
		response.Scores = append(response.Scores, scoreEntry{
			PlayerId: sc.PlayerID,
			Score:    sc.Score,
			Rank:     sc.Rank,
		})
	}

	writeJSONToResponse(w, http.StatusOK, response)
}

func (h *ScoreHandler) HandleGetHistory(w http.ResponseWriter, req *http.Request) {
	playerId, boardId, err := parsePlayerBoardPathValues(w, req)
	if err != nil {
		return
	}

	limit, err := parseIntQuery(req, limitQuery, defaultHistoryLimit, 1, 0)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	events, err := h.Storage.PlayerHistory(req.Context(), playerId, boardId, limit)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	// Note: an unknown player yields an empty list, not a 404
	response := HistoryResp{
		PlayerId: playerId,
		Events:   make([]HistoryEvent, 0, len(events)),
	}
	for _, e := range events {
		response.Events = append(response.Events, HistoryEvent{
			Id:        e.ID,
			Type:      string(e.Type),
			Amount:    e.Amount,
			RequestId: e.RequestID,
			CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	writeJSONToResponse(w, http.StatusOK, response)
}
