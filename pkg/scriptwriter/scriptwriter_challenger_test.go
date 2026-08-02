package scriptwriter_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/scriptwriter"
)

// TestScriptwriter_EdgeCase_EmptyTopicAndKeyword tests behavior when both topic and keyword are empty.
func TestScriptwriter_EdgeCase_EmptyTopicAndKeyword(t *testing.T) {
	ctx := context.Background()
	topic := ""
	keyword := ""
	targetURL := "https://example.com/target"
	anchorText := ""

	draft, err := scriptwriter.GenerateFullArticleDraft(ctx, topic, keyword, targetURL, anchorText)
	if err != nil {
		t.Fatalf("GenerateFullArticleDraft failed with empty topic/keyword: %v", err)
	}

	if draft == "" {
		t.Fatal("expected non-empty fallback draft even with empty inputs")
	}

	// Verify word count is still >= 1000
	words := strings.Fields(draft)
	if len(words) < 1000 {
		t.Errorf("expected draft word count >= 1000, got %d", len(words))
	}

	// Check if markdown link syntax is corrupted: e.g. "[]("
	if strings.Contains(draft, "[](") {
		t.Errorf("found empty anchor text markdown link '[](' in draft")
	}
}

// TestScriptwriter_EdgeCase_LongKeywordsAndSpecialChars tests handling of 1000+ char keywords and HTML tags.
func TestScriptwriter_EdgeCase_LongKeywordsAndSpecialChars(t *testing.T) {
	ctx := context.Background()
	longKeyword := strings.Repeat("UltraLongKeywordPhrase ", 50) + " <script>alert(1)</script> & special % chars"
	topic := "Scalable Vector Search"
	targetURL := "https://example.com/search"
	anchorText := "Vector Search Link"

	draft, err := scriptwriter.GenerateFullArticleDraft(ctx, topic, longKeyword, targetURL, anchorText)
	if err != nil {
		t.Fatalf("GenerateFullArticleDraft failed with long keyword: %v", err)
	}

	if !strings.Contains(draft, anchorText) {
		t.Errorf("expected draft to contain anchor text %q", anchorText)
	}

	// Check Markdown headings remain intact despite long keyword injection
	if !strings.Contains(draft, "# ") || !strings.Contains(draft, "## ") {
		t.Errorf("markdown heading structure missing")
	}
}

// TestScriptwriter_EdgeCase_SpecialCharactersInTargetURL tests URLs with parentheses, query strings, and quotes.
func TestScriptwriter_EdgeCase_SpecialCharactersInTargetURL(t *testing.T) {
	ctx := context.Background()
	topic := "API Security Best Practices"
	keyword := "API Security"
	anchorText := "Security Guidelines"

	testURLs := []string{
		"https://example.com/wiki/API_(security)?v=1&b=2#section",
		"https://example.com/path?query=\"quoted\"&tag=<script>",
		"https://sub.domain.example.org:8443/deep/path/index.html?param=value#anchor",
	}

	for _, targetURL := range testURLs {
		t.Run("URL_"+targetURL, func(t *testing.T) {
			draft, err := scriptwriter.GenerateFullArticleDraft(ctx, topic, keyword, targetURL, anchorText)
			if err != nil {
				t.Fatalf("GenerateFullArticleDraft failed for targetURL %q: %v", targetURL, err)
			}

			// Check link embedding
			expectedLink := fmt.Sprintf("[%s](%s)", anchorText, targetURL)
			if !strings.Contains(draft, expectedLink) {
				t.Errorf("expected draft to contain exact link %q", expectedLink)
			}
		})
	}
}

// TestScriptwriter_EdgeCase_FallbackWordCount verifies fallback draft meets word count requirement.
func TestScriptwriter_EdgeCase_FallbackWordCount(t *testing.T) {
	ctx := context.Background()
	writer := scriptwriter.New(nil, nil)

	draft, err := writer.GenerateFullArticleDraft(ctx, "Topic", "Keyword", "https://example.com", "Anchor")
	if err != nil {
		t.Fatalf("GenerateFullArticleDraft failed: %v", err)
	}

	words := strings.Fields(draft)
	if len(words) < 1000 || len(words) > 2000 {
		t.Errorf("fallback draft length must be between 1000 and 2000 words, got %d words", len(words))
	}
}

// TestScriptwriter_EdgeCase_InternExecutionGuideFormatting tests guide generation formatting under edge cases.
func TestScriptwriter_EdgeCase_InternExecutionGuideFormatting(t *testing.T) {
	ctx := context.Background()
	writer := scriptwriter.New(nil, nil)

	// Task with HTTP prefix already in BacklinkTarget
	task := &database.Task{
		ID:               99,
		Keyword:          "Edge Case Keyword",
		BacklinkTarget:   "https://already-prefixed-target.com",
		TargetAnchorText: "Exact Anchor",
		TargetLinkURL:    "https://already-prefixed-target.com/post/1",
		Title:            "Execution Guide Formatting Test",
	}

	guide, err := writer.GenerateInternExecutionGuide(ctx, task)
	if err != nil {
		t.Fatalf("GenerateInternExecutionGuide failed: %v", err)
	}

	// Check if URL formatting produces double https:// (e.g. https://https://)
	if strings.Contains(guide, "https://https://") {
		t.Errorf("found duplicate protocol prefix 'https://https://' in execution guide")
	}

	// Verify all 5 steps exist
	for i := 1; i <= 5; i++ {
		stepHeader := fmt.Sprintf("Step %d", i)
		if !strings.Contains(guide, stepHeader) {
			t.Errorf("missing %s in execution guide", stepHeader)
		}
	}
}
