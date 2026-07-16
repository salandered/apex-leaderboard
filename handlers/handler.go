package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/salandered/apex/player"
	"github.com/salandered/apex/storage"
)

const (
	playerIDPathValue string = "player_id"
	version           string = "0.1.0"
)

type HTTPHandler struct {
	Storage storage.Storage
	// storage map[string]PlayerData with mu
}

type PostRequestData struct {
	PlayerName  string  `json:"player_name"`
	PlayerScore float64 `json:"player_score"`
}

type IncrementScoreRequest struct {
	Amount float64 `json:"amount"`
}

type IncrementScoreResponse struct {
	Score float64 `json:"score"`
}

type PostResponseData struct {
	PlayerId string `json:"player_id"`
}

type GetResponseData struct {
	PlayerId    player.ID `json:"player_id"`
	PlayerName  string    `json:"player_name"`
	PlayerScore float64   `json:"player_score"`
}

func (h *HTTPHandler) HandleRoot(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(w, "apex %s — see /api/v1/scores\n", version)
	slog.Debug("root handled")
}

func (h *HTTPHandler) HandlePostPlayer(w http.ResponseWriter, req *http.Request) {
	var data PostRequestData
	err := json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	var id = player.GenerateID()

	err = h.Storage.CreatePlayer(
		req.Context(),
		&player.Profile{
			PlayerId:   id,
			PlayerName: data.PlayerName,
			// TODO: date
		},
		data.PlayerScore)

	if err != nil {
		writeStorageError(w, err)
		return
	}
	response := PostResponseData{PlayerId: string(id)}

	writeJSONToResponse(w, http.StatusCreated, response)
}

func (h *HTTPHandler) HandleIncrementScore(w http.ResponseWriter, req *http.Request) {
	playerId := player.ID(req.PathValue(playerIDPathValue))
	if err := playerId.Validate(); err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	var data IncrementScoreRequest
	err := json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	score, err := h.Storage.IncrementScore(req.Context(), playerId, data.Amount)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	response := IncrementScoreResponse{Score: score}
	writeJSONToResponse(w, http.StatusOK, response)
}

func (h *HTTPHandler) HandleGetScore(w http.ResponseWriter, req *http.Request) {
	playerId := player.ID(req.PathValue(playerIDPathValue))
	if err := playerId.Validate(); err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	profile, score, err := h.Storage.GetPlayer(req.Context(), playerId)

	if err != nil {
		writeStorageError(w, err)
		return
	}

	response := GetResponseData{
		PlayerId:    profile.PlayerId,
		PlayerName:  profile.PlayerName,
		PlayerScore: score,
	}

	writeJSONToResponse(w, http.StatusOK, response)
}
