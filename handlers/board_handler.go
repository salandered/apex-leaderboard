package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/salandered/apex/apextime"
	"github.com/salandered/apex/board"
	"github.com/salandered/apex/storage"
)

// BoardHandler serves the board endpoints.
type BoardHandler struct {
	Storage storage.BoardRepo
}

type PutBoardReq struct {
	BoardName string `json:"board_name"`
}

func (h *BoardHandler) HandlePutBoard(w http.ResponseWriter, req *http.Request) {
	boardId := board.ID(req.PathValue(boardIDPathValue))
	if err := boardId.Validate(); err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	var data PutBoardReq
	err := json.NewDecoder(req.Body).Decode(&data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}
	err = h.Storage.CreateBoard(
		req.Context(),
		&board.Board{
			BoardId:   boardId,
			BoardName: data.BoardName,
			CreatedAt: apextime.ApexNow(),
		},
		newRequestID())
	if err != nil {
		writeStorageError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
