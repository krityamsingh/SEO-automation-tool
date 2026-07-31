package rag

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"aeo_geo_seo_agent/internal/crawler"
)

type DocumentChunk struct {
	ID        string    `json:"id"`
	SourceURL string    `json:"source_url"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Words     []string  `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type RAGEngine struct {
	crawler *crawler.Crawler
	chunks  []DocumentChunk
	mu      sync.RWMutex
}

func New(cr *crawler.Crawler) *RAGEngine {
	return &RAGEngine{
		crawler: cr,
		chunks:  make([]DocumentChunk, 0),
	}
}

// IngestURL crawls a URL and stores semantic chunks in the RAG store
func (r *RAGEngine) IngestURL(ctx context.Context, targetURL string) (int, error) {
	slog.Info("RAG: ingesting knowledge from URL", "url", targetURL)

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
