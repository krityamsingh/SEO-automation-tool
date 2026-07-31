package chat

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/rag"
)

type ChatAssistant struct {
	db     *gorm.DB
	gemini *ai.GeminiClient
	rag    *rag.RAGEngine
}

func NewChatAssistant(db *gorm.DB, gemini *ai.GeminiClient, ragEngine *rag.RAGEngine) *ChatAssistant {
	return &ChatAssistant{
		db:     db,
		gemini: gemini,
		rag:    ragEngine,
	}
}

type ChatRequest struct {
	TaskID  uint   `json:"task_id"`
	Message string `json:"message"`
}

type ChatResponse struct {
	Answer   string   `json:"answer"`
	Sources  []string `json:"sources,omitempty"`
	TaskID   uint     `json:"task_id"`
}

// AnswerInternQuestion uses task context & RAG memory to answer intern questions
func (ca *ChatAssistant) AnswerInternQuestion(ctx context.Context, internID uint, req ChatRequest) (*ChatResponse, error) {
	slog.Info("INTERN CHAT: processing query", "intern_id", internID, "task_id", req.TaskID, "query", req.Message)

	taskContext := ""
	var task database.Task
	if req.TaskID > 0 {
		if err := ca.db.First(&task, req.TaskID).Error; err == nil {
			taskContext = fmt.Sprintf(`[TASK CONTEXT]
Task #%d: Keyword: "%s" | Target Site: "%s" | Angle: %s | Status: %s
Title Draft: "%s"
Rejection/Verification Notes: %s
Blog Draft snippet: %s`, task.ID, task.Keyword, task.BacklinkTarget, task.Angle, task.Status, task.Title, task.VerificationNotes, truncateStr(task.BlogDraft, 300))
		}
	}

	// Retrieve context from system-wide RAG memory
	ragContext := ca.rag.RetrieveContext(req.Message+" "+task.Keyword+" "+task.BacklinkTarget, 5)

	// Build comprehensive prompt for AI Intern Advisor
	prompt := fmt.Sprintf(`[ROLE: Senior AI SEO & Backlink Coach for kenerateai.com Interns]
You are advising an intern working on backlink creation & content publishing for kenerateai.com.

%s

%s

Intern's Question: "%s"

Provide a direct, friendly, and practical answer explaining step-by-step how the intern can complete this task or fix any rejection reason. Keep it encouraging, technical, and concise (under 250 words).`, taskContext, ragContext, req.Message)

	answerText, err := ca.gemini.GenerateText(ctx, prompt, 0.6, 2048)
	if err != nil {
		slog.Warn("INTERN CHAT: fallback response used", "error", err)
		answerText = fmt.Sprintf("To complete Task #%d for keyword '%s' on '%s': 1. Review the guest post draft provided in your task panel. 2. Visit %s and reach out to the editor or submit via their community form. 3. Copy the published live URL and submit it into your task proof box.", task.ID, task.Keyword, task.BacklinkTarget, task.BacklinkTarget)
	}

	// Store chat exchange in RAG store for future retrieval
	ca.rag.IngestWithMetadata(
		fmt.Sprintf("chat-task-%d", req.TaskID),
		fmt.Sprintf("Intern Q&A on Task #%d", req.TaskID),
		fmt.Sprintf("Question: %s\nAnswer: %s", req.Message, answerText),
		"chat",
		task.Keyword,
		task.BacklinkTarget,
		map[string]string{"task_id": fmt.Sprintf("%d", req.TaskID)},
	)

	return &ChatResponse{
		Answer:  answerText,
		TaskID:  req.TaskID,
		Sources: []string{"kenerateai Shared RAG Memory", "Agent Strategy Directives"},
	}, nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
