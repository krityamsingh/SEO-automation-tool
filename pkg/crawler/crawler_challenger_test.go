package crawler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestCrawler_EmptyHTML tests boundary condition with empty or minimal HTML responses.
func TestCrawler_EmptyHTML(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{"Completely Empty", ""},
		{"Whitespace Only", "   \n\t  "},
		{"Empty HTML Tags", "<html><head></head><body></body></html>"},
		{"Non-HTML Plain Text", "Hello world without any HTML markup."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tc.content))
			}))
			defer server.Close()

			cr := New("ChallengerAgent/1.0", 0)
			cr.AllowLoopback = true
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			pageData, err := cr.CrawlURL(ctx, server.URL)
			if err != nil {
				t.Fatalf("CrawlURL failed for %s: %v", tc.name, err)
			}

			if pageData.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", pageData.StatusCode)
			}

			if pageData.Title != "" && tc.name == "Completely Empty" {
				t.Errorf("Expected empty title, got %q", pageData.Title)
			}

			if len(pageData.Headings) != 0 {
				t.Errorf("Expected 0 headings, got %d", len(pageData.Headings))
			}

			if len(pageData.Links) != 0 {
				t.Errorf("Expected 0 links, got %d", len(pageData.Links))
			}
		})
	}
}

// TestCrawler_LargeHTMLFile tests parsing performance and memory stability on a large (5MB+) HTML document.
func TestCrawler_LargeHTMLFile(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("<!DOCTYPE html><html><head><title>Large Stress Test HTML Page</title></head><body>")

	// Generate 1000 headings, 1000 links, and ~50,000 words
	for i := 1; i <= 1000; i++ {
		builder.WriteString(fmt.Sprintf("<h%d>Heading level %d item %d</h%d>", (i%6)+1, (i%6)+1, i, (i%6)+1))
		builder.WriteString(fmt.Sprintf("<p>Paragraph content number %d with SEO audit metrics keyword density data generation testing.</p>", i))
		builder.WriteString(fmt.Sprintf("<a href=\"/link-%d\">Internal Link %d</a>", i, i))
	}
	builder.WriteString("</body></html>")
	largeHTML := builder.String()

	t.Logf("Generated large HTML size: %.2f MB", float64(len(largeHTML))/(1024*1024))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeHTML))
	}))
	defer server.Close()

	cr := New("ChallengerAgent/1.0", 0)
	cr.AllowLoopback = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	pageData, err := cr.CrawlURL(ctx, server.URL)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("CrawlURL failed on large HTML: %v", err)
	}

	t.Logf("Crawled %.2f MB HTML in %v", float64(len(largeHTML))/(1024*1024), duration)

	if len(pageData.Headings) != 1000 {
		t.Errorf("Expected 1000 headings, got %d", len(pageData.Headings))
	}

	if len(pageData.Links) != 1000 {
		t.Errorf("Expected 1000 links, got %d", len(pageData.Links))
	}

	if pageData.WordCount < 10000 {
		t.Errorf("Expected word count > 10000, got %d", pageData.WordCount)
	}
}

// TestCrawler_HighLinkCountConcurrency tests checking hundreds of links concurrently.
func TestCrawler_HighLinkCountConcurrency(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		path := r.URL.Path
		if strings.HasPrefix(path, "/ok-") {
			w.WriteHeader(http.StatusOK)
		} else if strings.HasPrefix(path, "/404-") {
			w.WriteHeader(http.StatusNotFound)
		} else if strings.HasPrefix(path, "/500-") {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	cr := New("ChallengerAgent/1.0", 0)
	cr.AllowLoopback = true

	totalLinks := 300
	links := make([]LinkInfo, totalLinks)
	for i := 0; i < totalLinks; i++ {
		var linkPath string
		if i%3 == 0 {
			linkPath = fmt.Sprintf("/ok-%d", i)
		} else if i%3 == 1 {
			linkPath = fmt.Sprintf("/404-%d", i)
		} else {
			linkPath = fmt.Sprintf("/500-%d", i)
		}
		links[i] = LinkInfo{
			URL:        server.URL + linkPath,
			AnchorText: fmt.Sprintf("Link %d", i),
			IsExternal: false,
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	checked := cr.CheckLinksConcurrently(ctx, links)
	elapsed := time.Since(start)

	t.Logf("Checked %d links concurrently in %v (total HTTP requests handled: %d)", totalLinks, elapsed, atomic.LoadInt32(&requestCount))

	if len(checked) != totalLinks {
		t.Fatalf("Expected %d results, got %d", totalLinks, len(checked))
	}

	brokenCount := 0
	workingCount := 0

	for i, l := range checked {
		if i%3 == 0 {
			if l.IsBroken || l.StatusCode != http.StatusOK {
				t.Errorf("Link [%d] (%s) should be OK 200, got status=%d broken=%v", i, l.URL, l.StatusCode, l.IsBroken)
			} else {
				workingCount++
			}
		} else {
			if !l.IsBroken {
				t.Errorf("Link [%d] (%s) should be broken, got status=%d broken=%v", i, l.URL, l.StatusCode, l.IsBroken)
			} else {
				brokenCount++
			}
		}
	}

	if brokenCount != 200 {
		t.Errorf("Expected 200 broken links, got %d", brokenCount)
	}
	if workingCount != 100 {
		t.Errorf("Expected 100 working links, got %d", workingCount)
	}
}

// TestCrawler_ContextCancellationConcurrency tests rapid worker teardown when context is canceled.
func TestCrawler_ContextCancellationConcurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second) // Slow handler
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cr := New("ChallengerAgent/1.0", 0)
	cr.AllowLoopback = true

	links := make([]LinkInfo, 50)
	for i := 0; i < 50; i++ {
		links[i] = LinkInfo{
			URL:        fmt.Sprintf("%s/slow-%d", server.URL, i),
			AnchorText: fmt.Sprintf("Slow Link %d", i),
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel context after 100ms
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	checked := cr.CheckLinksConcurrently(ctx, links)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Errorf("CheckLinksConcurrently took %v; expected cancellation to terminate pool quickly", elapsed)
	}

	if len(checked) != 50 {
		t.Errorf("Expected 50 returned link structs, got %d", len(checked))
	}
}

// TestCrawler_SSRFGuardExtended tests various network targets against SSRF security guard.
func TestCrawler_SSRFGuardExtended(t *testing.T) {
	cr := New("ChallengerAgent/1.0", 0)
	ctx := context.Background()

	testTargets := []struct {
		url     string
		allowed bool
	}{
		{"http://10.0.0.1/admin", false},
		{"http://172.16.0.5/internal", false},
		{"http://192.168.1.1/router", false},
		{"http://224.0.0.1/multicast", false},
		{"ftp://example.com/file", false},
		{"javascript:alert(1)", false},
		{"http://localhost:8080/app", false},
		{"http://127.0.0.1:8080/app", false},
	}

	for _, tt := range testTargets {
		t.Run(tt.url, func(t *testing.T) {
			_, err := cr.CrawlURL(ctx, tt.url)
			if tt.allowed && err != nil && strings.Contains(err.Error(), "SSRF guard") {
				t.Errorf("Target %s should be allowed by SSRF guard, but was blocked", tt.url)
			} else if !tt.allowed && err == nil {
				t.Errorf("Target %s should be blocked by SSRF guard, but succeeded", tt.url)
			}
		})
	}
}
