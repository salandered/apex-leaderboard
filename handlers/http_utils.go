package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/salandered/apex/storage"
)

func writeJSONToResponse(w http.ResponseWriter, statusCode int, data any) {
	createHeaders(w)

	rawJSON, err := json.Marshal(data)
	if err != nil {
		writeErrorToResponse(w, err, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(statusCode) // before Write

	_, err = w.Write(rawJSON)
	if err != nil {
		// headers with the status code were already sent to the client
		slog.Error("failed writing response body", "status", statusCode, "error", err)
		return
	}
	slog.Debug("response sent", "payload", string(rawJSON))
}

func createHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
}

func writeErrorToResponse(w http.ResponseWriter, err error, statusCode int) {
	http.Error(w, err.Error(), statusCode)
}

// maps a storage-layer error to an HTTP response
func writeStorageError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		writeErrorToResponse(w, fmt.Errorf("not found"), http.StatusNotFound)
		return
	}
	slog.Error("internal storage error", "error", err)
	writeErrorToResponse(w, fmt.Errorf("internal server error"), http.StatusInternalServerError)
}
