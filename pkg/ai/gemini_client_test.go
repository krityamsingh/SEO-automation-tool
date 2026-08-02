package ai

import (
	"context"
	"testing"
)

func TestClientInitializationAndClassification(t *testing.T) {
	rawKeys := "AIzaSyGeminiKey1,sk-EClKimiKey1,sk-api-MiniMaxKey1"
	client, err := NewGeminiClient(rawKeys, "gemini-1.5-flash", "gemini-2.0-flash-exp")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	if len(client.geminiKeys) == 0 {
		t.Errorf("expected gemini keys to be classified")
	}
	if len(client.kimiKeys) == 0 {
		t.Errorf("expected kimi keys to be classified")
	}
	if len(client.minimaxKeys) == 0 {
		t.Errorf("expected minimax keys to be classified")
	}
}

func TestProviderFailoverChain(t *testing.T) {
	// Initialize with dummy keys to test graceful error handling across failover chain
	client := NewGeminiClientMulti(
		[]string{"AIzaSyInvalidKey1"},
		[]string{"sk-invalid-kimi-1"},
		[]string{"sk-api-invalid-minimax-1"},
		"gemini-1.5-flash",
		"gemini-2.0-flash-exp",
	)

	ctx := context.Background()
	_, err := client.GenerateTextWithProvider(ctx, "kimi", "Hello world", 0.5, 100)

	// Since all keys are invalid dummy keys, it should attempt Kimi -> Gemini -> MiniMax and fail gracefully with a aggregated error
	if err == nil {
		t.Errorf("expected aggregated failover error for dummy keys, got nil")
	}
}

func TestKeyStringParsingAndClassification(t *testing.T) {
	rawKeys := "AIzaSyGeminiKey1,sk-EClKimiKey1,sk-api-MiniMaxKey1"
	g, k, m := parseKeysFromString(rawKeys)
	if len(g) != 1 || g[0] != "AIzaSyGeminiKey1" {
		t.Errorf("expected 1 gemini key, got %v", g)
	}
	if len(k) != 1 || k[0] != "sk-EClKimiKey1" {
		t.Errorf("expected 1 kimi key, got %v", k)
	}
	if len(m) != 1 || m[0] != "sk-api-MiniMaxKey1" {
		t.Errorf("expected 1 minimax key, got %v", m)
	}

	for _, key := range g {
		if !isValidGeminiKey(key) {
			t.Errorf("invalid gemini key in pool: %s", key)
		}
	}
}
