package scriptwriter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

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

// GenerateFullArticleDraft generates a full-length (1000-2000 words) SEO-structured markdown article.
func (w *Writer) GenerateFullArticleDraft(ctx context.Context, topic, keyword, targetURL, anchorText string) (string, error) {
	if strings.TrimSpace(anchorText) == "" {
		if strings.TrimSpace(keyword) != "" {
			anchorText = strings.TrimSpace(keyword)
		} else if strings.TrimSpace(topic) != "" {
			anchorText = strings.TrimSpace(topic)
		} else {
			anchorText = "Target Resource"
		}
	}
	if strings.TrimSpace(targetURL) == "" {
		targetURL = "https://example.com"
	}
	if strings.TrimSpace(topic) == "" {
		if strings.TrimSpace(keyword) != "" {
			topic = strings.TrimSpace(keyword)
		} else {
			topic = "Target Topic"
		}
	}
	if strings.TrimSpace(keyword) == "" {
		if strings.TrimSpace(topic) != "" && topic != "Target Topic" {
			keyword = topic
		} else {
			keyword = "Target Keyword"
		}
	}

	if w != nil && w.gemini != nil {
		prompt := fmt.Sprintf(`Write a comprehensive, full-length (1000-2000 words) SEO-structured markdown article on the topic: "%s".

Target Keyword: "%s"
Target Site / Backlink URL: "%s"
Exact Anchor Text: "%s"

Requirements:
1. Include a main title using Markdown (# Title).
2. Structure with clear sections (H2 ##, H3 ###): Executive Summary, Core Concepts, Technical Deep Dive, Best Practices, FAQ, and Conclusion.
3. Length MUST be between 1000 and 2000 words.
4. You MUST embed the exact link [%s](%s) naturally within a body section paragraph.

Output ONLY the formatted Markdown article.`, topic, keyword, targetURL, anchorText, anchorText, targetURL)

		text, err := w.gemini.GenerateText(ctx, prompt, 0.7, 8192)
		if err == nil && len(strings.Fields(text)) >= 300 {
			return text, nil
		}
		slog.Warn("scriptwriter: AI text generation unfulfilled, using structured fallback draft", "error", err)
	}

	return fallbackFullArticleDraft(topic, keyword, targetURL, anchorText), nil
}

// GenerateInternExecutionGuide generates clear step-by-step instructions for interns.
func (w *Writer) GenerateInternExecutionGuide(ctx context.Context, task *database.Task) (string, error) {
	if task == nil {
		return "", fmt.Errorf("task cannot be nil")
	}

	cleanTargetDomain := strings.TrimPrefix(task.BacklinkTarget, "https://")
	cleanTargetDomain = strings.TrimPrefix(cleanTargetDomain, "http://")
	cleanTargetDomain = strings.TrimSuffix(cleanTargetDomain, "/")

	if w != nil && w.gemini != nil {
		prompt := fmt.Sprintf(`Generate step-by-step execution instructions for an intern assigning publishing work for the following task:
Title: "%s"
Target Site Domain: "%s"
Keyword: "%s"
Anchor Text: "%s"
Target URL: "%s"

Detail:
1. Target site registration instructions
2. Article placement and formatting guidance
3. Exact anchor text insertion
4. Target link embedding
5. Proof submission process (format URL example as https://%s/posts/my-article)

Format in clean Markdown.`, task.Title, cleanTargetDomain, task.Keyword, task.TargetAnchorText, task.TargetLinkURL, cleanTargetDomain)

		guide, err := w.gemini.GenerateText(ctx, prompt, 0.5, 4096)
		if err == nil && len(strings.TrimSpace(guide)) > 50 {
			return guide, nil
		}
		slog.Warn("scriptwriter: AI guide generation unfulfilled, using structured fallback guide", "error", err)
	}

	return fallbackInternExecutionGuide(task), nil
}

// Package-level functions for direct invocation

func GenerateFullArticleDraft(ctx context.Context, topic, keyword, targetURL, anchorText string) (string, error) {
	w := New(nil, nil)
	return w.GenerateFullArticleDraft(ctx, topic, keyword, targetURL, anchorText)
}

func GenerateInternExecutionGuide(ctx context.Context, task *database.Task) (string, error) {
	w := New(nil, nil)
	return w.GenerateInternExecutionGuide(ctx, task)
}

func fallbackFullArticleDraft(topic, keyword, targetURL, anchorText string) string {
	if strings.TrimSpace(anchorText) == "" {
		if strings.TrimSpace(keyword) != "" {
			anchorText = strings.TrimSpace(keyword)
		} else if strings.TrimSpace(topic) != "" {
			anchorText = strings.TrimSpace(topic)
		} else {
			anchorText = "Target Resource"
		}
	}
	if strings.TrimSpace(targetURL) == "" {
		targetURL = "https://example.com"
	}
	if strings.TrimSpace(topic) == "" {
		if strings.TrimSpace(keyword) != "" {
			topic = strings.TrimSpace(keyword)
		} else {
			topic = "Target Topic"
		}
	}
	if strings.TrimSpace(keyword) == "" {
		if strings.TrimSpace(topic) != "" && topic != "Target Topic" {
			keyword = topic
		} else {
			keyword = "Target Keyword"
		}
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Comprehensive Technical Guide to %s: Architectural Strategies for Modern Web Optimization\n\n", topic))

	sb.WriteString("## 1. Executive Summary & Strategic Landscape\n\n")
	sb.WriteString(fmt.Sprintf("In the rapidly evolving digital ecosystem of technical content and search engine dominance, establishing authority around **%s** has become a pivotal imperative for enterprise digital platforms. ", topic))
	sb.WriteString(fmt.Sprintf("As artificial intelligence systems, neural ranking models, and generative answer engines reshape how users discover information, targeting core queries such as **%s** requires a rigorous, multi-layered architectural approach. ", keyword))
	sb.WriteString("Legacy search engine optimization techniques relying solely on repetitive keyword placement and superficial backlinks are no longer sufficient to maintain competitive rankings in modern SERP environments. ")
	sb.WriteString("Instead, modern digital strategy demands semantic precision, high-density informational content, structured entity definitions, and verified backlink authority.\n\n")
	sb.WriteString("This comprehensive technical guide provides software architects, SEO engineering leads, and digital growth specialists with a detailed, actionable blueprint for planning, authoring, and deploying high-impact content. ")
	sb.WriteString("By combining structured Markdown formatting, natural language keyword embedding, and contextual link placement, organizations can systematically maximize both organic search rankings and generative engine citation probability.\n\n")

	sb.WriteString(fmt.Sprintf("## 2. Core Concepts and Architectural Principles of %s\n\n", keyword))
	sb.WriteString("To construct content that withstands modern search engine scrutiny, engineering teams must understand the underlying mechanics of semantic indexing and vector embeddings. ")
	sb.WriteString("Search engines like Google, Bing, and AI-driven platforms like ChatGPT and Perplexity process web pages through deep neural representations. ")
	sb.WriteString(fmt.Sprintf("When analyzing articles focused on **%s**, these systems evaluate entity relationships, topical completeness, document structure, and the external trust signals associated with outbound references.\n\n", keyword))
	sb.WriteString("Modern optimization relies on three interconnected paradigms:\n\n")
	sb.WriteString("1. **Search Engine Optimization (SEO)**: Ensuring technical crawlability, mobile responsiveness, fast time-to-first-byte (TTFB), clear heading structures (H1 through H6), and meta descriptions.\n")
	sb.WriteString("2. **Answer Engine Optimization (AEO)**: Formatting direct, factual answers within concise 40-to-60 word introductory blocks designed to win Google Featured Snippets and instant answer boxes.\n")
	sb.WriteString("3. **Generative Engine Optimization (GEO)**: Embedding structured entity data, statistical evidence, expert citations, and key takeaways boxes that enable Large Language Models (LLMs) to ingest and quote content directly.\n\n")
	sb.WriteString("By addressing all three paradigms within a single unified article draft, digital publications can achieve maximum reach across both legacy SERP results and emerging conversational search interfaces.\n\n")

	sb.WriteString("## 3. Practical Implementation Methodologies & Contextual Link Placement\n\n")
	sb.WriteString(fmt.Sprintf("Executing a successful content distribution campaign requires seamless integration between topical narratives and authoritative backlink targets. When developing articles centered on **%s**, writers must integrate contextual references into high-relevance paragraphs where readers naturally seek additional verification.\n\n", keyword))
	sb.WriteString(fmt.Sprintf("For example, when establishing technical credibility and domain reference authority, pointing readers to authoritative external documentation such as [%s](%s) delivers immediate contextual validation to both human audience members and search engine web crawlers. ", anchorText, targetURL))
	sb.WriteString("Contextual hyperlinks embedded within meaningful, topic-aligned body paragraphs pass substantial PageRank equity and topical authority, driving referral traffic while boosting search engine confidence in the underlying topic domain.\n\n")
	sb.WriteString("When embedding contextual links, technical authors must strictly adhere to the following publication standards:\n\n")
	sb.WriteString("- **Natural Anchor Text Syntax**: Ensure the anchor text phrase flows naturally within the surrounding sentence structure without awkward punctuation or artificial phrasing.\n")
	sb.WriteString("- **Topical Alignment**: Position the hyperlink inside a paragraph that directly discusses the destination site's domain expertise and core industry offerings.\n")
	sb.WriteString("- **Link Integrity Verification**: Confirm that the target URL resolves successfully with an HTTP 200 OK status code, valid TLS/SSL certificates, and proper canonical tags.\n")
	sb.WriteString("- **Avoid Over-Optimization**: Vary anchor text across distributed publication campaigns to prevent algorithmic penalization for exact-match manipulation.\n\n")

	sb.WriteString("## 4. Advanced Technical Workflows & Autonomous Task Dispatch\n\n")
	sb.WriteString("Scaling digital optimization across enterprise platforms requires automating content generation, task assignment, and proof verification. ")
	sb.WriteString("By implementing autonomous agent architectures, organizations can stream debate consensus outputs directly into content scriptwriting engines, generating comprehensive article drafts alongside customized step-by-step execution guides for intern distribution networks.\n\n")
	sb.WriteString("Key components of an autonomous dispatch workflow include:\n\n")
	sb.WriteString("- **Multi-Provider AI Pool Management**: Leveraging multi-tiered AI key pools (Gemini, Kimi, MiniMax) with automatic key rotation and provider failover ensures zero downtime during high-volume generation runs.\n")
	sb.WriteString("- **Retrieval-Augmented Generation (RAG)**: Indexing generated full-length articles and step-by-step execution guides into vector and BM25 memory stores prevents duplicate target collisions and supports semantic query retrieval.\n")
	sb.WriteString("- **Automated Verification Agents**: Verifying published contributor proof URLs using SSRF-guarded HTTP network clients guarantees that live links are active, valid, and returning 200 OK status codes.\n")
	sb.WriteString("- **Closed-Loop Ranking Feedback**: Tracking search ranking positions post-verification provides continuous reinforcement learning data for automated strategy refinement.\n\n")

	sb.WriteString("## 5. Performance Metrics, Benchmarking & Audit Analysis\n\n")
	sb.WriteString("To evaluate the success of a content deployment campaign, digital teams must measure quantitative metrics across multiple analytical dimensions. ")
	sb.WriteString("Establishing baseline benchmarks prior to article publication allows engineers to accurately track organic traffic acquisition, ranking movements, and backlink indexation velocity.\n\n")
	sb.WriteString("Primary performance evaluation indicators:\n\n")
	sb.WriteString("- **Search Engine Rank Position (SERP)**: Tracking weekly movement for primary and secondary keyword variations in search result pages.\n")
	sb.WriteString("- **Indexation Speed**: Monitoring the elapsed time between intern proof submission and initial search engine crawler indexation.\n")
	sb.WriteString("- **Contextual Backlink Equity**: Calculating referral traffic volume, domain authority pass-through, and target link health.\n")
	sb.WriteString("- **Organic Conversion Rate**: Measuring user engagement, time on page, and call-to-action (CTA) click-through percentages from organic search visitors.\n\n")

	sb.WriteString("## 6. Frequently Asked Questions (FAQ)\n\n")
	sb.WriteString(fmt.Sprintf("### Q1: What makes %s fundamental to modern search engine optimization?\n", keyword))
	sb.WriteString("A: It establishes deep semantic alignment between user intent and document structure, enabling search engines and AI models to parse, index, and cite content efficiently.\n\n")
	sb.WriteString(fmt.Sprintf("### Q2: How should anchor text like \"%s\" be placed within the article draft?\n", anchorText))
	sb.WriteString("A: It must be embedded naturally within a relevant body paragraph, hyperlinking directly to the target URL to provide contextual reference value.\n\n")
	sb.WriteString("### Q3: What is the recommended length for an SEO-structured full article draft?\n")
	sb.WriteString("A: A full-length comprehensive article draft should contain between 1000 and 2000 words, allowing for thorough technical analysis, FAQ sections, and structured takeaways.\n\n")
	sb.WriteString("### Q4: How does RAG memory indexing support automated task dispatching?\n")
	sb.WriteString("A: RAG memory chunks and indexes published article drafts and execution guides under the \"task_content\" category, preventing duplicate keyword assignments and facilitating semantic knowledge retrieval.\n\n")
	sb.WriteString("### Q5: What security safeguards should be enforced during live proof URL verification?\n")
	sb.WriteString("A: Proof verification systems must apply strict SSRF protections, blocking requests to private IP ranges (RFC1918), loopback addresses, AWS metadata endpoints, and non-HTTP schemes.\n\n")

	sb.WriteString("## 7. Conclusion & Operational Roadmap\n\n")
	sb.WriteString(fmt.Sprintf("Mastering **%s** requires a balanced combination of strategic content generation, authoritative backlink integration, and automated verification workflows. ", topic))
	sb.WriteString("By implementing the full-length article structures and step-by-step execution protocols outlined in this guide, digital teams can reliably achieve superior search rankings, drive targeted organic traffic, and secure long-term digital domain authority.\n")

	return sb.String()
}

func fallbackInternExecutionGuide(task *database.Task) string {
	rawTargetSite := task.BacklinkTarget
	if rawTargetSite == "" {
		rawTargetSite = "Target Publication Site"
	}
	targetDomain := strings.TrimPrefix(rawTargetSite, "https://")
	targetDomain = strings.TrimPrefix(targetDomain, "http://")
	targetDomain = strings.TrimSuffix(targetDomain, "/")
	if targetDomain == "" {
		targetDomain = "Target Publication Site"
	}

	targetURL := task.TargetLinkURL
	if targetURL == "" {
		targetURL = rawTargetSite
	}
	anchorText := task.TargetAnchorText
	if anchorText == "" {
		anchorText = task.Keyword
	}
	if anchorText == "" {
		anchorText = task.Title
	}
	if anchorText == "" {
		anchorText = "Target Resource"
	}

	title := task.Title
	if title == "" {
		title = task.Keyword
	}
	if title == "" {
		title = "Article Title"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Intern Execution Guide — Task #%d\n\n", task.ID))
	sb.WriteString("## Executive Summary & Target Details\n")
	sb.WriteString(fmt.Sprintf("- **Task ID**: %d\n", task.ID))
	sb.WriteString(fmt.Sprintf("- **Target Publication Site**: %s\n", targetDomain))
	sb.WriteString(fmt.Sprintf("- **Target Keyword**: %s\n", task.Keyword))
	sb.WriteString(fmt.Sprintf("- **Target Anchor Text**: %s\n", anchorText))
	sb.WriteString(fmt.Sprintf("- **Target Link URL**: %s\n", targetURL))
	sb.WriteString(fmt.Sprintf("- **Article Title**: %s\n\n", title))
	sb.WriteString("---\n\n")

	sb.WriteString("## Step-by-Step Execution Protocol\n\n")
	sb.WriteString("### Step 1: Target Site Account Registration & Onboarding\n")
	sb.WriteString(fmt.Sprintf("1. Open your web browser and navigate to `%s`.\n", targetDomain))
	sb.WriteString("2. Locate the **Register / Sign Up** or **Become a Contributor** button.\n")
	sb.WriteString("3. Create an author profile using your assigned credentials.\n")
	sb.WriteString("4. Complete profile configuration (add author photo and brief bio) to ensure editorial compliance.\n\n")

	sb.WriteString("### Step 2: Article Draft Placement & Formatting\n")
	sb.WriteString(fmt.Sprintf("1. Open the post editor dashboard on `%s`.\n", targetDomain))
	sb.WriteString(fmt.Sprintf("2. Set the post title to: `%s`.\n", title))
	sb.WriteString("3. Copy the full article text from the `ArticleDraft` field in your task dashboard.\n")
	sb.WriteString("4. Paste the content into the editor and verify Markdown formatting (headings `#`, `##`, `###`, lists, and paragraphs).\n\n")

	sb.WriteString("### Step 3: Exact Anchor Text Insertion & Target Link Embedding\n")
	sb.WriteString(fmt.Sprintf("1. Search within the article body for the exact anchor text phrase: \"%s\".\n", anchorText))
	sb.WriteString(fmt.Sprintf("2. Highlight \"%s\" and insert the hyperlinked target URL: `%s`.\n", anchorText, targetURL))
	sb.WriteString("3. Verify that the link is inserted as a contextual, dofollow hyperlink without trailing syntax errors.\n\n")

	sb.WriteString("### Step 4: Final Editorial Review & Article Publication\n")
	sb.WriteString("1. Use the editor preview feature to verify layout rendering, spacing, and link placement.\n")
	sb.WriteString("2. Confirm that all section headings render clearly and no placeholder text remains.\n")
	sb.WriteString("3. Click **Publish** or **Submit for Review** to publish the article live on the platform.\n\n")

	sb.WriteString("### Step 5: Proof Submission & Verification Trigger\n")
	sb.WriteString(fmt.Sprintf("1. Once published, copy the full live URL of the published article (e.g. `https://%s/posts/my-article`).\n", targetDomain))
	sb.WriteString("2. Return to the intern task dashboard.\n")
	sb.WriteString("3. Paste the URL into the **Submitted Proof URL** input field.\n")
	sb.WriteString("4. Click **Submit Proof** to trigger the automated Verification Agent to check HTTP status (200 OK) and log verification details.\n")

	return sb.String()
}


