# agent-platform-api

Go REST API server for the Agent Platform. Wraps [agent-core](https://github.com/bitop-dev/agent-core) with persistence, authentication, real-time WebSocket streaming, and a skills registry.

> **Status**: Phase 2 complete — 32 files, 4.6K lines, 22 tests, 8 commits. All endpoints tested with real LLM execution.

---

## Quick Start

```bash
# Build
make build

# Run (SQLite dev mode)
PORT=8080 JWT_SECRET=your-secret-32-chars-min DATABASE_URL=sqlite://data/platform.db ./bin/api

# Or with make
JWT_SECRET=your-secret-32-chars-min make run
```

### Docker

```bash
docker compose up --build
```

---

## API Endpoints

### Public (no auth required)

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/api/v1/models` | List supported LLM models (filter by `?provider=`) |

### Auth (rate limited: 10/min per IP)

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/auth/register` | Register new user → returns access + refresh tokens |
| POST | `/api/v1/auth/login` | Login → returns access + refresh tokens |
| POST | `/api/v1/auth/refresh` | Exchange refresh token for new access token |

### Protected (Bearer token required, 120/min per IP)

#### User
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/me` | Current user info |
| GET | `/api/v1/dashboard` | Aggregate stats (agent count, run breakdown, recent runs) |

#### Agents
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/agents` | List user's agents |
| POST | `/api/v1/agents` | Create agent (validates YAML config) |
| GET | `/api/v1/agents/:id` | Get agent details |
| PUT | `/api/v1/agents/:id` | Update agent (returns updated agent) |
| DELETE | `/api/v1/agents/:id` | Delete agent |

#### Runs
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/runs` | Start a run (async, returns immediately) |
| GET | `/api/v1/runs` | List user's runs |
| GET | `/api/v1/runs/:id` | Get run status, output, metrics |
| GET | `/api/v1/runs/:id/events` | Get full event log |
| POST | `/api/v1/runs/:id/cancel` | Cancel an in-flight run |

#### API Keys
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/api-keys` | Store LLM API key (encrypted at rest) |
| GET | `/api/v1/api-keys` | List keys (hints only, never full key) |
| DELETE | `/api/v1/api-keys/:id` | Delete API key |

#### Skills
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/skills` | Create skill |
| GET | `/api/v1/skills` | List skills (filter by `?tier=`) |
| GET | `/api/v1/skills/:id` | Get skill details |
| PUT | `/api/v1/skills/:id` | Update skill (owner only) |
| DELETE | `/api/v1/skills/:id` | Delete skill (owner only) |
| POST | `/api/v1/agents/:id/skills` | Attach skill to agent |
| DELETE | `/api/v1/agents/:id/skills/:skill_id` | Detach skill from agent |
| GET | `/api/v1/agents/:id/skills` | List agent's skills |

### WebSocket

| Path | Description |
|---|---|
| `ws://host/ws/runs/:id` | Stream run events in real time |

---

## Architecture

```
                    HTTP Request
                         │
                    ┌────▼────┐
                    │  Fiber  │  (CORS, logger, recover)
                    └────┬────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
         Rate Limiter  Auth MW   Request ID
              │          │          │
              └──────────┼──────────┘
                         │
                    ┌────▼────┐
                    │ Handlers│  (auth, agents, runs, skills, api-keys)
                    └────┬────┘
                         │
              ┌──────────┼──────────┐
              │          │          │
           Store      Runner     WS Hub
          (sqlc)    (goroutine   (pub/sub
                      pool)     by run ID)
              │          │          │
              ▼          ▼          ▼
           SQLite    agent-core   WebSocket
           /Postgres  pkg/agent   clients
```

### Run Execution Flow

1. `POST /api/v1/runs` → creates DB record (status: queued) → enqueues `RunRequest`
2. Worker goroutine picks up request → resolves API key from DB → calls `agent.QuickRun()`
3. Agent-core executes: LLM calls, tool execution, streaming events
4. Events are written to DB (`run_events` table) AND broadcast to WebSocket hub
5. On completion: updates run record with output, metrics, status

### Authentication

- **Register/Login**: returns access token (60 min) + refresh token (7 days)
- **Refresh**: exchange refresh token for new access token
- **Token types enforced**: refresh tokens rejected on API routes, access tokens rejected on refresh endpoint
- **Passwords**: bcrypt hashed
- **API keys**: AES-256-GCM encrypted at rest (dev mode: plaintext with warning)

---

## Database

### Schema (3 migrations)

| Table | Description |
|---|---|
| `users` | User accounts (email, name, password hash) |
| `api_keys` | LLM provider keys (encrypted, with base_url) |
| `agents` | Agent configs (name, prompt, model, YAML) |
| `runs` | Run records (status, output, metrics, timestamps) |
| `run_events` | Event log per run (seq, type, JSON data) |
| `skills` | Skill registry (name, tier, SKILL.md content) |
| `agent_skills` | Agent ↔ skill linking (ordered, with config) |

### Tooling

- **goose**: embedded SQL migrations, auto-run on startup
- **sqlc**: type-safe Go code generated from SQL queries
- **SQLite** for dev (zero config), **PostgreSQL** for production
- Driver auto-detected from `DATABASE_URL` prefix (`sqlite://` vs `postgres://`)

---

## Configuration

All configuration via environment variables:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Server port |
| `HOST` | `0.0.0.0` | Bind address |
| `DATABASE_URL` | `sqlite://data/platform.db` | Database connection |
| `JWT_SECRET` | (required) | HS256 signing key (min 32 chars) |
| `ENCRYPTION_KEY` | (optional) | AES-256 hex key for API key encryption |
| `DEFAULT_MODEL` | `gpt-4o` | Default model for new agents |
| `DEFAULT_PROVIDER` | `openai` | Default provider for new agents |

---

## Tech Stack

| Layer | Choice |
|---|---|
| HTTP | [Fiber](https://gofiber.io/) v2 |
| Database | SQLite ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)) / PostgreSQL |
| SQL | [sqlc](https://sqlc.dev/) — type-safe generated Go |
| Migrations | [goose](https://github.com/pressly/goose) — embedded SQL |
| Auth | [golang-jwt/jwt](https://github.com/golang-jwt/jwt) v5 + bcrypt |
| WebSocket | [gofiber/contrib/websocket](https://github.com/gofiber/contrib/tree/main/websocket) |
| Agent Runtime | [agent-core/pkg/agent](https://github.com/bitop-dev/agent-core) |
| Logging | `log/slog` (JSON to stdout) |
| IDs | [google/uuid](https://github.com/google/uuid) v4 |

---

## Project Structure

```
cmd/api/                    Server entrypoint, slog setup, graceful shutdown
internal/
  api/                      Fiber router, error handler, integration tests
    handlers/
      auth.go               Register, login, refresh
      agents.go             Agent CRUD
      runs.go               Run create, get, list, events, cancel
      skills.go             Skill CRUD + agent-skill linking
      api_keys.go           API key store/list/delete
      models.go             Model catalog
      dashboard.go          Dashboard stats
      dto.go                Response DTOs (flatten sql.Null* types)
    middleware/
      auth.go               JWT middleware (rejects refresh tokens)
      ratelimit.go          Token bucket rate limiter
      requestid.go          X-Request-ID header
  auth/
    auth.go                 JWT generation (access + refresh pairs)
    encrypt.go              AES-256-GCM encryptor, key hints
  config/
    config.go               Environment variable loader
  db/
    db.go                   Database wrapper (SQLite/Postgres detection)
    migrations.go           Embedded goose migrations
    migrations/
      001_initial_schema.sql
      002_skills.sql
      003_api_key_base_url.sql
    query/                  sqlc query definitions (.sql)
    schema.sql              Full schema for sqlc codegen
    sqlc/                   Generated Go code (do not edit)
  runner/
    runner.go               Goroutine pool (4 workers), RunRequest dispatch
  ws/
    hub.go                  WebSocket hub (room-based pub/sub by run ID)
Dockerfile                  Multi-stage build (golang:1.25-alpine)
docker-compose.yml          Dev setup (SQLite) + commented Postgres
openapi.yaml                OpenAPI 3.1 spec for all endpoints
sqlc.yaml                   sqlc configuration
Makefile                    build, run, test, sqlc, migrate targets
```

---

## Testing

```bash
make test              # Run all 22 unit/integration tests
make test-race         # With race detector

# E2E test (requires real LLM credentials)
TEST_LLM_API_KEY=sk-... TEST_LLM_BASE_URL=https://api.openai.com/v1 make test
```

### Test Coverage

| Area | Tests | What's tested |
|---|---|---|
| Auth | 4 | JWT generate/validate, password hash, garbage tokens |
| Encryption | 4 | AES encrypt/decrypt, dev mode plaintext, bad keys, key hints |
| Router | 13 | Health, register/login, refresh tokens, agent CRUD, isolation, runs, API keys, rate limiting, dashboard, models, /me, request IDs |
| E2E | 1 | Full flow: register → store key → create agent → run → verify LLM output + events (skips without env vars) |

---

## API Examples

### Register + Login

```bash
# Register
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","name":"User","password":"pass123"}'
# Returns: { "token": "...", "refresh_token": "...", "user": {...} }

# Login
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"pass123"}'
```

### Store API Key

```bash
curl -X POST http://localhost:8080/api/v1/api-keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"provider":"openai","label":"My Key","key":"sk-...","is_default":true,"base_url":"https://api.openai.com/v1"}'
```

### Create Agent + Run

```bash
# Create agent
curl -X POST http://localhost:8080/api/v1/agents \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Research Bot","system_prompt":"You are a research assistant.","model_name":"gpt-4o","model_provider":"openai"}'

# Trigger run
curl -X POST http://localhost:8080/api/v1/runs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"agent_id":"<agent-id>","mission":"What are the latest AI trends?"}'
```

---

## Part of the Agent Platform

| Repo | Purpose | Status |
|---|---|---|
| [agent-core](https://github.com/bitop-dev/agent-core) | Standalone CLI + Go library | ✅ 111 tests |
| **agent-platform-api** (this repo) | REST API server | ✅ 22 tests |
| [agent-platform-web](https://github.com/bitop-dev/agent-platform-web) | Next.js web portal | ✅ 12 pages |
| [agent-platform-docs](https://github.com/bitop-dev/agent-platform-docs) | Architecture & planning | ✅ Comprehensive |

---

## License

TBD
