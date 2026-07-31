package handler

import (
	"fmt"
	"golang.org/x/exp/slog"
	"net/http"
	"os"
	"strings"
	"sync"

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

var (
	apiServer *api.Server
	initOnce  sync.Once
	initError error
)

func initializeServer() {
	defer func() {
		if r := recover(); r != nil {
			initError = fmt.Errorf("server initialization panic: %v", r)
			slog.Error("CRITICAL: initialization panic recovered", "error", initError)
		}
	}()

	os.Setenv("VERCEL", "1")

	cfg := config.Load()

	// Ensure SQLite path is in /tmp for Vercel serverless environment
	dbURL := cfg.DatabaseURL
	if dbURL == "" || dbURL == "sqlite://agent.db" || strings.HasSuffix(dbURL, "agent.db") {
		dbURL = "sqlite:///tmp/agent.db"
	}

	db, err := database.Connect(dbURL)
	if err != nil {
		slog.Warn("primary DB connection failed, attempting fallback to /tmp/agent.db", "error", err)
		db, err = database.Connect("sqlite:///tmp/agent.db")
		if err != nil {
			initError = fmt.Errorf("database connection failed: %w", err)
			return
		}
	}
	_ = database.AutoMigrate(db)

	gemini := ai.NewGeminiClientMulti(cfg.GeminiAPIKeys, cfg.KimiAPIKeys, cfg.MiniMaxAPIKeys, cfg.GeminiTextModel, cfg.GeminiImageModel)
	crawl := crawler.New(cfg.UserAgent, cfg.CrawlDelay)
	ragEngine := rag.New(crawl)
	agentSys := agent.NewAgentSystem(db, gemini, crawl, ragEngine)
	taskEng := task.NewTaskEngine(db, gemini, crawl, ragEngine)

	seoEngine := seo.New(gemini, crawl, db)
	aeoEngine := aeo.New(gemini, crawl, db)
	geoEngine := geo.New(gemini, crawl, db)

	wp := publisher.NewWordPress(cfg.WordPressURL, cfg.WordPressUsername, cfg.WordPressAppPassword)
	medium := publisher.NewMedium(cfg.MediumIntegrationToken)
	ghost := publisher.NewGhost(cfg.GhostURL, cfg.GhostAdminAPIKey)
	webhook := publisher.NewWebhook(cfg.WebhookURL)
	publishers := publisher.NewRegistry(wp, medium, ghost, webhook)

	writer := scriptwriter.New(gemini, db)
	sched := scheduler.New(cfg, gemini, crawl, seoEngine, writer, aeoEngine, geoEngine, publishers, db, agentSys, taskEng)

	apiServer = api.New(db, sched, gemini, cfg, crawl, ragEngine, agentSys, taskEng)
	slog.Info("Vercel serverless function initialized successfully")
}

// Handler is the Vercel serverless function entrypoint
func Handler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("CRITICAL: Vercel handler panic recovered", "panic", r)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error": "Internal Server Error", "details": "%v"}`, r)
		}
	}()

	initOnce.Do(initializeServer)

	if initError != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "Server initialization failed", "details": "%v"}`, initError)
		return
	}

	if apiServer != nil {
		apiServer.ServeHTTP(w, r)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"error": "Server unavailable"}`)
	}
}
