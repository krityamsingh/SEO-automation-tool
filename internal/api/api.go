package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"aeo_geo_seo_agent/internal/agent"
	"aeo_geo_seo_agent/internal/ai"
	"aeo_geo_seo_agent/internal/config"
	"aeo_geo_seo_agent/internal/database"
	"aeo_geo_seo_agent/internal/scheduler"
)

type Server struct {
	router     *gin.Engine
	db         *gorm.DB
	scheduler  *scheduler.Scheduler
	gemini     *ai.GeminiClient
	cfg        *config.Config
	httpServer *http.Server
}

func New(db *gorm.DB, sched *scheduler.Scheduler, gemini *ai.GeminiClient, cfg *config.Config) *Server {
	r := gin.Default()
	slog.Info("initializing Gin router")

	s := &Server{
		router:    r,
		db:        db,
		scheduler: sched,
		gemini:    gemini,
		cfg:       cfg,
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	// Public UI Dashboard & Monitoring Endpoints
	s.router.GET("/", s.dashboard)
	s.router.GET("/dashboard", s.dashboard)
	s.router.GET("/api/agent-chat", s.agentChat)
	s.router.GET("/health", s.health)
	s.router.GET("/status", s.status)
	s.router.GET("/keywords", s.listKeywords)
	s.router.GET("/content", s.listContent)
	s.router.GET("/content/:id", s.getContent)
	s.router.GET("/logs", s.listLogs)
	s.router.GET("/api-keys", s.listAPIKeys)

	// Action & Control Endpoints
	s.router.POST("/trigger", s.triggerCycle)
	s.router.POST("/content/:id/approve", s.approveContent)
	s.router.POST("/content/:id/reject", s.rejectContent)
	s.router.POST("/api-keys/select", s.selectAPIKey)
}

func (s *Server) dashboard(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, "%s", DashboardHTML)
}

func (s *Server) agentChat(c *gin.Context) {
	c.JSON(http.StatusOK, agent.GetAgentMessages())
}

func (s *Server) Start(addr string) error {
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"version": "1.0.0",
		"time":    time.Now().UTC(),
	})
}

func (s *Server) status(c *gin.Context) {
	created, geminiCalls := database.GetDailyUsage(s.db)

	var recentLogs []database.AgentLog
	s.db.Order("created_at DESC").Limit(10).Find(&recentLogs)

	var recentContent []database.ContentPiece
	s.db.Order("created_at DESC").Limit(5).Find(&recentContent)

	keyStatuses := s.gemini.GetKeyStatuses()

	c.JSON(http.StatusOK, gin.H{
		"status": "running",
		"daily_usage": gin.H{
			"content_created": created,
			"content_limit":   s.cfg.DailyContentLimit,
			"gemini_calls":    geminiCalls,
			"gemini_limit":    s.cfg.DailyGeminiLimit,
		},
		"api_key_pool": gin.H{
			"total_keys": len(keyStatuses),
			"keys":       keyStatuses,
		},
		"recent_logs":    recentLogs,
		"recent_content": recentContent,
		"next_cycle":     time.Now().Add(s.cfg.AgentCycleHours).Format(time.RFC3339),
	})
}

func (s *Server) listKeywords(c *gin.Context) {
	var keywords []database.Keyword
	query := s.db

	if niche := c.Query("niche"); niche != "" {
		query = query.Where("niche = ?", niche)
	}

	if err := query.Order("trend_score DESC").Find(&keywords).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"keywords": keywords})
}

func (s *Server) listContent(c *gin.Context) {
	var pieces []database.ContentPiece
	query := s.db

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	query = query.Order("created_at DESC").Limit(limit)

	if err := query.Find(&pieces).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, pieces)
}

func (s *Server) getContent(c *gin.Context) {
	id := c.Param("id")
	var piece database.ContentPiece

	if err := s.db.First(&piece, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content piece not found"})
		return
	}

	c.JSON(http.StatusOK, piece)
}

func (s *Server) listLogs(c *gin.Context) {
	var logs []database.AgentLog
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	if err := s.db.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, logs)
}

func (s *Server) listAPIKeys(c *gin.Context) {
	statuses := s.gemini.GetKeyStatuses()
	c.JSON(http.StatusOK, gin.H{
		"total_keys": len(statuses),
		"keys":       statuses,
	})
}

func (s *Server) selectAPIKey(c *gin.Context) {
	var req struct {
		Index int `json:"index"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	if err := s.gemini.SelectKey(req.Index); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "API key selected successfully",
		"active_index": req.Index,
		"key_statuses": s.gemini.GetKeyStatuses(),
	})
}

func (s *Server) triggerCycle(c *gin.Context) {
	go s.scheduler.RunNow()
	c.JSON(http.StatusOK, gin.H{
		"message":   "autonomous cycle triggered",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (s *Server) approveContent(c *gin.Context) {
	id := c.Param("id")
	var piece database.ContentPiece

	if err := s.db.First(&piece, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content piece not found"})
		return
	}

	s.db.Model(&piece).Update("status", "approved")
	database.LogAgentActivity(s.db, "content_approval", "success", "content approved: "+piece.Title)

	c.JSON(http.StatusOK, gin.H{
		"message": "content approved and queued for publishing",
		"id":      piece.ID,
		"title":   piece.Title,
	})
}

func (s *Server) rejectContent(c *gin.Context) {
	id := c.Param("id")
	var piece database.ContentPiece

	if err := s.db.First(&piece, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content piece not found"})
		return
	}

	s.db.Model(&piece).Update("status", "rejected")
	database.LogAgentActivity(s.db, "content_rejection", "info", "content rejected: "+piece.Title)

	c.JSON(http.StatusOK, gin.H{
		"message": "content rejected",
		"id":      piece.ID,
	})
}
