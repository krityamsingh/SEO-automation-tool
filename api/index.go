package handler

import (
	"net/http"
	"os"
	"sync"

	"aeo_geo_seo_agent/internal/aeo"
	"aeo_geo_seo_agent/internal/agent"
	"aeo_geo_seo_agent/internal/ai"
	"aeo_geo_seo_agent/internal/api"
	"aeo_geo_seo_agent/internal/config"
	"aeo_geo_seo_agent/internal/crawler"
	"aeo_geo_seo_agent/internal/database"
	"aeo_geo_seo_agent/internal/geo"
	"aeo_geo_seo_agent/internal/publisher"
	"aeo_geo_seo_agent/internal/rag"
	"aeo_geo_seo_agent/internal/scheduler"
	"aeo_geo_seo_agent/internal/scriptwriter"
	"aeo_geo_seo_agent/internal/seo"
	"aeo_geo_seo_agent/internal/task"
)

var (
	apiServer *api.Server
	initOnce  sync.Once
)

func initializeServer() {
	if os.Getenv("VERCEL") == "" {
		os.Setenv("VERCEL", "1")
	}

	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		db, _ = database.Connect("sqlite:///tmp/vercel_agent.db")
	}
	_ = database.AutoMigrate(db)

	gemini, _ := ai.NewGeminiClient(cfg.GeminiAPIKey, cfg.GeminiTextModel, cfg.GeminiImageModel)
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
}

// Handler is the Vercel serverless function entrypoint
func Handler(w http.ResponseWriter, r *http.Request) {
	initOnce.Do(initializeServer)
	if apiServer != nil {
		apiServer.ServeHTTP(w, r)
	} else {
		http.Error(w, "Server initialization failed", http.StatusInternalServerError)
	}
}
