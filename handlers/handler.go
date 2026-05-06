package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
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
	Id string `json:"id"`
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

	var id = generatePlayerId()

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
	response := PostResponseData{Id: string(id)}

	writeJSONToResponse(w, http.StatusCreated, response)
}

func (h *HTTPHandler) HandleGetScore(w http.ResponseWriter, req *http.Request) {
	id := req.PathValue("id")
	playerData, err := h.Storage.GetData(req.Context(), playerid.PlayerId(id))

	if err != nil {
		// todo: error classification
		writeErrorToResponse(w, fmt.Errorf("not found"), http.StatusNotFound)
		return
	}

	writeJSONToResponse(w, http.StatusOK, playerData)
}

func generatePlayerId() playerid.PlayerId {
	return playerid.PlayerId(uuid.New().String())
}
