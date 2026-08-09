package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/player"
	"github.com/salandered/apex/score"
	"github.com/salandered/apex/storage"
)

const asOfQuery = "as_of"

type ScoreHandler struct {
	Storage storage.ScoreRepo
}

// Using pointer: without it '{}' would write a 0
type PutScoreReq struct {
	PlayerScore *int64 `json:"player_score"`
}

type IncrementScoreReq struct {
	Amount *int64 `json:"amount"`
}

type HistoryResp struct {
	PlayerId player.ID    `json:"player_id"`
	Events   []ScoreEvent `json:"events"`
	Metadata limitMeta    `json:"metadata"`
}

// part of the ListScoresResp
type scoreEntry struct {
	PlayerId string `json:"player_id"`
	Score    int64  `json:"score"`
	Rank     int64  `json:"rank"`
}

type ListScoresResp struct {
	Scores   []scoreEntry `json:"scores"`
	Metadata offsetMeta   `json:"metadata"`
}

type RankResp struct {
	Standing scoreEntry `json:"standing"`
	Metadata totalMeta  `json:"metadata"`
}

func (h *ScoreHandler) HandlePutScore(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	playerId, err := playerIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	idempotencyKey, err := readIdempotencyKey(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	var data PutScoreReq
	if err := readJSON(w, req, &data); err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	if data.PlayerScore == nil {
		writeRequestError(req.Context(), w, fmt.Errorf("player_score is required"))
		return
	}
	if err := score.Validate(*data.PlayerScore); err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	err = h.Storage.SetScore(
		req.Context(),
		playerId,
		boardId,
		*data.PlayerScore,
		requestID(req),
		idempotencyKey,
	)
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ScoreHandler) HandleIncrementScore(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	playerId, err := playerIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	idempotencyKey, err := readIdempotencyKey(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	var data IncrementScoreReq
	if err := readJSON(w, req, &data); err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	if data.Amount == nil {
		writeRequestError(req.Context(), w, fmt.Errorf("amount is required"))
		return
	}
	// bounds the delta only: the resulting score is bounded atomically in the write script
	if err := score.Validate(*data.Amount); err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	err = h.Storage.IncrementScore(
		req.Context(),
		playerId,
		boardId,
		*data.Amount,
		requestID(req),
		idempotencyKey,
	)
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ScoreHandler) HandleGetRank(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	playerId, err := playerIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	standing, total, err := h.Storage.GetStanding(req.Context(), playerId, boardId)
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	writeJSONToResponse(req.Context(), w, http.StatusOK, RankResp{
		Standing: scoreEntry{
			PlayerId: string(playerId),
			Score:    standing.Score,
			Rank:     standing.Rank,
		},
		Metadata: totalMeta{Total: total},
	})
}

func (h *ScoreHandler) HandleListScores(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	limit, err := parseIntQuery(req, limitQuery, defaultListLimit, 1, maxListLimit)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	offset, err := parseIntQuery(req, offsetQuery, 0, 0, 0)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	var scores []storage.Standing
	var total int64
	if !req.URL.Query().Has(asOfQuery) {
		scores, total, err = h.Storage.ListStandings(req.Context(), boardId, limit, offset)
	} else {
		date, parseErr := parseDateQuery(req, asOfQuery)
		if parseErr != nil {
			writeRequestError(req.Context(), w, parseErr)
			return
		}
		now := apextime.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		if date.After(today) {
			writeRequestError(req.Context(), w, fmt.Errorf("%s must not be in the future", asOfQuery))
			return
		}
		scores, total, err = h.Storage.ListStandingsAsOf(
			req.Context(), boardId, date.AddDate(0, 0, 1), limit, offset,
		)
	}
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	response := ListScoresResp{
		Scores:   make([]scoreEntry, 0, len(scores)),
		Metadata: offsetMeta{Limit: limit, Offset: offset, Total: total},
	}
	for _, sc := range scores {
		response.Scores = append(response.Scores, scoreEntry{
			PlayerId: sc.PlayerID,
			Score:    sc.Score,
			Rank:     sc.Rank,
		})
	}

	writeJSONToResponse(req.Context(), w, http.StatusOK, response)
}

func (h *ScoreHandler) HandleGetHistory(w http.ResponseWriter, req *http.Request) {
	boardId, err := boardIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	playerId, err := playerIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	limit, err := parseIntQuery(req, limitQuery, defaultHistoryLimit, 1, maxHistoryLimit)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	events, err := h.Storage.PlayerHistory(req.Context(), playerId, boardId, limit)
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	// Note: an unknown player yields an empty list, not a 404
	response := HistoryResp{
		PlayerId: playerId,
		Events:   make([]ScoreEvent, 0, len(events)),
		Metadata: limitMeta{Limit: limit},
	}
	for _, e := range events {
		response.Events = append(response.Events, scoreEventFromLedger(e))
	}

	writeJSONToResponse(req.Context(), w, http.StatusOK, response)
}
