package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/lemonishi/supportsentinel/internal/alert"
	"github.com/lemonishi/supportsentinel/internal/classify"
	"github.com/lemonishi/supportsentinel/internal/config"
	"github.com/lemonishi/supportsentinel/internal/httpapi"
	"github.com/lemonishi/supportsentinel/internal/orchestrator"
	"github.com/lemonishi/supportsentinel/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	s, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	// Plan 1 uses the fake classifier; Plan 2 swaps in the Qwen client.
	o := orchestrator.New(s, classify.NewFake(), alert.NewLog(), cfg.ConfidenceThreshold)
	handler := httpapi.NewServer(o, s)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("SupportSentinel listening on :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
