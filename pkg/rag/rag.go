package rag

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"aeo_geo_seo_agent/pkg/crawler"
)

type DocumentChunk struct {
	ID         string            `json:"id"`
	SourceURL  string            `json:"source_url"`
	Title      string            `json:"title"`
	Content    string            `json:"content"`
	Category   string            `json:"category"` // debate, research, outcome, chat, audit
	Keyword    string            `json:"keyword"`
	TargetSite string            `json:"target_site"`
	Metadata   map[string]string `json:"metadata"`
	Words      []string          `json:"-"`
	CreatedAt  time.Time         `json:"created_at"`
}

type RAGEngine struct {
	crawler   *crawler.Crawler
	chunks    []DocumentChunk
	nextID    uint64
	mu        sync.RWMutex
}

func New(cr *crawler.Crawler) *RAGEngine {
	return &RAGEngine{
		crawler: cr,
		chunks:  make([]DocumentChunk, 0),
	}
}

// IngestWithMetadata stores chunk with tags for filtering & duplicate prevention
func (r *RAGEngine) IngestWithMetadata(source, title, content, category, keyword, targetSite string, metadata map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	chunk := DocumentChunk{
		ID:         fmt.Sprintf("%s-%d", category, r.nextID),
		SourceURL:  source,
		Title:      title,
		Content:    content,
		Category:   category,
		Keyword:    strings.ToLower(keyword),
		TargetSite: strings.ToLower(targetSite),
		Metadata:   metadata,
		Words:      tokenize(content + " " + title + " " + keyword + " " + targetSite),
		CreatedAt:  time.Now(),
	}
	r.nextID++
	r.chunks = append(r.chunks, chunk)
	slog.Info("RAG: ingested item with metadata", "category", category, "keyword", keyword, "target", targetSite)
}

// CheckDuplicateTargetOrKeyword returns true if a target backlink site or keyword is already in the RAG memory
func (r *RAGEngine) CheckDuplicateTargetOrKeyword(keyword, targetSite string) (bool, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	kwLower := strings.ToLower(strings.TrimSpace(keyword))
	targetLower := strings.ToLower(strings.TrimSpace(targetSite))

	for _, chunk := range r.chunks {
		if kwLower != "" && strings.EqualFold(chunk.Keyword, kwLower) {
			return true, fmt.Sprintf("Keyword '%s' was already researched/assigned in RAG entry: %s", keyword, chunk.Title)
		}
		if targetLower != "" && chunk.TargetSite != "" && strings.Contains(strings.ToLower(chunk.TargetSite), targetLower) {
			return true, fmt.Sprintf("Backlink target '%s' was already targeted/used in RAG entry: %s", targetSite, chunk.Title)
		}
	}
	return false, ""
}

// IngestDebateTranscript indexes an agent-to-agent debate into system-wide shared RAG memory
func (r *RAGEngine) IngestDebateTranscript(taskID uint, keyword, targetSite, transcriptJSON, finalDecision string) {
	summary := fmt.Sprintf("Agent Debate for Task #%d (Keyword: %s, Target: %s)\nFinal Decision: %s\nTranscript:\n%s",
		taskID, keyword, targetSite, finalDecision, transcriptJSON)
	r.IngestWithMetadata(
		fmt.Sprintf("debate-task-%d", taskID),
		fmt.Sprintf("Agent Debate: %s on %s", keyword, targetSite),
		summary,
		"debate",
		keyword,
		targetSite,
		map[string]string{"task_id": fmt.Sprintf("%d", taskID)},
	)
}

// IngestOutcomeResult indexes rank movement & traffic performance into RAG self-improvement memory
func (r *RAGEngine) IngestOutcomeResult(taskID uint, keyword, targetSite string, rankCurrent, rankPrev int, notes string) {
	summary := fmt.Sprintf("SEO Rank Outcome for Task #%d (Keyword: %s, Target: %s)\nPrevious Rank: %d -> Current Rank: %d\nNotes: %s",
		taskID, keyword, targetSite, rankPrev, rankCurrent, notes)
	r.IngestWithMetadata(
		fmt.Sprintf("outcome-task-%d", taskID),
		fmt.Sprintf("Rank Outcome: %s (Position %d)", keyword, rankCurrent),
		summary,
		"outcome",
		keyword,
		targetSite,
		map[string]string{
			"task_id":      fmt.Sprintf("%d", taskID),
			"rank_current": fmt.Sprintf("%d", rankCurrent),
			"rank_prev":    fmt.Sprintf("%d", rankPrev),
		},
	)
}

// IngestTaskContent indexes generated full article draft and step-by-step guide into system RAG memory under category "task_content"
func (r *RAGEngine) IngestTaskContent(taskID uint, title, articleDraft, executionGuide string) error {
	if strings.TrimSpace(articleDraft) == "" && strings.TrimSpace(executionGuide) == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	taskTag := fmt.Sprintf("task-content-%d", taskID)
	combinedText := fmt.Sprintf("TASK TITLE: %s\n\n=== FULL ARTICLE DRAFT ===\n%s\n\n=== EXECUTION GUIDE ===\n%s", title, articleDraft, executionGuide)

	paragraphs := strings.Split(combinedText, "\n")
	var currentChunk strings.Builder
	wordCount := 0
	count := 0

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if len(p) == 0 {
			continue
		}

		words := strings.Fields(p)
		if wordCount+len(words) > 300 && currentChunk.Len() > 0 {
			chunkText := currentChunk.String()
			chunk := DocumentChunk{
				ID:         fmt.Sprintf("%s-%d", taskTag, count),
				SourceURL:  fmt.Sprintf("task://%d", taskID),
				Title:      fmt.Sprintf("Task #%d Content: %s", taskID, title),
				Content:    chunkText,
				Category:   "task_content",
				Keyword:    strings.ToLower(title),
				TargetSite: "",
				Metadata: map[string]string{
					"task_id": fmt.Sprintf("%d", taskID),
					"type":    "task_content",
				},
				Words:     tokenize(chunkText + " " + title),
				CreatedAt: time.Now(),
			}
			r.chunks = append(r.chunks, chunk)
			count++
			currentChunk.Reset()
			wordCount = 0
		}

		currentChunk.WriteString(p)
		currentChunk.WriteString("\n")
		wordCount += len(words)
	}

	if currentChunk.Len() > 0 {
		chunkText := currentChunk.String()
		chunk := DocumentChunk{
			ID:         fmt.Sprintf("%s-%d", taskTag, count),
			SourceURL:  fmt.Sprintf("task://%d", taskID),
			Title:      fmt.Sprintf("Task #%d Content: %s", taskID, title),
			Content:    chunkText,
			Category:   "task_content",
			Keyword:    strings.ToLower(title),
			TargetSite: "",
			Metadata: map[string]string{
				"task_id": fmt.Sprintf("%d", taskID),
				"type":    "task_content",
			},
			Words:     tokenize(chunkText + " " + title),
			CreatedAt: time.Now(),
		}
		r.chunks = append(r.chunks, chunk)
		count++
	}

	r.nextID += uint64(count)
	slog.Info("RAG: ingested task content chunks", "task_id", taskID, "chunks_added", count, "category", "task_content")
	return nil
}

// IngestURL crawls a URL and stores semantic chunks in the RAG store
func (r *RAGEngine) IngestURL(ctx context.Context, targetURL string) (int, error) {
	slog.Info("RAG: ingesting knowledge from URL", "url", targetURL)

	if r.crawler == nil {
		return 0, fmt.Errorf("RAG crawler is nil")
	}

	pages, err := r.crawler.Crawl(targetURL, 1, 10)
	if err != nil {
		return 0, fmt.Errorf("RAG crawl failed for %s: %w", targetURL, err)
	}

	count := 0
	r.mu.Lock()
	defer r.mu.Unlock()

	for pageURL, page := range pages {
		if page.Text == "" {
			continue
		}

		// Split text into semantic chunks of ~300 words
		paragraphs := strings.Split(page.Text, "\n")
		var currentChunk strings.Builder
		wordCount := 0

		for _, p := range paragraphs {
			p = strings.TrimSpace(p)
			if len(p) == 0 {
				continue
			}

			words := strings.Fields(p)
			if wordCount+len(words) > 300 && currentChunk.Len() > 0 {
				chunkText := currentChunk.String()
				chunk := DocumentChunk{
					ID:        fmt.Sprintf("%s-%d", pageURL, count),
					SourceURL: pageURL,
					Title:     page.Title,
					Content:   chunkText,
					Words:     tokenize(chunkText),
					CreatedAt: time.Now(),
				}
				r.chunks = append(r.chunks, chunk)
				count++

				currentChunk.Reset()
				wordCount = 0
			}

			currentChunk.WriteString(p)
			currentChunk.WriteString("\n")
			wordCount += len(words)
		}

		if currentChunk.Len() > 0 {
			chunkText := currentChunk.String()
			chunk := DocumentChunk{
				ID:        fmt.Sprintf("%s-%d", pageURL, count),
				SourceURL: pageURL,
				Title:     page.Title,
				Content:   chunkText,
				Words:     tokenize(chunkText),
				CreatedAt: time.Now(),
			}
			r.chunks = append(r.chunks, chunk)
			count++
		}
	}

	slog.Info("RAG: ingested knowledge chunks", "url", targetURL, "chunks_added", count, "total_chunks", len(r.chunks))
	return count, nil
}

// IngestText manually adds factual text content to RAG store
func (r *RAGEngine) IngestText(sourceName, title, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	paragraphs := strings.Split(text, "\n")
	var currentChunk strings.Builder
	wordCount := 0
	count := 0

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if len(p) == 0 {
			continue
		}

		words := strings.Fields(p)
		if wordCount+len(words) > 300 && currentChunk.Len() > 0 {
			chunkText := currentChunk.String()
			r.chunks = append(r.chunks, DocumentChunk{
				ID:        fmt.Sprintf("%s-%d", sourceName, count),
				SourceURL: sourceName,
				Title:     title,
				Content:   chunkText,
				Words:     tokenize(chunkText),
				CreatedAt: time.Now(),
			})
			count++
			currentChunk.Reset()
			wordCount = 0
		}

		currentChunk.WriteString(p)
		currentChunk.WriteString("\n")
		wordCount += len(words)
	}

	if currentChunk.Len() > 0 {
		chunkText := currentChunk.String()
		r.chunks = append(r.chunks, DocumentChunk{
			ID:        fmt.Sprintf("%s-%d", sourceName, count),
			SourceURL: sourceName,
			Title:     title,
			Content:   chunkText,
			Words:     tokenize(chunkText),
			CreatedAt: time.Now(),
		})
	}
}

// RetrieveContext searches RAG store for relevant passages matching the query
func (r *RAGEngine) RetrieveContext(query string, topK int) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.chunks) == 0 {
		return ""
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return ""
	}

	type scoredChunk struct {
		chunk DocumentChunk
		score float64
	}

	scored := make([]scoredChunk, 0, len(r.chunks))

	for _, chunk := range r.chunks {
		s := scoreBM25Like(queryTokens, chunk.Words)
		if s > 0 {
			scored = append(scored, scoredChunk{chunk: chunk, score: s})
		}
	}

	// Sort scored chunks descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	if len(scored) == 0 {
		return ""
	}

	limit := topK
	if limit > len(scored) {
		limit = len(scored)
	}

	var sb strings.Builder
	sb.WriteString("--- RETRIEVED RAG KNOWLEDGE CONTEXT ---\n")
	for i := 0; i < limit; i++ {
		c := scored[i].chunk
		sb.WriteString(fmt.Sprintf("[Source: %s | Title: %s]\n%s\n\n", c.SourceURL, c.Title, c.Content))
	}
	sb.WriteString("--- END RAG CONTEXT ---\n")

	return sb.String()
}

func tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	tokens := make([]string, 0, len(words))
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "in": true, "on": true, "at": true, "of": true, "to": true, "is": true, "are": true,
		"and": true, "or": true, "for": true, "with": true, "by": true, "this": true, "that": true, "it": true, "as": true,
	}

	for _, w := range words {
		w = strings.Trim(w, ".,!?;:'\"()[]{}\\/|<>-=_+@#$%^&*")
		if len(w) > 2 && !stopWords[w] {
			tokens = append(tokens, w)
		}
	}
	return tokens
}

func scoreBM25Like(queryTokens, docTokens []string) float64 {
	docFreq := make(map[string]int)
	for _, w := range docTokens {
		docFreq[w]++
	}

	score := 0.0
	for _, q := range queryTokens {
		if freq, ok := docFreq[q]; ok {
			// Frequency score with diminishing returns (log scaling)
			score += 1.0 + math.Log(float64(freq))
		}
	}
	return score
}

// GetStats returns a summary of the RAG memory contents for the dashboard
func (r *RAGEngine) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categoryCounts := make(map[string]int)
	for _, c := range r.chunks {
		cat := c.Category
		if cat == "" {
			cat = "general"
		}
		categoryCounts[cat]++
	}

	return map[string]interface{}{
		"total_chunks":    len(r.chunks),
		"category_counts": categoryCounts,
		"status":          "active",
		"store_type":      "in-memory BM25/TF-IDF",
	}
}

// Size returns the number of stored document chunks
func (r *RAGEngine) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.chunks)
}
