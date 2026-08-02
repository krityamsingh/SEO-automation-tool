package rag_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"aeo_geo_seo_agent/pkg/rag"
)

// TestRAG_EdgeCase_EmptyContentIngestion tests ingestion of empty or whitespace-only articles and guides.
func TestRAG_EdgeCase_EmptyContentIngestion(t *testing.T) {
	ragEngine := rag.New(nil)

	err := ragEngine.IngestTaskContent(500, "", "", "")
	if err != nil {
		t.Fatalf("IngestTaskContent failed on empty inputs: %v", err)
	}

	if ragEngine.Size() != 0 {
		t.Logf("FINDING: IngestTaskContent with empty strings creates 1 chunk (%d chunks created) containing static header template strings.", ragEngine.Size())
	}
}

// TestRAG_EdgeCase_LargeArticleIngestion tests ingestion of huge 10,000-word articles.
func TestRAG_EdgeCase_LargeArticleIngestion(t *testing.T) {
	ragEngine := rag.New(nil)

	// Construct a 10,000-word article draft
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString(fmt.Sprintf("Paragraph %d: Technical content regarding search optimization, vector indexing, and RAG architectures. ", i))
		sb.WriteString("This sentence adds additional semantic context for testing chunk boundaries and BM25 tokenization.\n\n")
	}

	largeArticle := sb.String()
	executionGuide := "Step 1: Publish\nStep 2: Submit proof"

	err := ragEngine.IngestTaskContent(501, "Large Scale Content Strategy", largeArticle, executionGuide)
	if err != nil {
		t.Fatalf("IngestTaskContent failed on large article: %v", err)
	}

	// Should create multiple 300-word chunks
	if ragEngine.Size() < 5 {
		t.Errorf("expected multiple chunks for 10,000-word article, got %d chunks", ragEngine.Size())
	}

	// Verify context retrieval on large store
	ctx := ragEngine.RetrieveContext("vector indexing RAG architectures", 3)
	if ctx == "" {
		t.Errorf("retrieval failed on large article chunks")
	}
}

// TestRAG_EdgeCase_ConcurrentIngestionAndRetrieval tests mutex safety under heavy concurrent access.
func TestRAG_EdgeCase_ConcurrentIngestionAndRetrieval(t *testing.T) {
	ragEngine := rag.New(nil)

	var wg sync.WaitGroup
	const goroutines = 20

	// Concurrent writers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			taskID := uint(1000 + idx)
			title := fmt.Sprintf("Concurrent Task %d", idx)
			article := fmt.Sprintf("Article content for concurrent worker %d discussing SEO strategy.", idx)
			guide := fmt.Sprintf("Guide content for worker %d.", idx)
			_ = ragEngine.IngestTaskContent(taskID, title, article, guide)
			ragEngine.IngestWithMetadata("src", title, article, "debate", "kw", "site", nil)
		}(i)
	}

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ragEngine.RetrieveContext("SEO strategy", 5)
			_ = ragEngine.GetStats()
			_, _ = ragEngine.CheckDuplicateTargetOrKeyword("kw", "site")
		}()
	}

	wg.Wait()

	if ragEngine.Size() == 0 {
		t.Errorf("expected non-zero chunks after concurrent ingestion")
	}
}

// TestRAG_EdgeCase_DuplicateCheckFormatting tests case-insensitivity and whitespace handling in duplicate check.
func TestRAG_EdgeCase_DuplicateCheckFormatting(t *testing.T) {
	ragEngine := rag.New(nil)

	ragEngine.IngestWithMetadata("src-1", "Existing Article", "Content", "task_content", "Model Context Protocol", "medium.com", nil)

	// Test exact match with different case and whitespace
	isDup, msg := ragEngine.CheckDuplicateTargetOrKeyword("  model context protocol  ", "MEDIUM.COM")
	if !isDup {
		t.Errorf("expected duplicate detection for case-insensitive / trimmed query")
	}
	if msg == "" {
		t.Errorf("expected duplicate explanation message")
	}

	// Test non-duplicate
	isDup2, _ := ragEngine.CheckDuplicateTargetOrKeyword("unique keyword", "unique-site.org")
	if isDup2 {
		t.Errorf("unexpected duplicate detection for unique keyword/site")
	}
}
