package handler

import (
	"net/http"
	"os"
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
