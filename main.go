package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/salandered/apex/consumer"
	"github.com/salandered/apex/handlers"
	"github.com/salandered/apex/logging"
	"github.com/salandered/apex/server"
	"github.com/salandered/apex/storage"
)

const defaultRedisURL = "redis://localhost:6379/0"

// covers the consumer's 5s blocking XREAD plus margin.
const backgroundStopTimeout = 7 * time.Second

const banner = `
       _________        _________        _________        _________
      /    A    /\     /    P    /\     /    E    /\     /    X    /\
     /_________/  \___/_________/  \___/_________/  \___/_________/  \
     \         \  /   \         \  /   \         \  /   \         \  /
      \_________\/     \_________\/     \_________\/     \_________\/`

func main() {
	fmt.Printf("apex version %v %v \n\n", handlers.GetVersion(), banner)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	activityStore, err := storage.NewActivityStore(redisURL)
	if err != nil {
		slog.Error("activity store init failed", "error", err)
		os.Exit(1)
	}
	dailyActivityConsumer := consumer.NewDailyActivityConsumer(activityStore)
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		if err := dailyActivityConsumer.Run(ctx); err != nil {
			slog.Error("activity consumer stopped", "error", err)
		}
	}()

	startErr := server.Start(ctx, server.NewMux(store))
	if ctx.Err() == nil {
		// returned before any signal -> the server never ran (e.g. failed to bind).
		slog.Error("server failed", "error", startErr)
		os.Exit(1)
	}
	if startErr != nil {
		// signal-triggered shutdown that didn't finish cleanly (e.g. deadline).
		slog.Error("graceful shutdown incomplete", "error", startErr)
	}

	waitFor(consumerDone, "activity consumer", backgroundStopTimeout)
	if c, ok := activityStore.(io.Closer); ok {
		if err := c.Close(); err != nil {
			slog.Error("activity store close failed", "error", err)
		}
	}
}

func waitFor(done <-chan struct{}, name string, timeout time.Duration) {
	select {
	case <-done:
	case <-time.After(timeout):
		slog.Warn("background worker did not stop in time", "worker", name)
	}
}
