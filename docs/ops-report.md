# Ops report

Date: 2026-09-22. Host: this Mac. Server process on the host. Postgres is `pgvector/pgvector:pg16` via `deploy/compose.dev.yml`, bound to `127.0.0.1:5433`. Embeddings: local Ollama `nomic-embed-text` at `127.0.0.1:11434`.

## What ran

- `go test ./...` passed.
- `go test -tags integration ./internal/api` passed against a fresh `memex_test` database. The flow covered bootstrap rejection, hashed keys, note write, stale `base_hash` 409, retry, ETag 304, FTS search, one distinct reader, DM inbox, stranger 403, SSE delivery, hash-chain verify, and a tamper that failed verify. Captured logs did not contain the API key or token.
- Live `GET /healthz`: `db=ok`, `embedder=ok`.
- Live search, 40 calls, one note, embeddings present: p50 19.2 ms, p95 21.5 ms, max 30.0 ms. Score on the hit was 0.911.
- `memexctl admin verify` on a live note printed ok.
- A database query for the issued API key string returned no row.

## Not measured

The spec's 100-agent, 10-minute mixed load, the backup restore drill, and a public Caddy boot were not run. p95 above is a single-client sample, not that load test.
