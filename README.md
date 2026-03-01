# agent-platform-api

Go REST API server for the Agent Platform. Wraps [agent-core](https://github.com/bitop-dev/agent-core) with persistence, authentication, real-time WebSocket streaming, and a multi-source skill registry.

> **Status**: Feature-complete. 46 Go files, ~8.6K lines, 22 tests. OAuth, audit logging, teams, schedules, Prometheus metrics.

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
# From the agent-platform-docs (root) repo:
docker compose up --build

# Or standalone:
docker compose up --build
```

See [agent-platform-docs](https://github.com/bitop-dev/agent-platform-docs) for the full-stack `docker-compose.yml` that orchestrates API + Web + volumes.

---

## API Endpoints (62 routes)

### Public (no auth required)

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/readyz` | Readiness probe (DB connectivity) |
| GET | `/metrics` | Prometheus text exposition format |
| GET | `/api/v1/models` | List supported LLM models (filter by `?provider=`) |

### Auth (rate limited: 10/min per IP)

| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/auth/register` | Register new user → returns access + refresh tokens |
| POST | `/api/v1/auth/login` | Login → returns access + refresh tokens |
| POST | `/api/v1/auth/refresh` | Exchange refresh token for new access token |

### OAuth

| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/auth/github` | GitHub OAuth login redirect |
| GET | `/api/v1/auth/github/callback` | GitHub OAuth callback → redirect with JWT |
| GET | `/api/v1/auth/google` | Google OAuth login redirect |
| GET | `/api/v1/auth/google/callback` | Google OAuth callback → redirect with JWT |

OAuth flow: server redirects to provider → callback creates/links user → redirects to `/login?token=...&refresh_token=...` → frontend stores tokens.

### Protected (Bearer token required, 120/min per IP)

#### User & Dashboard
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/me` | Current user (includes `avatar_url`, `oauth_provider`) |
| GET | `/api/v1/dashboard` | Aggregate stats (agent count, run breakdown, recent runs) |

#### Agents
| Method | Path | Description |
|---|---|---|
| GET | `/api/v1/agents` | List user's agents (includes team-shared) |
| POST | `/api/v1/agents` | Create agent |
| GET | `/api/v1/agents/:id` | Get agent details |
| PUT | `/api/v1/agents/:id` | Update agent |
| DELETE | `/api/v1/agents/:id` | Delete agent |
| PUT | `/api/v1/agents/:id/team` | Assign agent to team |

#### Runs
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/runs` | Start a run (async, returns immediately) |
| GET | `/api/v1/runs` | List all user's runs (includes team-shared) |
| GET | `/api/v1/runs/:id` | Get run status, output, metrics |
| GET | `/api/v1/agents/:agent_id/runs` | List runs for a specific agent |
| GET | `/api/v1/runs/:id/events` | Get full event log |
| POST | `/api/v1/runs/:id/cancel` | Cancel an in-flight run |
| GET | `/api/v1/runs/:id/children` | List sub-agent (child) runs |

#### API Keys
| Method | Path | Description |
|---|---|---|
| POST | `/api/v1/api-keys` | Store LLM API key (AES-256-GCM encrypted at rest) |
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
                    │ Handlers│  ← Audit Logger (18 action types)
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

### Run Execution (WASM Sandboxed)

1. `POST /api/v1/runs` → creates DB record (status: queued) → enqueues `RunRequest`
2. Worker goroutine picks up request → resolves API key from DB
3. Initializes WASM sandbox registry → loads skill `.wasm` modules
4. Calls `agent.QuickRun()` which runs the full agent loop
5. Events written to DB (`run_events`) AND broadcast to WebSocket hub
6. On completion: updates run record with output, metrics, status

The runner uses `RegisterSkillToolsSandboxed()` from `pkg/agent` — all skill tools execute inside Wazero's WASM sandbox with capability-based security.

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

### Audit Logging

All state-changing operations are audit-logged with 18 action types:

| Category | Actions |
|---|---|
| **Auth** | `auth.login`, `auth.register`, `auth.oauth_login` |
| **Agents** | `agent.create`, `agent.update`, `agent.delete` |
| **Runs** | `run.create`, `run.cancel` |
| **API Keys** | `api_key.create`, `api_key.delete` |
| **Skills** | `skill.attach`, `skill.detach` |
| **Schedules** | `schedule.create`, `schedule.delete` |
| **Teams** | `team.create`, `team.invite`, `team.remove_member` |

### Observability

| Endpoint | Format | Metrics |
|---|---|---|
| `/health` | JSON | Liveness probe |
| `/readyz` | JSON | Readiness (DB check) |
| `/metrics` | Prometheus text | `agentops_runs_started_total`, `agentops_runs_finished_total`, `agentops_runs_failed_total`, `agentops_uptime_seconds`, `agentops_goroutines`, `agentops_alloc_bytes`, `agentops_gc_cycles_total` |

### Graceful Shutdown

On SIGINT/SIGTERM:
1. Stop scheduler (drain pending ticks)
2. Stop runner (drain in-flight runs with 30s timeout)
3. Close database
4. Shutdown Fiber server

---

## Database

### Schema (8 migrations, 13 tables)

| Table | Description |
|---|---|
| `users` | User accounts (email, name, password hash, avatar_url, oauth_provider) |
| `teams` | Team definitions (name, owner) |
| `team_members` | Team membership (user, role: owner/admin/member/viewer) |
| `team_invitations` | Pending team invites (email, role, status) |
| `api_keys` | LLM provider keys (AES-256-GCM encrypted, with base_url) |
| `agents` | Agent configs (name, prompt, model, YAML, optional team_id) |
| `runs` | Run records (status, output, metrics, parent_run_id for orchestration) |
| `run_events` | Event log per run (seq, type, JSON data) |
| `skills` | Skill registry (name, tier, SKILL.md, source tracking) |
| `agent_skills` | Agent ↔ skill linking (ordered, with config) |
| `skill_sources` | Registered skill repos (url, status, last synced) |
| `schedules` | Cron/interval/one-shot schedules with overlap policy |
| `audit_log` | Audit trail (user, action, resource, metadata, timestamp) |

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
| `BASE_URL` | `http://localhost:8080` | OAuth redirect base URL |
| `GITHUB_CLIENT_ID` | (optional) | GitHub OAuth app client ID |
| `GITHUB_CLIENT_SECRET` | (optional) | GitHub OAuth app client secret |
| `GOOGLE_CLIENT_ID` | (optional) | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | (optional) | Google OAuth client secret |

---

## Docker

### Dockerfile

Multi-stage build: Go 1.26 compile → scratch-like runner with non-root user and healthcheck.

```dockerfile
# Build
FROM golang:1.26-alpine AS builder
# ...
# Run
FROM alpine:3.21
COPY --from=builder /app/api /usr/local/bin/api
USER appuser
HEALTHCHECK CMD wget -qO- http://localhost:8080/health || exit 1
ENTRYPOINT ["api"]
```

### docker-compose.yml (root repo)

```yaml
services:
  api:
    build: ../agent-platform-api
    ports: ["8090:8080"]
    environment:
      - JWT_SECRET=${JWT_SECRET}
      - GITHUB_CLIENT_ID=${GITHUB_CLIENT_ID}
      # ... see .env.example
    volumes:
      - api-data:/data

  web:
    build: ../agent-platform-web
    ports: ["3002:80"]
    depends_on: [api]
```

---

## Tech Stack

| Layer | Choice |
|---|---|
| HTTP | [Fiber](https://gofiber.io/) v2 |
| Database | SQLite / PostgreSQL |
| SQL | [sqlc](https://sqlc.dev/) — type-safe generated Go |
| Migrations | [goose](https://github.com/pressly/goose) — embedded SQL |
| Auth | [golang-jwt/jwt](https://github.com/golang-jwt/jwt) v5 + bcrypt + OAuth |
| WebSocket | gofiber/contrib/websocket |
| Agent Runtime | [agent-core/pkg/agent](https://github.com/bitop-dev/agent-core) (WASM sandbox) |
| Metrics | Prometheus text exposition format |
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
| [agent-core](https://github.com/bitop-dev/agent-core) | Standalone CLI + Go library | ✅ 171 tests, 45 commits |
| **agent-platform-api** (this repo) | Go Fiber REST API | ✅ 22 tests, 24 commits |
| [agent-platform-web](https://github.com/bitop-dev/agent-platform-web) | React web portal | ✅ 14 pages, 18 commits |
| [agent-platform-skills](https://github.com/bitop-dev/agent-platform-skills) | Community skill registry | ✅ 10 skills (4 WASM + 6 instruction) |
| [agent-platform-docs](https://github.com/bitop-dev/agent-platform-docs) | Architecture & planning | ✅ Comprehensive |

---

## License

MIT
