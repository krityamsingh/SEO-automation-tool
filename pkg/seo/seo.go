package seo

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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

type KeywordFrequency struct {
	Keyword string  `json:"keyword"`
	Count   int     `json:"count"`
	Density float64 `json:"density"`
}

type AuditReport struct {
	URL                string                `json:"url"`
	StatusCode         int                   `json:"status_code"`
	Title              string                `json:"title"`
	Description        string                `json:"description"`
	Canonical          string                `json:"canonical"`
	OGTags             map[string]string     `json:"og_tags"`
	HeadingHierarchy   []crawler.HeadingNode `json:"heading_hierarchy"`
	HeadingIssues      []string              `json:"heading_issues"`
	KeywordDensity     map[string]float64    `json:"keyword_density"`
	TopKeywords        []KeywordFrequency    `json:"top_keywords"`
	BrokenLinks        []crawler.LinkInfo    `json:"broken_links"`
	InternalLinksCount int                   `json:"internal_links_count"`
	ExternalLinksCount int                   `json:"external_links_count"`
	OverallSEOScore    int                   `json:"overall_seo_score"`
	CrawledAt          time.Time             `json:"crawled_at,omitempty"`
	Issues             []Issue               `json:"issues,omitempty"`
}

func (e *Engine) OnPageAudit(ctx context.Context, targetURL string) (*AuditReport, error) {
	slog.Info("starting on-page audit", "url", targetURL)

	pageData, err := e.crawler.CrawlURL(ctx, targetURL)
	if err != nil {
		return nil, fmt.Errorf("crawl failed: %w", err)
	}

	checkedLinks := e.crawler.CheckLinksConcurrently(ctx, pageData.Links)
	pageData.Links = checkedLinks

	headingIssues := evaluateHeadingHierarchy(pageData.Headings)

	internalCount := 0
	externalCount := 0
	brokenLinks := make([]crawler.LinkInfo, 0)

	for _, link := range checkedLinks {
		if link.IsExternal {
			externalCount++
		} else {
			internalCount++
		}
		if link.IsBroken {
			brokenLinks = append(brokenLinks, link)
		}
	}

	topKeywords := calculateTopKeywords(pageData.KeywordDensity, pageData.WordCount)
	score, issues := calculateOverallSEOScore(pageData, headingIssues, brokenLinks)

	report := &AuditReport{
		URL:                pageData.URL,
		StatusCode:         pageData.StatusCode,
		Title:              pageData.Title,
		Description:        pageData.Description,
		Canonical:          pageData.Canonical,
		OGTags:             pageData.OGTags,
		HeadingHierarchy:   pageData.Headings,
		HeadingIssues:      headingIssues,
		KeywordDensity:     pageData.KeywordDensity,
		TopKeywords:        topKeywords,
		BrokenLinks:        brokenLinks,
		InternalLinksCount: internalCount,
		ExternalLinksCount: externalCount,
		OverallSEOScore:    score,
		CrawledAt:          time.Now(),
		Issues:             issues,
	}

	if e.db != nil {
		database.LogAgentActivity(e.db, "onpage_audit", "success", fmt.Sprintf("audited %s, score %d", targetURL, score))
	}

	return report, nil
}

func evaluateHeadingHierarchy(headings []crawler.HeadingNode) []string {
	var issues []string
	h1Count := 0

	for _, h := range headings {
		tag := strings.ToLower(h.Tag)
		if tag == "h1" {
			h1Count++
		}
		if strings.TrimSpace(h.Text) == "" {
			issues = append(issues, fmt.Sprintf("Empty %s tag found", strings.ToUpper(tag)))
		}
	}

	if h1Count == 0 {
		issues = append(issues, "Missing H1 tag")
	} else if h1Count > 1 {
		issues = append(issues, fmt.Sprintf("Multiple H1 tags found (%d)", h1Count))
	}

	for i := 1; i < len(headings); i++ {
		prevLevel := getHeadingLevel(headings[i-1].Tag)
		currLevel := getHeadingLevel(headings[i].Tag)

		if prevLevel > 0 && currLevel > 0 {
			if currLevel > prevLevel+1 {
				issues = append(issues, fmt.Sprintf("Skipped heading level: %s to %s", strings.ToUpper(headings[i-1].Tag), strings.ToUpper(headings[i].Tag)))
			}
		}
	}

	return issues
}

func getHeadingLevel(tag string) int {
	tag = strings.ToLower(tag)
	if len(tag) == 2 && tag[0] == 'h' {
		if tag[1] >= '1' && tag[1] <= '6' {
			return int(tag[1] - '0')
		}
	}
	return 0
}

func calculateTopKeywords(densityMap map[string]float64, wordCount int) []KeywordFrequency {
	type kwPair struct {
		keyword string
		density float64
		count   int
	}

	var pairs []kwPair
	for kw, density := range densityMap {
		count := int((density * float64(wordCount)) / 100.0 + 0.5)
		if count < 1 {
			count = 1
		}
		pairs = append(pairs, kwPair{keyword: kw, density: density, count: count})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].density == pairs[j].density {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].density > pairs[j].density
	})

	topN := 10
	if len(pairs) < topN {
		topN = len(pairs)
	}

	result := make([]KeywordFrequency, 0, topN)
	for i := 0; i < topN; i++ {
		result = append(result, KeywordFrequency{
			Keyword: pairs[i].keyword,
			Count:   pairs[i].count,
			Density: pairs[i].density,
		})
	}
	return result
}

func calculateOverallSEOScore(pageData *crawler.PageData, headingIssues []string, brokenLinks []crawler.LinkInfo) (int, []Issue) {
	score := 100
	var issues []Issue

	if pageData.Title == "" {
		score -= 20
		issues = append(issues, Issue{
			Severity: "error",
			Type:     "title",
			Message:  "Missing title tag",
		})
	} else if len(pageData.Title) < 30 {
		score -= 5
		issues = append(issues, Issue{
			Severity: "warning",
			Type:     "title",
			Message:  fmt.Sprintf("Title too short (%d chars), recommended 50-60", len(pageData.Title)),
		})
	} else if len(pageData.Title) > 70 {
		score -= 5
		issues = append(issues, Issue{
			Severity: "warning",
			Type:     "title",
			Message:  fmt.Sprintf("Title too long (%d chars), recommended 50-60", len(pageData.Title)),
		})
	}

	if pageData.Description == "" {
		score -= 10
		issues = append(issues, Issue{
			Severity: "warning",
			Type:     "meta_description",
			Message:  "Missing meta description",
		})
	} else if len(pageData.Description) > 160 {
		score -= 5
		issues = append(issues, Issue{
			Severity: "warning",
			Type:     "meta_description",
			Message:  fmt.Sprintf("Meta description too long (%d chars)", len(pageData.Description)),
		})
	}

	if pageData.Canonical == "" {
		score -= 5
		issues = append(issues, Issue{
			Severity: "warning",
			Type:     "canonical",
			Message:  "Missing canonical link tag",
		})
	}

	if len(pageData.OGTags) == 0 {
		score -= 5
		issues = append(issues, Issue{
			Severity: "info",
			Type:     "og_tags",
			Message:  "Missing Open Graph meta tags",
		})
	}

	for _, issueMsg := range headingIssues {
		score -= 5
		issues = append(issues, Issue{
			Severity: "warning",
			Type:     "headings",
			Message:  issueMsg,
		})
	}

	if pageData.WordCount < 300 {
		score -= 10
		issues = append(issues, Issue{
			Severity: "warning",
			Type:     "content",
			Message:  fmt.Sprintf("Low word count (%d words), recommended 500+ for SEO", pageData.WordCount),
		})
	}

	if len(pageData.Links) > 0 {
		brokenRatio := float64(len(brokenLinks)) / float64(len(pageData.Links))
		if brokenRatio > 0 {
			penalty := int(brokenRatio * 30)
			if penalty < 5 {
				penalty = 5
			}
			if penalty > 25 {
				penalty = 25
			}
			score -= penalty
			issues = append(issues, Issue{
				Severity: "error",
				Type:     "links",
				Message:  fmt.Sprintf("Found %d broken links out of %d total links", len(brokenLinks), len(pageData.Links)),
			})
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score, issues
}

func (e *Engine) KeywordAnalysis(ctx context.Context, content string) (*KeywordResult, error) {
	words := strings.Fields(strings.ToLower(content))

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

	type kv struct {
		Word  string
		Count int
	}
	var kvs []kv
	seen := make(map[string]int)
	for _, word := range words {
		word = strings.Trim(word, ".,!?;:'\"()[]{}\\/|<>-=_+@#$%^&*")
		if len(word) > 3 && !stopWords[word] {
			if idx, ok := seen[word]; ok {
				kvs[idx].Count++
			} else {
				seen[word] = len(kvs)
				kvs = append(kvs, kv{Word: word, Count: 1})
			}
		}
	}

	sort.SliceStable(kvs, func(i, j int) bool {
		return kvs[i].Count > kvs[j].Count
	})

	keywords := make([]KeywordData, 0, min(20, len(kvs)))
	for i := 0; i < min(20, len(kvs)); i++ {
		density := float64(kvs[i].Count) / float64(len(words)) * 100
		keywords = append(keywords, KeywordData{
			Word:    kvs[i].Word,
			Count:   kvs[i].Count,
			Density: density,
		})
	}

	return &KeywordResult{
		Keywords:   keywords,
		TotalWords: len(words),
	}, nil
}

func (e *Engine) CompetitorAnalysis(ctx context.Context, domain string, competitors []string) (*CompetitorResult, error) {
	result := &CompetitorResult{
		Target:        domain,
		Competitors:   make(map[string]CompetitorData),
		Opportunities: make([]string, 0),
	}

	targetPages, err := e.crawler.Crawl(normalizeDomainURL(domain), 1, 20)
	if err != nil {
		slog.Warn("failed to crawl target domain", "domain", domain, "error", err)
	}

	result.TargetPages = len(targetPages)

	for _, comp := range competitors {
		compPages, err := e.crawler.Crawl(normalizeDomainURL(comp), 1, 20)
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

type Issue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
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
	Target        string                    `json:"target"`
	TargetPages   int                       `json:"target_pages"`
	Competitors   map[string]CompetitorData `json:"competitors"`
	Opportunities []string                  `json:"opportunities"`
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

func normalizeDomainURL(d string) string {
	d = strings.TrimSpace(d)
	if strings.HasPrefix(strings.ToLower(d), "http://") || strings.HasPrefix(strings.ToLower(d), "https://") {
		return d
	}
	return "https://" + d
}

