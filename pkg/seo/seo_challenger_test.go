package seo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aeo_geo_seo_agent/pkg/crawler"
)

// TestSEO_HeadingHierarchyEdgeCases tests all heading issue detectors (missing H1, multiple H1s, skipped levels, empty text).
func TestSEO_HeadingHierarchyEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		headings       []crawler.HeadingNode
		expectedIssues []string
	}{
		{
			name:           "Empty Headings",
			headings:       []crawler.HeadingNode{},
			expectedIssues: []string{"Missing H1 tag"},
		},
		{
			name: "Multiple H1 Tags",
			headings: []crawler.HeadingNode{
				{Tag: "h1", Text: "First Main Title"},
				{Tag: "h1", Text: "Second Main Title"},
				{Tag: "h1", Text: "Third Main Title"},
			},
			expectedIssues: []string{"Multiple H1 tags found (3)"},
		},
		{
			name: "Skipped Heading Level H1 to H4",
			headings: []crawler.HeadingNode{
				{Tag: "h1", Text: "Top Level Title"},
				{Tag: "h4", Text: "Deep Subsection Heading"},
			},
			expectedIssues: []string{"Skipped heading level: H1 to H4"},
		},
		{
			name: "Skipped Heading Level H2 to H5",
			headings: []crawler.HeadingNode{
				{Tag: "h1", Text: "Main Title"},
				{Tag: "h2", Text: "Section Title"},
				{Tag: "h5", Text: "Skipped Section Title"},
			},
			expectedIssues: []string{"Skipped heading level: H2 to H5"},
		},
		{
			name: "Empty Heading Content",
			headings: []crawler.HeadingNode{
				{Tag: "h1", Text: "   "},
				{Tag: "h2", Text: ""},
			},
			expectedIssues: []string{
				"Empty H1 tag found",
				"Empty H2 tag found",
			},
		},
		{
			name: "Perfect Heading Hierarchy",
			headings: []crawler.HeadingNode{
				{Tag: "h1", Text: "Main Title"},
				{Tag: "h2", Text: "Section 1"},
				{Tag: "h3", Text: "Subsection 1.1"},
				{Tag: "h2", Text: "Section 2"},
				{Tag: "h3", Text: "Subsection 2.1"},
				{Tag: "h4", Text: "Deep Subsection 2.1.1"},
			},
			expectedIssues: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := evaluateHeadingHierarchy(tt.headings)
			for _, exp := range tt.expectedIssues {
				found := false
				for _, iss := range issues {
					if strings.Contains(iss, exp) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected issue containing %q, got issues: %v", exp, issues)
				}
			}
		})
	}
}

// TestSEO_SEOScoreBoundaryLimits verifies that SEOScore stays within [0, 100] even with extreme penalties.
func TestSEO_SEOScoreBoundaryLimits(t *testing.T) {
	// Case 1: Worst-case page with all conceivable penalties
	badPageData := &crawler.PageData{
		URL:         "https://example.com/disaster",
		Title:       "", // -20 penalty
		Description: "", // -10 penalty
		Canonical:   "", // -5 penalty
		OGTags:      nil, // -5 penalty
		WordCount:   10, // -10 penalty
		Links: []crawler.LinkInfo{
			{URL: "https://example.com/b1", IsBroken: true},
			{URL: "https://example.com/b2", IsBroken: true},
		},
	}
	headingIssues := []string{
		"Missing H1 tag",
		"Empty H2 tag found",
		"Skipped heading level: H1 to H4",
		"Skipped heading level: H2 to H6",
		"Multiple H1 tags found (5)",
	}
	brokenLinks := badPageData.Links

	score, issues := calculateOverallSEOScore(badPageData, headingIssues, brokenLinks)
	t.Logf("Calculated disaster page score: %d (issues count: %d)", score, len(issues))

	if score < 0 || score > 100 {
		t.Errorf("Score out of bounds [0, 100]: got %d", score)
	}

	if score != 0 {
		t.Errorf("Expected score 0 for maximum penalty page, got %d", score)
	}

	// Case 2: Perfect page
	healthyWords := make([]string, 500)
	for i := range healthyWords {
		healthyWords[i] = "content"
	}

	perfectPageData := &crawler.PageData{
		URL:         "https://example.com/perfect",
		Title:       "This Title Is Perfectly Between Fifty And Sixty Chars Long!",
		Description: "This meta description is written perfectly for search engines to index and rank cleanly.",
		Canonical:   "https://example.com/perfect",
		OGTags:      map[string]string{"og:title": "Perfect OG Title"},
		WordCount:   500,
		Headings: []crawler.HeadingNode{
			{Tag: "h1", Text: "Main Title"},
			{Tag: "h2", Text: "Sub Title"},
		},
		Links: []crawler.LinkInfo{
			{URL: "https://example.com/valid", IsBroken: false},
		},
	}

	perfectScore, perfectIssues := calculateOverallSEOScore(perfectPageData, nil, nil)
	if perfectScore != 100 {
		t.Errorf("Expected perfect score 100, got %d (issues: %v)", perfectScore, perfectIssues)
	}
}

// TestSEO_OnPageAudit_BrokenLinksAndNonExistentURLs tests auditing pages containing non-existent URLs and broken links.
func TestSEO_OnPageAudit_BrokenLinksAndNonExistentURLs(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<head>
    <title>SEO Audit Engine Broken Link & Unreachable Target Verification</title>
    <meta name="description" content="Meta description testing broken links and non-existent external URLs for audit engine.">
    <link rel="canonical" href="http://127.0.0.1/canonical">
    <meta property="og:title" content="OG Title">
</head>
<body>
    <h1>Audit Engine Header</h1>
    <p>Testing broken links detection in site audit.</p>
    <a href="/working-link">Working Link</a>
    <a href="/404-link">Broken 404 Link</a>
    <a href="http://127.0.0.1:59999/unreachable">Unreachable Link</a>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/working-link":
			w.WriteHeader(http.StatusOK)
		case "/404-link":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockHTML))
		}
	}))
	defer server.Close()

	cr := crawler.New("ChallengerAgent/1.0", 0)
	cr.AllowLoopback = true
	seoEngine := New(nil, cr, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report, err := seoEngine.OnPageAudit(ctx, server.URL)
	if err != nil {
		t.Fatalf("OnPageAudit failed: %v", err)
	}

	if len(report.BrokenLinks) < 2 {
		t.Errorf("Expected at least 2 broken links (404 and unreachable 59999), got %d broken links", len(report.BrokenLinks))
	}

	found404 := false
	foundUnreachable := false

	for _, link := range report.BrokenLinks {
		if strings.Contains(link.URL, "/404-link") && link.StatusCode == http.StatusNotFound {
			found404 = true
		}
		if strings.Contains(link.URL, ":59999/") && link.IsBroken {
			foundUnreachable = true
		}
	}

	if !found404 {
		t.Errorf("Expected 404 link to be detected in BrokenLinks")
	}

	if !foundUnreachable {
		t.Errorf("Expected unreachable link (:59999) to be detected in BrokenLinks")
	}
}

// TestSEO_KeywordAnalysis_EdgeCases tests KeywordAnalysis with empty text, stop words only, and special characters.
func TestSEO_KeywordAnalysis_EdgeCases(t *testing.T) {
	cr := crawler.New("ChallengerAgent/1.0", 0)
	seoEngine := New(nil, cr, nil)
	ctx := context.Background()

	t.Run("Empty Content", func(t *testing.T) {
		res, err := seoEngine.KeywordAnalysis(ctx, "")
		if err != nil {
			t.Fatalf("KeywordAnalysis failed: %v", err)
		}
		if res.TotalWords != 0 || len(res.Keywords) != 0 {
			t.Errorf("Expected empty result, got totalWords=%d keywords=%d", res.TotalWords, len(res.Keywords))
		}
	})

	t.Run("Stop Words Only", func(t *testing.T) {
		res, err := seoEngine.KeywordAnalysis(ctx, "the a an is are was were be been being have has had")
		if err != nil {
			t.Fatalf("KeywordAnalysis failed: %v", err)
		}
		if len(res.Keywords) != 0 {
			t.Errorf("Expected 0 keywords for stop words only, got %d", len(res.Keywords))
		}
	})

	t.Run("Special Characters and Repeated Keywords", func(t *testing.T) {
		text := "performance! performance? performance... optimization, optimization; engine #engine @engine"
		res, err := seoEngine.KeywordAnalysis(ctx, text)
		if err != nil {
			t.Fatalf("KeywordAnalysis failed: %v", err)
		}
		if len(res.Keywords) == 0 {
			t.Fatalf("Expected keywords extracted from special character text")
		}
		if res.Keywords[0].Word != "performance" {
			t.Errorf("Expected top keyword 'performance', got %q", res.Keywords[0].Word)
		}
	})
}
