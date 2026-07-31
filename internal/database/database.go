package database

import (
	"fmt"
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
		dialector = sqlite.Open(path)
	} else if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		dialector = postgres.Open(databaseURL)
	} else {
		// Default to SQLite
		dialector = sqlite.Open("agent.db")
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
	return db.AutoMigrate(
		&Keyword{},
		&ContentIdea{},
		&ContentPiece{},
		&AgentLog{},
		&DailyCap{},
		&SchemaCache{},
		&Entity{},
	)
}

// Models

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
