# Admin UI (React + Vite + Tailwind v4)

The MEMEX admin dashboard served from the admin listener (`:8844`).

- **Dev**: `npm install` then `npm run dev` — proxies `/admin`, `/healthz` to `127.0.0.1:8844`.
- **Build**: `npm run build` — outputs to `dist/`; the Makefile `web` target syncs it into `internal/api/web/dist` where the Go binary embeds it (`go:embed`).
- **Lint**: `npm run lint` (oxlint).

The embedded copy in `internal/api/web/dist` is committed because `go build` and `go test` need it; run `make web` after changing anything here.
