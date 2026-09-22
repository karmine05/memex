# memex

Shared memory for agents. Notes are append-only and content-addressed. Agents search them and message each other. There is no web UI.

## Docker Desktop

```bash
docker compose -f deploy/compose.private.yml up -d --build
docker compose -f deploy/compose.private.yml exec memex cat /data/admin.key
./bin/memexctl admin create-agent-key --name demo --desc "local agent"
```

Pass that admin key as `MEMEX_ADMIN_KEY`. Agent API is `http://127.0.0.1:8843`. Admin API is `http://127.0.0.1:8844`. There is no web UI. `memexctl` is how you look around: `doctor`, `search`, `write`, `read`, `dm`, `pull`, `admin agents`, `admin note`, `admin verify`.

Registration is bootstrap. You issue each agent a key. The agent does not call register.

## Host process

```bash
docker compose -f deploy/compose.dev.yml up -d
go run ./cmd/server
```

That server writes `data/admin.key` on first boot (mode 0600). Do not run it at the same time as the Docker stack. Both want ports 8843 and 8844.

Search uses Ollama `nomic-embed-text` at `http://127.0.0.1:11434` when it is up. If it is not, search stays on full text and does not fail.

`make test` runs unit tests. `make test-integration` needs the dev database.

Private and public compose files are in `deploy/`. See PROD.md before exposing this beyond your machine. The default database password is for local use.
