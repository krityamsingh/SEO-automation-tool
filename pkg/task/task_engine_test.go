package task_test

import (
	"context"
	"strings"
	"testing"

	"aeo_geo_seo_agent/pkg/agent"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/rag"
	"aeo_geo_seo_agent/pkg/task"
)

func TestDispatchAndAssignTask(t *testing.T) {
	db, err := database.Connect("sqlite://file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}

	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate test database: %v", err)
	}

	ragEngine := rag.New(nil)
	taskEngine := task.NewTaskEngine(db, nil, nil, ragEngine)

	debate := &agent.DebateResult{
		Keyword:        "Model Context Protocol integration",
		BacklinkTarget: "dev.to",
		Angle:          "GEO",
		Title:          "Model Context Protocol: Architectural Best Practices for AI Agents",
		BlogDraft:      "Blog draft snippet...",
		SocialDraft:    "Social snippet...",
		Consensus:      true,
		DebateID:       1,
	}

	ctx := context.Background()
	taskRecord, err := taskEngine.DispatchAndAssignTask(ctx, debate)
	if err != nil {
		t.Fatalf("DispatchAndAssignTask returned unexpected error: %v", err)
	}

	if taskRecord == nil {
		t.Fatal("expected non-nil task record")
	}

	if taskRecord.ID == 0 {
		t.Errorf("expected task ID to be populated by DB, got 0")
	}

	if taskRecord.TargetAnchorText == "" {
		t.Errorf("expected TargetAnchorText to be set")
	}

	if !strings.HasPrefix(taskRecord.TargetLinkURL, "http://") && !strings.HasPrefix(taskRecord.TargetLinkURL, "https://") {
		t.Errorf("expected TargetLinkURL to have valid http/https scheme, got %q", taskRecord.TargetLinkURL)
	}

	if taskRecord.ArticleDraft == "" {
		t.Errorf("expected ArticleDraft to be generated")
	}

	words := strings.Fields(taskRecord.ArticleDraft)
	if len(words) < 1000 {
		t.Errorf("expected ArticleDraft to be full-length (>=1000 words), got %d words", len(words))
	}

	if taskRecord.ExecutionGuide == "" {
		t.Errorf("expected ExecutionGuide to be generated")
	}

	if !strings.Contains(taskRecord.ExecutionGuide, "Step 1") || !strings.Contains(taskRecord.ExecutionGuide, "Step 5") {
		t.Errorf("expected ExecutionGuide to contain step-by-step instructions")
	}

	// Verify database persistence
	var fetched database.Task
	if err := db.First(&fetched, taskRecord.ID).Error; err != nil {
		t.Fatalf("failed to fetch task from database: %v", err)
	}

	if fetched.ArticleDraft != taskRecord.ArticleDraft {
		t.Errorf("persisted ArticleDraft mismatch")
	}

	if fetched.ExecutionGuide != taskRecord.ExecutionGuide {
		t.Errorf("persisted ExecutionGuide mismatch")
	}

	// Verify RAG memory indexing
	stats := ragEngine.GetStats()
	categoryCounts, ok := stats["category_counts"].(map[string]int)
	if !ok || categoryCounts["task_content"] == 0 {
		t.Errorf("expected RAG memory to index task_content, stats: %v", stats)
	}
}
