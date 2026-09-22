# memex — Rules

Standing rules for this codebase and platform. These are enforceable: code
review rejects violations, and the CI checks named in each rule are the
mechanism. If a rule needs an exception, the exception goes in this file
with a date and a reason — never silently.

## 1. Codebase rules

1. **Dependency ratchet.** Allowed third-party deps: `pgx`, `cobra`, one JCS
   package, `testcontainers` (test-only). Adding a dep requires a written
   justification in the PR and an update to INSTRUCTIONS.md §1. Removing is
   always fine. CI check: `go list -m all` diffed against `deps.lock` (a
   checked-in list of allowed modules).
2. **No framework growth.** No ORM, no DI container, no router library, no
   middleware framework, no validation framework. stdlib `net/http` mux +
   explicit functions. CI check: same dep list; review for new indirection
   layers.
3. **One package, one job.** The import graph is `cmd/*` → `internal/api` →
   `internal/{note,store,search,feed,auth}` → stdlib/`pgx`. No other edges,
   no cycles, no package importing a package at the same level except via
   `api`. CI check: a 30-line go/analysis script in `scripts/checkimports.go`
   run on every PR.
4. **Migrations are forward-only and numbered.** New file = new migration.
   Editing an applied migration is forbidden; it is re-cut as the next
   number. CI check: `git diff` against `migrations/` rejects edits to
   existing files.
5. **Every endpoint ships four things in one PR:** handler, integration
   test, `docs/api.md` line, regenerated `openapi.json`. CI check:
   `make api` output must be unchanged (a stale OpenAPI file fails the build).
6. **Wire contract stability.** Anything an agent observes (paths, field
   names, event names, status semantics) changes only via a major version
   (`/v1` → `/v2`, both live during a migration window). CI check: review
   gate — any change under `/v1` response shapes requires a signed-off
   "wire-contract" label.
7. **Timestamps are server-side.** No client-supplied time is ever stored
   as truth. CI check: grep for `time.Now()` outside `internal/api`
   middleware (the one place auth touches `last_active`) fails review.
8. **Errors are flat.** `fmt.Errorf("context: %w", err)`. No custom error
   types, no error-code enums, no panic across package boundaries.
9. **Secrets never in code, logs, or responses.** Keys/tokens exist only as
   SHA-256 hex at rest and in the single registration response. CI check:
   a log-scrub test — hit every endpoint with a valid key, assert the key
   and token strings appear nowhere in captured logs or responses.
10. **No TODOs without an owner and a date.** `// TODO(2026-11): add X —
    tracked in docs/open-questions.md`. CI check: grep for `TODO` without
    a date pattern fails.

## 2. Data rules

1. **Append-only versions.** `note_versions` rows are never UPDATEd or
   DELETEd by application code. The only DELETE is admin purge, which logs
   the full hash chain to the audit log first.
2. **Hashes are computed, never trusted.** `body_hash` is always recomputed
   from the body on write; `prev_hash` is read from the current version
   row. No client-supplied hashes are accepted except `base_hash`, which is
   compared, not stored.
3. **IDs are UUIDv7, server-generated.** Clients never choose note or agent
   IDs. (Predictable IDs + default-open ACLs = enumeration; UUIDv7 keeps
   IDs unguessable while staying sortable.)
4. **Embeddings are derived data.** Any row's `embedding` can be NULL at
   any time and rebuilt by backfill. Nothing in a correctness path may
   assume it is set.
5. **Backups are hash-verifiable.** A restore is not "done" until
   `memexctl admin verify` passes on a 20-note sample. The rule exists
   because the dump can be truncated and still restore cleanly.

## 3. Agent-facing contract rules

1. **Search returns bodies by default, always with provenance**
   (`agent_id`, `body_hash`, `created_at`, `prev_hash`). Never return a
   result without its provenance envelope — that is what lets agents reason
   about trust.
2. **Every error response is machine-actionable:** a status code, a one-line
   `error` field, and for 409/429 the data needed to retry (current hash /
   `Retry-After`). No bare 500s for expected conditions.
3. **Streams carry metadata only.** Bodies never ride the SSE stream
   (see DESIGN.md D4). The resume contract (`Last-Event-ID` → replay, 24 h
   retention) is a wire commitment, not an implementation detail.
4. **The agent protocol manual (`docs/SKILL.md`) is part of the API.**
   Endpoint changes that change agent behavior require a SKILL.md update in
   the same PR. It is tested: the Phase 4 definition of done includes a
   blind agent following it end-to-end.

## 4. Security rules

1. **Keys hashed, tokens short.** No plaintext key material in the database,
   ever. Tokens ≤1 h. This is not tunable per deployment — it is the
   platform's posture.
2. **The admin listener is never on the public edge.** In public mode,
   `/admin` is bound to 127.0.0.1 and reachable only via tunnel. CI check:
   the compose file for public mode is reviewed against a port-exposure
   checklist; a test asserts no admin route is reachable on the agent
   listener.
3. **Rate limits are per-agent, enforced in-process, and tested under
   load.** A single agent at 10× its limit must not degrade another
   agent's p95 by more than 10% (load-test assertion).
4. **Registration is gated in public mode by default.** `open` registration
   is a config choice that logs a loud warning at boot.
5. **Prompt-injection posture is structural, not aspirational:** provenance
   envelopes on all reads (rule 3.1), no endpoints that return executable
   instructions to agents (no heartbeat-style docs-by-URL), and SKILL.md §8
   written as if the agents reading it will be injected (they will be).
6. **Secret-scanning on the repo** (gitleaks in CI) — including test
   fixtures. Fake keys in tests must use the `mxk_test_` prefix and be
   flagged `#nosec` with a reason.

## 5. Process rules

1. **No phase starts until the previous phase's definition of done passes**
   (INSTRUCTIONS.md §3). "Done" means the named tests pass with output, not
   "the code is written."
2. **Wire-contract and schema changes require a DESIGN.md record.** A
   decision record is written (or a supersession note added) in the same
   PR as the change.
3. **Load numbers come from runs, not estimates.** Any performance claim in
   a doc or PR must cite a run (date, config, tool, p50/p95). The
   ops report (PROD.md §7) is the standing record.
4. **The cut list is pre-agreed** (INSTRUCTIONS.md §3, final paragraph).
   Scope cuts during a build may only take items from that list, in that
   order. New "nice to have" items do not enter v1 by default.
5. **Security incidents follow PROD.md §6.** The response is mechanical
   (revoke, isolate, audit, report) so it can be executed under pressure
   without re-deriving.

## 6. What this file is for

When a reviewer and an author disagree, the chain is: TECH.md (spec) →
DESIGN.md (why) → this file (what's enforceable) → judgment. If the
disagreement is about judgment, write the outcome as a new decision record
or rule. The point of the file is that the next person (human or agent)
building on this codebase does not have to interview the previous one.
