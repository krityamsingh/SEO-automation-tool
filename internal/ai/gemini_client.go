package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"aeo_geo_seo_agent/internal/util"
)

type GeminiClient struct {
	client      *genai.Client
	textModel   *genai.GenerativeModel
	imageModel  *genai.GenerativeModel
	apiKey      string
}

func NewGeminiClient(apiKey, textModel, imageModel string) (*GeminiClient, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	gc := &GeminiClient{
		client:     client,
		apiKey:     apiKey,
	}

	if textModel != "" {
		gc.textModel = client.GenerativeModel(textModel)
		gc.textModel.SetTemperature(0.7)
		gc.textModel.SetMaxOutputTokens(8192)
	}

	if imageModel != "" {
		gc.imageModel = client.GenerativeModel(imageModel)
	}

	return gc, nil
}

func (c *GeminiClient) GenerateText(ctx context.Context, prompt string, temperature float32, maxTokens int32) (string, error) {
	if c.textModel == nil {
		return "", fmt.Errorf("text model not configured")
	}

	modelName := strings.TrimPrefix(c.textModel.Name, "models/")
	model := c.client.GenerativeModel(modelName)
	model.SetTemperature(temperature)
	model.SetMaxOutputTokens(maxTokens)

	var resp *genai.GenerateContentResponse
	var err error

	retryFn := func() error {
		res, genErr := model.GenerateContent(ctx, genai.Text(prompt))
		if genErr != nil {
			return genErr
		}
		if len(res.Candidates) == 0 {
			slog.Warn("gemini returned 0 candidates")
			return fmt.Errorf("gemini returned 0 candidates")
		}
		resp = res
		return nil
	}

	retryCfg := util.DefaultRetryConfig()
	retryCfg.MaxRetries = 3
	if err = util.WithRetry(ctx, retryCfg, "gemini_generate_text", retryFn); err != nil {
		return "", fmt.Errorf("gemini text generation failed: %w", err)
	}

	return c.extractText(resp), nil
}

func (c *GeminiClient) GenerateImage(ctx context.Context, prompt string) ([]byte, error) {
	if c.imageModel == nil {
		return nil, fmt.Errorf("image model not configured")
	}

	resp, err := c.imageModel.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("gemini image generation failed: %w", err)
	}

	return c.extractImage(resp)
}

func (c *GeminiClient) GenerateStructured(ctx context.Context, prompt string, schema map[string]interface{}) (map[string]interface{}, error) {
	if c.textModel == nil {
		return nil, fmt.Errorf("text model not configured")
	}

	modelName := strings.TrimPrefix(c.textModel.Name, "models/")
	model := c.client.GenerativeModel(modelName)
	model.SetTemperature(0.2)
	model.SetMaxOutputTokens(4096)
	model.ResponseMIMEType = "application/json"

	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("gemini structured generation failed: %w", err)
	}

	text := c.extractText(resp)
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
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

	// Extract JSON array
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
	if c.client != nil {
		c.client.Close()
	}
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

type BlogPostResult struct {
	Title           string            `json:"title"`
	MetaDescription string            `json:"meta_description"`
	TLDR            string            `json:"tldr"`
	TableOfContents []string          `json:"table_of_contents"`
	Body            string            `json:"body"`
	FAQ             []FAQItem         `json:"faq"`
	Takeaways       []string          `json:"takeaways"`
	InternalLinks   []string          `json:"internal_links"`
	CTA             string            `json:"cta"`
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
	TitleOptions   []string       `json:"title_options"`
	Hook           string         `json:"hook"`
	ScriptSegments []ScriptSegment `json:"script_segments"`
	CTA            string         `json:"cta"`
	Description    string         `json:"description"`
	Tags           []string       `json:"tags"`
	ThumbnailIdeas []string       `json:"thumbnail_ideas"`
}

type ScriptSegment struct {
	Timestamp string `json:"timestamp"`
	Text      string `json:"text"`
	BRoll     string `json:"b_roll"`
}
