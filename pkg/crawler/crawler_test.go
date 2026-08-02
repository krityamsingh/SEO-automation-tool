package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCrawlURL_Success(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<head>
    <title>Sample Test Page for SEO Audit Engine Verification</title>
    <meta name="description" content="This is a detailed meta description for testing the SEO audit engine page parser logic thoroughly.">
    <link rel="canonical" href="https://example.com/canonical-page">
    <meta property="og:title" content="OG Sample Title">
    <meta property="og:description" content="OG Sample Description">
</head>
<body>
    <h1>Main Title Header</h1>
    <h2>Section Overview Header</h2>
    <h3>Subsection Detail Header</h3>
    <h4>Deep Paragraph Header</h4>
    <p>SEO automation tool SEO site audit engine verification testing. SEO automation tool delivers SEO performance metrics.</p>
    <a href="/internal-page">Internal Link</a>
    <a href="https://external-domain.com/page">External Link</a>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	cr := New("TestAgent/1.0", 0)
	cr.AllowLoopback = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pageData, err := cr.CrawlURL(ctx, server.URL)
	if err != nil {
		t.Fatalf("CrawlURL failed: %v", err)
	}

	if pageData.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", pageData.StatusCode)
	}

	if pageData.Title != "Sample Test Page for SEO Audit Engine Verification" {
		t.Errorf("Unexpected title: got %q", pageData.Title)
	}

	if pageData.Description != "This is a detailed meta description for testing the SEO audit engine page parser logic thoroughly." {
		t.Errorf("Unexpected description: got %q", pageData.Description)
	}

	if pageData.Canonical != "https://example.com/canonical-page" {
		t.Errorf("Unexpected canonical: got %q", pageData.Canonical)
	}

	if pageData.OGTags["og:title"] != "OG Sample Title" {
		t.Errorf("Unexpected og:title: got %q", pageData.OGTags["og:title"])
	}

	if pageData.OGTags["og:description"] != "OG Sample Description" {
		t.Errorf("Unexpected og:description: got %q", pageData.OGTags["og:description"])
	}

	if len(pageData.Headings) != 4 {
		t.Fatalf("Expected 4 headings, got %d", len(pageData.Headings))
	}

	expectedHeadings := []struct {
		tag  string
		text string
	}{
		{"h1", "Main Title Header"},
		{"h2", "Section Overview Header"},
		{"h3", "Subsection Detail Header"},
		{"h4", "Deep Paragraph Header"},
	}

	for i, h := range expectedHeadings {
		if pageData.Headings[i].Tag != h.tag || pageData.Headings[i].Text != h.text {
			t.Errorf("Heading [%d] mismatch: expected %s:%s, got %s:%s", i, h.tag, h.text, pageData.Headings[i].Tag, pageData.Headings[i].Text)
		}
	}

	if pageData.WordCount < 10 {
		t.Errorf("Expected word count > 10, got %d", pageData.WordCount)
	}

	if _, ok := pageData.KeywordDensity["seo"]; !ok {
		t.Errorf("Expected keyword 'seo' in density map")
	}

	if _, ok := pageData.KeywordDensity["seo automation"]; !ok {
		t.Errorf("Expected bigram 'seo automation' in density map")
	}

	if len(pageData.Links) != 2 {
		t.Fatalf("Expected 2 links, got %d", len(pageData.Links))
	}

	if pageData.Links[0].IsExternal {
		t.Errorf("Expected first link to be internal")
	}

	if pageData.Links[0].AnchorText != "Internal Link" {
		t.Errorf("Expected anchor text 'Internal Link', got %q", pageData.Links[0].AnchorText)
	}

	if !pageData.Links[1].IsExternal {
		t.Errorf("Expected second link to be external")
	}
}

func TestCrawlURL_SSRFGuard(t *testing.T) {
	cr := New("TestAgent/1.0", 0) // AllowLoopback is false by default
	ctx := context.Background()

	unsafeURLs := []string{
		"http://127.0.0.1",
		"http://127.0.0.1:8080/admin",
		"http://localhost",
		"http://localhost/internal",
		"http://169.254.169.254",
		"http://169.254.169.254/latest/meta-data",
		"http://0.0.0.0:8080/",
		"http://10.0.0.1/admin",
		"http://172.16.0.1/internal",
		"http://192.168.1.1/router",
		"http://[::1]:8080/",
		"ftp://127.0.0.1/test",
		"ftp://example.com/file",
	}

	for _, target := range unsafeURLs {
		if IsSafeOutboundURL(target) {
			t.Errorf("IsSafeOutboundURL(%q) returned true, expected false (SSRF rejection)", target)
		}

		_, err := cr.CrawlURL(ctx, target)
		if err == nil {
			t.Errorf("CrawlURL(%q) succeeded, expected SSRF rejection error", target)
		}
	}
}

func TestCrawlURL_LinkSchemeFiltering(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<body>
    <a href="JavaScript:alert(1)">JS Upper</a>
    <a href="javascript:void(0)">JS Lower</a>
    <a href="MailTo:user@example.com">Mail Upper</a>
    <a href="TEL:1234567890">Tel Upper</a>
    <a href="DATA:text/plain;base64,SGVsbG8=">Data Upper</a>
    <a href="#section-anchor">Anchor Link</a>
    <a href="/valid-path">Valid Link</a>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	cr := New("TestAgent/1.0", 0)
	cr.AllowLoopback = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pageData, err := cr.CrawlURL(ctx, server.URL)
	if err != nil {
		t.Fatalf("CrawlURL failed: %v", err)
	}

	if len(pageData.Links) != 1 {
		t.Fatalf("Expected exactly 1 valid link extracted, got %d links: %+v", len(pageData.Links), pageData.Links)
	}

	expectedURL := server.URL + "/valid-path"
	if pageData.Links[0].URL != expectedURL {
		t.Errorf("Expected link URL %q, got %q", expectedURL, pageData.Links[0].URL)
	}
}

func TestCheckLinksConcurrently(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/working":
			w.WriteHeader(http.StatusOK)
		case "/broken":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	cr := New("TestAgent/1.0", 0)
	cr.AllowLoopback = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	links := []LinkInfo{
		{URL: server.URL + "/working", AnchorText: "Working Link", IsExternal: false},
		{URL: server.URL + "/broken", AnchorText: "Broken Link", IsExternal: false},
		{URL: "http://127.0.0.1:59999/nonexistent", AnchorText: "Unreachable Link", IsExternal: true},
	}

	checked := cr.CheckLinksConcurrently(ctx, links)
	if len(checked) != 3 {
		t.Fatalf("Expected 3 checked links, got %d", len(checked))
	}

	if checked[0].IsBroken || checked[0].StatusCode != http.StatusOK {
		t.Errorf("Link 0 should be working 200, got status=%d broken=%v", checked[0].StatusCode, checked[0].IsBroken)
	}

	if !checked[1].IsBroken || checked[1].StatusCode != http.StatusNotFound {
		t.Errorf("Link 1 should be broken 404, got status=%d broken=%v", checked[1].StatusCode, checked[1].IsBroken)
	}

	if !checked[2].IsBroken {
		t.Errorf("Link 2 should be broken due to network error")
	}
}
