package scriptwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/util"
)

type Writer struct {
	gemini *ai.GeminiClient
	db     *gorm.DB
}

func New(gemini *ai.GeminiClient, db *gorm.DB) *Writer {
	return &Writer{
		gemini: gemini,
		db:     db,
	}
}

func (w *Writer) GenerateBlog(ctx context.Context, topic string, keywords []string, minWords, maxWords int) (*database.ContentPiece, error) {
	post, err := w.gemini.GenerateBlogPost(ctx, topic, keywords, minWords, maxWords)
	if err != nil {
		return nil, fmt.Errorf("blog generation failed: %w", err)
	}
	
	faqJSON, _ := json.Marshal(post.FAQ)
	
	content := &database.ContentPiece{
		Title:           post.Title,
		Body:            post.Body,
		MetaDescription: post.MetaDescription,
		TLDR:            post.TLDR,
		FAQSection:      string(faqJSON),
		Status:          "draft",
	}
	
	return content, nil
}

func (w *Writer) GenerateSocial(ctx context.Context, topic, platform string) (map[string]string, error) {
	scripts, err := w.gemini.GenerateSocialScripts(ctx, topic, platform)
	if err != nil {
		return nil, fmt.Errorf("social script generation failed: %w", err)
	}
	
	result := map[string]string{
		"twitter":   scripts.Twitter,
		"linkedin":  scripts.LinkedIn,
		"instagram": scripts.Instagram,
		"tiktok":    scripts.TikTok,
		"facebook":  scripts.Facebook,
	}
	
	return result, nil
}

func (w *Writer) GenerateVideo(ctx context.Context, topic, platform string, duration int) (*ai.VideoScript, error) {
	script, err := w.gemini.GenerateVideoScript(ctx, topic, platform, duration)
	if err != nil {
		return nil, fmt.Errorf("video script generation failed: %w", err)
	}
	
	return script, nil
}

func (w *Writer) GenerateEmailSequence(ctx context.Context, topic, sequenceType string) ([]string, error) {
	prompt := fmt.Sprintf(`Write a %s email sequence for: %s

Requirements:
- 3-5 emails in the sequence
- Each email: compelling subject line, engaging body, clear CTA
- Progressive value delivery (don't pitch on first email)
- Personal but professional tone
- Mobile-friendly formatting (short paragraphs)

Format as JSON array of objects with fields: subject, body, cta`, sequenceType, topic)
	
	text, err := w.gemini.GenerateText(ctx, prompt, 0.7, 8192)
	if err != nil {
		return nil, err
	}
	
	var emails []struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
		CTA     string `json:"cta"`
	}
	
	jsonStr := util.ExtractJSON(text)
	if err := json.Unmarshal([]byte(jsonStr), &emails); err != nil {
		return nil, fmt.Errorf("failed to parse email JSON: %w", err)
	}
	
	result := make([]string, len(emails))
	for i, e := range emails {
		result[i] = fmt.Sprintf("Subject: %s\n\n%s\n\nCTA: %s", e.Subject, e.Body, e.CTA)
	}
	
	return result, nil
}

func (w *Writer) GenerateAdCopy(ctx context.Context, product, platform string) (map[string]string, error) {
	prompt := fmt.Sprintf(`Write ad copy for "%s" on %s platform.

Requirements:
- 3-5 headline variations (under 30 chars each for Google Ads)
- 2-3 description variations (under 90 chars each)
- Primary text / body copy
- CTA options
- Platform-specific formatting

Format as JSON with fields: headlines, descriptions, primary_text, ctas`, product, platform)
	
	text, err := w.gemini.GenerateText(ctx, prompt, 0.8, 4096)
	if err != nil {
		return nil, err
	}
	
	var result map[string]string
	jsonStr := util.ExtractJSON(text)
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// Fallback: return raw text
		return map[string]string{"raw": text}, nil
	}
	
	return result, nil
}

func (w *Writer) GenerateLandingPage(ctx context.Context, product string) (map[string]string, error) {
	prompt := fmt.Sprintf(`Write landing page copy for: %s

Sections needed:
1. Hero headline (under 10 words, powerful)
2. Hero subheadline (1 sentence, value prop)
3. 3-5 feature/benefit sections with headlines and descriptions
4. Social proof section (testimonial placeholder)
5. FAQ section (5 questions)
6. CTA section (3 CTA variations)
7. Risk reversal (guarantee, free trial, etc.)

Format as JSON with fields: hero_headline, hero_subheadline, features (array), social_proof, faq (array), ctas (array), risk_reversal`, product)
	
	text, err := w.gemini.GenerateText(ctx, prompt, 0.7, 8192)
	if err != nil {
		return nil, err
	}
	
	var result map[string]string
	jsonStr := util.ExtractJSON(text)
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return map[string]string{"raw": text}, nil
	}
	
	return result, nil
}

func (w *Writer) OptimizeForAEO(ctx context.Context, content, targetQuery string) (string, error) {
	// Generate featured snippet optimization
	optimized, err := w.gemini.OptimizeForSnippet(ctx, targetQuery, content)
	if err != nil {
		slog.Warn("snippet optimization failed", "error", err)
		return content, nil
	}
	
	return optimized, nil
}

func (w *Writer) OptimizeForGEO(ctx context.Context, content, targetLLMs string) (string, error) {
	optimized, err := w.gemini.OptimizeForGEO(ctx, content, targetLLMs)
	if err != nil {
		slog.Warn("GEO optimization failed", "error", err)
		return content, nil
	}
	
	return optimized, nil
}

func (w *Writer) GenerateSchemaMarkup(ctx context.Context, schemaType, url, content string) (string, error) {
	markup, err := w.gemini.GenerateSchemaMarkup(ctx, schemaType, url, content)
	if err != nil {
		return "", fmt.Errorf("schema generation failed: %w", err)
	}
	
	return markup, nil
}


