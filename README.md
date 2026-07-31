# AEO + GEO + SEO Autonomous Agent

A fully autonomous Go-based agent that researches keywords, generates content (blogs, social posts, video scripts, emails, ads), optimizes for SEO, AEO (Answer Engine Optimization), and GEO (Generative Engine Optimization), and publishes to your platforms — all running on a schedule.

## What This Agent Does

1. **Autonomous Research**: Discovers trending keywords in your niches using Gemini
2. **Content Creation**: Generates full blog posts, social media scripts, video scripts, email sequences, ad copy, and landing pages
3. **SEO Optimization**: On-page audits, keyword analysis, competitor analysis, backlink monitoring
4. **AEO Optimization**: Featured snippet targeting, schema markup generation (FAQPage, HowTo, Article, Organization), voice search optimization, entity extraction for knowledge graphs
5. **GEO Optimization**: LLM citation optimization, entity relationship building, AI-friendly content structuring, E-E-A-T signal enhancement, AI Overview optimization (Google SGE, Bing Copilot, Perplexity)
6. **Auto-Publishing**: Publishes to WordPress, Medium, Ghost, or custom webhook
7. **Daily Reports**: Tracks usage, content performance, and agent health

## Quick Start (Railway.com)

### 1. Deploy to Railway

```bash
# Clone the repo
git clone <repo-url>
cd aeo_geo_seo_agent

# Deploy to Railway (requires Railway CLI)
railway login
railway link
railway up
```

Or manually:
1. Create new project on [Railway](https://railway.app)
2. Connect GitHub repo or upload code
3. Add environment variables (see `.env.example`)
4. Railway auto-detects Go and deploys

### 2. Environment Variables

Copy `.env.example` to `.env` and fill in:

```bash
# Required
GEMINI_API_KEY=your_gemini_api_key_here

# Agent Behavior
AGENT_NICHES=technology,saas,ai
AGENT_CYCLE_HOURS=6
CONTENT_AUTO_PUBLISH=false
CONTENT_MIN_WORDS=1500
CONTENT_MAX_WORDS=3000

# Publishing (at least one if auto-publish enabled)
WORDPRESS_URL=https://your-site.com
WORDPRESS_USERNAME=admin
WORDPRESS_APP_PASSWORD=your_app_password
MEDIUM_INTEGRATION_TOKEN=your_medium_token
GHOST_ADMIN_API_KEY=your_ghost_key
WEBHOOK_URL=https://your-webhook.com/publish

# Database (Railway provides this automatically)
DATABASE_URL=postgres://user:pass@host:5432/db
# Or use SQLite: DATABASE_URL=sqlite://agent.db

# Limits
DAILY_CONTENT_LIMIT=5
DAILY_GEMINI_LIMIT=200

# Server
PORT=8080
LOG_LEVEL=info
```

### 3. Run Locally

```bash
# Install dependencies
go mod tidy

# Run
go run cmd/agent/main.go

# Or build binary
go build -o agent cmd/agent/main.go
./agent
```

## API Endpoints

- `GET /health` — Health check
- `GET /status` — Agent status, daily usage, recent logs, next cycle
- `GET /keywords?niche=technology` — List tracked keywords
- `GET /content?status=draft&limit=20` — List content pieces
- `GET /content/:id` — Get specific content
- `POST /content/:id/approve` — Approve content for publishing
- `POST /content/:id/reject` — Reject content
- `GET /logs?limit=50` — Agent activity logs
- `POST /trigger` — Manually trigger a full cycle

## The Agent Cycle (Autonomous)

Every N hours (configurable, default 6):

1. **Keyword Research**: Generates trending keywords via Gemini for each configured niche
2. **Content Ideas**: Scores ideas by SEO + AEO + GEO potential, picks top ones
3. **Content Creation**: Generates full blog post + social scripts + video script + schema markup
4. **Optimization**: AEO (snippets, schema) + GEO (LLM citations, entity graphs) + SEO (meta, keywords)
5. **Publishing**: Publishes to configured platforms (if auto-publish enabled) or marks as "pending_review"
6. **Reporting**: Logs daily activity, generates usage report

## Daily Caps

- `DAILY_CONTENT_LIMIT`: Max content pieces created per day (default: 5)
- `DAILY_GEMINI_LIMIT`: Max Gemini API calls per day (default: 200)

Caps are enforced in code — the agent stops when limits are reached and resumes the next day.

## Architecture

```
cmd/agent/main.go              — Entry point, graceful shutdown
internal/config/               — Environment-based config
internal/database/             — GORM + PostgreSQL/SQLite models
internal/scheduler/            — Cron-based autonomous cycle
internal/ai/                   — Gemini API client (text + image)
internal/crawler/              — Colly-based web crawler
internal/seo/                  — On-page/off-page SEO analysis
internal/aeo/                  — Schema generation, snippet optimization, voice search, entity extraction
internal/geo/                  — LLM citation optimization, entity graphs, AI structuring, E-E-A-T, AI overview
internal/scriptwriter/         — Blog, social, video, email, ad, landing page generators
internal/publisher/            — WordPress, Medium, Ghost, webhook integrations
internal/api/                  — Gin REST API for monitoring
```

## Key Features

### SEO
- Full website crawl and audit (title, meta, headings, images, content)
- Keyword extraction and density analysis
- Competitor gap analysis
- Backlink monitoring (liveness checks)

### AEO (Answer Engine Optimization)
- Featured snippet optimization (paragraph, list, table)
- Schema markup generation: FAQPage, HowTo, Article, Organization, Person, Product
- Voice search optimization (conversational queries, natural language)
- Entity extraction and knowledge graph optimization
- Answer capsule generation

### GEO (Generative Engine Optimization)
- LLM citation optimization (ChatGPT, Claude, Perplexity, Gemini)
- Entity relationship graph building
- AI-friendly content structuring (clear headings, fact boxes, TL;DR)
- E-E-A-T signal enhancement (Experience, Expertise, Authoritativeness, Trustworthiness)
- AI Overview optimization (Google SGE, Bing Copilot)
- Citation potential analysis
- AI-quotable summary generation

### Content Generation
- Full blog posts (1500-3000 words) with TOC, FAQ, TL;DR, takeaways, CTAs
- Social media scripts: Twitter/X, LinkedIn, Instagram, TikTok, Facebook
- Video scripts: YouTube, TikTok, Reels with hooks, timestamps, B-roll, CTAs
- Email sequences: welcome, nurture, sales, re-engagement
- Ad copy: Google Ads, Facebook/Instagram Ads, LinkedIn Ads
- Landing page copy: headlines, features, testimonials, FAQ, CTAs

## Deployment

### Railway.com (Recommended)
- Go is natively supported
- PostgreSQL provided automatically
- Auto-scaling, zero-config

### Docker
```bash
docker build -t aeo-agent .
docker run -p 8080:8080 --env-file .env aeo-agent
```

### Systemd
```bash
# Copy the service file
sudo cp systemd/aeo-agent.service /etc/systemd/system/
sudo systemctl enable aeo-agent
sudo systemctl start aeo-agent
```

## Tech Stack

- **Go 1.22** — High-performance, concurrent, single binary
- **Gin** — HTTP web framework
- **GORM** — Database ORM (PostgreSQL + SQLite)
- **Colly** — Web crawler
- **Robfig/Cron** — Job scheduling
- **Google Generative AI (Gemini)** — AI content generation
- **Zap/Slog** — Structured logging

## License

MIT License
