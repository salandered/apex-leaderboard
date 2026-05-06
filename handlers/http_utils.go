package handlers

import (
	"encoding/json"
	"log"
	"net/http"
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
