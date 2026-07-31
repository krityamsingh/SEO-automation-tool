package database

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Connect(databaseURL string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	if strings.HasPrefix(databaseURL, "sqlite://") {
		path := strings.TrimPrefix(databaseURL, "sqlite://")
		if !strings.Contains(path, "?") {
			path += "?_journal_mode=WAL&_busy_timeout=5000"
		}
		dialector = sqlite.Open(path)
	} else if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		dialector = postgres.Open(databaseURL)
	} else {
		// Default to SQLite with WAL mode (/tmp/agent.db on Vercel serverless)
		dbPath := "agent.db"
		if os.Getenv("VERCEL") != "" {
			dbPath = "/tmp/agent.db"
		}
		dialector = sqlite.Open(dbPath + "?_journal_mode=WAL&_busy_timeout=5000")
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	DB = db

	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&Keyword{},
		&ContentIdea{},
		&ContentPiece{},
		&AgentLog{},
		&DailyCap{},
		&SchemaCache{},
		&Entity{},
		&User{},
		&Task{},
		&AgentDebate{},
		&Notification{},
		&RankHistory{},
	)
	if err == nil {
		SeedDefaultData(db)
	}
	return err
}

func SeedDefaultData(db *gorm.DB) {
	// Seed default users if empty
	var userCount int64
	db.Model(&User{}).Count(&userCount)
	if userCount == 0 {
		devUser := User{Username: "dev_admin", Email: "dev@kenerateai.com", Role: "dev", PasswordHash: "admin123", VerificationRate: 100}
		db.Create(&devUser)
		interns := []string{"anu", "master", "hirtik", "anuj"}
		for _, name := range interns {
			u := User{Username: name, Email: name + "@kenerateai.com", Role: "intern", PasswordHash: "intern123", VerificationRate: 100}
			db.Create(&u)
		}
	}

	// Seed default assigned task if empty so tasks are never lost on serverless cold starts
	var taskCount int64
	db.Model(&Task{}).Count(&taskCount)
	if taskCount == 0 {
		var internAnuj User
		db.Where("username = ?", "anuj").First(&internAnuj)
		var assignedID *uint
		if internAnuj.ID > 0 {
			assignedID = &internAnuj.ID
		}
		sampleTask := Task{
			Keyword:            "Model Context Protocol implementation",
			BacklinkTarget:     "dev.to",
			Angle:              "GEO",
			Title:              "Model Context Protocol Implementation: The Backbone of Generative Engine Optimization (GEO)",
			BlogDraft:          "Generative Engine Optimization (GEO) is rapidly redefining how brands maintain visibility in an AI-first search landscape. As search engines evolve from traditional link indexing to generative responses, a robust Model Context Protocol implementation has become the gold standard for technical GEO.",
			SocialDraft:        "Is your brand ready for the shift from SEO to Generative Engine Optimization (GEO)? 🚀",
			AssignedInternID:   assignedID,
			AssignedInternName: "anuj",
			Status:             "assigned",
			CreatedAt:          time.Now(),
		}
		db.Create(&sampleTask)
	}
}

// Models

type User struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Username         string    `gorm:"uniqueIndex;not null" json:"username"`
	Email            string    `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash     string    `json:"-"`
	Role             string    `gorm:"not null" json:"role"` // dev, intern
	TasksCompleted   int       `gorm:"default:0" json:"tasks_completed"`
	TasksPending     int       `gorm:"default:0" json:"tasks_pending"`
	TasksOverdue     int       `gorm:"default:0" json:"tasks_overdue"`
	VerificationRate float64   `gorm:"default:100.0" json:"verification_rate"`
	CreatedAt        time.Time `json:"created_at"`
	LastLogin        *time.Time`json:"last_login"`
}

type Task struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	Keyword           string     `gorm:"not null;index" json:"keyword"`
	BacklinkTarget    string     `gorm:"not null" json:"backlink_target"`
	Angle             string     `json:"angle"` // SEO, AEO, GEO
	Title             string     `json:"title"`
	BlogDraft         string     `gorm:"type:text" json:"blog_draft"`
	SocialDraft       string     `gorm:"type:text" json:"social_draft"`
	AssignedInternID  *uint      `gorm:"index" json:"assigned_intern_id"`
	AssignedInternName string    `json:"assigned_intern_name"`
	Status            string     `gorm:"default:proposed;index" json:"status"` // proposed, ready, assigned, in_progress, submitted, verified, rejected, closed
	SubmittedProofURL string     `json:"submitted_proof_url"`
	VerificationNotes string     `gorm:"type:text" json:"verification_notes"`
	RankCurrent       int        `gorm:"default:0" json:"rank_current"`
	RankPrevious      int        `gorm:"default:0" json:"rank_previous"`
	FlaggedForDev     bool       `gorm:"default:false" json:"flagged_for_dev"`
	FlagReason        string     `json:"flag_reason"`
	DebateID          *uint      `gorm:"index" json:"debate_id"`
	CreatedAt         time.Time  `json:"created_at"`
	SubmittedAt       *time.Time `json:"submitted_at"`
	VerifiedAt        *time.Time `json:"verified_at"`
}

type AgentDebate struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	TaskID           *uint     `gorm:"index" json:"task_id"`
	Keyword          string    `gorm:"index" json:"keyword"`
	BacklinkTarget   string    `json:"backlink_target"`
	Status           string    `json:"status"` // debating, consensus, disagreement_flagged
	DebateTranscript string    `gorm:"type:text" json:"debate_transcript"` // JSON array of messages
	FinalDecision    string    `gorm:"type:text" json:"final_decision"`
	RoundsCount      int       `json:"rounds_count"`
	CreatedAt        time.Time `json:"created_at"`
}

type Notification struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	UserRole  string    `json:"user_role"` // dev, intern
	Title     string    `json:"title"`
	Message   string    `gorm:"type:text" json:"message"`
	Type      string    `json:"type"` // task_assigned, submission_verified, submission_rejected, debate_disagreement, rank_drop, task_overdue
	Read      bool      `gorm:"default:false" json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

type RankHistory struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TaskID       uint      `gorm:"index" json:"task_id"`
	Keyword      string    `gorm:"index" json:"keyword"`
	RankPosition int       `json:"rank_position"`
	TrafficScore float64   `json:"traffic_score"`
	CheckedAt    time.Time `json:"checked_at"`
}

type Keyword struct {
	ID                   uint           `gorm:"primaryKey" json:"id"`
	Keyword              string         `gorm:"not null;index" json:"keyword"`
	Niche                string         `gorm:"not null" json:"niche"`
	SearchVolumeEstimate string         `json:"search_volume_estimate"`
	CompetitionEstimate  string         `json:"competition_estimate"`
	TrendScore           int            `json:"trend_score"`
	CreatedAt            time.Time      `json:"created_at"`
	LastUsed             *time.Time     `json:"last_used"`
}

type ContentIdea struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	KeywordID    uint       `gorm:"index" json:"keyword_id"`
	Keyword      Keyword    `gorm:"foreignKey:KeywordID" json:"keyword,omitempty"`
	Title        string     `gorm:"not null" json:"title"`
	ContentType  string     `json:"content_type"` // blog, video, social, email
	SEOScore     int        `json:"seo_score"`
	AEOScore     int        `json:"aeo_score"`
	GEOScore     int        `json:"geo_score"`
	Status       string     `gorm:"default:pending" json:"status"` // pending, approved, rejected, created
	CreatedAt    time.Time  `json:"created_at"`
}

type ContentPiece struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	IdeaID           uint       `gorm:"index" json:"idea_id"`
	Idea             ContentIdea `gorm:"foreignKey:IdeaID" json:"idea,omitempty"`
	Title            string     `gorm:"not null" json:"title"`
	Body             string     `gorm:"type:text" json:"body"`
	MetaDescription  string     `json:"meta_description"`
	SchemaMarkup     string     `gorm:"type:text" json:"schema_markup"`
	FAQSection       string     `gorm:"type:text" json:"faq_section"`
	TLDR             string     `json:"tldr"`
	SocialVariants   string     `gorm:"type:text" json:"social_variants"`
	VideoScript      string     `gorm:"type:text" json:"video_script"`
	Status           string     `gorm:"default:draft" json:"status"` // draft, pending_review, published, failed
	PublishedURL     string     `json:"published_url"`
	Platform         string     `json:"platform"` // wordpress, medium, ghost, webhook
	CreatedAt        time.Time  `json:"created_at"`
	PublishedAt      *time.Time `json:"published_at"`
}

type AgentLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Phase     string    `json:"phase"`   // keyword_research, content_idea, content_creation, optimization, publishing
	Status    string    `json:"status"`  // success, failed
	Details   string    `gorm:"type:text" json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

type DailyCap struct {
	Date          time.Time `gorm:"primaryKey" json:"date"`
	ContentCreated int       `json:"content_created"`
	GeminiCalls    int       `json:"gemini_calls"`
}

type SchemaCache struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	URL       string    `gorm:"index" json:"url"`
	SchemaType string   `json:"schema_type"` // FAQPage, HowTo, Organization, Article
	Markup    string    `gorm:"type:text" json:"markup"`
	CreatedAt time.Time `json:"created_at"`
}

type Entity struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"not null;index" json:"name"`
	Type        string    `json:"type"` // person, organization, product, place
	Description string    `gorm:"type:text" json:"description"`
	WikidataID  string    `json:"wikidata_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// Helpers

func LogAgentActivity(db *gorm.DB, phase, status, details string) {
	log := &AgentLog{
		Phase:     phase,
		Status:    status,
		Details:   details,
		CreatedAt: time.Now(),
	}
	db.Create(log)
}

func CheckDailyCap(db *gorm.DB, limitContent, limitGemini int) (bool, string) {
	today := time.Now().Truncate(24 * time.Hour)
	var cap DailyCap
	result := db.FirstOrCreate(&cap, DailyCap{Date: today})
	if result.Error != nil {
		return false, "database error checking daily cap"
	}

	if cap.ContentCreated >= limitContent {
		return false, fmt.Sprintf("daily content limit reached: %d/%d", cap.ContentCreated, limitContent)
	}
	if cap.GeminiCalls >= limitGemini {
		return false, fmt.Sprintf("daily Gemini call limit reached: %d/%d", cap.GeminiCalls, limitGemini)
	}
	return true, ""
}

func IncrementContentCap(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		today := time.Now().Truncate(24 * time.Hour)
		var cap DailyCap
		if err := tx.FirstOrCreate(&cap, DailyCap{Date: today}).Error; err != nil {
			return err
		}
		return tx.Model(&cap).UpdateColumn("content_created", gorm.Expr("content_created + 1")).Error
	})
}

func IncrementGeminiCap(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		today := time.Now().Truncate(24 * time.Hour)
		var cap DailyCap
		if err := tx.FirstOrCreate(&cap, DailyCap{Date: today}).Error; err != nil {
			return err
		}
		return tx.Model(&cap).UpdateColumn("gemini_calls", gorm.Expr("gemini_calls + 1")).Error
	})
}

func GetDailyUsage(db *gorm.DB) (int, int) {
	today := time.Now().Truncate(24 * time.Hour)
	var cap DailyCap
	db.FirstOrCreate(&cap, DailyCap{Date: today})
	return cap.ContentCreated, cap.GeminiCalls
}
