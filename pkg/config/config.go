package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// AI Provider Pools
	GeminiAPIKey     string
	GeminiAPIKeys    []string
	KimiAPIKey       string
	KimiAPIKeys      []string
	MiniMaxAPIKey    string
	MiniMaxAPIKeys   []string
	GeminiTextModel  string
	GeminiImageModel string

	// Agent Behavior
	AgentNiches        []string
	AgentCycleHours    time.Duration
	ContentAutoPublish bool
	ContentMinWords    int
	ContentMaxWords    int

	// Publishing
	WordPressURL           string
	WordPressUsername      string
	WordPressAppPassword   string
	MediumIntegrationToken string
	GhostURL               string
	GhostAdminAPIKey       string
	WebhookURL             string

	// Database
	DatabaseURL string

	// Limits
	DailyContentLimit int
	DailyGeminiLimit  int

	// Crawler
	UserAgent    string
	CrawlDelay   time.Duration
	CrawlTimeout time.Duration
	MaxDepth     int
	MaxPages     int

	// Server
	Port     string
	LogLevel *slog.LevelVar
	APIKey   string
}

func Load() *Config {
	_ = godotenv.Load()

	geminiPool, kimiPool, minimaxPool := parseAPIKeyPools()

	var primaryGemini, primaryKimi, primaryMiniMax string
	if len(geminiPool) > 0 {
		primaryGemini = geminiPool[0]
	}
	if len(kimiPool) > 0 {
		primaryKimi = kimiPool[0]
	}
	if len(minimaxPool) > 0 {
		primaryMiniMax = minimaxPool[0]
	}

	cfg := &Config{
		GeminiAPIKey:           primaryGemini,
		GeminiAPIKeys:          geminiPool,
		KimiAPIKey:             primaryKimi,
		KimiAPIKeys:            kimiPool,
		MiniMaxAPIKey:          primaryMiniMax,
		MiniMaxAPIKeys:         minimaxPool,
		GeminiTextModel:        getDefault("GEMINI_TEXT_MODEL", "gemini-1.5-flash"),
		GeminiImageModel:       getDefault("GEMINI_IMAGE_MODEL", "gemini-2.0-flash-exp"),
		AgentNiches:            split(getDefault("AGENT_NICHES", "technology,saas,ai")),
		AgentCycleHours:        parseDuration(getDefault("AGENT_CYCLE_HOURS", "6")) * time.Hour,
		ContentAutoPublish:     parseBool(getDefault("CONTENT_AUTO_PUBLISH", "false")),
		ContentMinWords:        parseInt(getDefault("CONTENT_MIN_WORDS", "1500")),
		ContentMaxWords:        parseInt(getDefault("CONTENT_MAX_WORDS", "3000")),
		WordPressURL:           os.Getenv("WORDPRESS_URL"),
		WordPressUsername:      os.Getenv("WORDPRESS_USERNAME"),
		WordPressAppPassword:   os.Getenv("WORDPRESS_APP_PASSWORD"),
		MediumIntegrationToken: os.Getenv("MEDIUM_INTEGRATION_TOKEN"),
		GhostURL:               os.Getenv("GHOST_URL"),
		GhostAdminAPIKey:       os.Getenv("GHOST_ADMIN_API_KEY"),
		WebhookURL:             os.Getenv("WEBHOOK_URL"),
		DatabaseURL:            getDefault("DATABASE_URL", "sqlite://agent.db"),
		DailyContentLimit:      parseInt(getDefault("DAILY_CONTENT_LIMIT", "5")),
		DailyGeminiLimit:       parseInt(getDefault("DAILY_GEMINI_LIMIT", "200")),
		UserAgent:              getDefault("USER_AGENT", "AEOAgent/1.0"),
		CrawlDelay:             parseDuration(getDefault("CRAWL_DELAY", "1")) * time.Second,
		CrawlTimeout:           parseDuration(getDefault("CRAWL_TIMEOUT", "30")) * time.Second,
		MaxDepth:               parseInt(getDefault("MAX_DEPTH", "3")),
		MaxPages:               parseInt(getDefault("MAX_PAGES", "100")),
		Port:                   getDefault("PORT", "8080"),
		LogLevel:               parseLevel(getDefault("LOG_LEVEL", "info")),
		APIKey:                 os.Getenv("API_KEY"),
	}

	return cfg
}

func require(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Warn("config: required env var missing", "key", key)
	}
	return v
}

func parseAPIKeyPools() (gemini []string, kimi []string, minimax []string) {
	// Raw string sources
	sources := []string{
		os.Getenv("GEMINI_API_KEYS"),
		os.Getenv("GEMINI_API_KEY"),
		os.Getenv("KIMI_API_KEY"),
		os.Getenv("KIMI_API_KEYS"),
		os.Getenv("MOONSHOT_API_KEY"),
		os.Getenv("MINIMAX_API_KEY"),
		os.Getenv("MINIMAX_API_KEYS"),
		os.Getenv("OPENAI_API_KEY"),
	}

	seen := make(map[string]bool)

	addKey := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" || seen[k] {
			return
		}
		seen[k] = true

		if strings.HasPrefix(k, "AIzaSy") {
			gemini = append(gemini, k)
		} else if strings.HasPrefix(k, "sk-api-") {
			minimax = append(minimax, k)
		} else if strings.HasPrefix(k, "sk-") || strings.HasPrefix(k, "AQ.") || strings.HasPrefix(k, "moonshot-") || strings.HasPrefix(k, "kimi-") {
			kimi = append(kimi, k)
		} else {
			// Default unknown key to Gemini pool if unclassified
			gemini = append(gemini, k)
		}
	}

	for _, raw := range sources {
		if raw == "" {
			continue
		}

		// Try base64 decoding first
		decodedStr := raw
		if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) > 0 {
			decodedStr = string(decoded)
		}

		parts := strings.Split(decodedStr, ",")
		for _, p := range parts {
			addKey(p)
		}
	}

	slog.Info("config: loaded multi-provider API key pools",
		"gemini_keys", len(gemini),
		"kimi_keys", len(kimi),
		"minimax_keys", len(minimax),
	)

	return gemini, kimi, minimax
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
	if len(c.GeminiAPIKeys) == 0 && len(c.KimiAPIKeys) == 0 && len(c.MiniMaxAPIKeys) == 0 && c.GeminiAPIKey == "" {
		return fmt.Errorf("at least one valid AI API key (Gemini, Kimi, or MiniMax) is required")
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
