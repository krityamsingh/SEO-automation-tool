# kenerateai.com — Multi-Agent Autonomous SEO & Backlink Platform

A full-stack, multi-user web application powering an autonomous **multi-agent AI system** that researches keywords/backlinks, debates and cross-checks its own findings, reaches consensus, auto-assigns tasks to human interns, verifies submitted work, and feeds real-world results back into its own shared memory to keep improving.

---

## 🏗️ Architecture Overview

```
cmd/agent/main.go              — Entry point, graceful shutdown
internal/
  agent/        — Multi-agent debate system (Trend Research, Backlink Discovery,
                  SEO/AEO/GEO Strategists, Critic/QA, Content Writer,
                  Orchestrator, Task Dispatcher)
  task/         — Task lifecycle engine (dispatch, auto-assign, verify, outcome tracking)
  rag/          — System-wide shared RAG memory (in-memory vector store, BM25 retrieval)
  chat/         — AI Chat Support for Interns (backed by RAG)
  auth/         — Session-based auth (Dev + Intern roles, SHA-256 hashed passwords)
  notification/ — In-dashboard notification manager
  ai/           — Gemini API client (text generation, structured output)
  crawler/      — Colly-based web crawler for verification & research
  scheduler/    — Cron-based autonomous cycle runner
  config/       — Environment-based configuration
  database/     — GORM models (PostgreSQL + SQLite)
  api/          — Gin REST API + embedded single-page dashboard
  seo/          — SEO engine
  aeo/          — AEO engine (Answer Engine Optimization)
  geo/          — GEO engine (Generative Engine Optimization)
  scriptwriter/ — Blog + social + video content generation
  publisher/    — WordPress, Medium, Ghost, webhook publishing
```

---

## 👥 Roles & Access

| Role | Capabilities |
|------|-------------|
| **Dev** | Full admin: add/remove Interns & Devs, trigger debates, review all tasks, act as tiebreaker, see all logs/transcripts |
| **Intern** | View full dashboard (transparent, not siloed), see & claim assigned tasks, submit proof URLs, AI chat support |

- Multiple Dev accounts allowed (no single super-admin hierarchy)
- Interns are added manually by Devs (no public self-signup)
- Default seeded accounts: `dev_admin / admin123` and `alex_intern / intern123`, `maya_intern / intern123`, `sam_intern / intern123`

---

## 🤖 Multi-Agent Debate Loop (11 Agents)

Each autonomous cycle runs a **5-round bounded debate**:

| Round | Agent | Action |
|-------|-------|--------|
| 1 | **Trend Research Agent** 📈 | Proposes a trending keyword (checks RAG for duplicates first) |
| 2 | **Backlink Discovery Agent** 🔗 | Finds candidate sites, asks clarifying question to Trend Agent |
| 3 | **SEO/AEO/GEO Strategist Agents** 🎯💡🌐 | Evaluate keyword/site pair across 3 angles, select winner |
| 4 | **Critic / QA Agent** 🛡️ | Audits for link farms, spam, duplicates; can veto & revise |
| 5 | **Content Writer Agent** ✍️ → **Orchestrator** 👑 | Drafts content; Orchestrator locks in consensus |

After consensus, the **Task Dispatcher Agent** creates the task record and the **Auto-Assignment Engine** routes it to the best-performing intern (weighted by `tasks_completed × 2 + verification_rate − tasks_pending × 10`).

The full agent-to-agent transcript is stored in the database and in the **shared RAG memory** for future retrieval.

---

## 🧠 System-Wide RAG Memory

The RAG engine is **not just a chatbot** — every agent queries and writes to it:

- **Debate transcripts** → prevents re-targeting same keyword/site
- **Research findings** → avoids repeat research already done
- **Task outcomes** → rank movement data self-trains future proposals  
- **Intern chat Q&A** → accumulated support knowledge
- **Verification results** → informs future quality checks

Implementation: In-memory BM25 + TF-IDF relevance scoring with full metadata tagging (keyword, target site, category, task ID, timestamp).

---

## 📋 Task Lifecycle

```
proposed → ready → assigned → in_progress → submitted → verified/rejected → closed
```

- **Proposed**: Agent debate is running
- **Ready**: Debate consensus reached, dispatching
- **Assigned**: Auto-assigned to intern (with notification)
- **In Progress**: Intern is working on it
- **Submitted**: Intern submitted a proof URL
- **Verified/Rejected**: Verification Agent checked the live URL
- **Closed**: Outcome tracking complete

---

## ✅ Verification & Outcome Loop

1. Intern submits proof URL → **Verification Agent** fetches the URL, checks HTTP status, confirms backlink domain matches
2. Task moves to `verified` or `rejected` (with notes); intern receives notification
3. For `verified` tasks: **Outcome/Ranking Agent** calculates rank movement and writes results back to RAG
4. If ranking drops, Devs receive a notification to investigate

---

## 💬 AI Chat Support (Interns)

In-dashboard chat backed by the **same RAG store** as all other agents:
- `POST /api/chat` with `{"task_id": 1, "message": "how do I complete this?"}`
- Returns context-aware answer using task details, debate transcript, and past intern Q&A
- All chat exchanges are ingested back into RAG for future retrieval

---

## 🔔 Notifications

| Trigger | Who Gets Notified |
|---------|------------------|
| New task assigned | Intern (in-dashboard) |
| Submission verified ✅ | Intern (in-dashboard) |
| Submission rejected ⚠️ | Intern (in-dashboard) |
| Debate disagreement (unresolved) | Dev (tiebreaker needed) |
| SEO rank drop detected | Dev |
| Task overdue threshold | Dev |

---

## 📊 Dashboard Features

- **Live Task Board** — Kanban with columns: Assigned → Submitted → Verified → Rejected
- **Metrics Ribbon** — Total/Verified/Pending/Rejected task counts + SEO trend
- **Agent Activity Feed** — Real-time agent-to-agent debate messages
- **Agent Transcript Viewer** — Full debate history for any task (expandable)
- **Per-Intern Stats** — Completion rate, tasks done/pending/overdue (visible to all)
- **SEO Results Panel** — Keyword ranking history chart (Chart.js)
- **Intern AI Chat** — RAG-backed support chat for stuck interns
- **Dev Controls** — Add/remove interns & devs, trigger debates, manage tiebreakers

---

## 🚀 Quick Start

### Run Locally

```bash
# Clone the repo
git clone <repo-url>
cd aeo_geo_seo_agent

# Copy environment config
cp .env.example .env
# Edit .env — at minimum set GEMINI_API_KEY

# Run (SQLite by default, no external DB needed)
go run cmd/agent/main.go

# Or build binary
go build -o kenerateai-agent cmd/agent/main.go
./kenerateai-agent
```

Open `http://localhost:8080` — the dashboard is at the root.

Login with:
- **Dev**: `dev_admin` / `admin123`
- **Intern**: `alex_intern` / `intern123`

### Docker

```bash
docker build -t kenerateai-agent .
docker run -p 8080:8080 --env-file .env kenerateai-agent
```

### Vercel Deployment (Recommended for Serverless)

1. **Option A: Vercel CLI**
   ```bash
   # Install Vercel CLI
   npm i -g vercel

   # Deploy to Vercel
   vercel
   ```

2. **Option B: Vercel Web Dashboard**
   - Import your GitHub repository into [Vercel](https://vercel.com/new).
   - Vercel automatically detects `vercel.json` and `@vercel/go` runtime handler in `api/index.go`.
   - Add environment variables under **Settings -> Environment Variables** (at minimum `GEMINI_API_KEY`).
   - Click **Deploy**.

### Railway.com (Production Container)

1. Create new project at [railway.app](https://railway.app)
2. Connect your GitHub repo
3. Add environment variables from `.env.example`
4. Railway auto-detects Go and deploys — PostgreSQL is optionally available as an add-on

---

## ⚙️ Environment Variables

```bash
# Required
GEMINI_API_KEY=AIzaSy...                # Or use GEMINI_API_KEYS for a comma-separated pool

# AI Models
GEMINI_TEXT_MODEL=gemini-1.5-flash     # Text model (gemini-1.5-flash, gemini-1.5-pro, etc.)
GEMINI_IMAGE_MODEL=gemini-2.0-flash-exp

# Agent Behavior
AGENT_NICHES=technology,saas,ai        # Comma-separated niches for keyword research
AGENT_CYCLE_HOURS=6                    # Hours between autonomous research cycles
CONTENT_AUTO_PUBLISH=false             # Auto-publish to platforms (false = review first)

# Database
DATABASE_URL=sqlite://agent.db         # SQLite (default) or postgres://user:pass@host/db

# Limits
DAILY_CONTENT_LIMIT=5                  # Max content pieces per day
DAILY_GEMINI_LIMIT=200                 # Max Gemini API calls per day

# Publishing (optional)
WORDPRESS_URL=https://your-site.com
WORDPRESS_USERNAME=admin
WORDPRESS_APP_PASSWORD=xxxx
MEDIUM_INTEGRATION_TOKEN=xxxx
GHOST_URL=https://your-ghost.com
GHOST_ADMIN_API_KEY=key_id:secret_hex
WEBHOOK_URL=https://your-webhook.com/publish

# Server
PORT=8080
LOG_LEVEL=info
```

---

## 🛡️ Guardrails (Section 9 Compliance)

- **Link Farm Detection**: Critic/QA Agent checks target domains against known spam patterns
- **Duplicate Prevention**: RAG memory checked before every debate to prevent re-targeting the same keyword or backlink site
- **Concurrent Assignment Guard**: Same keyword cannot be assigned to two interns simultaneously
- **Bounded Debate Rounds**: Max 5 rounds before Orchestrator forces a decision or escalates to Dev

---

## 🔌 API Reference

| Endpoint | Auth | Description |
|----------|------|-------------|
| `GET /` | None | Dashboard (SPA) |
| `POST /api/auth/login` | None | Login (returns session token) |
| `GET /api/auth/me` | Session | Current user info |
| `GET /api/tasks` | None | List all tasks |
| `GET /api/tasks/:id` | None | Task details + debate transcript |
| `POST /api/tasks/:id/submit` | Session | Intern submits proof URL |
| `POST /api/tasks/:id/tiebreaker` | Dev | Dev approve/reject/reassign |
| `GET /api/debates` | None | List agent debates |
| `GET /api/debates/:id` | None | Full debate transcript |
| `POST /api/debates/trigger` | Dev | Trigger new multi-agent debate |
| `GET /api/interns` | None | List all interns with stats |
| `POST /api/interns` | Dev | Add new intern |
| `DELETE /api/interns/:id` | Dev | Remove intern |
| `POST /api/devs` | Dev | Add new Dev account |
| `POST /api/chat` | Session | Intern AI chat (RAG-backed) |
| `GET /api/notifications` | Session | Get user notifications |
| `POST /api/notifications/:id/read` | Session | Mark notification as read |
| `GET /api/analytics` | None | Dashboard analytics |
| `GET /api/agent-chat` | None | Live agent debate messages |
| `GET /health` | None | Health check |
| `GET /status` | None | Agent status & daily usage |
| `POST /trigger` | None | Manually trigger full cycle |

---

## 🛠️ Tech Stack

- **Go** — High-performance, concurrent, single binary deployment
- **Gin** — HTTP web framework
- **GORM** — ORM for PostgreSQL & SQLite
- **Colly** — Web crawler for verification & research
- **Robfig/Cron** — Autonomous scheduling
- **Google Gemini API** — AI text generation for all 11 agents
- **In-Memory BM25/TF-IDF** — Lightweight vector RAG store (no external DB required)
- **Chart.js** — SEO results charting
- **Vanilla HTML/CSS/JS** — Premium glassmorphism dark-mode dashboard

---

## 📄 License

MIT License
