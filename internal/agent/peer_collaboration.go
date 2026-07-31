package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"aeo_geo_seo_agent/internal/ai"
	"aeo_geo_seo_agent/internal/rag"
)

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

// CollaborateAndGenerate executes a multi-agent peer dialogue loop (Gemini + MiniMax + RAG)
func (m *MultiAgentOrchestrator) CollaborateAndGenerate(ctx context.Context, topic string, keywords []string, minWords, maxWords int) (*ai.BlogPostResult, error) {
	slog.Info("MULTI-AGENT: starting peer collaboration pipeline", "topic", topic, "keywords", keywords)

	// Step 1: Retrieve RAG Knowledge Context
	ragContext := m.rag.RetrieveContext(topic+" "+strings.Join(keywords, " "), 5)
	if ragContext != "" {
		slog.Info("MULTI-AGENT: RAG context retrieved for prompt augmentation")
	}

	// Step 2: Agent 1 (Gemini - Creator) drafts initial content using RAG context
	creatorPrompt := fmt.Sprintf(`[ROLE: Primary Content Strategy & Creation Agent]
Topic: "%s"
Target Keywords: %s
Length: %d-%d words

%s

Generate a comprehensive blog post draft.
Output ONLY valid JSON with fields: title, meta_description, tldr, table_of_contents, body, faq, takeaways, internal_links, cta`,
		topic, strings.Join(keywords, ", "), minWords, maxWords, ragContext)

	draftJSON, err := m.gemini.GenerateText(ctx, creatorPrompt, 0.7, 8192)
	if err != nil {
		return nil, fmt.Errorf("creator agent failed: %w", err)
	}

	// Step 3: Agent 2 (Peer Reviewer - MiniMax/Secondary Model) Critiques and asks 3 refining questions
	critiquePrompt := fmt.Sprintf(`[ROLE: Peer Reviewer & Quality Control Specialist Agent]
Review the following content draft generated for topic: "%s".

Draft JSON:
%s

Analyze the draft for:
1. Generative Engine Optimization (GEO): Are source citations, E-E-A-T trust signals, and clear entity definitions present?
2. Answer Engine Optimization (AEO): Is the TL;DR and FAQ section formatted to win Google Featured Snippets?
3. Technical Depth & Readability.

Ask 3 critical refining questions or points of improvement to enhance this draft.`, topic, draftJSON)

	critiqueText, err := m.gemini.GenerateText(ctx, critiquePrompt, 0.4, 4096)
	if err != nil {
		slog.Warn("MULTI-AGENT: critique step warning", "error", err)
		// Proceed with initial draft if critique fails
		return m.gemini.GenerateBlogPost(ctx, topic, keywords, minWords, maxWords)
	}

	slog.Info("MULTI-AGENT: Peer Reviewer critique completed", "critique_summary", truncateString(critiqueText, 150))

	// Step 4: Agent 1 (Gemini - Master Synthesizer) answers critique questions and produces final optimized version
	synthesisPrompt := fmt.Sprintf(`[ROLE: Master Content Synthesizer Agent]
Original Topic: "%s"
Target Keywords: %s

RAG Knowledge Context:
%s

Peer Reviewer Critique & Questions:
%s

Draft Content:
%s

Incorporate the Peer Reviewer's feedback, answer the 3 critique points in the content body, and produce the FINAL, PERFECTED blog post JSON.

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
}`, topic, strings.Join(keywords, ", "), ragContext, critiqueText, draftJSON)

	finalResult, err := m.gemini.GenerateBlogPost(ctx, synthesisPrompt, keywords, minWords, maxWords)
	if err != nil {
		slog.Warn("MULTI-AGENT: synthesis parsing failed, returning initial draft", "error", err)
		return m.gemini.GenerateBlogPost(ctx, topic, keywords, minWords, maxWords)
	}

	slog.Info("MULTI-AGENT: peer collaboration completed successfully!", "title", finalResult.Title)
	return finalResult, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
