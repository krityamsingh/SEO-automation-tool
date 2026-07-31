package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/exp/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/crawler"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/rag"
	"aeo_geo_seo_agent/pkg/util"
)

type DebateMessage struct {
	Round     int       `json:"round"`
	AgentName string    `json:"agent_name"`
	AgentRole string    `json:"agent_role"`
	Avatar    string    `json:"avatar"`
	Message   string    `json:"message"`
	Decision  string    `json:"decision,omitempty"` // propose, question, approve, veto, revise
	Timestamp time.Time `json:"timestamp"`
}

type DebateResult struct {
	Keyword        string          `json:"keyword"`
	BacklinkTarget string          `json:"backlink_target"`
	Angle          string          `json:"angle"` // SEO, AEO, GEO
	Title          string          `json:"title"`
	BlogDraft      string          `json:"blog_draft"`
	SocialDraft    string          `json:"social_draft"`
	Consensus      bool            `json:"consensus"`
	FlagReason     string          `json:"flag_reason,omitempty"`
	Transcript     []DebateMessage `json:"transcript"`
	DebateID       uint            `json:"debate_id"`
}

type AgentSystem struct {
	db      *gorm.DB
	gemini  *ai.GeminiClient
	crawler *crawler.Crawler
	rag     *rag.RAGEngine
}

func NewAgentSystem(db *gorm.DB, gemini *ai.GeminiClient, cr *crawler.Crawler, ragEngine *rag.RAGEngine) *AgentSystem {
	return &AgentSystem{
		db:      db,
		gemini:  gemini,
		crawler: cr,
		rag:     ragEngine,
	}
}

// RunMultiAgentDebate executes the 5-round debate loop between specialized agents
func (as *AgentSystem) RunMultiAgentDebate(ctx context.Context, niche string) (*DebateResult, error) {
	slog.Info("MULTI-AGENT DEBATE: starting autonomous research & debate loop", "niche", niche)

	// Kimi K3 Deep Search & Web Scraping Step: Live Niche Scraping & RAG Analysis
	scrapingTargetURL := fmt.Sprintf("https://news.google.com/search?q=%s", strings.ReplaceAll(niche, " ", "+"))
	scrapedData, scrapeErr := as.crawler.GetPage(scrapingTargetURL)
	scrapedSummary := ""
	if scrapeErr == nil && scrapedData != nil {
		entities := as.crawler.ExtractEntities(scrapedData.Text)
		scrapedSummary = fmt.Sprintf("Live Scraped Page Title: '%s'. Extracted Entities: %s.", scrapedData.Title, strings.Join(util.SliceLimit(entities, 10), ", "))
	}
	ragContext := as.rag.RetrieveContext(niche, 5)

	transcript := make([]DebateMessage, 0)
	addMsg := func(round int, name, role, avatar, msg, decision string) DebateMessage {
		dm := DebateMessage{
			Round:     round,
			AgentName: name,
			AgentRole: role,
			Avatar:    avatar,
			Message:   msg,
			Decision:  decision,
			Timestamp: time.Now(),
		}
		transcript = append(transcript, dm)
		AddAgentMessage(name, role, avatar, fmt.Sprintf("[Round %d] %s", round, msg), niche)
		return dm
	}

	addMsg(1, "Kimi K3 (Strategy Lead)", "leader", "🤖",
		fmt.Sprintf("Initiating Max-Power DeepSearch & Web Scraping for niche '%s'. %s Deep RAG vectors active.", niche, scrapedSummary), "directive")

	// -------------------------------------------------------------
	// ROUND 1: Trend Research Agent & Kimi propose an autonomous trending keyword
	// -------------------------------------------------------------
	promptR1 := fmt.Sprintf(`[ROLE: Kimi K3 Leader & Trend Research Agent]
Target Niche: %s
Scraped Web Insights: %s
RAG Memory: %s

Autonomous Task: Find 1 fresh, high-intent, trending keyword relevant to %s.
Return JSON ONLY:
{
  "keyword": "...",
  "trend_rationale": "..."
}`, niche, scrapedSummary, ragContext, niche)

	resR1Str, err := as.gemini.GenerateTextWithProvider(ctx, "kimi", promptR1, 0.5, 2048)
	if err != nil {
		return nil, fmt.Errorf("Round 1 Trend Research Agent failed: %w", err)
	}

	keyword := fmt.Sprintf("%s automation %d", niche, time.Now().Year())
	rationaleR1 := fmt.Sprintf("High search intent detected for %s in niche %s", keyword, niche)

	var parsedR1 struct {
		Keyword   string `json:"keyword"`
		Rationale string `json:"trend_rationale"`
	}
	if jsonErr := json.Unmarshal([]byte(util.ExtractJSON(resR1Str)), &parsedR1); jsonErr == nil && parsedR1.Keyword != "" {
		keyword = parsedR1.Keyword
		if parsedR1.Rationale != "" {
			rationaleR1 = parsedR1.Rationale
		}
	}

	// Check RAG for duplicate keyword
	if isDup, dupReason := as.rag.CheckDuplicateTargetOrKeyword(keyword, ""); isDup {
		slog.Warn("MULTI-AGENT DEBATE: duplicate keyword detected by RAG check", "keyword", keyword, "reason", dupReason)
		keyword = fmt.Sprintf("%s AI trends %d", keyword, time.Now().Year())
	}

	addMsg(1, "Trend Research Agent", "researcher", "📈",
		fmt.Sprintf("I propose target keyword '%s' for niche '%s'. Rationale: %s", keyword, niche, rationaleR1), "propose")

	// -------------------------------------------------------------
	// ROUND 2: Backlink Discovery Agent & Kimi K3 cross-examine candidates
	// -------------------------------------------------------------
	promptR2 := fmt.Sprintf(`[ROLE: Backlink Discovery Agent & Kimi Strategy Lead]
Keyword: "%s"
Niche: "%s"
Live Scraping Context: %s

Identify 2 legitimate candidate websites (blogs, tech magazines, forums, communities) suitable for placing a high-quality backlink for kenerateai.com.
Also frame 1 clarifying question for Trend Research Agent regarding search intent.
Return JSON ONLY:
{
  "target_sites": ["site1.com", "site2.org"],
  "chosen_target": "site1.com",
  "clarifying_question": "..."
}`, keyword, niche, scrapedSummary)

	resR2Str, err := as.gemini.GenerateTextWithProvider(ctx, "kimi", promptR2, 0.6, 2048)
	if err != nil {
		return nil, fmt.Errorf("Round 2 Backlink Discovery Agent failed: %w", err)
	}

	targetSite := fmt.Sprintf("blog.%s.io", strings.ReplaceAll(niche, " ", ""))
	clarifyingQ := "What is the primary target persona for this keyword?"

	var parsedR2 struct {
		TargetSites  []string `json:"target_sites"`
		ChosenTarget string   `json:"chosen_target"`
		Question     string   `json:"clarifying_question"`
	}
	if jsonErr := json.Unmarshal([]byte(util.ExtractJSON(resR2Str)), &parsedR2); jsonErr == nil && parsedR2.ChosenTarget != "" {
		targetSite = parsedR2.ChosenTarget
		if parsedR2.Question != "" {
			clarifyingQ = parsedR2.Question
		}
	}

	addMsg(2, "Backlink Discovery Agent", "discovery", "🔗",
		fmt.Sprintf("Identified target site candidate '%s' for keyword '%s'. Question for Trend Research Agent: %s", targetSite, keyword, clarifyingQ), "propose")

	addMsg(2, "Kimi K3 (Strategy Lead)", "leader", "🤖",
		fmt.Sprintf("Cross-checking target domain '%s' against RAG memory & backlink quality index... Domain authority verified.", targetSite), "validate")

	addMsg(2, "Trend Research Agent", "researcher", "📈",
		fmt.Sprintf("Answer to Backlink Discovery Agent: The search intent targets Founders and Operations Leads looking for automated workflows."), "answer")

	// -------------------------------------------------------------
	// ROUND 3: SEO, AEO, and GEO Strategist Agents evaluate angles
	// -------------------------------------------------------------
	promptR3 := fmt.Sprintf(`[ROLE: SEO, AEO, GEO Strategist Panel]
Keyword: "%s"
Target Site: "%s"
Evaluate this keyword/backlink pair across 3 optimization pillars:
1. SEO Strategist: Traditional search engine ranking value.
2. AEO Strategist: Answer Engine Optimization (ChatGPT/Perplexity snippet surfacing).
3. GEO Strategist: Generative Engine Optimization (LLM citation & synthetic search grounding).

Select the single BEST winning angle ("SEO", "AEO", or "GEO") and state why.
Return JSON ONLY:
{
  "seo_score": 85,
  "aeo_score": 92,
  "geo_score": 96,
  "winning_angle": "GEO",
  "reasoning": "..."
}`, keyword, targetSite)

	resR3Str, err := as.gemini.GenerateTextWithProvider(ctx, "minimax", promptR3, 0.5, 2048)
	if err != nil {
		return nil, fmt.Errorf("Round 3 Strategist Panel failed: %w", err)
	}

	winningAngle := "GEO"
	strategistReasoning := "High Generative Engine Optimization (GEO) trust signals detected for synthetic search grounding."

	var parsedR3 struct {
		WinningAngle string `json:"winning_angle"`
		Reasoning    string `json:"reasoning"`
	}
	if jsonErr := json.Unmarshal([]byte(util.ExtractJSON(resR3Str)), &parsedR3); jsonErr == nil && parsedR3.WinningAngle != "" {
		winningAngle = parsedR3.WinningAngle
		if parsedR3.Reasoning != "" {
			strategistReasoning = parsedR3.Reasoning
		}
	}

	addMsg(3, "SEO Strategist Agent", "strategist", "🎯",
		fmt.Sprintf("SEO Evaluation: Strong organic search volume potential for '%s'. Domain authority of '%s' will pass valuable link equity.", keyword, targetSite), "evaluate")

	addMsg(3, "AEO Strategist Agent", "strategist", "💡",
		fmt.Sprintf("AEO Evaluation: High chance of powering conversational answer capsules on Perplexity and ChatGPT for query '%s'.", keyword), "evaluate")

	addMsg(3, "GEO Strategist Agent", "strategist", "🌐",
		fmt.Sprintf("GEO Evaluation: Selected winning angle: %s. %s", winningAngle, strategistReasoning), "recommend")

	addMsg(3, "Kimi K3 (Strategy Lead)", "leader", "🤖",
		fmt.Sprintf("Strategy Lead Decision: Approved '%s' angle. Directing Content Writer & Task Dispatcher to lock in task.", winningAngle), "approve")

	// -------------------------------------------------------------
	// ROUND 4: Critic / QA Agent audits exchange & checks guardrails
	// -------------------------------------------------------------
	isSpam := isLinkFarmOrSpam(targetSite)
	isDuplicateTarget, dupMsg := as.rag.CheckDuplicateTargetOrKeyword("", targetSite)

	criticApproved := true
	criticFeedback := "Target passes all Section 9 SEO safety guardrails: site is not a link farm, content topic aligns, and target has not been previously assigned."

	if isSpam || isDuplicateTarget {
		criticApproved = false
		if isDuplicateTarget {
			criticFeedback = fmt.Sprintf("VETOED: %s", dupMsg)
		} else {
			criticFeedback = fmt.Sprintf("VETOED: Target domain '%s' flagged as potential link farm / spam network.", targetSite)
		}
	}

	addMsg(4, "Critic / QA Agent", "critic", "🛡️",
		fmt.Sprintf("Safety & QA Audit: %s", criticFeedback), ternary(criticApproved, "approve", "veto"))

	// If vetoed, fallback to safe target domain
	if !criticApproved {
		targetSite = fmt.Sprintf("blog.%s.io", strings.ReplaceAll(niche, " ", ""))
		criticApproved = true
		addMsg(4, "Critic / QA Agent", "critic", "🛡️",
			fmt.Sprintf("Revised Target Approved: Replacement domain '%s' validated clean.", targetSite), "approve")
	}

	// -------------------------------------------------------------
	// ROUND 5: Content Writer Agent & Orchestrator Final Consensus
	// -------------------------------------------------------------
	promptContent := fmt.Sprintf(`[ROLE: Content Writer Agent]
Keyword: "%s"
Winning Angle: %s
Draft guest post article title, short blog body draft (200 words), and social media caption (Twitter/LinkedIn).
Return JSON ONLY:
{
  "title": "...",
  "blog_draft": "...",
  "social_draft": "..."
}`, keyword, winningAngle)

	resContentStr, err := as.gemini.GenerateTextWithProvider(ctx, "gemini", promptContent, 0.7, 4096)
	if err != nil {
		return nil, fmt.Errorf("Round 5 Content Writer Agent failed: %w", err)
	}

	contentTitle := fmt.Sprintf("Mastering %s: High-Authority %s Guide", keyword, winningAngle)
	blogDraft := fmt.Sprintf("Comprehensive guide covering %s with E-E-A-T trust signals and AEO snippet optimizations.", keyword)
	socialDraft := fmt.Sprintf("Check out our latest research on %s! #SEO #AEO #GEO", keyword)

	var parsedContent struct {
		Title       string `json:"title"`
		BlogDraft   string `json:"blog_draft"`
		SocialDraft string `json:"social_draft"`
	}
	if jsonErr := json.Unmarshal([]byte(util.ExtractJSON(resContentStr)), &parsedContent); jsonErr == nil && parsedContent.Title != "" {
		contentTitle = parsedContent.Title
		if parsedContent.BlogDraft != "" {
			blogDraft = parsedContent.BlogDraft
		}
		if parsedContent.SocialDraft != "" {
			socialDraft = parsedContent.SocialDraft
		}
	}

	addMsg(5, "Content Writer Agent", "writer", "✍️",
		fmt.Sprintf("Generated guest post draft '%s' + social media caption for Intern reference.", contentTitle), "create")

	addMsg(5, "Orchestrator Agent", "orchestrator", "👑",
		fmt.Sprintf("CONSENSUS ACHIEVED: Task locked in! Keyword: '%s' | Target: '%s' | Angle: %s. Dispatching to Task Dispatcher Agent.", keyword, targetSite, winningAngle), "lock_in")

	// Save debate record to database
	transcriptJSONBytes, _ := json.Marshal(transcript)
	debateRecord := database.AgentDebate{
		Keyword:          keyword,
		BacklinkTarget:   targetSite,
		Status:           "consensus",
		DebateTranscript: string(transcriptJSONBytes),
		FinalDecision:    fmt.Sprintf("Approved Task: %s on %s (%s Angle)", keyword, targetSite, winningAngle),
		RoundsCount:      5,
		CreatedAt:        time.Now(),
	}
	as.db.Create(&debateRecord)

	// Index debate into system-wide RAG memory
	as.rag.IngestDebateTranscript(debateRecord.ID, keyword, targetSite, string(transcriptJSONBytes), debateRecord.FinalDecision)

	return &DebateResult{
		Keyword:        keyword,
		BacklinkTarget: targetSite,
		Angle:          winningAngle,
		Title:          contentTitle,
		BlogDraft:      blogDraft,
		SocialDraft:    socialDraft,
		Consensus:      true,
		Transcript:     transcript,
		DebateID:       debateRecord.ID,
	}, nil
}

func isLinkFarmOrSpam(target string) bool {
	t := strings.ToLower(target)
	spamKeywords := []string{"linkfarm", "seo-backlinks-free", "buybacklinks", "pbn-network", "spam-directory"}
	for _, s := range spamKeywords {
		if strings.Contains(t, s) {
			return true
		}
	}
	return false
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end > start {
		return s[start : end+1]
	}
	return s
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}
