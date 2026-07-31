package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"aeo_geo_seo_agent/pkg/config"
	"aeo_geo_seo_agent/pkg/util"
)

type GeminiClient struct {
	geminiKeys     []string
	kimiKeys       []string
	minimaxKeys    []string
	geminiIdx      int
	kimiIdx        int
	minimaxIdx     int
	mu             sync.Mutex
	httpClient     *http.Client
	textModelName  string
	imageModelName string
}

func NewGeminiClient(apiKey, textModel, imageModel string) (*GeminiClient, error) {
	cfg := config.Load()
	geminiKeys := cfg.GeminiAPIKeys
	kimiKeys := cfg.KimiAPIKeys
	minimaxKeys := cfg.MiniMaxAPIKeys

	if apiKey != "" {
		g, k, m := parseKeysFromString(apiKey)
		geminiKeys = appendUnique(geminiKeys, g...)
		kimiKeys = appendUnique(kimiKeys, k...)
		minimaxKeys = appendUnique(minimaxKeys, m...)
	}

	if textModel == "" {
		textModel = cfg.GeminiTextModel
	}
	if textModel == "" {
		textModel = "gemini-1.5-flash"
	}
	if imageModel == "" {
		imageModel = cfg.GeminiImageModel
	}
	if imageModel == "" {
		imageModel = "gemini-2.0-flash-exp"
	}

	gc := &GeminiClient{
		geminiKeys:     geminiKeys,
		kimiKeys:       kimiKeys,
		minimaxKeys:    minimaxKeys,
		textModelName:  textModel,
		imageModelName: imageModel,
		httpClient:     &http.Client{Timeout: 60 * time.Second},
	}

	slog.Info("ai: GeminiClient initialized with multi-provider key pools",
		"gemini_keys_count", len(geminiKeys),
		"kimi_keys_count", len(kimiKeys),
		"minimax_keys_count", len(minimaxKeys),
	)

	return gc, nil
}

func NewGeminiClientMulti(geminiKeys, kimiKeys, minimaxKeys []string, textModel, imageModel string) *GeminiClient {
	if textModel == "" {
		textModel = "gemini-1.5-flash"
	}
	if imageModel == "" {
		imageModel = "gemini-2.0-flash-exp"
	}
	return &GeminiClient{
		geminiKeys:     geminiKeys,
		kimiKeys:       kimiKeys,
		minimaxKeys:    minimaxKeys,
		textModelName:  textModel,
		imageModelName: imageModel,
		httpClient:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *GeminiClient) GenerateText(ctx context.Context, prompt string, temperature float32, maxTokens int32) (string, error) {
	return c.GenerateTextWithProvider(ctx, "gemini", prompt, temperature, maxTokens)
}

func (c *GeminiClient) GenerateTextWithProvider(ctx context.Context, preferredProvider, prompt string, temperature float32, maxTokens int32) (string, error) {
	providersOrder := []string{"gemini", "kimi", "minimax"}
	switch strings.ToLower(preferredProvider) {
	case "kimi":
		providersOrder = []string{"kimi", "gemini", "minimax"}
	case "minimax":
		providersOrder = []string{"minimax", "gemini", "kimi"}
	}

	var errs []string

	for _, provider := range providersOrder {
		var text string
		var err error

		switch provider {
		case "gemini":
			if len(c.geminiKeys) > 0 {
				text, err = c.executeGeminiWithPool(ctx, prompt, temperature, maxTokens)
			} else {
				err = fmt.Errorf("no gemini API keys available")
			}
		case "kimi":
			if len(c.kimiKeys) > 0 {
				text, err = c.executeKimiWithPool(ctx, prompt, temperature, maxTokens)
			} else {
				err = fmt.Errorf("no kimi API keys available")
			}
		case "minimax":
			if len(c.minimaxKeys) > 0 {
				text, err = c.executeMiniMaxWithPool(ctx, prompt, temperature, maxTokens)
			} else {
				err = fmt.Errorf("no minimax API keys available")
			}
		}

		if err == nil && strings.TrimSpace(text) != "" {
			return text, nil
		}
		if err != nil {
			slog.Warn("ai: provider execution failed, attempting failover", "provider", provider, "error", err)
			errs = append(errs, fmt.Sprintf("[%s: %v]", provider, err))
		}
	}

	return "", fmt.Errorf("all AI providers and API keys failed: %s", strings.Join(errs, "; "))
}

func (c *GeminiClient) executeGeminiWithPool(ctx context.Context, prompt string, temperature float32, maxTokens int32) (string, error) {
	c.mu.Lock()
	keys := make([]string, len(c.geminiKeys))
	copy(keys, c.geminiKeys)
	startIdx := c.geminiIdx
	c.mu.Unlock()

	if len(keys) == 0 {
		return "", fmt.Errorf("gemini key pool empty")
	}

	var lastErr error
	for i := 0; i < len(keys); i++ {
		keyIdx := (startIdx + i) % len(keys)
		key := keys[keyIdx]

		text, err := c.callGeminiSingleKey(ctx, key, prompt, temperature, maxTokens)
		if err == nil && strings.TrimSpace(text) != "" {
			c.mu.Lock()
			c.geminiIdx = (keyIdx + 1) % len(keys)
			c.mu.Unlock()
			return text, nil
		}

		slog.Warn("ai: Gemini API key failed, rotating to next key in pool",
			"key_prefix", safeKeyPrefix(key),
			"error", err,
		)
		lastErr = err
	}

	return "", fmt.Errorf("gemini pool exhausted: %w", lastErr)
}

func (c *GeminiClient) callGeminiSingleKey(ctx context.Context, key, prompt string, temperature float32, maxTokens int32) (string, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(key))
	if err != nil {
		return "", fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	modelsToTry := []string{c.textModelName, "gemini-3.6-flash", "gemini-3.5-flash", "gemini-2.0-flash", "gemini-2.5-pro", "gemini-2.0-flash-exp"}
	var errMsgs []string

	for _, mName := range modelsToTry {
		if mName == "" {
			continue
		}
		model := client.GenerativeModel(mName)
		model.SetTemperature(temperature)
		if maxTokens > 0 {
			model.SetMaxOutputTokens(maxTokens)
		}

		resp, err := model.GenerateContent(ctx, genai.Text(prompt))
		if err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", mName, err))
			continue
		}

		if len(resp.Candidates) == 0 {
			errMsgs = append(errMsgs, fmt.Sprintf("%s: 0 candidates", mName))
			continue
		}

		text := c.extractText(resp)
		if strings.TrimSpace(text) != "" {
			return text, nil
		}
	}

	return "", fmt.Errorf("gemini key %s failed: %s", safeKeyPrefix(key), strings.Join(errMsgs, " | "))
}

func (c *GeminiClient) executeKimiWithPool(ctx context.Context, prompt string, temperature float32, maxTokens int32) (string, error) {
	c.mu.Lock()
	keys := make([]string, len(c.kimiKeys))
	copy(keys, c.kimiKeys)
	startIdx := c.kimiIdx
	c.mu.Unlock()

	if len(keys) == 0 {
		return "", fmt.Errorf("kimi key pool empty")
	}

	var lastErr error
	for i := 0; i < len(keys); i++ {
		keyIdx := (startIdx + i) % len(keys)
		key := keys[keyIdx]

		text, err := c.callKimiSingleKey(ctx, key, prompt, temperature, maxTokens)
		if err == nil && strings.TrimSpace(text) != "" {
			c.mu.Lock()
			c.kimiIdx = (keyIdx + 1) % len(keys)
			c.mu.Unlock()
			return text, nil
		}

		slog.Warn("ai: Kimi API key failed, rotating to next key in pool",
			"key_prefix", safeKeyPrefix(key),
			"error", err,
		)
		lastErr = err
	}

	return "", fmt.Errorf("kimi pool exhausted: %w", lastErr)
}

func (c *GeminiClient) callKimiSingleKey(ctx context.Context, key, prompt string, temperature float32, maxTokens int32) (string, error) {
	endpoints := []string{"https://api.moonshot.cn/v1/chat/completions", "https://api.openai.com/v1/chat/completions"}
	modelsToTry := []string{"moonshot-v1-8k", "moonshot-v1-32k", "kimi-latest", "gpt-4o-mini"}

	var errMsgs []string

	for _, endpoint := range endpoints {
		for _, mName := range modelsToTry {
			reqBody := map[string]interface{}{
				"model": mName,
				"messages": []map[string]string{
					{"role": "user", "content": prompt},
				},
				"temperature": temperature,
			}
			if maxTokens > 0 {
				reqBody["max_tokens"] = maxTokens
			}

			bodyBytes, _ := json.Marshal(reqBody)
			req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyBytes))
			if err != nil {
				errMsgs = append(errMsgs, fmt.Sprintf("req build error: %v", err))
				continue
			}

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+key)

			resp, err := c.httpClient.Do(req)
			if err != nil {
				errMsgs = append(errMsgs, fmt.Sprintf("%s/%s error: %v", endpoint, mName, err))
				continue
			}

			respBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode != 200 {
				errMsgs = append(errMsgs, fmt.Sprintf("%s status %d: %s", mName, resp.StatusCode, util.SafeTruncate(string(respBytes), 150)))
				continue
			}

			var apiResp struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}

			if err := json.Unmarshal(respBytes, &apiResp); err == nil && len(apiResp.Choices) > 0 {
				content := apiResp.Choices[0].Message.Content
				if strings.TrimSpace(content) != "" {
					return content, nil
				}
			}
		}
	}

	return "", fmt.Errorf("kimi call failed for key %s: %s", safeKeyPrefix(key), strings.Join(errMsgs, " | "))
}

func (c *GeminiClient) executeMiniMaxWithPool(ctx context.Context, prompt string, temperature float32, maxTokens int32) (string, error) {
	c.mu.Lock()
	keys := make([]string, len(c.minimaxKeys))
	copy(keys, c.minimaxKeys)
	startIdx := c.minimaxIdx
	c.mu.Unlock()

	if len(keys) == 0 {
		return "", fmt.Errorf("minimax key pool empty")
	}

	var lastErr error
	for i := 0; i < len(keys); i++ {
		keyIdx := (startIdx + i) % len(keys)
		key := keys[keyIdx]

		text, err := c.callMiniMaxSingleKey(ctx, key, prompt, temperature, maxTokens)
		if err == nil && strings.TrimSpace(text) != "" {
			c.mu.Lock()
			c.minimaxIdx = (keyIdx + 1) % len(keys)
			c.mu.Unlock()
			return text, nil
		}

		slog.Warn("ai: MiniMax API key failed, rotating to next key in pool",
			"key_prefix", safeKeyPrefix(key),
			"error", err,
		)
		lastErr = err
	}

	return "", fmt.Errorf("minimax pool exhausted: %w", lastErr)
}

func (c *GeminiClient) callMiniMaxSingleKey(ctx context.Context, key, prompt string, temperature float32, maxTokens int32) (string, error) {
	endpoints := []string{
		"https://api.minimaxi.chat/v1/text/chatcompletion_v2",
		"https://api.minimaxi.chat/v1/chat/completions",
		"https://api.minimax.chat/v1/chat/completions",
	}

	modelsToTry := []string{"abab6.5g-chat", "abab6.5t-chat", "minimax-text-01"}

	for _, endpoint := range endpoints {
		for _, mName := range modelsToTry {
			reqBody := map[string]interface{}{
				"model": mName,
				"messages": []map[string]interface{}{
					{"sender_type": "USER", "sender_name": "user", "text": prompt, "role": "user", "content": prompt},
				},
				"temperature": temperature,
			}
			if maxTokens > 0 {
				reqBody["max_tokens"] = maxTokens
			}

			bodyBytes, _ := json.Marshal(reqBody)
			req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(bodyBytes))
			if err != nil {
				return "", err
			}

			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+key)

			resp, err := c.httpClient.Do(req)
			if err != nil {
				continue
			}

			respBytes, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != 200 {
				slog.Debug("ai: minimax request non-200", "status", resp.StatusCode, "body", string(respBytes))
				continue
			}

			var apiResp struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
				Reply string `json:"reply"`
			}

			if err := json.Unmarshal(respBytes, &apiResp); err == nil {
				if len(apiResp.Choices) > 0 && strings.TrimSpace(apiResp.Choices[0].Message.Content) != "" {
					return apiResp.Choices[0].Message.Content, nil
				}
				if strings.TrimSpace(apiResp.Reply) != "" {
					return apiResp.Reply, nil
				}
			}
		}
	}

	return "", fmt.Errorf("minimax call failed for key prefix %s", safeKeyPrefix(key))
}

func (c *GeminiClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	c.mu.Lock()
	keys := make([]string, len(c.geminiKeys))
	copy(keys, c.geminiKeys)
	c.mu.Unlock()

	if len(keys) == 0 {
		return nil, fmt.Errorf("no gemini keys available for image generation")
	}

	var lastErr error
	for _, key := range keys {
		client, err := genai.NewClient(ctx, option.WithAPIKey(key))
		if err != nil {
			lastErr = err
			continue
		}
		model := client.GenerativeModel(c.imageModelName)
		resp, err := model.GenerateContent(ctx, genai.Text(prompt))
		client.Close()

		if err != nil {
			lastErr = err
			continue
		}

		imgData, err := c.extractImage(resp)
		if err == nil {
			return imgData, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("gemini image generation failed: %w", lastErr)
}

func (c *GeminiClient) GenerateStructured(ctx context.Context, prompt string, schema map[string]interface{}) (map[string]interface{}, error) {
	jsonPrompt := "Respond ONLY with valid JSON, no markdown or explanation.\n" + prompt
	text, err := c.GenerateText(ctx, jsonPrompt, 0.2, 4096)
	if err != nil {
		return nil, fmt.Errorf("structured generation failed: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(util.ExtractJSON(text)), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}
	return result, nil
}

func (c *GeminiClient) Summarize(ctx context.Context, text string, maxLength int) (string, error) {
	prompt := fmt.Sprintf("Summarize the following text in under %d characters, preserving key facts and insights:\n\n%s", maxLength, text[:min(len(text), 8000)])
	return c.GenerateText(ctx, prompt, 0.3, 1024)
}

func (c *GeminiClient) GenerateKeywords(ctx context.Context, niche string, count int) ([]KeywordSuggestion, error) {
	prompt := fmt.Sprintf(`Generate %d SEO keyword suggestions for the niche: "%s".

For each keyword, provide:
- keyword (string)
- search_volume_estimate (string, labeled "LLM estimate, not measured search data")
- competition_estimate (string, labeled "LLM estimate")
- trend_score (1-10)

Respond ONLY as JSON array of objects.`, count, niche)

	text, err := c.GenerateText(ctx, prompt, 0.8, 4096)
	if err != nil {
		return nil, err
	}

	jsonStr := c.extractJSON(text)
	var keywords []KeywordSuggestion
	if err := json.Unmarshal([]byte(jsonStr), &keywords); err != nil {
		slog.Warn("failed to parse keyword JSON, returning raw", "error", err)
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}
	return keywords, nil
}

func (c *GeminiClient) GenerateBlogPost(ctx context.Context, topic string, keywords []string, minWords, maxWords int) (*BlogPostResult, error) {
	prompt := fmt.Sprintf(`Write a comprehensive SEO blog post about "%s".

Target keywords: %s
Target length: %d-%d words

Requirements:
1. Catchy title (60-70 chars)
2. Meta description (150-160 chars)
3. Table of contents (H2/H3 structure)
4. TL;DR summary at top (2-3 sentences)
5. Body with H2/H3 sections
6. FAQ section with 5-7 questions (for FAQPage schema)
7. Key takeaways box
8. CTA at end
9. Internal linking suggestions (3-5)
10. Natural keyword usage, no stuffing

Format as JSON (Respond ONLY with raw JSON, no markdown formatting or commentary):
{
  "title": "...",
  "meta_description": "...",
  "tldr": "...",
  "table_of_contents": ["H2: ...", "H3: ..."],
  "body": "...",
  "faq": [{"question": "...", "answer": "..."}],
  "takeaways": ["..."],
  "internal_links": ["..."],
  "cta": "..."
}`, topic, strings.Join(keywords, ", "), minWords, maxWords)

	text, err := c.GenerateText(ctx, prompt, 0.7, 8192)
	if err != nil {
		return nil, err
	}

	jsonStr := c.extractJSON(text)
	var result BlogPostResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse blog post JSON: %w", err)
	}
	return &result, nil
}

func (c *GeminiClient) GenerateSocialScripts(ctx context.Context, topic, platform string) (*SocialScripts, error) {
	prompt := fmt.Sprintf(`Generate social media content for "%s" on %s platform.

Requirements:
- Twitter/X: concise, punchy, under 280 chars, 2-3 hashtags
- LinkedIn: professional, insightful, 300-500 chars, 3-5 hashtags
- Instagram: engaging caption, 150-300 chars, 5-10 hashtags, emoji
- TikTok: hook + CTA, under 150 chars, 3-5 trending hashtags
- Facebook: conversational, 100-300 chars, 2-3 hashtags

Format as JSON:
{
  "twitter": "...",
  "linkedin": "...",
  "instagram": "...",
  "tiktok": "...",
  "facebook": "..."
}`, topic, platform)

	text, err := c.GenerateText(ctx, prompt, 0.8, 4096)
	if err != nil {
		return nil, err
	}

	jsonStr := c.extractJSON(text)
	var result SocialScripts
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse social scripts JSON: %w", err)
	}
	return &result, nil
}

func (c *GeminiClient) GenerateVideoScript(ctx context.Context, topic, platform string, duration int) (*VideoScript, error) {
	prompt := fmt.Sprintf(`Write a video script for "%s" on %s platform, target duration %d seconds.

Include:
1. Hook (first 3 seconds) - attention grabber
2. Script body with timestamps (every 10-15 seconds)
3. B-roll suggestions
4. CTA (call to action)
5. SEO-optimized title (5-10 options)
6. Description with keywords
7. Tags (10-15)
8. Thumbnail text ideas (3 options)

Format as JSON:
{
  "title_options": ["..."],
  "hook": "...",
  "script_segments": [{"timestamp": "0:00-0:15", "text": "...", "b_roll": "..."}],
  "cta": "...",
  "description": "...",
  "tags": ["..."],
  "thumbnail_ideas": ["..."]
}`, topic, platform, duration)

	text, err := c.GenerateText(ctx, prompt, 0.8, 8192)
	if err != nil {
		return nil, err
	}

	jsonStr := c.extractJSON(text)
	var result VideoScript
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse video script JSON: %w", err)
	}
	return &result, nil
}

func (c *GeminiClient) GenerateSchemaMarkup(ctx context.Context, schemaType, url, content string) (string, error) {
	prompt := fmt.Sprintf(`Generate valid JSON-LD Schema.org markup for a %s.

URL: %s
Content: %s

Requirements:
- Must be valid JSON-LD
- Must pass Schema.org validation
- Include all required properties
- Include @context and @type

Output ONLY the JSON-LD code, no explanation.`, schemaType, url, util.SafeTruncate(content, 2000))

	return c.GenerateText(ctx, prompt, 0.2, 4096)
}

func (c *GeminiClient) OptimizeForSnippet(ctx context.Context, question, content string) (string, error) {
	prompt := fmt.Sprintf(`Optimize the following content to win a Google featured snippet for the question: "%s"

Content: %s

Requirements:
- Direct answer in first 40-60 words
- Concise, factual, scannable
- Use list format if appropriate
- Include the question naturally in the answer
- No fluff, no marketing speak

Output ONLY the optimized answer paragraph.`, question, util.SafeTruncate(content, 2000))

	return c.GenerateText(ctx, prompt, 0.3, 1024)
}

func (c *GeminiClient) OptimizeForGEO(ctx context.Context, content, targetLLMs string) (string, error) {
	prompt := fmt.Sprintf(`Optimize the following content for Generative Engine Optimization (GEO) to be cited by AI models like %s.

Content: %s

Requirements:
1. Clear, factual statements with specific data points
2. Structured with clear headings and bullet points
3. Include citations or source attributions where possible
4. Add a "Key Facts" summary box at the top
5. Ensure each paragraph has a clear, citable thesis statement
6. Use entity names explicitly (people, organizations, products)
7. Add an "AI Summary" paragraph that can be directly quoted by LLMs

Output the optimized full content.`, targetLLMs, util.SafeTruncate(content, 4000))

	return c.GenerateText(ctx, prompt, 0.5, 8192)
}

func (c *GeminiClient) Close() {
	// No persistent resources to close
}

func (c *GeminiClient) extractText(resp *genai.GenerateContentResponse) string {
	if resp == nil {
		return ""
	}
	var parts []string
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if text, ok := part.(genai.Text); ok {
				parts = append(parts, string(text))
			}
		}
	}
	return strings.Join(parts, "")
}

func (c *GeminiClient) extractImage(resp *genai.GenerateContentResponse) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil response")
	}
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if data, ok := part.(genai.Blob); ok {
				return data.Data, nil
			}
		}
	}
	return nil, fmt.Errorf("no image found in response")
}

func (c *GeminiClient) extractJSON(text string) string {
	return util.ExtractJSON(text)
}

func parseKeysFromString(raw string) (gemini []string, kimi []string, minimax []string) {
	raw = strings.Trim(strings.TrimSpace(raw), "\"'")
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) > 0 {
		raw = string(decoded)
	}
	parts := strings.Split(raw, ",")
	for _, p := range parts {
		k := strings.TrimSpace(p)
		if k == "" {
			continue
		}
		if strings.HasPrefix(k, "AIzaSy") {
			gemini = append(gemini, k)
		} else if strings.HasPrefix(k, "sk-api-") {
			minimax = append(minimax, k)
		} else if strings.HasPrefix(k, "sk-") || strings.HasPrefix(k, "AQ.") || strings.HasPrefix(k, "moonshot-") || strings.HasPrefix(k, "kimi-") {
			kimi = append(kimi, k)
		} else {
			gemini = append(gemini, k)
		}
	}
	return gemini, kimi, minimax
}

func appendUnique(target []string, items ...string) []string {
	seen := make(map[string]bool)
	for _, t := range target {
		seen[t] = true
	}
	for _, item := range items {
		if item != "" && !seen[item] {
			seen[item] = true
			target = append(target, item)
		}
	}
	return target
}

func safeKeyPrefix(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:8] + "..."
}

// FAQItem helper for JSON serialization
func (f FAQItem) toJSON() (string, error) {
	bytes, err := json.Marshal(f)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s SocialScripts) toJSON() (string, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (v VideoScript) toJSON() (string, error) {
	bytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Result types

type KeywordSuggestion struct {
	Keyword              string `json:"keyword"`
	SearchVolumeEstimate string `json:"search_volume_estimate"`
	CompetitionEstimate  string `json:"competition_estimate"`
	TrendScore           int    `json:"trend_score"`
}

type FlexibleStringArray []string

func (f *FlexibleStringArray) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*f = arr
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str != "" {
			*f = []string{str}
		} else {
			*f = []string{}
		}
		return nil
	}
	*f = []string{}
	return nil
}

type BlogPostResult struct {
	Title           string              `json:"title"`
	MetaDescription string              `json:"meta_description"`
	TLDR            string              `json:"tldr"`
	TableOfContents FlexibleStringArray `json:"table_of_contents"`
	Body            string              `json:"body"`
	FAQ             []FAQItem           `json:"faq"`
	Takeaways       FlexibleStringArray `json:"takeaways"`
	InternalLinks   FlexibleStringArray `json:"internal_links"`
	CTA             string              `json:"cta"`
}

type FAQItem struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type SocialScripts struct {
	Twitter   string `json:"twitter"`
	LinkedIn  string `json:"linkedin"`
	Instagram string `json:"instagram"`
	TikTok    string `json:"tiktok"`
	Facebook  string `json:"facebook"`
}

type VideoScript struct {
	TitleOptions   []string        `json:"title_options"`
	Hook           string          `json:"hook"`
	ScriptSegments []ScriptSegment `json:"script_segments"`
	CTA            string          `json:"cta"`
	Description    string          `json:"description"`
	Tags           []string        `json:"tags"`
	ThumbnailIdeas []string        `json:"thumbnail_ideas"`
}

type ScriptSegment struct {
	Timestamp string `json:"timestamp"`
	Text      string `json:"text"`
	BRoll     string `json:"b_roll"`
}
