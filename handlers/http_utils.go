package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
		log.Printf(
			"error while writing JSON data to the connection. Note that headers with status code %v was already sent to client. Error: %v\n",
			statusCode,
			err)
		return
	}
	log.Printf("response sent - status: %d, payload: %s\n", statusCode, string(rawJSON))
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
	log.Printf("internal storage error: %v", err)
	writeErrorToResponse(w, fmt.Errorf("internal server error"), http.StatusInternalServerError)
}
