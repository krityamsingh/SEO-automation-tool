package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aeo_geo_seo_agent/pkg/aeo"
	"aeo_geo_seo_agent/pkg/agent"
	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/api"
	"aeo_geo_seo_agent/pkg/config"
	"aeo_geo_seo_agent/pkg/crawler"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/geo"
	"aeo_geo_seo_agent/pkg/publisher"
	"aeo_geo_seo_agent/pkg/rag"
	"aeo_geo_seo_agent/pkg/scheduler"
	"aeo_geo_seo_agent/pkg/scriptwriter"
	"aeo_geo_seo_agent/pkg/seo"
	"aeo_geo_seo_agent/pkg/task"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load config
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("config validation failed", "error", err)
		os.Exit(1)
	}
	if cfg.GeminiAPIKey == "" {
		slog.Error("GEMINI_API_KEY is required")
	}

	// Setup logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	})))

	slog.Info("starting kenerateai.com agent platform", "version", "2.0.0", "cycle_hours", cfg.AgentCycleHours)

	// Database
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	if err := database.AutoMigrate(db); err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	// AI Client
	gemini, err := ai.NewGeminiClient(cfg.GeminiAPIKey, cfg.GeminiTextModel, cfg.GeminiImageModel)
	if err != nil {
		slog.Error("gemini client failed", "error", err)
		os.Exit(1)
	}

	// Crawler
	crawl := crawler.New(cfg.UserAgent, cfg.CrawlDelay)

	// Shared System-Wide RAG Engine
	ragEngine := rag.New(crawl)

	// Multi-Agent System & Task Engine
	agentSys := agent.NewAgentSystem(db, gemini, crawl, ragEngine)
	taskEng := task.NewTaskEngine(db, gemini, crawl, ragEngine)

	// SEO Engine
	seoEngine := seo.New(gemini, crawl, db)

	// AEO and GEO Engines
	aeoEngine := aeo.New(gemini, crawl, db)
	geoEngine := geo.New(gemini, crawl, db)

	// Publishers
	wp := publisher.NewWordPress(cfg.WordPressURL, cfg.WordPressUsername, cfg.WordPressAppPassword)
	medium := publisher.NewMedium(cfg.MediumIntegrationToken)
	ghost := publisher.NewGhost(cfg.GhostURL, cfg.GhostAdminAPIKey)
	webhook := publisher.NewWebhook(cfg.WebhookURL)
	publishers := publisher.NewRegistry(wp, medium, ghost, webhook)

	// Script Writer
	writer := scriptwriter.New(gemini, db)

	// Scheduler
	sched := scheduler.New(cfg, gemini, crawl, seoEngine, writer, aeoEngine, geoEngine, publishers, db, agentSys, taskEng)

	// API Server
	apiServer := api.New(db, sched, gemini, cfg, crawl, ragEngine, agentSys, taskEng)
	go func() {
		if err := apiServer.Start(":" + cfg.Port); err != nil && err != http.ErrServerClosed {
			slog.Error("api server error", "error", err)
		}
	}()

	// Start scheduler
	sched.Start(ctx)

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	slog.Info("kenerateai.com platform running", "port", cfg.Port, "cycle", cfg.AgentCycleHours)
	<-sig

	slog.Info("shutting down gracefully")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("api shutdown error", "error", err)
	}
	sched.Stop()
	gemini.Close()

	// Final log
	database.LogAgentActivity(db, "shutdown", "success", "agent stopped gracefully")
	slog.Info("agent stopped")
}
