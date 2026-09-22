# memex — Memory Subsystem

Deep design of the memory layer: how notes become retrievable memory, how
retrieval quality is engineered, and how the system behaves as a shared
memory across many agents. The rest of the platform is plumbing; this file
is the product.

## 1. Memory model

memex implements the two memory problems described in the 2026 agent-memory
literature, but deliberately at the *network* level rather than the
*agent* level:

- **Episodic / solution memory:** "I solved X in environment E, here is how."
  This is the note. Written by one agent, retrieved by any.
- **Institutional / shared-state memory:** "The node runs cgroup v2 with
  swap disabled; the deploy window is 03:00 UTC." Notes with
  `status: note` in a team space.
- **Procedural memory:** reusable workflows, checklists. Notes with
  `topic: playbook/*`.

What memex does NOT manage (and never should): an agent's working context,
its core identity, its conversation history. Those belong to the agent's own
runtime (Letta-style core/recall, MemGPT-style archival, or whatever the
agent uses). memex is the *external* disk that outlives any one agent and is
shared across them. The MemGPT framing maps cleanly:

```
agent-local                    memex (shared, external)
─────────────                  ─────────────────────────
core memory (in-context)       —  (none: memex is never in-context by default)
recall (conversation log)      —  (none)
archival (agent's own disk)    ◄─►  notes (the collective archive)
```

The key property: **retrieval is the interface, not storage.** An agent
treats memex like a very fast, very persistent colleague it can ask — it
searches with its current problem and gets an answer-shaped document back.
The storage model (append-only, content-addressed) is invisible to agents
except through two features: stable note IDs and version history.

## 2. The write path: what a good memory looks like

The quality of a shared memory is determined at write time. The protocol
(docs/SKILL.md §4) encodes this as rules; the server enforces only shape,
not quality:

| Rule | Why it matters for retrieval |
|---|---|
| One topic per note | Search returns whole notes; a note about 5 things retrieves 5× less cleanly than 5 notes |
| `topic` as a slug path | Gives FTS a stable, queryable identifier independent of phrasing |
| `answer` separated from `context` | The answer is what gets used; context (env, versions) is what gets matched |
| `refs` to related/superseded notes | Edges for traversal; also a quality signal — chained notes outrank orphans in the score |
| `status` transitions question→answer | The version chain turns a problem into a resolved document over time |

**Write quality cannot be enforced by the server** (it's semantic). Two
controls exist:

1. **Retrieval-side:** the score weights recency and provenance, so sloppy
   old notes sink naturally.
2. **Social:** `warning` versions + `refs` let the network correct itself.
   PROD.md §7 includes a periodic "orphan note" report (notes with zero
   reads and zero refs after 30 days) — the admin prunes or flags them.

## 3. Retrieval: the hybrid query

### 3.1 Why hybrid

The 2026 benchmark data (LongMemEval: Zep 63.8% temporal-graph vs Mem0 49.0%
vector-first) shows a flat vector store fails on exactly the queries agents
make: "pod OOMKilled with limit 512Mi on cgroupv2" — where the identifiers
(limit value, version, error string) are the signal and semantics are noise.
Embeddings smear exact tokens; FTS nails them. Neither alone is enough:

- Vector only: misses "the fix for CVE-2024-1234" when the note says "the
  CVE-2024-1234 patch" — actually catches it — but misses "the thing where
  the pod restarts because of memory accounting" when the note is
  cgroup-jargon-heavy.
- FTS only: misses paraphrase entirely. Agents don't copy-paste each other's
  phrasing.

Hybrid (vector ∪ FTS, score-fused) is what every serious system converges
on (Mem0's vector+graph+KV, Zep's graph+vector, RAG practice generally).
memex does the two-signal version; the graph layer is v2 (§6).

### 3.2 The query (one SQL statement)

```sql
WITH vec AS (
  SELECT note_id,
         1 - (embedding <=> $1::vector) AS v_score,   -- cosine similarity
         version, body, body_hash, agent_id, created_at, prev_hash, space_id
  FROM note_versions
  WHERE embedding IS NOT NULL
    AND ($2::text IS NULL OR space_id = $2)
  ORDER BY embedding <=> $1::vector
  LIMIT 100
),
lex AS (
  SELECT note_id,
         ts_rank_cd(to_tsvector('english', body), q) AS l_score,
         version, body, body_hash, agent_id, created_at, prev_hash, space_id
  FROM note_versions, plainto_tsquery('english', $3) q
  WHERE to_tsvector('english', body) @@ q
    AND ($2::text IS NULL OR space_id = $2)
  LIMIT 100
)
SELECT v.note_id, v.version, v.body, v.body_hash, v.agent_id,
       v.created_at, v.prev_hash, v.space_id,
       0.5 * COALESCE(v.v_score, 0)
     + 0.3 * COALESCE(l.l_score, 0) * 10      -- ts_rank ≈ 0.05–0.5; scale
     + 0.2 * exp(-0.693 *
           extract(epoch FROM now() - greatest(v.created_at,
                                               (SELECT max(created_at) FROM note_versions WHERE note_id = v.note_id)))
           / 259200)                           -- recency, 72 h half-life
     + 0.0 * 0                                -- v2: graph/trust term reserved
AS score
FROM vec v LEFT JOIN lex l USING (note_id, version)
ORDER BY score DESC
LIMIT $4;                                       -- default 5
```

Properties:

- **Single round-trip** to Postgres (plus one embedder call for `$1`).
- **Degrades to FTS+recency** when the embedder is down (`vec` CTE skipped).
- **Space-scoped** when the agent passes `space` (its team's memory) —
  default is global.
- **Dedup by latest version:** if a note has versions 1–7, only v7 (current)
  is a retrieval candidate — history is for `diff`, not search. (Index:
  `WHERE version = latest` via a generated "current" index maintained by the
  trigger; detail in §3.4.)
- The `0.0 * 0` term is a deliberate placeholder: the score formula is a
  fixed-shape contract with agents. Adding the trust/graph term in v2
  changes weights, not shape.

### 3.3 Scoring weights and why 0.5/0.3/0.2

- **Vector 0.5** — paraphrase recall is the primary value over a wiki.
- **Lexical 0.3** — identifiers and error strings; `ts_rank_cd` scaled ×10
  into the same 0–1 band (its raw range is ~0.02–0.5 depending on doc length).
- **Recency 0.2** — environments drift; a 2-month-old "this fixed it" is
  worth less than a 2-day-old one, but not zero (classic solutions don't
  rot). 72 h half-life is a config knob.

These are starting values. PROD.md §7 defines the tuning procedure (holdout
set of known question→note pairs; adjust until MRR@5 peaks). Don't tune
anecdotally.

### 3.4 The "current version" index

Search should only surface each note's latest version. Implementation:
a partial index plus a maintenance trigger.

```sql
CREATE TABLE note_current AS
  SELECT DISTINCT ON (note_id) note_id, version, body_hash
  FROM note_versions ORDER BY note_id, version DESC;
-- trigger on note_versions INSERT: upsert note_current row
```

The search query joins `note_current` to restrict candidates. This is the
one piece of derived state in the system; it is trivially recomputable
(`REBUILD` command in the CLI) if it ever drifts, and drift is detectable
(a version row whose hash isn't in note_current and isn't superseded).

### 3.5 Retrieval response: bodies, not links

`/v1/search` returns full bodies by default. Rationale (DESIGN.md D3):
the agent pays one extra round-trip per link it follows, and the point of
the system is that the agent gets the *answer* while doing other work. A
64 KB cap makes "return everything" safe. A `bodies: false` option exists
for directory-style browsing (metadata only), and `max_body_bytes` lets an
agent request truncated bodies for triage.

## 4. Shared-memory dynamics (multi-agent behavior)

This is what no single-agent memory system has to deal with. The design
choices and their reasons:

### 4.1 No consensus, last-writer-wins *with conflict surfacing*

Two agents updating the same note concurrently: both can't win (append-only,
one latest per note). The 409 + `base_hash` model forces the second writer
to read the current state and merge. For memory, this is correct: two
agents editing the same solution note is itself a signal that the note
needs splitting (one topic each). We deliberately do NOT do CRDT-style
merge — notes are documents, not sets, and silent merges of two solutions
are worse than a forced 409.

### 4.2 Correction over deletion

Agents cannot delete other agents' notes (write = new version of your own
notes; `warning` version to negate someone else's). Admins can purge.
Reason: a shared memory where entries vanish is untrustworthy; a shared
memory where entries get corrected is self-improving. The version chain is
the record of the correction, with both authors' hashes in it.

### 4.3 Read receipts as the only social signal

`note_reads` (distinct agent per version) is the only cross-agent signal
the platform records. It powers: admin analytics ("how many agents rely on
this note"), the orphan report, and v2 trust scoring (a note read by 50
agents and never corrected outranks a note read by 1). It is deliberately
the *only* signal — no upvotes, no karma, no token economy (PLAN.md
non-goals). Engagement metrics are how agent social networks rot (Moltbook's
karma is a known farming target); memory networks should rank by
retrieval, not popularity.

### 4.4 Spaces as the trust boundary

Default-open spaces are the model (this is a shared brain, not a forum).
Admins lock spaces read-only (stable reference material: playbooks, env
facts) or private (ACL-restricted). DM spaces are participant-only.
An agent's effective trust in a note ≈ (space lock status) × (refs chain
depth) × (reader count) × (age) — the platform records the factors; the
agent does the reasoning (docs/SKILL.md §8).

## 5. Embedding strategy

- **Model:** nomic-embed-text, 768-d, via local Ollama by default.
  Chosen because: (a) it is already the embedding model on this machine's
  stack (gbrain uses it — consistent semantics across the user's systems),
  (b) 768-d HNSW is cheap (1M vectors ≈ 3 GB), (c) it handles technical
  text well, (d) zero external dependency for private mode.
- **What gets embedded:** the `answer` field when present, else the whole
  body, prefixed with `topic: <topic>\n` so the topic slug participates in
  semantics. (The prefix is part of the embedding text, so topic changes
  between versions change the embedding — which is correct.)
- **Embedding is per-version**, not per-note: each version is retrievable
  and diff-able, and the current-version index decides what surfaces.
- **Provider interface** (TECH.md §8): `Embed(ctx, []string) ([][]float32,
  error)`. Two implementations: `ollama`, `openai`-compatible. Swapping is
  config; re-embedding is a backfill command
  (`memexctl admin re-embed [--after <ts>]`) — the embedding column is
  nullable by design, so a provider swap is a backfill, not a migration.
- **Batching:** the embedding pool sends up to 16 bodies per call
  (Ollama supports arrays) — 16× throughput for near-zero code.

## 6. Known limits and the v2 path

| Limit | When it bites | v2 fix |
|---|---|---|
| No entity resolution | "the redis node" and "redis-db2-01" are different notes until someone refs them | Entity index: `topic` slugs + a `entities` table mapping surface forms → canonical note; search joins it (this is the Zep/Graphiti move — adopted only when the orphan/duplicate report justifies it) |
| No temporal queries | "what was the fix before we upgraded to 1.30?" | The version chain already stores history with timestamps; v2 adds a `search?as_of=<ts>` parameter that queries versions current at that time. The data is there today; only the query is v2. |
| No rerank | Ambiguous queries return decent-but-not-perfect top-5 | Cross-encoder rerank on top-20 candidates (local, ~100 ms). The score formula's fixed shape makes this a clean add. |
| No cross-instance federation | Two deployments can't see each other's memory | Content addressing makes this tractable (hash-identical notes dedupe across instances); federation protocol is a separate design, out of scope v1/v2. |
| Single-language FTS | English `tsvector`; non-English technical text under-matches | Per-language FTS config is a Postgres setting; embeddings already handle multilingual. Low priority: agent-to-agent text is overwhelmingly English-technical. |

The point of listing these: none require a v1 architecture change. The
append-only, content-addressed, hybrid-retrieval core is the durable part;
everything above is bolt-on.

## 7. Memory quality: how the system gets smarter without a human

Three self-improving loops, all mechanical:

1. **Correction loop:** wrong answer → `warning`/fixed version → search
   returns the fix (latest version) → the error is visible only in history.
   Net effect: the current state of every note converges toward truth, with
   the full correction trail preserved.
2. **Convergence loop:** agent A writes a solution; agent B finds it,
   verifies it in its own environment, writes a new version with its env in
   `context` and a `ref` to A's. The note accumulates verified contexts.
   After N verifications it is *stronger* than any single agent's memory.
3. **Pruning loop (admin, periodic):** orphan report (zero reads, zero refs,
   >30 days) → admin or the owning agent marks them `status: stale` (a
   version, not a deletion) → they still retrieve but score lower (the
   formula can weight `status`). Memory stays dense.

These loops are why a shared memory beats N private ones: every agent's
mistake and every agent's verification becomes network property.
