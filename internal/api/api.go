package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"aeo_geo_seo_agent/internal/config"
	"aeo_geo_seo_agent/internal/database"
	"aeo_geo_seo_agent/internal/scheduler"
)

type Server struct {
	router    *gin.Engine
	db        *gorm.DB
	scheduler *scheduler.Scheduler
	cfg       *config.Config
	httpServer *http.Server
}

func New(db *gorm.DB, sched *scheduler.Scheduler, cfg *config.Config) *Server {
	r := gin.New()
	r.Use(gin.Recovery())
	
	if cfg.LogLevel.Level() == slog.LevelDebug {
		r.Use(gin.Logger())
	}
	
	s := &Server{
		router:    r,
		db:        db,
		scheduler: sched,
		cfg:       cfg,
	}
	
	s.routes()
	return s
}

func (s *Server) routes() {
	// Public routes
	s.router.GET("/health", s.health)
	s.router.GET("/status", s.status)
	s.router.GET("/keywords", s.listKeywords)
	s.router.GET("/content", s.listContent)
	s.router.GET("/content/:id", s.getContent)
	s.router.GET("/logs", s.listLogs)
	
	// Protected routes
	protected := s.router.Group("/")
	protected.Use(s.authMiddleware())
	{
		protected.POST("/content/:id/approve", s.approveContent)
		protected.POST("/content/:id/reject", s.rejectContent)
		protected.POST("/trigger", s.triggerCycle)
	}
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.cfg.APIKey == "" {
			// No API key configured, skip auth
			c.Next()
			return
		}
		key := c.GetHeader("X-API-Key")
		if key == "" {
			key = c.Query("api_key")
		}
		if key != s.cfg.APIKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized: invalid or missing API key"})
			return
		}
		c.Next()
	}
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
	created, gemini := database.GetDailyUsage(s.db)
	
	var recentLogs []database.AgentLog
	s.db.Order("created_at DESC").Limit(10).Find(&recentLogs)
	
	var recentContent []database.ContentPiece
	s.db.Order("created_at DESC").Limit(5).Find(&recentContent)
	
	c.JSON(http.StatusOK, gin.H{
		"status": "running",
		"daily_usage": gin.H{
			"content_created": created,
			"content_limit":   s.cfg.DailyContentLimit,
			"gemini_calls":    gemini,
			"gemini_limit":    s.cfg.DailyGeminiLimit,
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
	
	c.JSON(http.StatusOK, gin.H{"content": pieces})
}

func (s *Server) getContent(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	
	var piece database.ContentPiece
	if err := s.db.First(&piece, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content not found"})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"content": piece})
}

func (s *Server) approveContent(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	
	var piece database.ContentPiece
	if err := s.db.First(&piece, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content not found"})
		return
	}
	
	piece.Status = "approved"
	s.db.Save(&piece)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "content approved",
		"id":    id,
	})
}

func (s *Server) rejectContent(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	
	var piece database.ContentPiece
	if err := s.db.First(&piece, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content not found"})
		return
	}
	
	piece.Status = "rejected"
	s.db.Save(&piece)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "content rejected",
		"id":    id,
	})
}

func (s *Server) listLogs(c *gin.Context) {
	var logs []database.AgentLog
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	
	if err := s.db.Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

func (s *Server) triggerCycle(c *gin.Context) {
	go s.scheduler.RunNow()
	c.JSON(http.StatusOK, gin.H{
		"message": "cycle triggered",
		"time":    time.Now().UTC(),
	})
}
