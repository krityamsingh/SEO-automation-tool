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

// TestSEO_AdversarialOnPageAudit tests OnPageAudit under adversarial input conditions.
func TestSEO_AdversarialOnPageAudit(t *testing.T) {
	cr := crawler.New("SecurityTestAgent/1.0", 0)
	cr.AllowLoopback = true
	seoEngine := New(nil, cr, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("Script Injection & Malformed Attributes in OnPageAudit", func(t *testing.T) {
		mockHTML := `<!DOCTYPE html>
<html>
<head>
    <title><script>alert('xss title')</script> Title With Malformed Head</title>
    <meta name="description" content="Meta description with <script>alert('xss desc')</script> and quotes &quot;">
    <link rel="canonical" href="http://127.0.0.1/canonical">
</head>
<body>
    <h1 id="<script>alert(1)</script>">Main Title <script>alert('xss h1')</script></h1>
    <a href="JavaScript:alert('xss link')">Malicious JS Link</a>
    <a href="http://169.254.169.254/latest/meta-data/">SSRF Target Link</a>
    <p>Body content for SEO testing with script injection <script>document.cookie</script>.</p>
</body>
</html>`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockHTML))
		}))
		defer server.Close()

		report, err := seoEngine.OnPageAudit(ctx, server.URL)
		if err != nil {
			t.Fatalf("OnPageAudit failed: %v", err)
		}

		t.Logf("Audited Title: %q", report.Title)
		t.Logf("Audited Description: %q", report.Description)
		t.Logf("Extracted Headings: %+v", report.HeadingHierarchy)
		t.Logf("Extracted Broken Links count: %d", len(report.BrokenLinks))

		// Verify SSRF link to 169.254.169.254 was marked broken by CheckLinksConcurrently
		foundSSRFFiltered := false
		for _, bl := range report.BrokenLinks {
			if strings.Contains(bl.URL, "169.254.169.254") {
				foundSSRFFiltered = true
				if !bl.IsBroken {
					t.Errorf("SSRF link to 169.254.169.254 should be marked broken")
				}
			}
		}
		if !foundSSRFFiltered {
			t.Errorf("Expected SSRF link 169.254.169.254 to be in broken links")
		} else {
			t.Logf("PASS: SSRF link 169.254.169.254 correctly marked broken by CheckLinksConcurrently")
		}

		// Check JavaScript: link extraction (case-sensitivity flaw)
		for _, link := range report.BrokenLinks {
			if strings.HasPrefix(strings.ToLower(link.URL), "javascript:") {
				t.Logf("FINDING: Bypassed javascript: link was passed to link checker and marked broken: %q", link.URL)
			}
		}
	})

	t.Run("Deeply Nested HTML in OnPageAudit", func(t *testing.T) {
		var builder strings.Builder
		builder.WriteString("<!DOCTYPE html><html><head><title>Deep Audit Page Title</title></head><body><h1>Deep H1</h1>")
		for i := 0; i < 3000; i++ {
			builder.WriteString("<div>")
		}
		builder.WriteString("<p>Nested paragraph text optimization search engine.</p>")
		for i := 0; i < 3000; i++ {
			builder.WriteString("</div>")
		}
		builder.WriteString("</body></html>")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(builder.String()))
		}))
		defer server.Close()

		report, err := seoEngine.OnPageAudit(ctx, server.URL)
		if err != nil {
			t.Fatalf("OnPageAudit failed on 3000-level HTML: %v", err)
		}
		t.Logf("PASS: OnPageAudit completed for 3000-level nested HTML. Score: %d", report.OverallSEOScore)
	})

	t.Run("Invalid UTF-8 in KeywordAnalysis and OnPageAudit", func(t *testing.T) {
		invalidUTF8 := "Keyword analysis text \xff\xfe\xfd with invalid bytes \x80\x81\x82 optimization engine."
		res, err := seoEngine.KeywordAnalysis(ctx, invalidUTF8)
		if err != nil {
			t.Fatalf("KeywordAnalysis failed on invalid UTF-8: %v", err)
		}
		t.Logf("PASS: KeywordAnalysis extracted %d keywords from invalid UTF-8 string without panic", len(res.Keywords))

		mockHTML := fmt.Sprintf("<!DOCTYPE html><html><head><title>Title \xff\xfe</title></head><body><h1>H1 \x80\x81</h1><p>%s</p></body></html>", invalidUTF8)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(mockHTML))
		}))
		defer server.Close()

		report, err := seoEngine.OnPageAudit(ctx, server.URL)
		if err != nil {
			t.Fatalf("OnPageAudit failed on invalid UTF-8 HTML: %v", err)
		}
		if report.Title == "" {
			t.Errorf("Expected title extracted")
		}
		t.Logf("PASS: OnPageAudit completed for invalid UTF-8 HTML without panic (Title=%q)", report.Title)
	})
}
