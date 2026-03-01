# agent-platform-api

Go REST API server for the Agent Platform. Wraps [agent-core](https://github.com/bitop-dev/agent-core) with persistence, authentication, and real-time WebSocket streaming.

> **Status**: Phase 2 — API foundation complete. Auth, agent CRUD, runs, API keys, rate limiting. 18 tests passing.

## Quick Start

```bash
# Build
make build

# Run (SQLite, dev mode)
JWT_SECRET=your-secret-here make run

# Or with environment:
PORT=8090 DATABASE_URL=sqlite://data/platform.db JWT_SECRET=mysecret ./bin/api
```

## API Endpoints

### Public
| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/auth/register` | Register new user |
| POST | `/api/v1/auth/login` | Login, returns JWT |

### Protected (Bearer token required)
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/me` | Current user info |
| GET | `/api/v1/agents` | List user's agents |
| POST | `/api/v1/agents` | Create agent |
| GET | `/api/v1/agents/:id` | Get agent details |
| PUT | `/api/v1/agents/:id` | Update agent |
| DELETE | `/api/v1/agents/:id` | Delete agent |
| POST | `/api/v1/runs` | Start a run |
| GET | `/api/v1/runs/:id` | Get run status/result |
| GET | `/api/v1/agents/:agent_id/runs` | List runs for agent |
| GET | `/api/v1/runs/:id/events` | Get run event log |
| POST | `/api/v1/runs/:id/cancel` | Cancel an in-flight run |
| POST | `/api/v1/api-keys` | Store encrypted LLM API key |
| GET | `/api/v1/api-keys` | List API keys (hints only) |
| DELETE | `/api/v1/api-keys/:id` | Delete API key |

### WebSocket
| Path | Description |
|------|-------------|
| `ws://host/ws/runs/:id` | Stream run events in real time |

## Tech Stack

- **Framework**: [Fiber](https://gofiber.io/) v2
- **Database**: SQLite (dev) / PostgreSQL (prod)
- **SQL**: [sqlc](https://sqlc.dev/) — type-safe generated Go from SQL
- **Migrations**: [goose](https://github.com/pressly/goose) — embedded SQL migrations
- **Auth**: JWT (HS256) + bcrypt password hashing
- **Agent Runtime**: [agent-core/pkg/agent](https://github.com/bitop-dev/agent-core)
- **WebSocket**: Fiber WebSocket for real-time run streaming

## Project Structure

```
cmd/api/                  Server entrypoint
internal/
  api/                    Fiber router + integration tests
  api/handlers/           HTTP handlers (auth, agents, runs)
  api/middleware/         JWT auth middleware
  auth/                   JWT tokens + password hashing
  config/                 Server configuration (env vars)
  db/                     Database wrapper (SQLite/Postgres + goose)
  db/migrations/          SQL migration files
  db/query/               sqlc query definitions
  db/sqlc/                Generated Go code (do not edit)
  runner/                 Agent execution engine (bridges API → agent-core)
  ws/                     WebSocket hub for run streaming
```

## Part of the Agent Platform

| Repo | Purpose |
|---|---|
| [agent-core](https://github.com/bitop-dev/agent-core) | Standalone CLI + Go library |
| **agent-platform-api** (this repo) | REST API server |
| **platform-web** | Next.js web portal (coming soon) |
| **skills** | Community skill registry (coming soon) |
