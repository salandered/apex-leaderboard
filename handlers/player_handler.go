package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/player"
	"github.com/salandered/apex/storage"
)

// PlayerHandler serves the player-profile endpoints.
type PlayerHandler struct {
	Storage storage.PlayerRepo
}

type PostPlayerReq struct {
	PlayerName string `json:"player_name"`
}

type PostPlayerResp struct {
	PlayerId string `json:"player_id"`
}

type GetPlayerResp struct {
	PlayerId   player.ID `json:"player_id"`
	PlayerName string    `json:"player_name"`
}

func (h *PlayerHandler) HandlePostPlayer(w http.ResponseWriter, req *http.Request) {
	idempotencyKey, err := readIdempotencyKey(req)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	var data PostPlayerReq
	err = json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	playerId, err := h.Storage.CreatePlayerProfile(
		req.Context(),
		&player.Profile{
			PlayerId:   player.GenerateID(),
			PlayerName: data.PlayerName,
			CreatedAt:  apextime.Now(),
		},
		idempotencyKey)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/players/"+string(playerId))
	writeJSONToResponse(w, http.StatusCreated, PostPlayerResp{PlayerId: string(playerId)})
}

func (h *PlayerHandler) HandleGetPlayer(w http.ResponseWriter, req *http.Request) {
	playerId := player.ID(req.PathValue(playerIDPathValue))
	if err := playerId.Validate(); err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	profile, err := h.Storage.GetPlayerProfile(req.Context(), playerId)

	if err != nil {
		writeStorageError(w, err)
		return
	}

	response := GetPlayerResp{
		PlayerId:   profile.PlayerId,
		PlayerName: profile.PlayerName,
	}

	writeJSONToResponse(w, http.StatusOK, response)
}
