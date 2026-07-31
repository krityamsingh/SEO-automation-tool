package seo

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/crawler"
	"aeo_geo_seo_agent/pkg/database"
)

type Engine struct {
	gemini  *ai.GeminiClient
	crawler *crawler.Crawler
	db      *gorm.DB
}

func New(gemini *ai.GeminiClient, crawler *crawler.Crawler, db *gorm.DB) *Engine {
	return &Engine{
		gemini:  gemini,
		crawler: crawler,
		db:      db,
	}
}

func (e *Engine) OnPageAudit(ctx context.Context, targetURL string) (*OnPageResult, error) {
	slog.Info("starting on-page audit", "url", targetURL)
	
	pages, err := e.crawler.Crawl(targetURL, 2, 50)
	if err != nil {
		return nil, fmt.Errorf("crawl failed: %w", err)
	}
	
	result := &OnPageResult{
		URL:        targetURL,
		CrawledAt:  time.Now(),
		PageCount:  len(pages),
		Pages:      make([]PageAudit, 0, len(pages)),
		Issues:     make([]Issue, 0),
		Score:      0,
	}
	
	for url, page := range pages {
		audit := e.auditPage(page)
		audit.URL = url
		result.Pages = append(result.Pages, audit)
		result.Issues = append(result.Issues, audit.Issues...)
	}
	
	result.Score = e.calculateScore(result)
	
	database.LogAgentActivity(e.db, "onpage_audit", "success", fmt.Sprintf("audited %d pages, score %d", result.PageCount, result.Score))
	return result, nil
}

func (e *Engine) auditPage(page *crawler.PageData) PageAudit {
	audit := PageAudit{
		Title:       page.Title,
		Description: page.Description,
		Issues:      make([]Issue, 0),
		Score:       100,
	}
	
	// Title checks
	if len(page.Title) < 30 {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "warning",
			Type:     "title",
			Message:  fmt.Sprintf("Title too short (%d chars), recommended 50-60", len(page.Title)),
		})
		audit.Score -= 5
	}
	if len(page.Title) > 70 {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "warning",
			Type:     "title",
			Message:  fmt.Sprintf("Title too long (%d chars), recommended 50-60", len(page.Title)),
		})
		audit.Score -= 5
	}
	if page.Title == "" {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "error",
			Type:     "title",
			Message:  "Missing title tag",
		})
		audit.Score -= 20
	}
	
	// Description checks
	if page.Description == "" {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "warning",
			Type:     "meta_description",
			Message:  "Missing meta description",
		})
		audit.Score -= 10
	}
	if len(page.Description) > 160 {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "warning",
			Type:     "meta_description",
			Message:  fmt.Sprintf("Meta description too long (%d chars)", len(page.Description)),
		})
		audit.Score -= 5
	}
	
	// Heading checks
	h1Count := 0
	for _, h := range page.Headings {
		if strings.HasPrefix(h, "h1: ") || strings.HasPrefix(h, "H1: ") {
			h1Count++
		}
	}
	if h1Count == 0 {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "error",
			Type:     "headings",
			Message:  "Missing H1 tag",
		})
		audit.Score -= 15
	}
	if h1Count > 1 {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "warning",
			Type:     "headings",
			Message:  fmt.Sprintf("Multiple H1 tags found (%d)", h1Count),
		})
		audit.Score -= 5
	}
	
	// Image alt text
	missingAlt := 0
	for _, img := range page.Images {
		if img.Alt == "" {
			missingAlt++
		}
	}
	if missingAlt > 0 {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "warning",
			Type:     "images",
			Message:  fmt.Sprintf("%d images missing alt text", missingAlt),
		})
		audit.Score -= min(missingAlt*2, 10)
	}
	
	// Word count
	wordCount := len(strings.Fields(page.Text))
	if wordCount < 300 {
		audit.Issues = append(audit.Issues, Issue{
			Severity: "warning",
			Type:     "content",
			Message:  fmt.Sprintf("Low word count (%d), recommended 500+ for SEO", wordCount),
		})
		audit.Score -= 10
	}
	
	// Ensure score is non-negative
	if audit.Score < 0 {
		audit.Score = 0
	}
	
	return audit
}

func (e *Engine) calculateScore(result *OnPageResult) int {
	if len(result.Pages) == 0 {
		return 0
	}
	
	total := 0
	for _, p := range result.Pages {
		total += p.Score
	}
	return total / len(result.Pages)
}

func (e *Engine) KeywordAnalysis(ctx context.Context, content string) (*KeywordResult, error) {
	// Extract keywords from content
	words := strings.Fields(strings.ToLower(content))
	wordFreq := make(map[string]int)
	
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true, "have": true, "has": true, "had": true, "do": true,
		"does": true, "did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "can": true, "shall": true, "this": true,
		"that": true, "these": true, "those": true, "i": true, "you": true, "he": true, "she": true,
		"it": true, "we": true, "they": true, "me": true, "him": true, "her": true, "us": true,
		"them": true, "my": true, "your": true, "his": true, "hers": true, "its": true, "our": true,
		"their": true, "and": true, "but": true, "or": true, "nor": true, "for": true, "yet": true,
		"so": true, "if": true, "then": true, "than": true, "when": true, "where": true, "why": true,
		"how": true, "what": true, "which": true, "who": true, "whom": true, "whose": true, "of": true,
		"to": true, "in": true, "on": true, "at": true, "by": true, "with": true, "from": true,
		"up": true, "about": true, "into": true, "through": true, "during": true, "before": true,
		"after": true, "above": true, "below": true, "between": true, "among": true, "within": true,
	}
	
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:'\"()[]{}\\/|<>-=_+@#$%^&*")
		if len(word) > 3 && !stopWords[word] {
			wordFreq[word]++
		}
	}
	
	// Sort by frequency
	type kv struct {
		Word  string
		Count int
	}
	var kvs []kv
	for k, v := range wordFreq {
		kvs = append(kvs, kv{k, v})
	}
	
	// Simple sort (bubble sort for small data)
	for i := 0; i < len(kvs); i++ {
		for j := i + 1; j < len(kvs); j++ {
			if kvs[j].Count > kvs[i].Count {
				kvs[i], kvs[j] = kvs[j], kvs[i]
			}
		}
	}
	
	keywords := make([]KeywordData, 0, min(20, len(kvs)))
	for i := 0; i < min(20, len(kvs)); i++ {
		density := float64(kvs[i].Count) / float64(len(words)) * 100
		keywords = append(keywords, KeywordData{
			Word:     kvs[i].Word,
			Count:    kvs[i].Count,
			Density:  density,
		})
	}
	
	return &KeywordResult{
		Keywords: keywords,
		TotalWords: len(words),
	}, nil
}

func (e *Engine) CompetitorAnalysis(ctx context.Context, domain string, competitors []string) (*CompetitorResult, error) {
	result := &CompetitorResult{
		Target:       domain,
		Competitors:  make(map[string]CompetitorData),
		Opportunities: make([]string, 0),
	}
	
	// Crawl target domain
	targetPages, err := e.crawler.Crawl("https://"+domain, 1, 20)
	if err != nil {
		slog.Warn("failed to crawl target domain", "domain", domain, "error", err)
	}
	
	result.TargetPages = len(targetPages)
	
	// Crawl competitors
	for _, comp := range competitors {
		compPages, err := e.crawler.Crawl("https://"+comp, 1, 20)
		if err != nil {
			slog.Warn("failed to crawl competitor", "domain", comp, "error", err)
			continue
		}
		
		result.Competitors[comp] = CompetitorData{
			Pages: len(compPages),
		}
		
		if len(compPages) > len(targetPages) {
			result.Opportunities = append(result.Opportunities, 
				fmt.Sprintf("%s has more pages (%d vs %d) - consider expanding content", comp, len(compPages), len(targetPages)))
		}
	}
	
	return result, nil
}

// Result types

type OnPageResult struct {
	URL       string      `json:"url"`
	CrawledAt time.Time   `json:"crawled_at"`
	PageCount int         `json:"page_count"`
	Pages     []PageAudit `json:"pages"`
	Issues    []Issue     `json:"issues"`
	Score     int         `json:"score"`
}

type PageAudit struct {
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Score       int     `json:"score"`
	Issues      []Issue `json:"issues"`
}

type Issue struct {
	Severity string `json:"severity"` // error, warning, info
	Type     string `json:"type"`     // title, meta_description, headings, images, content
	Message  string `json:"message"`
}

type KeywordResult struct {
	Keywords   []KeywordData `json:"keywords"`
	TotalWords int           `json:"total_words"`
}

type KeywordData struct {
	Word    string  `json:"word"`
	Count   int     `json:"count"`
	Density float64 `json:"density"`
}

type CompetitorResult struct {
	Target        string                   `json:"target"`
	TargetPages   int                      `json:"target_pages"`
	Competitors   map[string]CompetitorData `json:"competitors"`
	Opportunities []string                 `json:"opportunities"`
}

type CompetitorData struct {
	Pages int `json:"pages"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
