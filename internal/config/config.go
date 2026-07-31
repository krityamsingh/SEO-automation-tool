package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// AI
	GeminiAPIKey      string
	GeminiTextModel   string
	GeminiImageModel  string

	// Agent Behavior
	AgentNiches        []string
	AgentCycleHours    time.Duration
	ContentAutoPublish bool
	ContentMinWords    int
	ContentMaxWords    int

	// Publishing
	WordPressURL          string
	WordPressUsername     string
	WordPressAppPassword  string
	MediumIntegrationToken string
	GhostURL              string
	GhostAdminAPIKey      string
	WebhookURL            string

	// Database
	DatabaseURL string

	// Limits
	DailyContentLimit int
	DailyGeminiLimit  int

	// Crawler
	UserAgent   string
	CrawlDelay  time.Duration
	CrawlTimeout time.Duration
	MaxDepth    int
	MaxPages    int

	// Server
	Port     string
	LogLevel *slog.LevelVar
	APIKey   string
}

func Load() *Config {
	_ = godotenv.Load()

	cfg := &Config{
		GeminiAPIKey:      getAPIKey(),
		GeminiTextModel:   getDefault("GEMINI_TEXT_MODEL", "gemini-flash-latest"),
		GeminiImageModel:  getDefault("GEMINI_IMAGE_MODEL", "gemini-2.5-flash-image"),
		AgentNiches:       split(getDefault("AGENT_NICHES", "technology,saas,ai")),
		AgentCycleHours:   parseDuration(getDefault("AGENT_CYCLE_HOURS", "6")) * time.Hour,
		ContentAutoPublish: parseBool(getDefault("CONTENT_AUTO_PUBLISH", "false")),
		ContentMinWords:   parseInt(getDefault("CONTENT_MIN_WORDS", "1500")),
		ContentMaxWords:   parseInt(getDefault("CONTENT_MAX_WORDS", "3000")),
		WordPressURL:      os.Getenv("WORDPRESS_URL"),
		WordPressUsername: os.Getenv("WORDPRESS_USERNAME"),
		WordPressAppPassword: os.Getenv("WORDPRESS_APP_PASSWORD"),
		MediumIntegrationToken: os.Getenv("MEDIUM_INTEGRATION_TOKEN"),
		GhostURL:          os.Getenv("GHOST_URL"),
		GhostAdminAPIKey:  os.Getenv("GHOST_ADMIN_API_KEY"),
		WebhookURL:        os.Getenv("WEBHOOK_URL"),
		DatabaseURL:       getDefault("DATABASE_URL", "sqlite://agent.db"),
		DailyContentLimit: parseInt(getDefault("DAILY_CONTENT_LIMIT", "5")),
		DailyGeminiLimit:  parseInt(getDefault("DAILY_GEMINI_LIMIT", "200")),
		UserAgent:         getDefault("USER_AGENT", "AEOAgent/1.0"),
		CrawlDelay:        parseDuration(getDefault("CRAWL_DELAY", "1")) * time.Second,
		CrawlTimeout:      parseDuration(getDefault("CRAWL_TIMEOUT", "30")) * time.Second,
		MaxDepth:          parseInt(getDefault("MAX_DEPTH", "3")),
		MaxPages:          parseInt(getDefault("MAX_PAGES", "100")),
		Port:              getDefault("PORT", "8080"),
		LogLevel:          parseLevel(getDefault("LOG_LEVEL", "info")),
		APIKey:            os.Getenv("API_KEY"),
	}

	return cfg
}

func require(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("required env var missing: " + key)
	}
	return v
}

func getAPIKey() string {
	if keys := os.Getenv("GEMINI_API_KEYS"); keys != "" {
		return keys
	}
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		return key
	}
	panic("required env var missing: GEMINI_API_KEY or GEMINI_API_KEYS")
}

func getDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func split(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseDuration(s string) time.Duration {
	// Try standard Go duration format first (e.g., "6h", "30s")
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	// Fall back to numeric value (will be multiplied by caller)
	v, _ := strconv.ParseFloat(s, 64)
	return time.Duration(v)
}

func parseBool(s string) bool {
	v, _ := strconv.ParseBool(s)
	return v
}

func parseLevel(s string) *slog.LevelVar {
	lv := new(slog.LevelVar)
	switch strings.ToLower(s) {
	case "debug":
		lv.Set(slog.LevelDebug)
	case "warn":
		lv.Set(slog.LevelWarn)
	case "error":
		lv.Set(slog.LevelError)
	default:
		lv.Set(slog.LevelInfo)
	}
	return lv
}

func (c *Config) Validate() error {
	if c.GeminiAPIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is required")
	}
	if c.AgentCycleHours <= 0 {
		return fmt.Errorf("AGENT_CYCLE_HOURS must be positive")
	}
	if c.DailyContentLimit <= 0 {
		return fmt.Errorf("DAILY_CONTENT_LIMIT must be positive")
	}
	if c.DailyGeminiLimit <= 0 {
		return fmt.Errorf("DAILY_GEMINI_LIMIT must be positive")
	}
	if c.Port == "" {
		return fmt.Errorf("PORT is required")
	}
	return nil
}
