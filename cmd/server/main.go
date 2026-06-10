package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/lemonishi/supportsentinel/internal/alert"
	"github.com/lemonishi/supportsentinel/internal/classify"
	"github.com/lemonishi/supportsentinel/internal/config"
	"github.com/lemonishi/supportsentinel/internal/domain"
	"github.com/lemonishi/supportsentinel/internal/httpapi"
	"github.com/lemonishi/supportsentinel/internal/ingest/imap"
	"github.com/lemonishi/supportsentinel/internal/orchestrator"
	"github.com/lemonishi/supportsentinel/internal/qwen"
	"github.com/lemonishi/supportsentinel/internal/store"
	"github.com/lemonishi/supportsentinel/internal/tools"
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

	var clf domain.Classifier
	if cfg.DashScopeAPIKey != "" {
		clf = qwen.New(cfg.DashScopeAPIKey, cfg.DashScopeBaseURL, cfg.QwenModel, nil).
			WithTools(tools.New(s))
		if err := s.SeedDemoCustomers(ctx); err != nil {
			log.Printf("seed demo customers: %v", err)
		}
		log.Printf("classifier: Qwen via DashScope (model=%s) with tools", cfg.QwenModel)
	} else {
		clf = classify.NewFake()
		log.Printf("classifier: fake (DASHSCOPE_API_KEY not set)")
	}
	o := orchestrator.New(s, clf, alert.FromConfig(cfg), cfg.ConfidenceThreshold)

	if cfg.IMAPEnabled() {
		poller := imap.New(cfg, o)
		go poller.Run(ctx)
		log.Printf("imap poller watching %s/%s every %ds", cfg.IMAPHost, cfg.IMAPMailbox, cfg.IMAPPollSeconds)
	}

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
