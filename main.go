package main

import (
	"log"
	"net/http"
	"time"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/storage"
)

func getMux(s storage.Storage) *http.ServeMux {
	handler := &handlers.HTTPHandler{
		Storage: s,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", handler.HandleRoot)
	mux.HandleFunc("GET /api/scores/{id}", handler.HandleGetScore)
	mux.HandleFunc("POST /api/scores", handler.HandlePostScore)

	return mux
}

func startServer(mux *http.ServeMux) {
	s := &http.Server{
		Addr:           ":8090",
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 mb
	}
	log.Fatal(s.ListenAndServe())
}

func main() {
	// TODO: get range of users (pagination)
	// TODO: api/v1/ prefix

	mux := getMux(storage.NewStorage())
	startServer(mux)

}
