package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/salandered/apex/models"
	playerid "github.com/salandered/apex/player_id"
	"github.com/salandered/apex/storage"
)

type HTTPHandler struct {
	Storage storage.Storage
	// storage map[string]PlayerData
}

type PostRequestData struct {
	PlayerName  string  `json:"player_name"`
	PlayerScore float64 `json:"player_score"`
}

type PostResponseData struct {
	PlayerId string `json:"player_id"`
}

type GetResponseData struct {
	PlayerId    playerid.PlayerId `json:"player_id"`
	PlayerName  string            `json:"player_name"`
	PlayerScore float64           `json:"player_score"`
}

func (h *HTTPHandler) HandleRoot(w http.ResponseWriter, req *http.Request) {
	fmt.Fprintln(w, "Root handled")
	fmt.Println("Root handled")
}

func (h *HTTPHandler) HandlePostScore(w http.ResponseWriter, req *http.Request) {
	var data PostRequestData
	err := json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	var id = playerid.GeneratePlayerId()

	err = h.Storage.PutData(
		req.Context(),
		&models.PlayerData{
			PlayerId:    id,
			PlayerName:  data.PlayerName,
			PlayerScore: data.PlayerScore,
		})

	if err != nil {
		writeErrorToResponse(w, err, http.StatusInternalServerError) // todo: error code
		return

	}
	response := PostResponseData{PlayerId: string(id)}

	writeJSONToResponse(w, http.StatusCreated, response)
}

func (h *HTTPHandler) HandleGetScore(w http.ResponseWriter, req *http.Request) {
	id := playerid.PlayerId(req.PathValue("id"))
	if err := id.Validate(); err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	playerData, err := h.Storage.GetData(req.Context(), id)

	if err != nil {
		// todo: error classification
		writeErrorToResponse(w, fmt.Errorf("not found"), http.StatusNotFound)
		return
	}

	response := GetResponseData{
		PlayerId:    playerData.PlayerId,
		PlayerName:  playerData.PlayerName,
		PlayerScore: playerData.PlayerScore,
	}

	writeJSONToResponse(w, http.StatusOK, response)
}
