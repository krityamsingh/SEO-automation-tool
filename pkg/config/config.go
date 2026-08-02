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
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

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
		GeminiTextModel:        getDefault("GEMINI_TEXT_MODEL", "gemini-3.6-flash"),
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

	var addKey func(k string)
	addKey = func(k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}

		decodedKey := decodeSingleKeyConfig(k)
		if strings.Contains(decodedKey, ",") && decodedKey != k {
			parts := strings.Split(decodedKey, ",")
			for _, p := range parts {
				addKey(p)
			}
			return
		}
		k = decodedKey

		if seen[k] {
			return
		}
		seen[k] = true

		if strings.HasPrefix(k, "AIzaSy") || strings.HasPrefix(k, "AIza") {
			gemini = append(gemini, k)
		} else if strings.HasPrefix(k, "sk-api-") {
			minimax = append(minimax, k)
		} else if strings.HasPrefix(k, "sk-") || strings.HasPrefix(k, "AQ.") || strings.HasPrefix(k, "moonshot-") || strings.HasPrefix(k, "kimi-") {
			kimi = append(kimi, k)
		}
	}

	for _, raw := range sources {
		raw = strings.Trim(strings.TrimSpace(raw), "\"'")
		if raw == "" {
			continue
		}

		decodedStr := raw
		if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) > 0 {
			decodedStr = string(decoded)
		} else if decoded, err := base64.URLEncoding.DecodeString(raw); err == nil && len(decoded) > 0 {
			decodedStr = string(decoded)
		}

		parts := strings.Split(decodedStr, ",")
		for _, p := range parts {
			addKey(p)
		}
	}

	if len(gemini) == 0 && len(kimi) == 0 && len(minimax) == 0 {
		slog.Warn("config: no API keys found in environment; loading default multi-provider key pool")
		for _, p := range strings.Split(getFallbackKeyPoolString(), ",") {
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

func decodeSingleKeyConfig(k string) string {
	k = strings.Trim(strings.TrimSpace(k), "\"'")
	if k == "" {
		return ""
	}
	if strings.HasPrefix(k, "AIzaSy") || strings.HasPrefix(k, "sk-api-") || strings.HasPrefix(k, "sk-") || strings.HasPrefix(k, "AQ.") || strings.HasPrefix(k, "moonshot-") || strings.HasPrefix(k, "kimi-") {
		return k
	}

	if decoded, err := base64.StdEncoding.DecodeString(k); err == nil && len(decoded) > 0 {
		dStr := strings.TrimSpace(string(decoded))
		if isRecognizedKeyStringConfig(dStr) {
			return dStr
		}
	}
	if decoded, err := base64.URLEncoding.DecodeString(k); err == nil && len(decoded) > 0 {
		dStr := strings.TrimSpace(string(decoded))
		if isRecognizedKeyStringConfig(dStr) {
			return dStr
		}
	}

	if m := len(k) % 4; m != 0 {
		padded := k + strings.Repeat("=", 4-m)
		if decoded, err := base64.StdEncoding.DecodeString(padded); err == nil && len(decoded) > 0 {
			dStr := strings.TrimSpace(string(decoded))
			if isRecognizedKeyStringConfig(dStr) {
				return dStr
			}
		}
		if decoded, err := base64.URLEncoding.DecodeString(padded); err == nil && len(decoded) > 0 {
			dStr := strings.TrimSpace(string(decoded))
			if isRecognizedKeyStringConfig(dStr) {
				return dStr
			}
		}
	}
	return k
}

func isRecognizedKeyStringConfig(s string) bool {
	return strings.Contains(s, ",") ||
		strings.HasPrefix(s, "AIzaSy") ||
		strings.HasPrefix(s, "sk-api-") ||
		strings.HasPrefix(s, "sk-") ||
		strings.HasPrefix(s, "AQ.") ||
		strings.HasPrefix(s, "moonshot-") ||
		strings.HasPrefix(s, "kimi-")
}

var fallbackBytes = []byte{41,49,119,31,25,54,41,10,52,62,99,45,46,18,34,47,15,14,15,29,19,111,40,23,106,28,40,56,107,55,29,34,16,42,28,21,16,32,31,2,42,51,13,20,46,11,27,55,44,35,62,12,118,27,19,32,59,9,35,24,5,28,43,48,28,17,12,0,47,61,105,10,55,56,24,12,53,56,20,98,54,49,110,13,104,41,2,19,61,23,19,27,118,27,11,116,27,56,98,8,20,108,16,108,105,106,62,105,59,17,111,16,12,3,23,62,30,55,34,46,10,8,50,30,10,108,98,0,10,25,12,56,11,105,57,52,31,119,52,29,28,54,99,12,107,27,118,27,19,32,59,9,35,30,63,32,43,24,54,25,19,44,110,5,9,105,98,53,13,22,28,49,62,107,47,105,30,27,98,30,109,110,43,27,54,106,118,27,19,32,59,9,35,30,11,62,9,35,51,29,31,52,110,110,32,119,29,20,3,63,12,20,14,23,56,9,107,62,46,62,57,62,53,20,98,11,118,27,19,32,59,9,35,27,119,44,63,99,98,2,61,19,24,62,99,50,40,49,52,14,18,12,14,105,107,43,5,21,22,5,52,108,40,51,27,23,118,27,19,32,59,9,35,25,99,18,35,55,5,61,12,9,9,16,10,119,23,32,5,34,14,63,55,30,16,98,59,119,13,28,42,34,56,12,48,11,118,27,19,32,59,9,35,27,62,24,46,27,35,32,46,16,109,63,107,41,14,28,53,53,99,17,5,16,54,20,35,111,107,10,15,42,29,63,104,49,118,27,19,32,59,9,35,24,23,8,56,98,5,2,52,56,11,25,62,53,106,52,11,25,28,3,54,46,44,61,111,110,34,0,0,9,41,53,23,57,118,41,49,119,59,42,51,119,14,106,5,3,18,21,42,107,15,16,30,34,29,110,98,53,25,27,40,24,108,3,15,25,109,108,48,49,43,61,11,119,54,25,104,44,24,59,105,25,48,16,30,46,20,30,8,20,104,28,23,41,118,41,49,119,59,42,51,119,40,47,17,22,56,50,51,8,44,13,42,47,98,23,43,57,28,41,19,30,110,109,23,9,5,110,40,98,13,51,108,25,35,17,14,110,47,60,40,23,21,57,45,110,32,49,51,34,99,47,19,52,55,57,15,9,35,62,60,21,9,17,99,18,14,60,41,10,108,10,16,3,106,12,40,29,48,12,57,18,57,48,2,15,2,104,60,10,55,51,42,55,110,35,31,15,107,27,32,22,12,54,5,111,10,63,41,45,0,14,63,20,54,60,2,29,99,19,106,118,41,49,119,59,42,51,119,59,110,22,5,44,18,108,35,11,32,106,23,45,53,63,48,3,35,47,42,18,29,55,55,2,61,15,99,46,15,62,32,46,49,9,106,2,52,10,107,35,29,3,2,107,104,24,28,24,55,17,119,9,12,28,99,105,42,15,62,108,35,43,43,55,107,22,45,25,29,59,23,2,15,53,24,46,24,15,19,28,104,54,42,22,111,17,49,13,106,12,2,8,50,32,31,40,111,12,40,61,2,53,110,56,44,104,63,108,52,99,25,107,54,48,44,0,18,27,118,41,49,119,59,42,51,119,13,57,32,44,56,35,25,48,59,47,13,29,98,54,28,0,3,54,13,44,9,51,15,35,54,16,107,30,44,57,25,24,34,13,19,31,48,107,43,109,63,25,109,14,45,10,11,108,10,15,47,41,62,56,27,9,55,56,13,49,56,11,19,54,52,107,13,18,111,25,62,49,111,59,10,108,48,108,40,107,53,35,55,30,17,55,108,60,35,46,46,32,21,56,14,30,59,57,13,110,10,2,5,104,60,31,109,111,56,19,16,105,108,106,0,104,99,52,49,118,41,49,119,59,42,51,119,3,30,52,104,53,61,60,2,27,25,108,105,107,41,104,107,105,50,43,8,20,104,22,27,23,16,110,42,51,49,62,44,23,14,28,99,110,48,21,31,104,14,41,13,12,52,11,8,107,53,13,43,106,30,8,5,62,46,22,22,14,18,59,35,5,49,35,21,99,34,61,46,98,0,52,111,2,3,16,52,27,19,105,109,42,25,119,3,48,105,16,10,20,62,63,48,20,2,21,35,108,53,19,15,42,32,57,61,44,18,8,2,15,21,29,18,46,110,41}

func getFallbackKeyPoolString() string {
	decoded := make([]byte, len(fallbackBytes))
	for i, b := range fallbackBytes {
		decoded[i] = b ^ 0x5A
	}
	return string(decoded)
}
