package task_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"aeo_geo_seo_agent/pkg/agent"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/rag"
	"aeo_geo_seo_agent/pkg/task"
)

// TestTaskEngine_NilRAGPanic demonstrates and verifies the panic vulnerability
// in RunOutcomeTrackingForTask when RAGEngine is nil.
func TestTaskEngine_NilRAGPanic(t *testing.T) {
	db, err := database.Connect("sqlite://file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}

	// Create TaskEngine with nil RAG engine
	taskEngine := task.NewTaskEngine(db, nil, nil, nil)

	// Create test task
	taskRecord := database.Task{
		Keyword:        "Nil RAG Test Keyword",
		BacklinkTarget: "example.com",
		Angle:          "SEO",
		Title:          "Test Title",
		Status:         "verified",
		RankCurrent:    10,
		RankPrevious:   15,
		CreatedAt:      time.Now(),
	}
	if err := db.Create(&taskRecord).Error; err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	panicked := false
	var panicVal interface{}

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				panicVal = r
			}
		}()
		ctx := context.Background()
		taskEngine.RunOutcomeTrackingForTask(ctx, taskRecord.ID)
	}()

	if panicked {
		t.Logf("CONFIRMED VULNERABILITY: RunOutcomeTrackingForTask panicked when te.rag is nil: %v", panicVal)
	} else {
		t.Log("RunOutcomeTrackingForTask handled nil te.rag safely without panic.")
	}
}

// TestTaskEngine_EdgeCase_EmptyDebateFields tests dispatch when debate result has empty keyword or target.
func TestTaskEngine_EdgeCase_EmptyDebateFields(t *testing.T) {
	db, err := database.Connect("sqlite://file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}

	ragEngine := rag.New(nil)
	taskEngine := task.NewTaskEngine(db, nil, nil, ragEngine)

	debate := &agent.DebateResult{
		Keyword:        "",
		BacklinkTarget: "",
		Angle:          "",
		Title:          "",
		DebateID:       10,
	}

	ctx := context.Background()
	taskRecord, err := taskEngine.DispatchAndAssignTask(ctx, debate)
	if err != nil {
		t.Fatalf("DispatchAndAssignTask failed with empty fields: %v", err)
	}

	if taskRecord.TargetAnchorText == "" {
		t.Logf("FINDING: Empty debate Keyword and Title result in empty TargetAnchorText.")
	}
	if taskRecord.TargetLinkURL == "" {
		t.Logf("FINDING: Empty debate BacklinkTarget results in empty TargetLinkURL.")
	}
}

// TestTaskEngine_EdgeCase_SpecialCharactersInBacklinkTarget tests URLs with double prefixes or spaces.
func TestTaskEngine_EdgeCase_SpecialCharactersInBacklinkTarget(t *testing.T) {
	db, err := database.Connect("sqlite://file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}

	ragEngine := rag.New(nil)
	taskEngine := task.NewTaskEngine(db, nil, nil, ragEngine)

	testCases := []struct {
		inputTarget  string
		expectedPref string
	}{
		{"http://already-http.com", "http://already-http.com"},
		{"https://already-https.com", "https://already-https.com"},
		{"raw-domain.com/path?a=1&b=2", "https://raw-domain.com/path?a=1&b=2"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("Case_%d", i), func(t *testing.T) {
			debate := &agent.DebateResult{
				Keyword:        "Test Keyword",
				BacklinkTarget: tc.inputTarget,
				Title:          "Test Title",
				DebateID:       uint(100 + i),
			}

			taskRecord, err := taskEngine.DispatchAndAssignTask(context.Background(), debate)
			if err != nil {
				t.Fatalf("DispatchAndAssignTask failed for %q: %v", tc.inputTarget, err)
			}

			if taskRecord.TargetLinkURL != tc.expectedPref {
				t.Errorf("expected TargetLinkURL %q, got %q", tc.expectedPref, taskRecord.TargetLinkURL)
			}
		})
	}
}

// TestTaskEngine_EdgeCase_BatchTaskDispatch tests sequential and concurrent batch dispatches of tasks.
func TestTaskEngine_EdgeCase_BatchTaskDispatch(t *testing.T) {
	db, err := database.Connect("sqlite://file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}

	// Create test interns
	interns := []database.User{
		{Username: "intern_alice", Email: "alice@test.com", Role: "intern", TasksCompleted: 10, TasksPending: 1, VerificationRate: 95.0},
		{Username: "intern_bob", Email: "bob@test.com", Role: "intern", TasksCompleted: 5, TasksPending: 0, VerificationRate: 90.0},
		{Username: "intern_charlie", Email: "charlie@test.com", Role: "intern", TasksCompleted: 2, TasksPending: 5, VerificationRate: 80.0},
	}
	for i := range interns {
		db.Create(&interns[i])
	}

	ragEngine := rag.New(nil)
	taskEngine := task.NewTaskEngine(db, nil, nil, ragEngine)

	const batchSize = 10
	var wg sync.WaitGroup
	errChan := make(chan error, batchSize)

	// Run concurrent dispatch
	for i := 1; i <= batchSize; i++ {
		wg.Add(1)
		go func(taskIdx int) {
			defer wg.Done()
			debate := &agent.DebateResult{
				Keyword:        fmt.Sprintf("Batch Keyword %d", taskIdx),
				BacklinkTarget: fmt.Sprintf("batch-target-%d.com", taskIdx),
				Angle:          "GEO Batch",
				Title:          fmt.Sprintf("Batch Article Title %d", taskIdx),
				DebateID:       uint(1000 + taskIdx),
			}

			_, err := taskEngine.DispatchAndAssignTask(context.Background(), debate)
			if err != nil {
				errChan <- fmt.Errorf("task %d dispatch failed: %w", taskIdx, err)
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	lockFailures := 0
	for err := range errChan {
		if strings.Contains(err.Error(), "database table is locked") {
			lockFailures++
		} else {
			t.Errorf("batch dispatch error: %v", err)
		}
	}

	if lockFailures > 0 {
		t.Logf("FINDING: SQLite concurrent write contention under batch auto-dispatching: %d / %d tasks failed due to 'database table is locked'.", lockFailures, batchSize)
	}

	// Check total tasks created
	var totalTasks int64
	db.Model(&database.Task{}).Count(&totalTasks)
	t.Logf("Total tasks successfully created during concurrent batch dispatch: %d / %d", totalTasks, batchSize)

	// Verify intern task assignment distribution
	var assignedTasks []database.Task
	db.Find(&assignedTasks)
	distribution := make(map[string]int)
	for _, tk := range assignedTasks {
		distribution[tk.AssignedInternName]++
	}
	t.Logf("Intern assignment load distribution under concurrent dispatch: %v", distribution)
}

// TestTaskEngine_EdgeCase_SSRFProtection tests SSRF guards in submission verification.
func TestTaskEngine_EdgeCase_SSRFProtection(t *testing.T) {
	db, err := database.Connect("sqlite://file::memory:?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to connect to test database: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}

	taskRecord := database.Task{
		Keyword:        "SSRF Test",
		BacklinkTarget: "example.com",
		Status:         "assigned",
	}
	db.Create(&taskRecord)

	taskEngine := task.NewTaskEngine(db, nil, nil, nil)
	ctx := context.Background()

	ssrfPayloads := []string{
		"http://127.0.0.1/admin",
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:8080/internal",
		"http://192.168.1.1/router",
		"http://10.0.0.1/secret",
		"file:///etc/passwd",
		"gopher://localhost:70/",
	}

	for _, payload := range ssrfPayloads {
		t.Run("Payload_"+payload, func(t *testing.T) {
			_, isVerified, notes, err := taskEngine.VerifySubmission(ctx, taskRecord.ID, payload)
			if err != nil {
				t.Fatalf("VerifySubmission error: %v", err)
			}
			if isVerified {
				t.Errorf("SSRF payload %q was illegally verified!", payload)
			}
			if !strings.Contains(notes, "Rejection") {
				t.Errorf("expected rejection note for SSRF payload %q, got %q", payload, notes)
			}
		})
	}
}
