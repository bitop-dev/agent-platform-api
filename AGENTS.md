# AGENTS.md

Instructions for AI coding agents working with this codebase.

## Project Overview

`agent-platform-api` is the REST API server for the Agent Platform. It wraps `agent-core/pkg/agent` with user auth, persistent storage, and WebSocket streaming.

## Module Structure

```
github.com/bitop-dev/agent-platform-api

cmd/api/                  Server entrypoint (main.go)
internal/
  api/                    Fiber router setup + integration tests
  api/handlers/           HTTP handlers: auth.go, agents.go, runs.go
  api/middleware/         JWT auth middleware
  auth/                   JWT token generation/validation + bcrypt
  config/                 Config from env vars (PORT, JWT_SECRET, DATABASE_URL, etc.)
  db/                     Store wrapper (SQLite/Postgres, goose migrations)
  db/migrations/          Goose SQL migration files
  db/query/               sqlc query definitions (.sql files)
  db/sqlc/                Generated code — DO NOT EDIT (run `sqlc generate`)
  runner/                 Async agent execution via agent-core/pkg/agent
  ws/                     WebSocket hub — room-based pub/sub by run ID
```

## Key Patterns

- **Fiber v2** for HTTP — use `c.BodyParser()`, `c.JSON()`, `c.Locals()`
- **sqlc** for database — write SQL in `internal/db/query/*.sql`, run `sqlc generate`
- **goose** for migrations — add `internal/db/migrations/NNN_name.sql` with `-- +goose Up/Down`
- **JWT auth** — all `/api/v1/*` routes require `Authorization: Bearer <token>`
- **User isolation** — agents/runs belong to users; handlers check ownership before returning
- **Runner** — goroutine pool executes runs async; results persisted + broadcast via WebSocket

## Conventions

- No global state — dependencies injected through constructors
- Handlers receive `*db.Store` (which embeds `*sqlc.Queries`)
- Tests use Fiber's `app.Test()` with httptest — no real server needed
- Config from env vars only (no config files)

## Build & Test

```bash
make build     # → bin/api
make test      # go test ./... -race
make sqlc      # regenerate sqlc code
make run       # build + run with dev JWT secret
```

## Related Repos

- **agent-core**: github.com/bitop-dev/agent-core (imported as Go module)
- **Planning docs**: github.com/bitop-dev/agent-platform-docs
