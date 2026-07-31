package config

import (
	"os"
	"testing"
)

func TestParseAPIKeyPools(t *testing.T) {
	// Base64 string containing Gemini (AIzaSy...), Kimi (sk-EC...), and MiniMax (sk-api-...)
	rawPool := "c2stRUNsUG5kOXd0SHh1VVRVR0k1ck0wRnJiMW1HeEpwRk9KekVYcGlXTnRRQW12eWRWLEFJemFTeUJfRnFqRktWWnVnM1BtYkJWb2JOOGxrNFcyc1hJZ01JQSxzay1hcGktVDBfWUhPcDFVSkR4RzQ4bzZDQXJCNllVQzc2amtxZ1E="
	os.Setenv("GEMINI_API_KEYS", rawPool)
	defer os.Unsetenv("GEMINI_API_KEYS")

	gemini, kimi, minimax := parseAPIKeyPools()

	if len(gemini) == 0 {
		t.Errorf("expected gemini keys, got 0")
	}
	if len(kimi) == 0 {
		t.Errorf("expected kimi keys, got 0")
	}
	if len(minimax) == 0 {
		t.Errorf("expected minimax keys, got 0")
	}
}

func TestConfigValidation(t *testing.T) {
	cfg := &Config{
		GeminiAPIKeys:     []string{"AIzaSyTest"},
		AgentCycleHours:   6,
		DailyContentLimit: 5,
		DailyGeminiLimit:  200,
		Port:              "8080",
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}
