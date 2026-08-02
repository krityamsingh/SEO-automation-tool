package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"aeo_geo_seo_agent/pkg/config"
	"aeo_geo_seo_agent/pkg/crawler"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/seo"
)

func TestSEOAudit_InvalidURL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	database.AutoMigrate(db)

	cr := crawler.New("kenerateai-bot", 2*time.Second)
	srv := New(db, nil, nil, &config.Config{}, cr, nil, nil, nil)

	// Test 1: Empty URL
	reqBody := `{"url": ""}`
	req := httptest.NewRequest("POST", "/api/seo/audit", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty URL, got %d", w.Code)
	}

	// Test 2: Invalid scheme/format
	reqBody2 := `{"url": "ftp://invalid-scheme.com"}`
	req2 := httptest.NewRequest("POST", "/api/seo/audit", strings.NewReader(reqBody2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid scheme, got %d", w2.Code)
	}
}

func TestSEOAudit_ValidURL(t *testing.T) {
	testHTML := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page Title for SEO Audit Testing</title>
    <meta name="description" content="This is a comprehensive meta description for testing the SEO audit engine endpoints in milestone 3.">
    <link rel="canonical" href="http://127.0.0.1/test">
    <meta property="og:title" content="Test OG Title">
</head>
<body>
    <h1>Main Heading One</h1>
    <h2>Subheading Two</h2>
    <p>This is a paragraph containing keyword density test data with keyword optimization and audit functions.</p>
</body>
</html>`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testHTML))
	}))
	defer mockServer.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	database.AutoMigrate(db)

	cr := crawler.New("kenerateai-bot", 2*time.Second)
	cr.AllowLoopback = true
	srv := New(db, nil, nil, &config.Config{}, cr, nil, nil, nil)

	reqBody := `{"url": "` + mockServer.URL + `"}`
	req := httptest.NewRequest("POST", "/api/seo/audit", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var report seo.AuditReport
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("failed to unmarshal AuditReport JSON: %v", err)
	}

	if report.OverallSEOScore <= 0 {
		t.Errorf("expected positive overall SEO score, got %d", report.OverallSEOScore)
	}
	if report.Title != "Test Page Title for SEO Audit Testing" {
		t.Errorf("unexpected title in audit report: %s", report.Title)
	}

	// Test GET /api/seo/audits
	reqAudits := httptest.NewRequest("GET", "/api/seo/audits", nil)
	wAudits := httptest.NewRecorder()
	srv.ServeHTTP(wAudits, reqAudits)

	if wAudits.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/seo/audits, got %d", wAudits.Code)
	}

	var audits []database.SEOAudit
	if err := json.Unmarshal(wAudits.Body.Bytes(), &audits); err != nil {
		t.Fatalf("failed to unmarshal audits list: %v", err)
	}

	if len(audits) == 0 {
		t.Errorf("expected at least 1 historical audit in DB, got 0")
	}
}

func TestTaskInstructions_Export_Toggle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	database.AutoMigrate(db)

	sampleTask := database.Task{
		Keyword:            "Model Context Protocol",
		BacklinkTarget:     "dev.to",
		TargetAnchorText:   "Model Context Protocol guide",
		TargetLinkURL:      "https://dev.to/mcp-guide",
		Title:              "Model Context Protocol: Comprehensive GEO Guide",
		ArticleDraft:       "# Model Context Protocol Implementation\n\nFull article body content...",
		ExecutionGuide:     "### Step 1: Register Account\nRegister on dev.to\n### Step 2: Publish Article\nPublish article draft",
		Status:             "assigned",
		AssignedInternName: "anuj",
		CreatedAt:          time.Now(),
	}
	db.Create(&sampleTask)

	taskIDStr := strconv.FormatUint(uint64(sampleTask.ID), 10)

	cr := crawler.New("kenerateai-bot", 2*time.Second)
	cr.AllowLoopback = true
	srv := New(db, nil, nil, &config.Config{}, cr, nil, nil, nil)

	// 1. GET /api/tasks/:id/instructions
	reqInst := httptest.NewRequest("GET", "/api/tasks/"+taskIDStr+"/instructions", nil)
	wInst := httptest.NewRecorder()
	srv.ServeHTTP(wInst, reqInst)

	if wInst.Code != http.StatusOK {
		t.Fatalf("expected status 200 for instructions, got %d: %s", wInst.Code, wInst.Body.String())
	}

	var instMap map[string]interface{}
	if err := json.Unmarshal(wInst.Body.Bytes(), &instMap); err != nil {
		t.Fatalf("failed to parse instructions JSON: %v", err)
	}

	if instMap["article_draft"] != sampleTask.ArticleDraft {
		t.Errorf("article_draft mismatch in instructions response")
	}
	if instMap["target_anchor_text"] != sampleTask.TargetAnchorText {
		t.Errorf("target_anchor_text mismatch: got %v", instMap["target_anchor_text"])
	}

	// 2. GET /api/tasks/:id/export (Markdown format)
	reqExpMd := httptest.NewRequest("GET", "/api/tasks/"+taskIDStr+"/export?format=md", nil)
	wExpMd := httptest.NewRecorder()
	srv.ServeHTTP(wExpMd, reqExpMd)

	if wExpMd.Code != http.StatusOK {
		t.Fatalf("expected 200 for export md, got %d", wExpMd.Code)
	}
	if !strings.Contains(wExpMd.Header().Get("Content-Type"), "text/markdown") {
		t.Errorf("expected Content-Type text/markdown, got %s", wExpMd.Header().Get("Content-Type"))
	}
	if !strings.Contains(wExpMd.Header().Get("Content-Disposition"), "attachment; filename=task-"+taskIDStr+"-article.md") {
		t.Errorf("unexpected Content-Disposition: %s", wExpMd.Header().Get("Content-Disposition"))
	}
	if wExpMd.Body.String() != sampleTask.ArticleDraft {
		t.Errorf("exported content mismatch")
	}

	// 3. GET /api/tasks/:id/export (Text format)
	reqExpTxt := httptest.NewRequest("GET", "/api/tasks/"+taskIDStr+"/export?format=txt", nil)
	wExpTxt := httptest.NewRecorder()
	srv.ServeHTTP(wExpTxt, reqExpTxt)

	if wExpTxt.Code != http.StatusOK {
		t.Fatalf("expected 200 for export txt, got %d", wExpTxt.Code)
	}
	if !strings.Contains(wExpTxt.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %s", wExpTxt.Header().Get("Content-Type"))
	}

	// 4. POST /api/tasks/:id/steps/:step_id/toggle (Toggle Step 1 -> completed)
	reqToggle1 := httptest.NewRequest("POST", "/api/tasks/"+taskIDStr+"/steps/1/toggle", nil)
	wToggle1 := httptest.NewRecorder()
	srv.ServeHTTP(wToggle1, reqToggle1)

	if wToggle1.Code != http.StatusOK {
		t.Fatalf("expected 200 for toggle step 1, got %d: %s", wToggle1.Code, wToggle1.Body.String())
	}

	var toggleRes map[string]interface{}
	json.Unmarshal(wToggle1.Body.Bytes(), &toggleRes)

	if toggleRes["completed"] != true {
		t.Errorf("expected step 1 to be completed: true, got %v", toggleRes["completed"])
	}

	// Toggle Step 1 again -> uncheck
	reqToggle2 := httptest.NewRequest("POST", "/api/tasks/"+taskIDStr+"/steps/1/toggle", nil)
	wToggle2 := httptest.NewRecorder()
	srv.ServeHTTP(wToggle2, reqToggle2)

	if wToggle2.Code != http.StatusOK {
		t.Fatalf("expected 200 for toggle step 1 uncheck, got %d", wToggle2.Code)
	}
	json.Unmarshal(wToggle2.Body.Bytes(), &toggleRes)
	if toggleRes["completed"] != false {
		t.Errorf("expected step 1 to be completed: false after toggle, got %v", toggleRes["completed"])
	}

	// Test 404 for non-existent task
	req404 := httptest.NewRequest("GET", "/api/tasks/999/instructions", nil)
	w404 := httptest.NewRecorder()
	srv.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing task, got %d", w404.Code)
	}
}
