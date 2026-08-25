package handlers

import (
	"net/http"

	"github.com/salandered/apex/player"
	"github.com/salandered/apex/storage"
	"github.com/salandered/httputils/httputils"
)

// PlayerHandler serves the player-profile endpoints.
type PlayerHandler struct {
	Storage storage.PlayerRepo
}

type CreatePlayerReq struct {
	PlayerName string `json:"player_name"`
}

type PlayerResp struct {
	PlayerId   player.ID `json:"player_id"`
	PlayerName string    `json:"player_name"`
}

type CreatePlayerResp struct {
	Player PlayerResp `json:"player"`
}

type GetPlayerResp struct {
	Player PlayerResp `json:"player"`
}

func (h *PlayerHandler) HandleCreatePlayer(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	idempotencyKey, err := readIdempotencyKey(req)
	if err != nil {
		writeRequestError(ctx, w, err)
		return
	}
	var data CreatePlayerReq
	if err := httputils.ReadJSON(w, req, &data, maxRequestBodyBytes); err != nil {
		writeRequestError(ctx, w, err)
		return
	}

	profile, err := player.NewProfile(data.PlayerName)
	if err != nil {
		writeRequestError(ctx, w, err)
		return
	}

	playerId, err := h.Storage.CreatePlayerProfile(ctx, profile, idempotencyKey)
	if err != nil {
		writeStorageError(ctx, w, err)
		return
	}

	w.Header().Set("Location", "/api/v1/players/"+string(playerId))
	httputils.WriteJSON(ctx, w, http.StatusCreated, CreatePlayerResp{
		Player: PlayerResp{PlayerId: playerId, PlayerName: profile.PlayerName},
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

	httputils.WriteJSON(req.Context(), w, http.StatusOK, response)
}
