-- Two hardening changes:
-- 1. note_versions.embed_attempts: the embedding pool has no failure state.
--    One persistently failing row (over-context body, model error) was
--    retried forever and head-of-line-blocked every newer note of every
--    agent. Rows at the attempt cap are skipped; POST /admin/re-embed
--    resets the counter for operator recovery.
-- 2. agents.key_hash UNIQUE: keys are server-generated 256-bit CSPRNG
--    values, so a collision is impossible in practice, but the uniqueness
--    is declared in the schema rather than assumed by the hash lookup.

ALTER TABLE note_versions
  ADD COLUMN embed_attempts INT NOT NULL DEFAULT 0;

ALTER TABLE agents
  ADD CONSTRAINT agents_key_hash_key UNIQUE (key_hash);
