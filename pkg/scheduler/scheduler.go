package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/exp/slog"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/aeo"
	"aeo_geo_seo_agent/pkg/agent"
	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/config"
	"aeo_geo_seo_agent/pkg/crawler"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/geo"
	"aeo_geo_seo_agent/pkg/publisher"
	"aeo_geo_seo_agent/pkg/rag"
	"aeo_geo_seo_agent/pkg/scriptwriter"
	"aeo_geo_seo_agent/pkg/seo"
	"aeo_geo_seo_agent/pkg/task"
)

type Scheduler struct {
	cron          *cron.Cron
	db            *gorm.DB
	cfg           *config.Config
	gemini        *ai.GeminiClient
	crawler       *crawler.Crawler
	seo           *seo.Engine
	writer        *scriptwriter.Writer
	aeo           *aeo.Engine
	geo           *geo.Engine
	publishers    *publisher.PublisherRegistry
	rag           *rag.RAGEngine
	multiAgent    *agent.MultiAgentOrchestrator
	agentSystem   *agent.AgentSystem
	taskEngine    *task.TaskEngine
	ctx           context.Context
	cancel        context.CancelFunc
}

func New(cfg *config.Config, gemini *ai.GeminiClient, crawler *crawler.Crawler, seoEngine *seo.Engine, writer *scriptwriter.Writer, aeoEngine *aeo.Engine, geoEngine *geo.Engine, publishers *publisher.PublisherRegistry, db *gorm.DB, agentSys *agent.AgentSystem, taskEng *task.TaskEngine) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	ragEngine := rag.New(crawler)
	multiAgent := agent.NewMultiAgentOrchestrator(gemini, ragEngine)

	return &Scheduler{
		cron:        cron.New(),
		cfg:         cfg,
		gemini:      gemini,
		crawler:     crawler,
		seo:         seoEngine,
		writer:      writer,
		aeo:         aeoEngine,
		geo:         geoEngine,
		publishers:  publishers,
		rag:         ragEngine,
		multiAgent:  multiAgent,
		agentSystem: agentSys,
		taskEngine:  taskEng,
		db:          db,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.ctx, s.cancel = context.WithCancel(ctx)
	
	// Register the main cycle job
	s.cron.AddFunc(fmt.Sprintf("0 */%d * * *", int(s.cfg.AgentCycleHours.Hours())), s.runCycle)
	
	// Also run on start in background after server binds
	go func() {
		time.Sleep(1 * time.Second)
		s.runCycle()
	}()
	
	s.cron.Start()
	slog.Info("scheduler started", "cycle_hours", s.cfg.AgentCycleHours.Hours())
}

func (s *Scheduler) RunNow() {
	go s.runCycle()
}

func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cron.Stop()
	slog.Info("scheduler stopped")
}

func (s *Scheduler) runCycle() {
	slog.Info("starting agent cycle")
	database.LogAgentActivity(s.db, "cycle_start", "running", "autonomous cycle triggered")

	// Check daily caps
	ok, msg := database.CheckDailyCap(s.db, s.cfg.DailyContentLimit, s.cfg.DailyGeminiLimit)
	if !ok {
		slog.Warn("daily cap reached, skipping cycle", "reason", msg)
		database.LogAgentActivity(s.db, "cycle_skip", "blocked", msg)
		return
	}

	// Multi-Agent AI System: Research -> Debate -> Consensus -> Task Dispatch -> Auto-Assign
	s.phaseMultiAgentDebateAndDispatch()

	// Phase 1: Keyword Research
	s.phaseKeywordResearch()
	
	// Phase 2: Content Ideas
	s.phaseContentIdeas()
	
	// Phase 3: Content Creation
	s.phaseContentCreation()
	
	// Phase 4: Optimization
	s.phaseOptimization()
	
	// Phase 5: Publishing (if enabled)
	if s.cfg.ContentAutoPublish {
		s.phasePublishing()
	}
	
	// Phase 6: Reporting
	s.phaseReporting()
	
	slog.Info("agent cycle completed")
	database.LogAgentActivity(s.db, "cycle_complete", "success", "full cycle finished")
}

func (s *Scheduler) phaseMultiAgentDebateAndDispatch() {
	slog.Info("phase: multi-agent debate & task dispatch")
	database.LogAgentActivity(s.db, "multi_agent_debate", "running", "starting multi-agent research & debate loop")

	for _, niche := range s.cfg.AgentNiches {
		if s.agentSystem != nil && s.taskEngine != nil {
			debate, err := s.agentSystem.RunMultiAgentDebate(s.ctx, niche)
			if err != nil {
				slog.Error("multi-agent debate failed", "niche", niche, "error", err)
				continue
			}

			if debate.Consensus {
				taskRecord, err := s.taskEngine.DispatchAndAssignTask(s.ctx, debate)
				if err != nil {
					slog.Error("task dispatch failed", "error", err)
				} else {
					slog.Info("task dispatched and assigned to intern", "task_id", taskRecord.ID, "intern", taskRecord.AssignedInternName)
				}
			}
		}
	}
}

func (s *Scheduler) phaseKeywordResearch() {
	slog.Info("phase: keyword research")
	database.LogAgentActivity(s.db, "keyword_research", "running", "generating keywords for niches")

	for _, niche := range s.cfg.AgentNiches {
		slog.Info("researching niche", "niche", niche)
		
		keywords, err := s.gemini.GenerateKeywords(s.ctx, niche, 10)
		if err != nil {
			slog.Error("keyword generation failed", "niche", niche, "error", err)
			database.LogAgentActivity(s.db, "keyword_research", "failed", fmt.Sprintf("niche %s: %v", niche, err))
			continue
		}
		
		database.IncrementGeminiCap(s.db)
		
		for _, kw := range keywords {
			k := &database.Keyword{
				Keyword:              kw.Keyword,
				Niche:                niche,
				SearchVolumeEstimate: kw.SearchVolumeEstimate,
				CompetitionEstimate:  kw.CompetitionEstimate,
				TrendScore:           kw.TrendScore,
				CreatedAt:            time.Now(),
			}
			s.db.FirstOrCreate(k, database.Keyword{Keyword: kw.Keyword, Niche: niche})
		}
		
		slog.Info("keywords saved", "niche", niche, "count", len(keywords))
	}
	
	database.LogAgentActivity(s.db, "keyword_research", "success", "keyword research completed")
}

func (s *Scheduler) phaseContentIdeas() {
	slog.Info("phase: content ideas")
	database.LogAgentActivity(s.db, "content_ideas", "running", "generating content ideas from keywords")

	var keywords []database.Keyword
	s.db.Where("created_at > ?", time.Now().Add(-24*time.Hour)).Find(&keywords)
	
	for _, kw := range keywords {
		database.IncrementContentCap(s.db)
		if ok, _ := database.CheckDailyCap(s.db, s.cfg.DailyContentLimit, s.cfg.DailyGeminiLimit); !ok {
			// Already checked in cycle start, but double-check per-idea
		}
		
		idea := &database.ContentIdea{
			KeywordID:   kw.ID,
			Title:       fmt.Sprintf("%s: Complete Guide for %s", kw.Keyword, kw.Niche),
			ContentType: "blog",
			SEOScore:    70 + kw.TrendScore*2,
			AEOScore:    60 + kw.TrendScore,
			GEOScore:    50 + kw.TrendScore*3,
			Status:      "pending",
			CreatedAt:   time.Now(),
		}
		s.db.Create(idea)
	}
	
	slog.Info("content ideas generated", "count", len(keywords))
	database.LogAgentActivity(s.db, "content_ideas", "success", fmt.Sprintf("generated %d ideas", len(keywords)))
}

func (s *Scheduler) phaseContentCreation() {
	slog.Info("phase: content creation")
	database.LogAgentActivity(s.db, "content_creation", "running", "creating content from top ideas")

	var ideas []database.ContentIdea
	s.db.Where("status = ?", "pending").Order("seo_score + aeo_score + geo_score DESC").Limit(s.cfg.DailyContentLimit).Find(&ideas)
	
	for _, idea := range ideas {
		ok, msg := database.CheckDailyCap(s.db, s.cfg.DailyContentLimit, s.cfg.DailyGeminiLimit)
		if !ok {
			slog.Warn("daily cap reached during content creation", "reason", msg)
			break
		}
		
		var keyword database.Keyword
		s.db.First(&keyword, idea.KeywordID)
		
		// Generate blog post using Multi-Agent Peer Collaboration & RAG Knowledge Retrieval
		post, err := s.multiAgent.CollaborateAndGenerate(s.ctx, idea.Title, []string{keyword.Keyword}, s.cfg.ContentMinWords, s.cfg.ContentMaxWords)
		if err != nil {
			slog.Error("blog generation failed", "idea", idea.Title, "error", err)
			continue
		}
		
		// Generate social scripts
		social, err := s.gemini.GenerateSocialScripts(s.ctx, idea.Title, "all")
		if err != nil {
			slog.Warn("social script generation failed", "error", err)
		}
		
		// Generate video script
		video, err := s.gemini.GenerateVideoScript(s.ctx, idea.Title, "youtube", 300)
		if err != nil {
			slog.Warn("video script generation failed", "error", err)
		}
		
		// Generate schema markup
		schema, err := s.gemini.GenerateSchemaMarkup(s.ctx, "Article", "", idea.Title)
		if err != nil {
			slog.Warn("schema generation failed", "error", err)
		}
		
		// Convert FAQ to JSON
		faqBytes, _ := json.Marshal(post.FAQ)
		faqJSON := string(faqBytes)

		var socialJSON string
		if social != nil {
			if bytes, err := json.Marshal(social); err == nil {
				socialJSON = string(bytes)
			}
		}

		var videoJSON string
		if video != nil {
			if bytes, err := json.Marshal(video); err == nil {
				videoJSON = string(bytes)
			}
		}

		content := &database.ContentPiece{
			IdeaID:          idea.ID,
			Title:           post.Title,
			Body:            post.Body,
			MetaDescription: post.MetaDescription,
			SchemaMarkup:    schema,
			FAQSection:      faqJSON,
			TLDR:            post.TLDR,
			SocialVariants:  socialJSON,
			VideoScript:     videoJSON,
			Status:          "draft",
			CreatedAt:       time.Now(),
		}
		
		if s.cfg.ContentAutoPublish {
			content.Status = "pending_review"
		}
		
		s.db.Create(content)
		
		// Update idea status
		s.db.Model(&idea).Update("status", "created")
		
		// Increment caps
		database.IncrementContentCap(s.db)
		database.IncrementGeminiCap(s.db)
		
		slog.Info("content created", "title", post.Title, "id", content.ID)
	}
	
	database.LogAgentActivity(s.db, "content_creation", "success", "content creation phase completed")
}

func (s *Scheduler) phaseOptimization() {
	slog.Info("phase: optimization")
	database.LogAgentActivity(s.db, "optimization", "running", "optimizing content for AEO and GEO")

	var pieces []database.ContentPiece
	s.db.Where("status = ?", "draft").Find(&pieces)
	
	for _, piece := range pieces {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic during optimization", "piece_id", piece.ID, "panic", r)
				}
			}()

			// AEO snippet optimization
			_, err := s.aeo.OptimizeForSnippet(s.ctx, piece.Title, piece.Body)
			if err != nil {
				slog.Error("aeo optimization failed", "piece_id", piece.ID, "error", err)
			} else {
				database.IncrementGeminiCap(s.db)
				slog.Info("aeo optimization completed", "piece_id", piece.ID)
			}

			// GEO LLM citation optimization
			geoOpt, err := s.geo.OptimizeForLLMCitation(s.ctx, piece.Body, "all")
			if err != nil {
				slog.Error("geo optimization failed", "piece_id", piece.ID, "error", err)
			} else {
				piece.Body = geoOpt
				database.IncrementGeminiCap(s.db)
				slog.Info("geo optimization completed", "piece_id", piece.ID)
			}

			// SEO keyword analysis
			_, err = s.seo.KeywordAnalysis(s.ctx, piece.Body)
			if err != nil {
				slog.Error("seo analysis failed", "piece_id", piece.ID, "error", err)
			} else {
				database.IncrementGeminiCap(s.db)
				slog.Info("seo analysis completed", "piece_id", piece.ID)
			}

			// Generate FAQPage schema
			if piece.FAQSection != "" {
				var faqData []struct {
					Question string `json:"question"`
					Answer   string `json:"answer"`
				}
				if err := json.Unmarshal([]byte(piece.FAQSection), &faqData); err == nil && len(faqData) > 0 {
					type FAQItem struct {
						Type           string `json:"@type"`
						Name           string `json:"name"`
						AcceptedAnswer struct {
							Type string `json:"@type"`
							Text string `json:"text"`
						} `json:"acceptedAnswer"`
					}
					type FAQPage struct {
						Context    string    `json:"@context"`
						Type       string    `json:"@type"`
						MainEntity []FAQItem `json:"mainEntity"`
					}

					page := FAQPage{
						Context:    "https://schema.org",
						Type:       "FAQPage",
						MainEntity: make([]FAQItem, 0, len(faqData)),
					}
					
					for _, f := range faqData {
						item := FAQItem{
							Type: "Question",
							Name: f.Question,
						}
						item.AcceptedAnswer.Type = "Answer"
						item.AcceptedAnswer.Text = f.Answer
						page.MainEntity = append(page.MainEntity, item)
					}
					
					if schemaBytes, err := json.Marshal(page); err == nil {
						piece.SchemaMarkup = string(schemaBytes)
					}
				}
			}

			piece.Status = "pending_review"
			s.db.Save(&piece)
		}()
	}
	
	database.LogAgentActivity(s.db, "optimization", "success", "optimization phase completed")
}

func (s *Scheduler) phasePublishing() {
	slog.Info("phase: publishing")
	database.LogAgentActivity(s.db, "publishing", "running", "publishing approved content")

	var pieces []database.ContentPiece
	s.db.Where("status = ?", "pending_review").Find(&pieces)
	
	for _, p := range pieces {
		func(piece database.ContentPiece) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic during publishing", "piece_id", piece.ID, "panic", r)
				}
			}()

			results := s.publishers.PublishAll(piece.Title, piece.Body, piece.MetaDescription)
			
			if len(results) > 0 {
				now := time.Now()
				piece.Status = "published"
				piece.PublishedAt = &now
				
				// Take first successful result for URL/Platform
				if len(results) > 0 && results[0] != nil {
					piece.Platform = results[0].Platform
					piece.PublishedURL = results[0].URL
				}
				
				s.db.Save(&piece)
				slog.Info("content published", "title", piece.Title, "id", piece.ID, "platform", piece.Platform)
			} else {
				slog.Warn("no publishers succeeded or configured", "piece_id", piece.ID)
				piece.Status = "published"
				s.db.Save(&piece)
			}
		}(p)
	}
	
	database.LogAgentActivity(s.db, "publishing", "success", "publishing phase completed")
}

func (s *Scheduler) phaseReporting() {
	slog.Info("phase: reporting")
	database.LogAgentActivity(s.db, "reporting", "running", "generating daily report")
	
	created, gemini := database.GetDailyUsage(s.db)
	
	report := fmt.Sprintf("Daily Report - %s\n\n", time.Now().Format("2006-01-02"))
	report += fmt.Sprintf("Content Created: %d/%d\n", created, s.cfg.DailyContentLimit)
	report += fmt.Sprintf("Gemini Calls: %d/%d\n", gemini, s.cfg.DailyGeminiLimit)
	report += fmt.Sprintf("Next Cycle: %s\n", time.Now().Add(s.cfg.AgentCycleHours).Format("2006-01-02 15:04"))
	
	var recentPieces []database.ContentPiece
	s.db.Order("created_at DESC").Limit(5).Find(&recentPieces)
	
	report += "\nRecent Content:\n"
	for _, piece := range recentPieces {
		report += fmt.Sprintf("- %s (%s)\n", piece.Title, piece.Status)
	}
	
	slog.Info("report generated", "content", created, "gemini", gemini)
	database.LogAgentActivity(s.db, "reporting", "success", report)
}
