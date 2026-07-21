package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/salandered/apex/consumer"
	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/logging"
	"github.com/salandered/apex/server"
	"github.com/salandered/apex/storage"
)

const defaultRedisURL = "redis://localhost:6379/0"

const banner = `
       _________        _________        _________        _________
      /    A    /\     /    P    /\     /    E    /\     /    X    /\
     /_________/  \___/_________/  \___/_________/  \___/_________/  \
     \         \  /   \         \  /   \         \  /   \         \  /
      \_________\/     \_________\/     \_________\/     \_________\/`

func main() {
	fmt.Printf("apex version %v %v \n\n", handlers.GetVersion(), banner)

	logCloser, err := logging.Setup()
	if err != nil {
		// logger isn't ready yet, report to stderr directly
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer logCloser.Close()

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = defaultRedisURL
	}

	store, err := storage.NewStorage(redisURL)
	if err != nil {
		slog.Error("storage init failed", "error", err)
		os.Exit(1)
	}

	// the default board must exist before the server accepts writes
	if err := storage.SeedMainBoard(store); err != nil {
		slog.Error("seeding main board failed", "error", err)
		os.Exit(1)
	}

	activityStore, err := storage.NewActivityStore(redisURL)
	if err != nil {
		slog.Error("activity store init failed", "error", err)
		os.Exit(1)
	}
	dailyActivityConsumer := consumer.NewDailyActivityConsumer(activityStore)
	go func() {
		if err := dailyActivityConsumer.Run(context.Background()); err != nil {
			slog.Error("activity consumer stopped", "error", err)
		}
	}()

	if err := server.Start(server.NewMux(store)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
