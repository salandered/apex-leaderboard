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

type BoardResp struct {
	BoardId   string `json:"board_id"`
	BoardName string `json:"board_name"`
	CreatedAt string `json:"created_at"`
}

type ListBoardsResp struct {
	Boards []BoardResp `json:"boards"`
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
			CreatedAt: apextime.Now(),
		},
		newRequestID())
	if err != nil {
		writeStorageError(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *BoardHandler) HandleGetBoard(w http.ResponseWriter, req *http.Request) {
	boardId := board.ID(req.PathValue(boardIDPathValue))
	if err := boardId.Validate(); err != nil {
		writeErrorToResponse(w, err, http.StatusBadRequest)
		return
	}

	b, err := h.Storage.GetBoard(req.Context(), boardId)
	if err != nil {
		writeStorageError(w, err)
		return
	}

	writeJSONToResponse(w, http.StatusOK, boardToResp(b))
}

func (h *BoardHandler) HandleListBoards(w http.ResponseWriter, req *http.Request) {
	boards, err := h.Storage.ListBoards(req.Context())
	if err != nil {
		writeStorageError(w, err)
		return
	}

	response := ListBoardsResp{Boards: make([]BoardResp, 0, len(boards))}
	for i := range boards {
		response.Boards = append(response.Boards, boardToResp(&boards[i]))
	}

	writeJSONToResponse(w, http.StatusOK, response)
}

func boardToResp(b *board.Board) BoardResp {
	return BoardResp{
		BoardId:   string(b.BoardId),
		BoardName: b.BoardName,
		CreatedAt: apextime.Format(b.CreatedAt),
	}
}
