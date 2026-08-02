package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSSRF_TargetURLs tests security boundaries for SSRF attempts:
// http://127.0.0.1:8080, http://169.254.169.254, http://localhost/
func TestSSRF_TargetURLs(t *testing.T) {
	cr := New("SecurityTestAgent/1.0", 0)
	ctx := context.Background()

	t.Run("SSRF AWS Metadata IP 169.254.169.254", func(t *testing.T) {
		urlStr := "http://169.254.169.254/latest/meta-data/"
		isSafe := isSafeOutboundURL(urlStr)
		if isSafe {
			t.Errorf("SECURITY RISK: 169.254.169.254 was allowed by isSafeOutboundURL")
		} else {
			t.Logf("PASS: 169.254.169.254 blocked by SSRF guard")
		}

		_, err := cr.CrawlURL(ctx, urlStr)
		if err == nil {
			t.Errorf("SECURITY RISK: CrawlURL succeeded for 169.254.169.254")
		} else {
			t.Logf("PASS: CrawlURL rejected 169.254.169.254 with error: %v", err)
		}
	})

	t.Run("SSRF Loopback IP 127.0.0.1:8080", func(t *testing.T) {
		urlStr := "http://127.0.0.1:8080/admin"
		isSafe := isSafeOutboundURL(urlStr)
		if isSafe {
			t.Errorf("SECURITY RISK: 127.0.0.1:8080 was allowed by isSafeOutboundURL")
		} else {
			t.Logf("PASS: 127.0.0.1:8080 correctly blocked by SSRF guard")
		}
	})

	t.Run("SSRF Localhost http://localhost/", func(t *testing.T) {
		urlStr := "http://localhost/"
		isSafe := isSafeOutboundURL(urlStr)
		if isSafe {
			t.Errorf("SECURITY RISK: http://localhost/ was allowed by isSafeOutboundURL")
		} else {
			t.Logf("PASS: http://localhost/ correctly blocked by SSRF guard")
		}
	})

	t.Run("SSRF IPv4 0.0.0.0", func(t *testing.T) {
		urlStr := "http://0.0.0.0:8080/"
		isSafe := isSafeOutboundURL(urlStr)
		if isSafe {
			t.Logf("FINDING (SSRF Vulnerability): 0.0.0.0 is ALLOWED because net.ParseIP(0.0.0.0).IsLoopback() is false!")
		}
	})

	t.Run("SSRF Non-IP Domain (DNS Rebinding/Internal Domain)", func(t *testing.T) {
		urlStr := "http://internal.company.local/secret"
		isSafe := isSafeOutboundURL(urlStr)
		if isSafe {
			t.Logf("FINDING (SSRF Vulnerability): Domain hostnames (non-IP literals) bypass IP checks and return true!")
		}
	})
}

// TestAdversarialHTML_ScriptInjections tests script tag handling and case variation in link schemes.
func TestAdversarialHTML_ScriptInjections(t *testing.T) {
	mockHTML := `<!DOCTYPE html>
<html>
<head>
    <title>XSS & Script Injection Test</title>
    <script>alert('head script');</script>
</head>
<body>
    <h1>Header <script>alert('heading script');</script> End</h1>
    <p>Body text <script>alert('body script');</script> with content.</p>
    <a href="javascript:alert('lowercase')">Lowercase JS Link</a>
    <a href="JavaScript:alert('mixedcase')">MixedCase JS Link</a>
    <a href="JAVASCRIPT:alert('uppercase')">UpperCase JS Link</a>
    <a href="   javascript:alert('spaces')">Spaced JS Link</a>
    <img src="x" onerror="alert(1)" alt="<script>alert('alt xss')</script>">
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	cr := New("SecurityTestAgent/1.0", 0)
	cr.AllowLoopback = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pageData, err := cr.CrawlURL(ctx, server.URL)
	if err != nil {
		t.Fatalf("CrawlURL failed: %v", err)
	}

	t.Run("Script text removal from body", func(t *testing.T) {
		if strings.Contains(pageData.BodyText, "alert('body script')") {
			t.Errorf("Body text contains script content: %q", pageData.BodyText)
		} else {
			t.Logf("PASS: Body script tag stripped from BodyText")
		}
	})

	t.Run("Script text inside heading tags", func(t *testing.T) {
		for _, h := range pageData.Headings {
			t.Logf("Extracted heading: %s -> %q", h.Tag, h.Text)
			if strings.Contains(h.Text, "alert('heading script')") {
				t.Logf("FINDING: Heading text includes inline script content: %q", h.Text)
			}
		}
	})

	t.Run("JavaScript scheme case variations in links", func(t *testing.T) {
		t.Logf("Extracted links count: %d", len(pageData.Links))
		for _, l := range pageData.Links {
			t.Logf("Extracted link: URL=%q, Anchor=%q", l.URL, l.AnchorText)
			if strings.HasPrefix(strings.ToLower(l.URL), "javascript:") {
				t.Logf("FINDING (Case Sensitivity Bypass): Mixed-case 'javascript:' link extracted into Links list: %q", l.URL)
			}
		}
	})
}

// TestAdversarialHTML_MalformedAndDeepNesting tests robustness against malformed HTML and deep tag nesting.
func TestAdversarialHTML_MalformedAndDeepNesting(t *testing.T) {
	t.Run("Malformed HTML syntax", func(t *testing.T) {
		malformedHTML := `<html><body>
			<h1 title=>Malformed Header</h1>
			<p>Unclosed paragraph
			<a href="http://example.com" broken-attr=>Link
			<<<<<>>>>><<<<>>>>>
			<div><span>Unclosed span
		`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(malformedHTML))
		}))
		defer server.Close()

		cr := New("SecurityTestAgent/1.0", 0)
		cr.AllowLoopback = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		pageData, err := cr.CrawlURL(ctx, server.URL)
		if err != nil {
			t.Fatalf("CrawlURL failed on malformed HTML: %v", err)
		}

		if len(pageData.Headings) != 1 {
			t.Errorf("Expected 1 heading, got %d", len(pageData.Headings))
		}
		t.Logf("PASS: Malformed HTML parsed safely without panic")
	})

	t.Run("Deeply Nested HTML Tags (5000 levels)", func(t *testing.T) {
		var builder strings.Builder
		builder.WriteString("<!DOCTYPE html><html><body><h1>Deep Nesting Header</h1>")
		depth := 5000
		for i := 0; i < depth; i++ {
			builder.WriteString("<div>")
		}
		builder.WriteString("<p>Deeply nested text content inside 5000 divs.</p>")
		for i := 0; i < depth; i++ {
			builder.WriteString("</div>")
		}
		builder.WriteString("</body></html>")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(builder.String()))
		}))
		defer server.Close()

		cr := New("SecurityTestAgent/1.0", 0)
		cr.AllowLoopback = true
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		start := time.Now()
		pageData, err := cr.CrawlURL(ctx, server.URL)
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("CrawlURL failed on deeply nested HTML: %v", err)
		}

		t.Logf("PASS: Crawled 5000-level nested HTML in %v without stack overflow/panic", duration)
		if !strings.Contains(pageData.BodyText, "Deeply nested text content") {
			t.Errorf("Expected deeply nested text content in BodyText")
		}
	})
}

// TestAdversarialHTML_InvalidUTF8Strings tests behavior on non-UTF-8 / invalid UTF-8 byte sequences.
func TestAdversarialHTML_InvalidUTF8Strings(t *testing.T) {
	invalidUTF8Bytes := []byte("<!DOCTYPE html><html><body><h1>Header \xff\xfe\xfd Invalid</h1><p>Body \x80\x81\x82 text content testing.</p></body></html>")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(invalidUTF8Bytes)
	}))
	defer server.Close()

	cr := New("SecurityTestAgent/1.0", 0)
	cr.AllowLoopback = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pageData, err := cr.CrawlURL(ctx, server.URL)
	if err != nil {
		t.Fatalf("CrawlURL failed on invalid UTF-8: %v", err)
	}

	t.Logf("PASS: CrawlURL handled invalid UTF-8 bytes without panic")
	t.Logf("Extracted heading: %q", pageData.Headings[0].Text)
	t.Logf("Extracted body text: %q", pageData.BodyText)
	t.Logf("Word count: %d", pageData.WordCount)
}
