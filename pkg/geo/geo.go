package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"aeo_geo_seo_agent/pkg/ai"
	"aeo_geo_seo_agent/pkg/crawler"
	"aeo_geo_seo_agent/pkg/util"
)

// GEO (Generative Engine Optimization) module
// Optimizes content for LLM citations, AI overviews, and generative search engines

type Engine struct {
	gemini  *ai.GeminiClient
	crawler *crawler.Crawler
	db      *gorm.DB
}

func New(gemini *ai.GeminiClient, crawler *crawler.Crawler, db *gorm.DB) *Engine {
	return &Engine{gemini: gemini, crawler: crawler, db: db}
}

// OptimizeForLLMCitation optimizes content to be cited by AI models
func (e *Engine) OptimizeForLLMCitation(ctx context.Context, content, targetLLMs string) (string, error) {
	return e.gemini.OptimizeForGEO(ctx, content, targetLLMs)
}

// BuildEntityGraph creates semantic relationships between entities
func (e *Engine) BuildEntityGraph(ctx context.Context, content string) (*EntityGraph, error) {
	prompt := fmt.Sprintf(`Analyze the following content and build a semantic entity graph showing relationships between key concepts.

Content: %s

For each entity, identify:
- entity: The entity name
- type: Person, Organization, Product, Concept, Technology, Industry
- related_entities: Array of related entities with relationship type
- importance: 1-10 score for how central this entity is to the content

Format as JSON with a "entities" array and "relationships" array.

	Example relationship: {"source": "Entity A", "target": "Entity B", "type": "created_by"}`, util.SafeTruncate(content, 3000))
	
	text, err := e.gemini.GenerateText(ctx, prompt, 0.5, 4096)
	if err != nil {
		return nil, err
	}
	
	var graph EntityGraph
	jsonStr := util.ExtractJSON(text)
	if err := json.Unmarshal([]byte(jsonStr), &graph); err != nil {
		slog.Warn("failed to parse entity graph", "error", err)
		return nil, err
	}
	
	return &graph, nil
}

// StructureForAI creates AI-friendly content structure
func (e *Engine) StructureForAI(ctx context.Context, content string) (*AIStructure, error) {
	prompt := fmt.Sprintf(`Restructure the following content for optimal AI comprehension and citation.

Content: %s

Requirements:
1. Clear hierarchical heading structure (H1 → H2 → H3)
2. Each section has a one-sentence "TL;DR" at the top
3. Key facts in bold, with inline citations [Source: ...]
4. Bullet points for complex lists (easier for LLMs to parse)
5. Tables for comparisons (explicit structure)
6. Summary box at the end with 3-5 key takeaways
7. Explicit entity mentions (don't use pronouns for key entities)
8. Clear cause-effect relationships
9. Structured data where possible

Format output as JSON:
{
  "structured_content": "full markdown content",
  "key_facts": ["fact 1", "fact 2"],
  "entities_mentioned": ["entity 1", "entity 2"],
  "citation_opportunities": ["claim that needs citation"]
}`, util.SafeTruncate(content, 3000))
	
	text, err := e.gemini.GenerateText(ctx, prompt, 0.5, 8192)
	if err != nil {
		return nil, err
	}
	
	var structure AIStructure
	jsonStr := util.ExtractJSON(text)
	if err := json.Unmarshal([]byte(jsonStr), &structure); err != nil {
		slog.Warn("failed to parse AI structure", "error", err)
		return nil, err
	}
	
	return &structure, nil
}

// EnhanceTrustSignals adds E-E-A-T signals for AI credibility
func (e *Engine) EnhanceTrustSignals(ctx context.Context, content, author, credentials string) (string, error) {
	prompt := fmt.Sprintf(`Enhance the following content with E-E-A-T (Experience, Expertise, Authoritativeness, Trustworthiness) signals for AI credibility.

Content: %s
Author: %s
Credentials: %s

Add:
1. Author bio block with credentials and experience
2. Date published and last updated
3. Expert review or fact-checker mention
4. Source citations for claims and statistics
5. Methodology or process description
6. Limitations and scope clarification
7. Disclosure statements (affiliates, sponsored, etc.)

Output the enhanced content with all trust signals integrated.`, util.SafeTruncate(content, 3000), author, credentials)
	
	return e.gemini.GenerateText(ctx, prompt, 0.5, 8192)
}

// OptimizeForAIOverview optimizes for Google AI Overviews and similar features
func (e *Engine) OptimizeForAIOverview(ctx context.Context, content, targetQuery string) (string, error) {
	prompt := fmt.Sprintf(`Optimize the following content for Google AI Overviews (Search Generative Experience) for the query: "%s"

Content: %s

Requirements for AI Overview optimization:
1. Direct, unambiguous answer in the first 2-3 sentences
2. Multiple perspectives or aspects covered
3. Supporting evidence and data points
4. Nuanced but clear conclusions
5. Avoid ambiguous language that could be misinterpreted by LLMs
6. Include both sides of controversial topics
7. Fact-check all claims - LLMs are sensitive to misinformation
8. Use clear topic sentences that can be extracted as standalone answers

Output the optimized content.`, targetQuery, util.SafeTruncate(content, 3000))
	
	return e.gemini.GenerateText(ctx, prompt, 0.5, 8192)
}

// AnalyzeCitationPotential evaluates how likely content is to be cited by LLMs
func (e *Engine) AnalyzeCitationPotential(ctx context.Context, content string) (*CitationAnalysis, error) {
	prompt := fmt.Sprintf(`Analyze the citation potential of the following content for AI models like ChatGPT, Claude, Perplexity, and Gemini.

Content: %s

Evaluate:
1. factual_density: Score 1-10 for how many facts vs. opinions
2. unique_insights: Score 1-10 for original research or unique perspectives
3. data_richness: Score 1-10 for data, statistics, charts
4. source_quality: Score 1-10 for quality of citations and references
5. clarity: Score 1-10 for how clearly information is presented
6. overall_score: Average of all scores
7. improvement_suggestions: Array of specific improvements to increase citation likelihood

Format as JSON.`, util.SafeTruncate(content, 3000))
	
	text, err := e.gemini.GenerateText(ctx, prompt, 0.5, 4096)
	if err != nil {
		return nil, err
	}
	
	var analysis CitationAnalysis
	jsonStr := util.ExtractJSON(text)
	if err := json.Unmarshal([]byte(jsonStr), &analysis); err != nil {
		slog.Warn("failed to parse citation analysis", "error", err)
		return nil, err
	}
	
	return &analysis, nil
}

// GenerateAICitationSummary creates a summary paragraph designed to be directly quoted by LLMs
func (e *Engine) GenerateAICitationSummary(ctx context.Context, content string) (string, error) {
	prompt := fmt.Sprintf(`Create a concise, citable summary paragraph from the following content that an AI model could directly quote.

Content: %s

Requirements:
- 2-3 sentences maximum
- Contains the core thesis or key finding
- Includes specific numbers, dates, or entities if applicable
- Self-contained (no context needed to understand)
- Factual, not opinionated
- Properly attributed if needed

Output ONLY the summary paragraph.`, util.SafeTruncate(content, 3000))
	
	return e.gemini.GenerateText(ctx, prompt, 0.3, 1024)
}

// TrackAICitations monitors where content is being cited by AI models
func (e *Engine) TrackAICitations(ctx context.Context, url string) (*CitationTracking, error) {
	// This would require integrating with AI platform APIs
	// For now, return a placeholder with instructions
	return &CitationTracking{
		URL:        url,
		CheckedAt:  time.Now(),
		Status:     "tracking_setup",
		Note:       "AI citation tracking requires integration with Perplexity API, Google Search Console, or custom monitoring. Set up Perplexity API key to track citations.",
		Citations:  []AICitation{},
	}, nil
}

type EntityGraph struct {
	Entities      []GraphEntity      `json:"entities"`
	Relationships []GraphRelationship `json:"relationships"`
}

type GraphEntity struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Importance int    `json:"importance"`
}

type GraphRelationship struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

type AIStructure struct {
	StructuredContent      string   `json:"structured_content"`
	KeyFacts              []string `json:"key_facts"`
	EntitiesMentioned     []string `json:"entities_mentioned"`
	CitationOpportunities []string `json:"citation_opportunities"`
}

type CitationAnalysis struct {
	FactualDensity         int      `json:"factual_density"`
	UniqueInsights         int      `json:"unique_insights"`
	DataRichness           int      `json:"data_richness"`
	SourceQuality          int      `json:"source_quality"`
	Clarity                int      `json:"clarity"`
	OverallScore           int      `json:"overall_score"`
	ImprovementSuggestions []string `json:"improvement_suggestions"`
}

type CitationTracking struct {
	URL       string       `json:"url"`
	CheckedAt time.Time    `json:"checked_at"`
	Status    string       `json:"status"`
	Note      string       `json:"note"`
	Citations []AICitation `json:"citations"`
}

type AICitation struct {
	Source    string    `json:"source"` // e.g., "ChatGPT", "Perplexity", "Claude"
	Query     string    `json:"query"`
	Context   string    `json:"context"`
	FoundAt   time.Time `json:"found_at"`
}
