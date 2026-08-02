package seo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aeo_geo_seo_agent/pkg/crawler"
)

func TestOnPageAudit_HealthyPage(t *testing.T) {
	// Build mock page with 350+ words to ensure high word count score
	words := make([]string, 350)
	for i := range words {
		if i%5 == 0 {
			words[i] = "optimization"
		} else if i%3 == 0 {
			words[i] = "engine"
		} else {
			words[i] = "performance"
		}
	}
	bodyParagraph := strings.Join(words, " ")

	mockHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Optimal Target Page Title for SEO Site Audit Engine Verification</title>
    <meta name="description" content="This is an optimal length meta description for testing the SEO audit engine page analyzer with high accuracy metrics.">
    <link rel="canonical" href="https://example.com/optimal-page">
    <meta property="og:title" content="Optimal OG Title">
    <meta property="og:description" content="Optimal OG Description">
</head>
<body>
    <h1>Primary Section Heading One</h1>
    <h2>Secondary Subheading Two</h2>
    <h3>Tertiary Subheading Three</h3>
    <p>%s</p>
    <a href="/internal-health">Internal Page</a>
</body>
</html>`, bodyParagraph)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	cr := crawler.New("TestAgent/1.0", 0)
	cr.AllowLoopback = true
	seoEngine := New(nil, cr, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report, err := seoEngine.OnPageAudit(ctx, server.URL)
	if err != nil {
		t.Fatalf("OnPageAudit failed: %v", err)
	}

	if report.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", report.StatusCode)
	}

	if report.Title != "Optimal Target Page Title for SEO Site Audit Engine Verification" {
		t.Errorf("Unexpected title: got %q", report.Title)
	}

	if report.Canonical != "https://example.com/optimal-page" {
		t.Errorf("Unexpected canonical URL: got %q", report.Canonical)
	}

	if report.OGTags["og:title"] != "Optimal OG Title" {
		t.Errorf("Unexpected og:title: got %q", report.OGTags["og:title"])
	}

	if len(report.HeadingIssues) != 0 {
		t.Errorf("Expected 0 heading issues, got %v", report.HeadingIssues)
	}

	if len(report.BrokenLinks) != 0 {
		t.Errorf("Expected 0 broken links, got %v", report.BrokenLinks)
	}

	if report.OverallSEOScore < 80 {
		t.Errorf("Expected overall score >= 80 for healthy page, got %d", report.OverallSEOScore)
	}

	if len(report.TopKeywords) == 0 {
		t.Errorf("Expected top keywords to be populated")
	}
}

func TestOnPageAudit_PageWithSEOIssues(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<head>
</head>
<body>
    <h2>Skipped Level Heading</h2>
    <h4>Another Skipped Level Heading</h4>
    <p>Low word count body text.</p>
    <a href="/broken-link-target">Broken Link</a>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/broken-link-target":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockHTML))
		}
	}))
	defer server.Close()

	cr := crawler.New("TestAgent/1.0", 0)
	cr.AllowLoopback = true
	seoEngine := New(nil, cr, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report, err := seoEngine.OnPageAudit(ctx, server.URL)
	if err != nil {
		t.Fatalf("OnPageAudit failed: %v", err)
	}

	if report.OverallSEOScore >= 80 {
		t.Errorf("Expected score < 80 due to missing title, missing description, missing canonical, missing H1, skipped heading, low word count, broken link; got %d", report.OverallSEOScore)
	}

	hasMissingH1 := false
	hasSkippedHeading := false
	for _, issue := range report.HeadingIssues {
		if strings.Contains(issue, "Missing H1 tag") {
			hasMissingH1 = true
		}
		if strings.Contains(issue, "Skipped heading level") {
			hasSkippedHeading = true
		}
	}

	if !hasMissingH1 {
		t.Errorf("Expected 'Missing H1 tag' in HeadingIssues, got %v", report.HeadingIssues)
	}

	if !hasSkippedHeading {
		t.Errorf("Expected 'Skipped heading level' in HeadingIssues, got %v", report.HeadingIssues)
	}

	if len(report.BrokenLinks) == 0 {
		t.Errorf("Expected broken link to be detected, got 0 broken links")
	} else if report.BrokenLinks[0].StatusCode != http.StatusNotFound {
		t.Errorf("Expected broken link status 404, got %d", report.BrokenLinks[0].StatusCode)
	}
}

func TestKeywordAnalysis_SortingPerformanceAndAccuracy(t *testing.T) {
	cr := crawler.New("TestAgent/1.0", 0)
	seoEngine := New(nil, cr, nil)

	// Create test dataset with known word frequencies
	words := make([]string, 0, 5000)
	// "alpha" appears 100 times, "beta" 80 times, "gamma" 60 times, "delta" 40 times, "epsilon" 20 times
	for i := 0; i < 100; i++ {
		words = append(words, "alpha")
	}
	for i := 0; i < 80; i++ {
		words = append(words, "beta")
	}
	for i := 0; i < 60; i++ {
		words = append(words, "gamma")
	}
	for i := 0; i < 40; i++ {
		words = append(words, "delta")
	}
	for i := 0; i < 20; i++ {
		words = append(words, "epsilon")
	}
	// Fill remaining with distinct words to test large N sort
	for i := 0; i < 4700; i++ {
		words = append(words, fmt.Sprintf("word%d", i))
	}

	content := strings.Join(words, " ")
	ctx := context.Background()

	startTime := time.Now()
	res, err := seoEngine.KeywordAnalysis(ctx, content)
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("KeywordAnalysis failed: %v", err)
	}

	if duration > 100*time.Millisecond {
		t.Errorf("KeywordAnalysis took too long (%v), expected fast O(N log N) sorting", duration)
	}

	if len(res.Keywords) == 0 {
		t.Fatalf("Expected keywords in result")
	}

	// Verify strict descending order
	for i := 1; i < len(res.Keywords); i++ {
		if res.Keywords[i].Count > res.Keywords[i-1].Count {
			t.Errorf("Keywords not sorted in descending order: index %d (%s: %d) > index %d (%s: %d)",
				i, res.Keywords[i].Word, res.Keywords[i].Count,
				i-1, res.Keywords[i-1].Word, res.Keywords[i-1].Count)
		}
	}

	// Verify top keyword frequencies match expected counts
	if res.Keywords[0].Word != "alpha" || res.Keywords[0].Count != 100 {
		t.Errorf("Expected top keyword 'alpha' with count 100, got %s:%d", res.Keywords[0].Word, res.Keywords[0].Count)
	}
	if res.Keywords[1].Word != "beta" || res.Keywords[1].Count != 80 {
		t.Errorf("Expected second keyword 'beta' with count 80, got %s:%d", res.Keywords[1].Word, res.Keywords[1].Count)
	}
	if res.Keywords[2].Word != "gamma" || res.Keywords[2].Count != 60 {
		t.Errorf("Expected third keyword 'gamma' with count 60, got %s:%d", res.Keywords[2].Word, res.Keywords[2].Count)
	}
}
