package handlers

import (
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

type PlayerResp struct {
	PlayerId   player.ID `json:"player_id"`
	PlayerName string    `json:"player_name"`
}

type PostPlayerResp struct {
	Player PlayerResp `json:"player"`
}

type GetPlayerResp struct {
	Player PlayerResp `json:"player"`
}

func (h *PlayerHandler) HandlePostPlayer(w http.ResponseWriter, req *http.Request) {
	idempotencyKey, err := readIdempotencyKey(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}
	var data PostPlayerReq
	if err := readJSON(w, req, &data); err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	playerName, err := player.NormalizeName(data.PlayerName)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	playerId, err := h.Storage.CreatePlayerProfile(
		req.Context(),
		&player.Profile{
			PlayerId:   player.GenerateID(),
			PlayerName: playerName,
			CreatedAt:  apextime.Now(),
		},
		idempotencyKey)
	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/players/"+string(playerId))
	writeJSONToResponse(req.Context(), w, http.StatusCreated, PostPlayerResp{
		Player: PlayerResp{PlayerId: playerId, PlayerName: playerName},
	})
}

func (h *PlayerHandler) HandleGetPlayer(w http.ResponseWriter, req *http.Request) {
	playerId, err := playerIdFromPath(req)
	if err != nil {
		writeRequestError(req.Context(), w, err)
		return
	}

	profile, err := h.Storage.GetPlayerProfile(req.Context(), playerId)

	if err != nil {
		writeStorageError(req.Context(), w, err)
		return
	}

	response := GetPlayerResp{Player: PlayerResp{
		PlayerId:   profile.PlayerId,
		PlayerName: profile.PlayerName,
	}}

	writeJSONToResponse(req.Context(), w, http.StatusOK, response)
}
