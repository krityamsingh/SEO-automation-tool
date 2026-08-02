package rag_test

import (
	"strings"
	"testing"

	"aeo_geo_seo_agent/pkg/rag"
)

func TestIngestTaskContent(t *testing.T) {
	ragEngine := rag.New(nil)

	taskID := uint(101)
	title := "Generative Engine Optimization Strategies"
	articleDraft := "# Generative Engine Optimization\n\nGEO is the next generation of search engine optimization focusing on AI responses. Modern LLM models retrieve structured citations from authoritative sources."
	executionGuide := "Step 1: Register on platform\nStep 2: Place article\nStep 3: Insert anchor text\nStep 4: Publish\nStep 5: Submit proof"

	err := ragEngine.IngestTaskContent(taskID, title, articleDraft, executionGuide)
	if err != nil {
		t.Fatalf("IngestTaskContent returned unexpected error: %v", err)
	}

	if ragEngine.Size() == 0 {
		t.Fatal("expected RAG store to contain chunks after IngestTaskContent")
	}

	stats := ragEngine.GetStats()
	categoryCounts, ok := stats["category_counts"].(map[string]int)
	if !ok {
		t.Fatalf("expected category_counts in RAG stats, got %v", stats)
	}

	if categoryCounts["task_content"] == 0 {
		t.Errorf("expected task_content category count > 0, got %d", categoryCounts["task_content"])
	}

	// Retrieve context and confirm semantic match
	retrieved := ragEngine.RetrieveContext("Generative Engine Optimization", 3)
	if retrieved == "" {
		t.Fatal("expected non-empty retrieved context from RAG memory")
	}

	if !strings.Contains(retrieved, "Generative Engine Optimization") {
		t.Errorf("expected retrieved context to contain title/content, got %q", retrieved)
	}
}
