package scriptwriter_test

import (
	"context"
	"strings"
	"testing"

	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/scriptwriter"
)

func TestGenerateFullArticleDraft(t *testing.T) {
	ctx := context.Background()
	topic := "Generative Engine Optimization (GEO)"
	keyword := "Generative Engine Optimization"
	targetURL := "https://dev.to/geoguide"
	anchorText := "GEO Best Practices"

	draft, err := scriptwriter.GenerateFullArticleDraft(ctx, topic, keyword, targetURL, anchorText)
	if err != nil {
		t.Fatalf("GenerateFullArticleDraft returned unexpected error: %v", err)
	}

	if draft == "" {
		t.Fatal("expected non-empty article draft")
	}

	// Verify word count meets 1000+ words criteria
	words := strings.Fields(draft)
	if len(words) < 1000 {
		t.Errorf("expected article draft to have at least 1000 words, got %d words", len(words))
	}

	// Verify Markdown structure (headings)
	if !strings.Contains(draft, "# ") || !strings.Contains(draft, "## ") {
		t.Errorf("expected markdown headings (# and ##) in article draft")
	}

	// Verify embedded target link with anchor text
	expectedLink := "[" + anchorText + "](" + targetURL + ")"
	if !strings.Contains(draft, expectedLink) {
		t.Errorf("expected article draft to contain embedded link %q", expectedLink)
	}
}

func TestGenerateInternExecutionGuide(t *testing.T) {
	ctx := context.Background()
	task := &database.Task{
		ID:               42,
		Keyword:          "Model Context Protocol",
		BacklinkTarget:   "medium.com",
		TargetAnchorText: "MCP protocol reference",
		TargetLinkURL:    "https://medium.com/@dev/mcp-guide",
		Title:            "Model Context Protocol: Comprehensive Integration Guide",
	}

	guide, err := scriptwriter.GenerateInternExecutionGuide(ctx, task)
	if err != nil {
		t.Fatalf("GenerateInternExecutionGuide returned unexpected error: %v", err)
	}

	if guide == "" {
		t.Fatal("expected non-empty intern execution guide")
	}

	// Verify step-by-step instructions details
	requiredKeywords := []string{
		"Step 1", "Account Registration",
		"Step 2", "Article Draft Placement",
		"Step 3", "Anchor Text Insertion",
		"Step 4", "Publication",
		"Step 5", "Proof Submission",
		task.TargetAnchorText,
		task.TargetLinkURL,
	}

	for _, kw := range requiredKeywords {
		if !strings.Contains(guide, kw) {
			t.Errorf("expected execution guide to contain %q", kw)
		}
	}
}

func TestGenerateInternExecutionGuide_NilTask(t *testing.T) {
	ctx := context.Background()
	_, err := scriptwriter.GenerateInternExecutionGuide(ctx, nil)
	if err == nil {
		t.Fatal("expected error when passing nil task, got nil")
	}
}
