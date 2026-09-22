-- Initial schema. Tables from TECH.md §3, plus the pieces the spec
-- requires outside that list: note_current (MEMORY.md §3.4), stream_events
-- (TECH.md §4.3, 24h resume), and memex_can_read for search ACLs.
-- note_versions is append-only except the embedding column, which is
-- derived and nullable (RULES.md §2.4). Admin purge is the only delete.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE agents (
  agent_id    UUID PRIMARY KEY,
  name        TEXT UNIQUE NOT NULL,
  description TEXT,
  card_json   JSONB,
  key_hash    TEXT NOT NULL,
  quota       JSONB NOT NULL DEFAULT '{}',
  status      TEXT NOT NULL DEFAULT 'active',
  last_active TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT agents_status_chk CHECK (status IN ('active', 'suspended', 'revoked'))
);

CREATE TABLE agent_tokens (
  token_hash TEXT PRIMARY KEY,
  agent_id   UUID NOT NULL REFERENCES agents(agent_id),
  expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX agent_tokens_agent_idx ON agent_tokens (agent_id);

CREATE TABLE spaces (
  space_id   TEXT PRIMARY KEY,
  acl_json   JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE note_versions (
  note_id    UUID NOT NULL,
  version    BIGINT NOT NULL,
  agent_id   UUID NOT NULL REFERENCES agents(agent_id),
  space_id   TEXT NOT NULL REFERENCES spaces(space_id),
  body       TEXT NOT NULL,
  body_hash  TEXT NOT NULL,
  prev_hash  TEXT,
  embedding  vector(768),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (note_id, version)
);
CREATE INDEX note_versions_note_version_idx ON note_versions (note_id, version DESC);
CREATE INDEX note_versions_embedding_hnsw ON note_versions USING hnsw (embedding vector_cosine_ops);
CREATE INDEX note_versions_body_fts ON note_versions USING gin (to_tsvector('english', body));
CREATE INDEX note_versions_agent_idx ON note_versions (agent_id, created_at DESC);
CREATE INDEX note_versions_space_idx ON note_versions (space_id, created_at DESC);
CREATE INDEX note_versions_pending_embed ON note_versions (created_at) WHERE embedding IS NULL;

CREATE TABLE note_reads (
  note_id  UUID NOT NULL,
  version  BIGINT NOT NULL,
  agent_id UUID NOT NULL REFERENCES agents(agent_id),
  read_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (note_id, version, agent_id)
);
CREATE INDEX note_reads_agent_idx ON note_reads (agent_id);
CREATE INDEX note_reads_note_idx ON note_reads (note_id);

CREATE TABLE note_current (
  note_id    UUID PRIMARY KEY,
  version    BIGINT NOT NULL,
  body_hash  TEXT NOT NULL,
  space_id   TEXT NOT NULL,
  agent_id   UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE stream_events (
  id         BIGSERIAL PRIMARY KEY,
  space_id   TEXT NOT NULL,
  note_id    UUID NOT NULL,
  version    BIGINT NOT NULL,
  body_hash  TEXT NOT NULL,
  agent_id   UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX stream_events_space_idx ON stream_events (space_id, id);
CREATE INDEX stream_events_created_idx ON stream_events (created_at);

CREATE OR REPLACE FUNCTION memex_can_read(acl jsonb, agent text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
  SELECT
    acl IS NULL
    OR acl = '{}'::jsonb
    OR NOT (acl ? 'read')
    OR jsonb_typeof(acl->'read') <> 'array'
    OR jsonb_array_length(acl->'read') = 0
    OR COALESCE((acl->'read') ? '*', false)
    OR COALESCE((acl->'read') ? agent, false)
    OR COALESCE((acl->'write') ? agent, false)
    OR COALESCE((acl->'write') ? '*', false)
$$;

CREATE OR REPLACE FUNCTION note_version_after_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  eid bigint;
BEGIN
  INSERT INTO note_current (note_id, version, body_hash, space_id, agent_id, created_at)
  VALUES (NEW.note_id, NEW.version, NEW.body_hash, NEW.space_id, NEW.agent_id, NEW.created_at)
  ON CONFLICT (note_id) DO UPDATE SET
    version = EXCLUDED.version,
    body_hash = EXCLUDED.body_hash,
    space_id = EXCLUDED.space_id,
    agent_id = EXCLUDED.agent_id,
    created_at = EXCLUDED.created_at
  WHERE note_current.version < EXCLUDED.version;

  INSERT INTO stream_events (space_id, note_id, version, body_hash, agent_id, created_at)
  VALUES (NEW.space_id, NEW.note_id, NEW.version, NEW.body_hash, NEW.agent_id, NEW.created_at)
  RETURNING id INTO eid;

  PERFORM pg_notify('memex_stream', json_build_object(
    'id', eid,
    'space_id', NEW.space_id,
    'note_id', NEW.note_id,
    'version', NEW.version,
    'body_hash', NEW.body_hash,
    'agent_id', NEW.agent_id,
    'created_at', to_char(NEW.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
  )::text);
  RETURN NEW;
END;
$$;

CREATE TRIGGER note_versions_after_insert
AFTER INSERT ON note_versions
FOR EACH ROW EXECUTE FUNCTION note_version_after_insert();
