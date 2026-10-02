# memex — Agent Instructions

> Compact guidance for OpenCode agents working in this repo. Only facts that took reading multiple files to infer.

---

## Quick Reference

| Need | Command |
|------|---------|
| **Build + test** | `make build && make test` |
| **Run stack (private)** | `docker compose -f deploy/compose.private.yml up -d --build` |
| **Health check** | `curl -s http://127.0.0.1:8843/healthz` |
| **Admin UI** | http://127.0.0.1:8844/ (needs `data/admin.key`) |
| **Regenerate API docs** | `make api` |
| **Integration tests** | `docker compose -f deploy/compose.dev.yml up -d && make test-integration` |

---

## Architecture (what you must internalize)

- **Single Go binary** (`memex-server`) with two HTTP listeners:
  - `:8843` — agent API (`/v1/*`, SSE streams)
  - `:8844` — admin API (`/admin/*`, graph dashboard, `/skill.md`) — **bound to 127.0.0.1 only**
- **In-process modules** — no IPC: `note`, `search`, `feed`, `auth`, `store`
- **Postgres 16 + pgvector** — one DB does relational, FTS (tsvector GIN), and vector (HNSW 768-d)
- **Ollama embedder** — `nomic-embed-text` on host port 11434; optional (writes succeed with `NULL` embedding, backfilled later)
- **Embedding pool** — 4 goroutines drain `note_versions WHERE embedding IS NULL`; idempotent, never on critical path

---

## Non-Obvious Conventions

### Config Loading (cmd/server/main.go → internal/config)
```
Defaults → config file (YAML/JSON) → env vars (MEMEX_*) → validation
```
Env vars win. See `internal/config/config.go:156-172` for exact mapping (`MEMEX_DB`, `MEMEX_EMBED_PROVIDER`, etc.).

### Two Modes (config.Config.Mode)
| Mode | Registration | Admin port | Agent port |
|------|--------------|------------|------------|
| `private` (default) | `bootstrap` (first agent self-registers) | 127.0.0.1:8844 | 0.0.0.0:8843 |
| `public` | `invite` or `open` | 127.0.0.1:8844 (never public) | docker network only |

Private compose publishes 8843 on all interfaces; public compose puts Caddy in front.

### Key Handling (RULES.md §2)
- Keys are **SHA-256 at rest**, shown **once** at creation (`mxk_` for agents, `mxa_` for admin)
- Tokens are 1-hour JWT-like (also SHA-256 at rest); revocation kills *new* issuance instantly; live tokens die ≤1h

### Append-Only Notes
- `PUT /v1/notes/{id}` requires `If-Match: <base_hash>` — fails with 409 if stale
- Every version is a row in `note_versions` with `body_hash = sha256(JCS(body))`
- Trigger on insert updates `note_current`, writes `stream_events`, fires `pg_notify('memex_stream')`

### Search = One SQL Query
Hybrid score = `0.5*vector + 0.3*lexical + 0.2*recency` (configurable in `config.Search`)

### SSE Streams
- Per-subscriber buffered channel (256 events, drop-oldest + hint)
- Events carry metadata only (id, version, hash, agent, time)
- Body fetched via conditional GET (304 if reader has the hash)
- Resume via `Last-Event-ID` → replay from `stream_events` (24h retention)

---

## Testing

| Command | What it runs |
|---------|--------------|
| `make test` | Unit tests only (`go test ./...` + import check) — **no Docker** |
| `make test-integration` | Needs Postgres on `127.0.0.1:5433` (`deploy/compose.dev.yml`) |

### Integration Test DSN
```bash
MEMEX_TEST_DSN='postgres://memex:memex@127.0.0.1:5433/memex_test?sslmode=disable'
```
Set by Makefile. Only runs `./internal/api` with `-tags integration`.

### Test Patterns
- Tests live next to code: `internal/note/note_test.go`
- Table-driven for >2 cases
- Names describe behavior: `TestUpdateConflictsWhenBaseHashStale`
- Routes test ensures every `Route` has a handler (`TestEveryRouteHasAHandler`)
- Admin routes return 404 on agent listener (`TestAdminRoutesAreAbsentFromAgentListener`)

---

## Migrations

- **Forward-only**, numbered: `migrations/0001_init.sql`, `0002_...`
- Embedded in binary via `migrations/embed.go` (`go:embed`)
- `store.Migrate(ctx)` runs on startup
- **Never edit an applied migration** — add a new one
- Commit message must note schema changes

---

## Common Tasks

### Add an Endpoint
1. Handler in `internal/api/<feature>.go`
2. Add `Route` to `Routes()` in `internal/api/server.go`
3. Integration test in `internal/api/<feature>_test.go` (if integration)
4. `make api` → regenerates `docs/api.md` and `docs/openapi.json`

### Change Search Weights
Config file or env:
```yaml
search:
  weights: { vector: 0.5, lexical: 0.3, recency: 0.2 }
  recency_halflife: "72h"
```

### Key Revocation
```bash
memexctl admin revoke <agent_id>   # in container
```

### Backup / Restore
```bash
# backup
docker compose exec postgres pg_dump -Fc memex > backup-$(date +%F).dump

# restore
docker compose up -d postgres
docker compose exec -T postgres pg_restore -d memex --clean backup-<date>.dump
```

---

## Files to Read When Stuck

| Topic | File |
|-------|------|
| Agent protocol (what agents paste) | `docs/SKILL.md` |
| Full spec | `TECH.md` |
| Protocol decisions (why X not Y) | `DESIGN.md` |
| Governance / security rules | `RULES.md` |
| Operations / incidents | `PROD.md` |
| Memory subsystem deep dive | `MEMORY.md` |
| Build phases / scope | `PLAN.md` |

---

## Gotchas

1. **Agent key is shown once** — copy it from the admin UI or `memexctl admin create-agent-key` output immediately
2. **Admin port is 127.0.0.1 only** — SSH tunnel required for remote access
3. **Ollama must be reachable** at `MEMEX_EMBED_URL` (default `http://host.docker.internal:11434` in Docker). If down, `/healthz` reports `embedder: degraded`; search falls back to FTS
4. **`make test` passes without DB** — integration tests are separate
4. **No generated code except OpenAPI** — `make api` runs `cmd/openapi`
5. **`memexctl` lives in the image** — prefix with `docker compose ... exec -T -e MEMEX_API_KEY="$AK" memex`
6. **`data/admin.key` created on first run** — never commit it
7. **SSE stream = change feed, not content feed** — bodies fetched separately via conditional GET
8. **Public mode needs Caddy** — `deploy/compose.public.yml`, auto-TLS, rate limits, invite gate

---

## Dependency Ratchet (INSTRUCTIONS.md §1)

Only these third-party deps allowed (in `go.mod`):
- `github.com/jackc/pgx/v5` — Postgres driver
- `github.com/spf13/cobra` — CLI
- One JCS package (canonicalization)
- `testcontainers` (test-only)

Adding a dep = written justification in PR description.