package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/storage"
)

const defaultRedisURL = "redis://localhost:6379/0"

func getMux(s storage.Storage) *http.ServeMux {
	handler := &handlers.HTTPHandler{
		Storage: s,
	}

	mux := http.NewServeMux()

	// using snaked 'player_id' naming to match OpenAPI docs
	mux.HandleFunc("GET /{$}", handler.HandleRoot)
	mux.HandleFunc("GET /api/v1/scores/{player_id}", handler.HandleGetScore)
	mux.HandleFunc("POST /api/v1/scores/{player_id}/increment", handler.HandleIncrementScore)
	mux.HandleFunc("POST /api/v1/scores", handler.HandlePostPlayer)

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

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = defaultRedisURL
	}

	store, err := storage.NewStorage(redisURL)
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}

	mux := getMux(store)
	startServer(mux)
}
