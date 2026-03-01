# agent-platform-api

Go REST API server for the Agent Platform. Wraps [agent-core](https://github.com/bitop-dev/agent-core) with persistence, authentication, real-time WebSocket streaming, and a multi-source skill registry.

> **Status**: Phase 2–9 complete — WASM sandbox runner, OAuth, audit logging, team-scoped resources, Prometheus metrics.

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
| GET | `/api/v1/auth/github` | GitHub OAuth login redirect |
| GET | `/api/v1/auth/github/callback` | GitHub OAuth callback |
| GET | `/api/v1/auth/google` | Google OAuth login redirect |
| GET | `/api/v1/auth/google/callback` | Google OAuth callback |

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
| POST | `/api/v1/agents` | Create agent |
| GET | `/api/v1/agents/:id` | Get agent details |
| PUT | `/api/v1/agents/:id` | Update agent |
| DELETE | `/api/v1/agents/:id` | Delete agent |
| PUT | `/api/v1/agents/:id/team` | Assign agent to team |

#### Runs
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/runs` | Start a run (async, returns immediately) |
| GET | `/api/v1/runs` | List all user's runs |
| GET | `/api/v1/runs/:id` | Get run status, output, metrics |
| GET | `/api/v1/agents/:agent_id/runs` | List runs for a specific agent |
| GET | `/api/v1/runs/:id/events` | Get full event log |
| POST | `/api/v1/runs/:id/cancel` | Cancel an in-flight run |
| GET | `/api/v1/runs/:id/children` | List sub-agent (child) runs |

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
| PUT | `/api/v1/skills/:id` | Update skill |
| DELETE | `/api/v1/skills/:id` | Delete skill |
| POST | `/api/v1/agents/:id/skills` | Attach skill to agent |
| DELETE | `/api/v1/agents/:id/skills/:skill_id` | Detach skill from agent |
| GET | `/api/v1/agents/:id/skills` | List agent's skills |

#### Skill Sources
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/skill-sources` | List all skill sources (user's + system defaults) |
| POST | `/api/v1/skill-sources` | Add custom GitHub skill repo |
| DELETE | `/api/v1/skill-sources/:id` | Remove source (can't delete default) |
| POST | `/api/v1/skill-sources/:id/sync` | Re-sync one source |
| POST | `/api/v1/skill-sources/sync` | Re-sync all sources |

#### Schedules
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/schedules` | Create schedule (cron/interval/one-shot) |
| GET | `/api/v1/schedules` | List schedules |
| GET | `/api/v1/schedules/:id` | Get schedule |
| PUT | `/api/v1/schedules/:id` | Update schedule |
| DELETE | `/api/v1/schedules/:id` | Delete schedule |
| POST | `/api/v1/schedules/:id/enable` | Enable schedule |
| POST | `/api/v1/schedules/:id/disable` | Disable schedule |
| POST | `/api/v1/schedules/:id/trigger` | Manual trigger |

#### Teams
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/teams` | Create team |
| GET | `/api/v1/teams` | List teams |
| GET | `/api/v1/teams/:id` | Get team |
| DELETE | `/api/v1/teams/:id` | Delete team |
| GET | `/api/v1/teams/:id/members` | List members |
| POST | `/api/v1/teams/:id/invitations` | Invite user by email |
| POST | `/api/v1/invitations/:id/accept` | Accept invitation |
| DELETE | `/api/v1/teams/:id/members/:user_id` | Remove member |

#### Audit Log
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/audit-log` | Paginated audit trail (`?page=&per_page=`) |

#### Observability
| Method | Path | Description |
|---|---|---|
| GET | `/health` | Liveness probe |
| GET | `/readyz` | Readiness probe (DB check) |
| GET | `/metrics` | Prometheus text exposition format |

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
                    │ Handlers│
                    └────┬────┘
                         │
           ┌─────────┬───┼────┬──────────┐
           │         │   │    │          │
        Store    Runner  Hub  Syncer   Encryptor
       (sqlc)  (goroutine (WS (registry (AES-256)
               pool×4)  pub/sub) sync)
           │         │   │    │          │
           ▼         ▼   ▼    ▼          ▼
        SQLite   agent-core  GitHub    API keys
        /Postgres pkg/agent  repos     encrypted
```

### Skill Registry Sync

On startup, the API:
1. Creates a default skill source pointing to `github.com/bitop-dev/agent-platform-skills`
2. Fetches `registry.json` from each source (GitHub raw content)
3. Downloads `SKILL.md` for each skill
4. Upserts skills into the database with source tracking

Users can add custom sources (any GitHub repo with `registry.json`):
```bash
curl -X POST http://localhost:8080/api/v1/skill-sources \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"url":"github.com/mycorp/internal-skills","label":"Internal"}'
```

### Run Execution Flow

1. `POST /api/v1/runs` → creates DB record (status: queued) → enqueues `RunRequest`
2. Worker goroutine picks up request → resolves API key from DB → calls `agent.QuickRun()`
3. Agent-core executes: LLM calls, tool execution, streaming events
4. Events written to DB (`run_events`) AND broadcast to WebSocket hub
5. On completion: updates run record with output, metrics, status

---

## Database

### Schema (4 migrations)

| Table | Description |
|---|---|
| `users` | User accounts (email, name, password hash) |
| `api_keys` | LLM provider keys (encrypted, with base_url) |
| `agents` | Agent configs (name, prompt, model, YAML) |
| `runs` | Run records (status, output, metrics, timestamps) |
| `run_events` | Event log per run (seq, type, JSON data) |
| `skills` | Skill registry (name, tier, SKILL.md, source tracking) |
| `agent_skills` | Agent ↔ skill linking (ordered, with config) |
| `skill_sources` | Registered skill repos (url, status, last synced) |

### Tooling

- **goose**: embedded SQL migrations, auto-run on startup
- **sqlc**: type-safe Go code generated from SQL queries
- **SQLite** for dev (zero config), **PostgreSQL** for production
- Driver auto-detected from `DATABASE_URL` prefix

---

## Configuration

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
| Database | SQLite / PostgreSQL |
| SQL | [sqlc](https://sqlc.dev/) — type-safe generated Go |
| Migrations | [goose](https://github.com/pressly/goose) — embedded SQL |
| Auth | [golang-jwt/jwt](https://github.com/golang-jwt/jwt) v5 + bcrypt |
| WebSocket | gofiber/contrib/websocket |
| Agent Runtime | [agent-core/pkg/agent](https://github.com/bitop-dev/agent-core) |
| Logging | `log/slog` (JSON to stdout) |

---

## Testing

```bash
make test              # Run all 22 unit/integration tests
make test-race         # With race detector

# E2E test (requires real LLM credentials)
TEST_LLM_API_KEY=sk-... TEST_LLM_BASE_URL=https://api.openai.com/v1 make test
```

---

## Part of the Agent Platform

| Repo | Purpose | Status |
|---|---|---|
| [agent-core](https://github.com/bitop-dev/agent-core) | Standalone CLI + Go library | ✅ 111 tests, 26 commits |
| **agent-platform-api** (this repo) | Go Fiber REST API | ✅ 22 tests, 11 commits |
| [agent-platform-web](https://github.com/bitop-dev/agent-platform-web) | Bun + Vite + React web portal | ✅ 11 pages, 6 commits |
| [agent-platform-skills](https://github.com/bitop-dev/agent-platform-skills) | Community skill registry | ✅ 5 skills, 2 commits |
| [agent-platform-docs](https://github.com/bitop-dev/agent-platform-docs) | Architecture & planning | ✅ Comprehensive |

---

## License

MIT
