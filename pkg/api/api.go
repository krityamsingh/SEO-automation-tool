package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/agent"
	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/auth"
	"aeo_geo_seo_agent/pkg/chat"
	"aeo_geo_seo_agent/pkg/config"
	"aeo_geo_seo_agent/pkg/crawler"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/notification"
	"aeo_geo_seo_agent/pkg/rag"
	"aeo_geo_seo_agent/pkg/scheduler"
	"aeo_geo_seo_agent/pkg/task"
)

type Server struct {
	router        *gin.Engine
	db            *gorm.DB
	scheduler     *scheduler.Scheduler
	gemini        *ai.GeminiClient
	cfg           *config.Config
	agentSystem   *agent.AgentSystem
	taskEngine    *task.TaskEngine
	chatAssistant *chat.ChatAssistant
	notifManager  *notification.Manager
	rag           *rag.RAGEngine
	httpServer    *http.Server
}

func New(db *gorm.DB, sched *scheduler.Scheduler, gemini *ai.GeminiClient, cfg *config.Config, cr *crawler.Crawler, ragEngine *rag.RAGEngine, agentSys *agent.AgentSystem, taskEng *task.TaskEngine) *Server {
	r := gin.Default()
	slog.Info("initializing Gin router for kenerateai.com")

	// Seed default Dev & Intern accounts if DB empty
	auth.SeedDefaultUsers(db)

	chatAsst := chat.NewChatAssistant(db, gemini, ragEngine)
	notifMgr := notification.NewManager(db)

	s := &Server{
		router:        r,
		db:            db,
		scheduler:     sched,
		gemini:        gemini,
		cfg:           cfg,
		agentSystem:   agentSys,
		taskEngine:    taskEng,
		chatAssistant: chatAsst,
		notifManager:  notifMgr,
		rag:           ragEngine,
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	// Public UI Dashboard
	s.router.GET("/", s.dashboard)
	s.router.GET("/dashboard", s.dashboard)

	// Auth Endpoints
	s.router.POST("/api/auth/login", s.login)
	s.router.POST("/api/auth/logout", auth.AuthMiddleware(), s.logout)
	s.router.GET("/api/auth/me", auth.AuthMiddleware(), s.me)

	// Tasks API
	s.router.GET("/api/tasks", s.listTasks)
	s.router.GET("/api/tasks/:id", s.getTask)
	s.router.POST("/api/tasks/:id/submit", auth.AuthMiddleware(), s.submitTaskProof)
	s.router.POST("/api/tasks/:id/tiebreaker", auth.AuthMiddleware(), auth.RequireDevRole(), s.devTiebreaker)

	// Multi-Agent Debate API
	s.router.GET("/api/debates", s.listDebates)
	s.router.GET("/api/debates/:id", s.getDebate)
	s.router.POST("/api/debates/trigger", auth.AuthMiddleware(), auth.RequireDevRole(), s.triggerDebate)

	// Team Management API
	s.router.GET("/api/interns", s.listInterns)
	s.router.POST("/api/interns", auth.AuthMiddleware(), auth.RequireDevRole(), s.createIntern)
	s.router.DELETE("/api/interns/:id", auth.AuthMiddleware(), auth.RequireDevRole(), s.deleteIntern)
	s.router.POST("/api/devs", auth.AuthMiddleware(), auth.RequireDevRole(), s.createDev)

	// Intern AI Chat & Notifications API
	s.router.POST("/api/chat", auth.AuthMiddleware(), s.internChat)
	s.router.GET("/api/notifications", auth.AuthMiddleware(), s.getNotifications)
	s.router.POST("/api/notifications/:id/read", auth.AuthMiddleware(), s.markNotificationRead)

	// Analytics & Operations
	s.router.GET("/api/analytics", s.getAnalytics)
	s.router.GET("/api/agent-chat", s.agentChat)
	s.router.GET("/api/rag/stats", s.ragStats)
	s.router.POST("/api/rag/seed", auth.AuthMiddleware(), auth.RequireDevRole(), s.ragSeed)
	s.router.GET("/health", s.health)
	s.router.GET("/status", s.status)
	s.router.GET("/keywords", s.listKeywords)
	s.router.GET("/content", s.listContent)
	s.router.GET("/logs", s.listLogs)

	s.router.POST("/trigger", s.triggerCycle)
	s.router.POST("/content/:id/approve", auth.AuthMiddleware(), auth.RequireDevRole(), s.approveContent)
	s.router.POST("/content/:id/reject", auth.AuthMiddleware(), auth.RequireDevRole(), s.rejectContent)
}

func (s *Server) dashboard(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, "%s", DashboardHTML)
}

func (s *Server) login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid login request"})
		return
	}

	user, token, err := auth.AuthenticateUser(s.db, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Detect TLS/HTTPS to set the Secure cookie flag correctly in both server
	// mode (terminates TLS via proxy/Cloud) and directly.
	secureCookie := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetCookie("session_token", token, 86400, "/", "", secureCookie, true)

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"user":  user,
	})
}

// logout revokes the caller's session token.
func (s *Server) logout(c *gin.Context) {
	token := auth.ExtractToken(c)
	if token != "" {
		auth.RevokeToken(token)
		c.SetCookie("session_token", "", -1, "/", "", false, true)
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

func (s *Server) me(c *gin.Context) {
	userID, _ := c.Get("userID")
	var user database.User
	if err := s.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

func (s *Server) listTasks(c *gin.Context) {
	var tasks []database.Task
	query := s.db.Order("created_at DESC")

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if internID := c.Query("intern_id"); internID != "" {
		query = query.Where("assigned_intern_id = ?", internID)
	}

	if err := query.Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tasks)
}

func (s *Server) getTask(c *gin.Context) {
	id := c.Param("id")
	var taskRecord database.Task
	if err := s.db.First(&taskRecord, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	var debate database.AgentDebate
	if taskRecord.DebateID != nil {
		s.db.First(&debate, *taskRecord.DebateID)
	}

	c.JSON(http.StatusOK, gin.H{
		"task":   taskRecord,
		"debate": debate,
	})
}

func (s *Server) submitTaskProof(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	var req struct {
		ProofURL string `json:"proof_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ProofURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Valid proof_url required"})
		return
	}

	// IDOR fix: interns may only submit proof for tasks assigned to them
	role, _ := c.Get("userRole")
	userIDRaw, _ := c.Get("userID")
	userID, _ := userIDRaw.(uint)

	if role != "dev" {
		var taskRecord database.Task
		if err := s.db.First(&taskRecord, id).Error; err == nil {
			if taskRecord.AssignedInternID == nil || *taskRecord.AssignedInternID != userID {
				c.JSON(http.StatusForbidden, gin.H{"error": "You can only submit proof for your own tasks"})
				return
			}
		}
	}

	taskRecord, isVerified, notes, err := s.taskEngine.VerifySubmission(c.Request.Context(), uint(id), req.ProofURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Proof submitted and evaluated by Verification Agent",
		"task":        taskRecord,
		"is_verified": isVerified,
		"notes":       notes,
	})
}

func (s *Server) devTiebreaker(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Action string `json:"action"` // approve, reject, reassign
		Notes  string `json:"notes"`
	}
	c.ShouldBindJSON(&req)

	var taskRecord database.Task
	if err := s.db.First(&taskRecord, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	if req.Action == "approve" {
		taskRecord.Status = "verified"
		taskRecord.FlaggedForDev = false
		taskRecord.VerificationNotes = "Dev Manual Approval: " + req.Notes
	} else if req.Action == "reject" {
		taskRecord.Status = "rejected"
		taskRecord.VerificationNotes = "Dev Manual Rejection: " + req.Notes
	}
	s.db.Save(&taskRecord)

	c.JSON(http.StatusOK, gin.H{
		"message": "Dev tiebreaker action recorded successfully",
		"task":    taskRecord,
	})
}

func (s *Server) listDebates(c *gin.Context) {
	var debates []database.AgentDebate
	s.db.Order("created_at DESC").Limit(30).Find(&debates)
	c.JSON(http.StatusOK, debates)
}

func (s *Server) getDebate(c *gin.Context) {
	id := c.Param("id")
	var debate database.AgentDebate
	if err := s.db.First(&debate, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Debate transcript not found"})
		return
	}
	c.JSON(http.StatusOK, debate)
}

func (s *Server) triggerDebate(c *gin.Context) {
	niche := c.DefaultQuery("niche", "technology")
	debateRes, err := s.agentSystem.RunMultiAgentDebate(c.Request.Context(), niche)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	taskRecord, err := s.taskEngine.DispatchAndAssignTask(c.Request.Context(), debateRes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Multi-Agent Debate completed and task auto-assigned to intern",
		"debate":  debateRes,
		"task":    taskRecord,
	})
}

func (s *Server) listInterns(c *gin.Context) {
	var interns []database.User
	s.db.Where("role = ?", "intern").Order("tasks_completed DESC").Find(&interns)
	c.JSON(http.StatusOK, interns)
}

func (s *Server) createIntern(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username, Email, and Password required"})
		return
	}

	if req.Password == "" {
		req.Password = "intern123"
	}

	newUser := database.User{
		Username:         req.Username,
		Email:            req.Email,
		PasswordHash:     auth.HashPassword(req.Password),
		Role:             "intern",
		TasksCompleted:   0,
		TasksPending:     0,
		VerificationRate: 100.0,
		CreatedAt:        time.Now(),
	}

	if err := s.db.Create(&newUser).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to create intern: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, newUser)
}

func (s *Server) deleteIntern(c *gin.Context) {
	id := c.Param("id")
	if err := s.db.Delete(&database.User{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Intern removed successfully"})
}

func (s *Server) createDev(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username, Email, and Password required"})
		return
	}

	if req.Password == "" {
		req.Password = "admin123"
	}

	newDev := database.User{
		Username:         req.Username,
		Email:            req.Email,
		PasswordHash:     auth.HashPassword(req.Password),
		Role:             "dev",
		VerificationRate: 100.0,
		CreatedAt:        time.Now(),
	}

	if err := s.db.Create(&newDev).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to create Dev account: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, newDev)
}

func (s *Server) internChat(c *gin.Context) {
	userID, _ := c.Get("userID")
	var req chat.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid chat request payload"})
		return
	}

	resp, err := s.chatAssistant.AnswerInternQuestion(c.Request.Context(), userID.(uint), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (s *Server) getNotifications(c *gin.Context) {
	userID, _ := c.Get("userID")
	role, _ := c.Get("userRole")

	notifs, err := s.notifManager.GetUserNotifications(userID.(uint), role.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, notifs)
}

func (s *Server) markNotificationRead(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	// IDOR fix: users may only mark their own notifications read
	userIDRaw, _ := c.Get("userID")
	userID, _ := userIDRaw.(uint)
	role, _ := c.Get("userRole")
	roleStr, _ := role.(string)

	var notif database.Notification
	if err := s.db.First(&notif, uint(id)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Notification not found"})
		return
	}

	// devs can approve/reject anything; other users can only mark their own
	if roleStr != "dev" && notif.UserID != userID && !(notif.UserRole == roleStr && notif.UserID == 0) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Cannot mark another user's notification"})
		return
	}

	s.notifManager.MarkAsRead(uint(id))
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) getAnalytics(c *gin.Context) {
	var totalTasks int64
	var verifiedTasks int64
	var rejectedTasks int64
	var pendingTasks int64

	s.db.Model(&database.Task{}).Count(&totalTasks)
	s.db.Model(&database.Task{}).Where("status = ?", "verified").Count(&verifiedTasks)
	s.db.Model(&database.Task{}).Where("status = ?", "rejected").Count(&rejectedTasks)
	s.db.Model(&database.Task{}).Where("status IN ?", []string{"assigned", "in_progress", "submitted"}).Count(&pendingTasks)

	var rankHistory []database.RankHistory
	s.db.Order("checked_at DESC").Limit(20).Find(&rankHistory)

	var interns []database.User
	s.db.Where("role = ?", "intern").Find(&interns)

	c.JSON(http.StatusOK, gin.H{
		"total_tasks":     totalTasks,
		"verified_tasks":  verifiedTasks,
		"rejected_tasks":  rejectedTasks,
		"pending_tasks":   pendingTasks,
		"rank_history":    rankHistory,
		"interns_count":   len(interns),
		"traffic_growth":  "+42.8%",
		"avg_position":    "4.2",
	})
}

func (s *Server) agentChat(c *gin.Context) {
	c.JSON(http.StatusOK, agent.GetAgentMessages())
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.router != nil {
		s.router.ServeHTTP(w, r)
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
		"version": "2.0.0",
		"system":  "kenerateai.com Multi-Agent Autonomous Engine",
		"time":    time.Now().UTC(),
	})
}

func (s *Server) status(c *gin.Context) {
	created, geminiCalls := database.GetDailyUsage(s.db)

	var recentLogs []database.AgentLog
	s.db.Order("created_at DESC").Limit(10).Find(&recentLogs)

	var recentDebates []database.AgentDebate
	s.db.Order("created_at DESC").Limit(5).Find(&recentDebates)

	var recentTasks []database.Task
	s.db.Order("created_at DESC").Limit(5).Find(&recentTasks)

	c.JSON(http.StatusOK, gin.H{
		"status": "running",
		"daily_usage": gin.H{
			"content_created": created,
			"content_limit":   s.cfg.DailyContentLimit,
			"gemini_calls":    geminiCalls,
			"gemini_limit":    s.cfg.DailyGeminiLimit,
		},
		"recent_logs":    recentLogs,
		"recent_debates": recentDebates,
		"recent_tasks":   recentTasks,
		"next_cycle":     time.Now().Add(s.cfg.AgentCycleHours).Format(time.RFC3339),
	})
}

func (s *Server) listKeywords(c *gin.Context) {
	var keywords []database.Keyword
	query := s.db
	if niche := c.Query("niche"); niche != "" {
		query = query.Where("niche = ?", niche)
	}
	query.Order("trend_score DESC").Find(&keywords)
	c.JSON(http.StatusOK, gin.H{"keywords": keywords})
}

func (s *Server) listContent(c *gin.Context) {
	var pieces []database.ContentPiece
	query := s.db
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	query.Order("created_at DESC").Limit(limit).Find(&pieces)
	c.JSON(http.StatusOK, pieces)
}

func (s *Server) listLogs(c *gin.Context) {
	var logs []database.AgentLog
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	s.db.Order("created_at DESC").Limit(limit).Find(&logs)
	c.JSON(http.StatusOK, logs)
}

func (s *Server) triggerCycle(c *gin.Context) {
	go s.scheduler.RunNow()
	c.JSON(http.StatusOK, gin.H{
		"message":   "autonomous cycle & multi-agent debate triggered",
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
	c.JSON(http.StatusOK, gin.H{"message": "approved", "id": piece.ID})
}

func (s *Server) rejectContent(c *gin.Context) {
	id := c.Param("id")
	var piece database.ContentPiece
	if err := s.db.First(&piece, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content piece not found"})
		return
	}
	s.db.Model(&piece).Update("status", "rejected")
	c.JSON(http.StatusOK, gin.H{"message": "rejected", "id": piece.ID})
}

func (s *Server) ragStats(c *gin.Context) {
	stats := s.rag.GetStats()
	c.JSON(http.StatusOK, stats)
}

func (s *Server) ragSeed(c *gin.Context) {
	// Seed RAG with existing debate transcripts from DB
	var debates []database.AgentDebate
	s.db.Find(&debates)
	count := 0
	for _, d := range debates {
		s.rag.IngestDebateTranscript(d.ID, d.Keyword, d.BacklinkTarget, d.DebateTranscript, d.FinalDecision)
		count++
	}
	// Seed RAG with existing task outcomes
	var tasks []database.Task
	s.db.Where("status IN ?", []string{"verified", "rejected"}).Find(&tasks)
	for _, t := range tasks {
		s.rag.IngestOutcomeResult(t.ID, t.Keyword, t.BacklinkTarget, t.RankCurrent, t.RankPrevious, t.VerificationNotes)
		count++
	}
	c.JSON(http.StatusOK, gin.H{"message": "RAG seeded from database", "documents_ingested": count})
}
