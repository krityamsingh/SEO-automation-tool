package aeo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/crawler"
	"aeo_geo_seo_agent/pkg/database"
	"aeo_geo_seo_agent/pkg/util"
)

// AEO (Answer Engine Optimization) module
// Optimizes content for featured snippets, voice search, knowledge panels, and direct answers

type Engine struct {
	gemini  *ai.GeminiClient
	crawler *crawler.Crawler
	db      *gorm.DB
}

func New(gemini *ai.GeminiClient, crawler *crawler.Crawler, db *gorm.DB) *Engine {
	return &Engine{gemini: gemini, crawler: crawler, db: db}
}

// GenerateSchema creates JSON-LD schema markup for various types
func (e *Engine) GenerateSchema(ctx context.Context, schemaType, url, content string) (string, error) {
	markup, err := e.gemini.GenerateSchemaMarkup(ctx, schemaType, url, content)
	if err != nil {
		return "", err
	}
	
	// Store in cache
	e.db.Create(&database.SchemaCache{
		URL:        url,
		SchemaType: schemaType,
		Markup:     markup,
		CreatedAt:  time.Now(),
	})
	
	return markup, nil
}

// OptimizeForSnippet rewrites content to target featured snippets
func (e *Engine) OptimizeForSnippet(ctx context.Context, question, content string) (string, error) {
	return e.gemini.OptimizeForSnippet(ctx, question, content)
}

// BuildFAQSchema generates FAQPage schema from Q&A pairs
func (e *Engine) BuildFAQSchema(questions []ai.FAQItem) string {
	faqSchema := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "FAQPage",
		"mainEntity": make([]map[string]interface{}, 0, len(questions)),
	}
	
	for _, q := range questions {
		faqSchema["mainEntity"] = append(faqSchema["mainEntity"].([]map[string]interface{}), map[string]interface{}{
			"@type":          "Question",
			"name":           q.Question,
			"acceptedAnswer": map[string]interface{}{
				"@type": "Answer",
				"text":  q.Answer,
			},
		})
	}
	
	jsonBytes, _ := json.MarshalIndent(faqSchema, "", "  ")
	return string(jsonBytes)
}

// BuildHowToSchema generates HowTo schema from step-by-step instructions
func (e *Engine) BuildHowToSchema(title, description string, steps []Step) string {
	schema := map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "HowTo",
		"name":        title,
		"description": description,
		"step":        make([]map[string]interface{}, 0, len(steps)),
	}
	
	for i, step := range steps {
		schema["step"] = append(schema["step"].([]map[string]interface{}), map[string]interface{}{
			"@type": "HowToStep",
			"position": i + 1,
			"name":    step.Name,
			"text":    step.Text,
		})
	}
	
	jsonBytes, _ := json.MarshalIndent(schema, "", "  ")
	return string(jsonBytes)
}

// BuildOrganizationSchema generates Organization schema
func (e *Engine) BuildOrganizationSchema(name, url, logo, description string, socialLinks []string) string {
	schema := map[string]interface{}{
		"@context": "https://schema.org",
		"@type":    "Organization",
		"name":     name,
		"url":      url,
		"logo":     logo,
		"description": description,
		"sameAs":   socialLinks,
	}
	
	jsonBytes, _ := json.MarshalIndent(schema, "", "  ")
	return string(jsonBytes)
}

// BuildArticleSchema generates Article schema
func (e *Engine) BuildArticleSchema(title, url, author, datePublished, dateModified, description, image string) string {
	schema := map[string]interface{}{
		"@context":         "https://schema.org",
		"@type":            "Article",
		"headline":         title,
		"url":              url,
		"author":           map[string]interface{}{"@type": "Person", "name": author},
		"datePublished":    datePublished,
		"dateModified":     dateModified,
		"description":      description,
		"image":            image,
		"publisher":        map[string]interface{}{"@type": "Organization", "name": "Publisher"},
	}
	
	jsonBytes, _ := json.MarshalIndent(schema, "", "  ")
	return string(jsonBytes)
}

// VoiceSearchOptimization converts content to conversational, voice-friendly format
func (e *Engine) VoiceSearchOptimization(ctx context.Context, content string) (string, error) {
	prompt := fmt.Sprintf(`Rewrite the following content for voice search optimization.

Content: %s

Requirements:
1. Conversational tone (natural, spoken language)
2. Direct answers to questions in first 30-40 words
3. Use natural language questions as headings (Who, What, When, Where, Why, How)
4. Short, punchy sentences (under 15 words each)
5. Include question-answer pairs
6. Optimize for "near me" and local queries if applicable
7. Avoid complex jargon - use simple words
8. Readability: Flesch-Kincaid grade 6-8

Output the rewritten content.`, util.SafeTruncate(content, 3000))
	
	return e.gemini.GenerateText(ctx, prompt, 0.7, 4096)
}

// ExtractAnswerCandidates identifies potential featured snippet opportunities
func (e *Engine) ExtractAnswerCandidates(ctx context.Context, content string) ([]AnswerCandidate, error) {
	prompt := fmt.Sprintf(`Analyze the following content and extract potential featured snippet opportunities.

Content: %s

For each snippet opportunity, identify:
- question: The question this content answers
- answer_type: paragraph, list, or table
- optimized_answer: The answer rewritten to win a featured snippet (concise, factual, 40-60 words for paragraph; 5-7 items for list)
- confidence: 1-10 score for how likely this is to win a snippet

Respond as JSON array of objects.`, util.SafeTruncate(content, 3000))
	
	text, err := e.gemini.GenerateText(ctx, prompt, 0.5, 4096)
	if err != nil {
		return nil, err
	}
	
	var candidates []AnswerCandidate
	jsonStr := util.ExtractJSON(text)
	if err := json.Unmarshal([]byte(jsonStr), &candidates); err != nil {
		slog.Warn("failed to parse snippet candidates", "error", err)
		return nil, err
	}
	
	return candidates, nil
}

// EntityExtraction extracts named entities from content for knowledge graph optimization
func (e *Engine) EntityExtraction(ctx context.Context, content string) ([]Entity, error) {
	prompt := fmt.Sprintf(`Extract named entities from the following content that should be linked to knowledge graphs (Wikidata, Google Knowledge Graph).

Content: %s

For each entity, identify:
- name: The entity name
- type: Person, Organization, Product, Place, Event, Concept, Technology
- description: Brief description
- wikidata_query: Search query for Wikidata

Respond as JSON array. Only include significant, notable entities.`, util.SafeTruncate(content, 3000))
	
	text, err := e.gemini.GenerateText(ctx, prompt, 0.5, 4096)
	if err != nil {
		return nil, err
	}
	
	var entities []Entity
	jsonStr := util.ExtractJSON(text)
	if err := json.Unmarshal([]byte(jsonStr), &entities); err != nil {
		slog.Warn("failed to parse entities", "error", err)
		return nil, err
	}
	
	// Save to database
	for _, ent := range entities {
		e.db.FirstOrCreate(&database.Entity{
			Name:        ent.Name,
			Type:        ent.Type,
			Description: ent.Description,
			WikidataID:  ent.WikidataQuery,
		})
	}
	
	return entities, nil
}

type Step struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

type AnswerCandidate struct {
	Question       string `json:"question"`
	AnswerType     string `json:"answer_type"`
	OptimizedAnswer string `json:"optimized_answer"`
	Confidence     int    `json:"confidence"`
}

type Entity struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	Description   string `json:"description"`
	WikidataQuery string `json:"wikidata_query"`
}


