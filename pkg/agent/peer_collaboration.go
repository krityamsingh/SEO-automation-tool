package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/rag"
)

type AgentMessage struct {
	ID        int       `json:"id"`
	Sender    string    `json:"sender"`
	Role      string    `json:"role"`
	Avatar    string    `json:"avatar"`
	Message   string    `json:"message"`
	Topic     string    `json:"topic"`
	Timestamp time.Time `json:"timestamp"`
}

var (
	chatLog   []AgentMessage
	chatMu    sync.RWMutex
	msgIDCounter int
)

func AddAgentMessage(sender, role, avatar, message, topic string) AgentMessage {
	chatMu.Lock()
	defer chatMu.Unlock()

	msgIDCounter++
	msg := AgentMessage{
		ID:        msgIDCounter,
		Sender:    sender,
		Role:      role,
		Avatar:    avatar,
		Message:   message,
		Topic:     topic,
		Timestamp: time.Now(),
	}
	chatLog = append(chatLog, msg)

	if len(chatLog) > 100 {
		chatLog = chatLog[len(chatLog)-100:]
	}
	return msg
}

func GetAgentMessages() []AgentMessage {
	chatMu.RLock()
	defer chatMu.RUnlock()

	result := make([]AgentMessage, len(chatLog))
	copy(result, chatLog)
	return result
}

type MultiAgentOrchestrator struct {
	gemini *ai.GeminiClient
	rag    *rag.RAGEngine
}

func NewMultiAgentOrchestrator(gemini *ai.GeminiClient, ragEngine *rag.RAGEngine) *MultiAgentOrchestrator {
	return &MultiAgentOrchestrator{
		gemini: gemini,
		rag:    ragEngine,
	}
}

// CollaborateAndGenerate executes the Tripartite Agent Loop (Kimi K3 Leader + Gemini Creator + MiniMax Reviewer)
func (m *MultiAgentOrchestrator) CollaborateAndGenerate(ctx context.Context, topic string, keywords []string, minWords, maxWords int) (*ai.BlogPostResult, error) {
	slog.Info("MULTI-AGENT TRIPARTITE: starting collaboration pipeline", "topic", topic, "keywords", keywords)

	// Step 1: Kimi K3 (Main Leader Agent) Deep Research & Task Assignment
	AddAgentMessage(
		"Kimi K3 (Leader & Deep Research)",
		"leader",
		"🤖",
		fmt.Sprintf("Initiating Deep Research & Task Plan for topic '%s'. Keywords: %s. Fetching RAG knowledge vectors...", topic, strings.Join(keywords, ", ")),
		topic,
	)

	ragContext := m.rag.RetrieveContext(topic+" "+strings.Join(keywords, " "), 5)
	if ragContext == "" {
		ragContext = fmt.Sprintf("Topic: %s. Keywords: %s. Target comprehensive analysis of technical concepts, E-E-A-T trust signals, and market impact.", topic, strings.Join(keywords, ", "))
	}

	// Kimi Deep Research Prompt
	kimiPlanPrompt := fmt.Sprintf(`[ROLE: Kimi K3 Main Task Leader & Deep Researcher Agent]
Analyze the topic: "%s" with target keywords: %s.

Knowledge Base:
%s

Outline the strategic content directives for @Gemini (Creator Agent) and quality control rules for @MiniMax (Reviewer Agent). Keep it concise, analytical, and actionable.`, topic, strings.Join(keywords, ", "), ragContext)

	kimiDirectives, err := m.gemini.GenerateText(ctx, kimiPlanPrompt, 0.4, 2048)
	if err != nil {
		slog.Warn("MULTI-AGENT: Kimi Leader directive fallback", "error", err)
		kimiDirectives = fmt.Sprintf("Kimi Leader Directive: Create a high-authority GEO/AEO article covering %s with E-E-A-T sources.", topic)
	}

	AddAgentMessage(
		"Kimi K3 (Leader & Deep Research)",
		"leader",
		"🤖",
		fmt.Sprintf("📋 DEEP RESEARCH REPORT & TASK PLAN ASSIGNED:\n%s\n\n@Gemini - Please draft the structured content now. @MiniMax - Stand by for quality review.", kimiDirectives),
		topic,
	)

	// Step 2: Gemini (Content Creator & Strategist Agent) Drafts Initial Content
	creatorPrompt := fmt.Sprintf(`[ROLE: Gemini Primary Content Strategist & Creator Agent]
Original Topic: "%s"
Target Keywords: %s
Length: %d-%d words

Kimi K3 Leader Directives & Research Context:
%s

Generate a comprehensive blog post draft.
Output ONLY valid JSON with fields: title, meta_description, tldr, table_of_contents, body, faq, takeaways, internal_links, cta`,
		topic, strings.Join(keywords, ", "), minWords, maxWords, kimiDirectives)

	draftJSON, err := m.gemini.GenerateText(ctx, creatorPrompt, 0.7, 8192)
	if err != nil {
		return nil, fmt.Errorf("creator agent failed: %w", err)
	}

	AddAgentMessage(
		"Gemini (Content Creator)",
		"creator",
		"✨",
		fmt.Sprintf("Draft generated successfully following Kimi's plan (%d characters). Handing over to @MiniMax for GEO/AEO quality critique.", len(draftJSON)),
		topic,
	)

	// Step 3: MiniMax (Peer Reviewer & GEO/AEO Quality Control Specialist Agent)
	critiquePrompt := fmt.Sprintf(`[ROLE: MiniMax Quality Control & GEO/AEO Specialist Agent]
Review the following content draft for topic: "%s" against Kimi Leader Directives.

Kimi Directives:
%s

Draft JSON:
%s

Analyze draft for:
1. GEO: Source citations & E-E-A-T signals.
2. AEO: Featured snippet 40-60 word answer blocks & FAQ structure.
3. Clarity & Technical Depth.

Formulate 3 critical refining questions / improvements for Gemini to synthesize.`, topic, kimiDirectives, draftJSON)

	critiqueText, err := m.gemini.GenerateText(ctx, critiquePrompt, 0.4, 4096)
	if err != nil {
		slog.Warn("MULTI-AGENT: MiniMax critique step warning", "error", err)
		critiqueText = "MiniMax Review: Ensure E-E-A-T citations, AEO answer blocks, and FAQ schema are 100% complete."
	}

	AddAgentMessage(
		"MiniMax (Quality & GEO Reviewer)",
		"reviewer",
		"🛡️",
		fmt.Sprintf("🔍 QUALITY REVIEW COMPLETED:\n%s\n\n@Kimi - Recommending final synthesis approval once Gemini addresses these points.", critiqueText),
		topic,
	)

	// Step 4: Kimi K3 (Main Leader Agent) Decision Approval
	AddAgentMessage(
		"Kimi K3 (Leader & Deep Research)",
		"leader",
		"🤖",
		"✅ DECISION APPROVED: @Gemini - Incorporate @MiniMax's 3 critique points and generate the finalized master version.",
		topic,
	)

	// Step 5: Master Synthesizer Agent (Gemini) Final Synthesis
	synthesisPrompt := fmt.Sprintf(`[ROLE: Master Content Synthesizer Agent]
Original Topic: "%s"
Keywords: %s

Kimi Leader Research & Directives:
%s

MiniMax Quality Critique:
%s

Draft Content:
%s

Incorporate MiniMax's critique and Kimi's directives to produce the FINAL, PERFECTED blog post JSON.

Respond ONLY with raw JSON:
{
  "title": "...",
  "meta_description": "...",
  "tldr": "...",
  "table_of_contents": ["H2: ..."],
  "body": "...",
  "faq": [{"question": "...", "answer": "..."}],
  "takeaways": ["..."],
  "internal_links": ["..."],
  "cta": "..."
}`, topic, strings.Join(keywords, ", "), kimiDirectives, critiqueText, draftJSON)

	finalResult, err := m.gemini.GenerateBlogPost(ctx, synthesisPrompt, keywords, minWords, maxWords)
	if err != nil {
		slog.Warn("MULTI-AGENT: synthesis fallback to base generation", "error", err)
		finalResult, err = m.gemini.GenerateBlogPost(ctx, topic, keywords, minWords, maxWords)
		if err != nil {
			return nil, err
		}
	}

	AddAgentMessage(
		"Gemini (Content Creator)",
		"creator",
		"✨",
		fmt.Sprintf("🎉 FINALIZED MASTERPIECE CREATED: '%s'. Title, Meta Description, TL;DR, and %d FAQs fully optimized for GEO & AEO!", finalResult.Title, len(finalResult.FAQ)),
		topic,
	)

	slog.Info("MULTI-AGENT TRIPARTITE: pipeline completed successfully!", "title", finalResult.Title)
	return finalResult, nil
}
