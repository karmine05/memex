package store

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) Search(ctx context.Context, q SearchQuery) ([]Hit, error) {
	if q.Limit <= 0 {
		q.Limit = 5
	}
	pat := likePattern(q.Space)
	var (
		rows interface {
			Next() bool
			Scan(...any) error
			Err() error
			Close()
		}
		err error
	)
	if q.Vector == "" {
		rows, err = s.Pool.Query(ctx, lexSQL, q.Text, q.Space, pat, q.Since, q.AgentID, q.LexicalW, q.RecencyW, q.HalfLife, q.Limit)
	} else {
		rows, err = s.Pool.Query(ctx, hybridSQL, q.Vector, q.Space, pat, q.Since, q.AgentID, q.Text, q.VectorW, q.LexicalW, q.RecencyW, q.HalfLife, q.Limit)
	}
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()
	var out []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.Version.NoteID, &h.Version.Version, &h.Version.Body, &h.Version.BodyHash, &h.Version.AgentID, &h.Version.CreatedAt, &h.Version.PrevHash, &h.Version.SpaceID, &h.Score); err != nil {
			return nil, fmt.Errorf("search: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

const spaceFilter = `($2 = '' OR nv.space_id = $2 OR nv.space_id LIKE $3 ESCAPE '\')`
const timeFilter = `($4::timestamptz IS NULL OR nv.created_at >= $4)`
const aclFilter = `memex_can_read(s.acl_json, $5)`

const lexSQL = `
SELECT note_id, version, body, body_hash, agent_id, created_at, prev_hash, space_id, score FROM (
  SELECT nv.note_id::text AS note_id, nv.version, nv.body, nv.body_hash,
         nv.agent_id::text AS agent_id, nv.created_at, COALESCE(nv.prev_hash,'') AS prev_hash, nv.space_id,
         ($6::float8 * ts_rank_cd(to_tsvector('english', nv.body), plainto_tsquery('english', $1)) * 10
           + $7::float8 * exp(-0.693147 * extract(epoch FROM (now() - nv.created_at)) / $8::float8)) AS score
  FROM note_versions nv
  JOIN note_current nc ON nc.note_id = nv.note_id AND nc.version = nv.version
  JOIN spaces s ON s.space_id = nv.space_id
  WHERE to_tsvector('english', nv.body) @@ plainto_tsquery('english', $1)
    AND ` + spaceFilter + `
    AND ` + timeFilter + `
    AND ` + aclFilter + `
  ORDER BY score DESC
  LIMIT 100
) lex
ORDER BY score DESC
LIMIT $9`

const hybridSQL = `
SELECT note_id, version, body, body_hash, agent_id, created_at, prev_hash, space_id, score FROM (
  SELECT
    COALESCE(v.note_id, l.note_id) AS note_id,
    COALESCE(v.version, l.version) AS version,
    COALESCE(v.body, l.body) AS body,
    COALESCE(v.body_hash, l.body_hash) AS body_hash,
    COALESCE(v.agent_id, l.agent_id) AS agent_id,
    COALESCE(v.created_at, l.created_at) AS created_at,
    COALESCE(v.prev_hash, l.prev_hash, '') AS prev_hash,
    COALESCE(v.space_id, l.space_id) AS space_id,
    ($7::float8 * COALESCE(v.v_score, 0)
      + $8::float8 * COALESCE(l.l_score, 0) * 10
      + $9::float8 * exp(-0.693147 * extract(epoch FROM (now() - COALESCE(v.created_at, l.created_at))) / $10::float8)
    ) AS score
  FROM (
    SELECT nv.note_id::text AS note_id, nv.version, nv.body, nv.body_hash,
           nv.agent_id::text AS agent_id, nv.created_at, nv.prev_hash, nv.space_id,
           1 - (nv.embedding <=> $1::vector) AS v_score
    FROM note_versions nv
    JOIN note_current nc ON nc.note_id = nv.note_id AND nc.version = nv.version
    JOIN spaces s ON s.space_id = nv.space_id
    WHERE nv.embedding IS NOT NULL
      AND ($2 = '' OR nv.space_id = $2 OR nv.space_id LIKE $3 ESCAPE '\')
      AND ($4::timestamptz IS NULL OR nv.created_at >= $4)
      AND memex_can_read(s.acl_json, $5)
    ORDER BY nv.embedding <=> $1::vector
    LIMIT 100
  ) v
  FULL OUTER JOIN (
    SELECT nv.note_id::text AS note_id, nv.version, nv.body, nv.body_hash,
           nv.agent_id::text AS agent_id, nv.created_at, nv.prev_hash, nv.space_id,
           ts_rank_cd(to_tsvector('english', nv.body), plainto_tsquery('english', $6)) AS l_score
    FROM note_versions nv
    JOIN note_current nc ON nc.note_id = nv.note_id AND nc.version = nv.version
    JOIN spaces s ON s.space_id = nv.space_id
    WHERE to_tsvector('english', nv.body) @@ plainto_tsquery('english', $6)
      AND ($2 = '' OR nv.space_id = $2 OR nv.space_id LIKE $3 ESCAPE '\')
      AND ($4::timestamptz IS NULL OR nv.created_at >= $4)
      AND memex_can_read(s.acl_json, $5)
    ORDER BY l_score DESC
    LIMIT 100
  ) l ON v.note_id = l.note_id AND v.version = l.version
) ranked
ORDER BY score DESC
LIMIT $11`

func (s *Store) EventsAfter(ctx context.Context, space string, after int64, limit int) ([]StreamRow, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, note_id::text, version, body_hash, agent_id::text, created_at
		FROM stream_events
		WHERE space_id=$1 AND id > $2
		ORDER BY id
		LIMIT $3`, space, after, limit)
	if err != nil {
		return nil, fmt.Errorf("events %s: %w", space, err)
	}
	return collectStreams(rows)
}

func (s *Store) InboxEvents(ctx context.Context, agentID string, after int64, limit int) ([]StreamRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, space_id, note_id::text, version, body_hash, agent_id::text, created_at
		FROM stream_events
		WHERE id > $2
		  AND (space_id LIKE 'dm/' || $1 || '/%' OR space_id LIKE 'dm/%/' || $1)
		ORDER BY id
		LIMIT $3`, agentID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox events: %w", err)
	}
	return collectStreams(rows)
}

func collectStreams(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]StreamRow, error) {
	defer rows.Close()
	var out []StreamRow
	for rows.Next() {
		var r StreamRow
		if err := rows.Scan(&r.ID, &r.SpaceID, &r.NoteID, &r.Version, &r.BodyHash, &r.AgentID, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) InboxNotes(ctx context.Context, agentID string, since int64, limit int, latest bool) ([]Version, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT ` + versionColsNV + `
		FROM stream_events se
		JOIN note_versions nv ON nv.note_id = se.note_id AND nv.version = se.version
		WHERE se.id > $2
		  AND (se.space_id LIKE 'dm/' || $1 || '/%' OR se.space_id LIKE 'dm/%/' || $1)
		ORDER BY se.id ASC
		LIMIT $3`
	args := []any{agentID, since, limit}
	if latest {
		q = `
			SELECT ` + versionColsNV + `
			FROM stream_events se
			JOIN note_versions nv ON nv.note_id = se.note_id AND nv.version = se.version
			WHERE se.id IN (
			  SELECT id FROM stream_events
			  WHERE space_id LIKE 'dm/' || $1 || '/%' OR space_id LIKE 'dm/%/' || $1
			  ORDER BY id DESC
			  LIMIT $2
			)
			ORDER BY se.id ASC`
		args = []any{agentID, limit}
	}
	rows, err := s.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("inbox: %w", err)
	}
	return collectVersions(rows)
}

func (s *Store) PendingEmbeddings(ctx context.Context, limit int) ([]Version, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT note_id::text, version, ''::text, ''::text, body, ''::text, ''::text, created_at
		FROM note_versions
		WHERE embedding IS NULL
		ORDER BY created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("pending embeds: %w", err)
	}
	return collectVersions(rows)
}

func (s *Store) SetEmbedding(ctx context.Context, noteID string, version int64, literal string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE note_versions SET embedding = $3::vector
		WHERE note_id=$1::uuid AND version=$2 AND embedding IS NULL`, noteID, version, literal)
	if err != nil {
		return fmt.Errorf("embed %s: %w", noteID, err)
	}
	return nil
}

func (s *Store) ClearEmbeddings(ctx context.Context, after *time.Time) (int64, error) {
	var n int64
	err := s.Pool.QueryRow(ctx, `
		WITH u AS (
		  UPDATE note_versions SET embedding = NULL
		  WHERE ($1::timestamptz IS NULL OR created_at >= $1)
		  RETURNING 1
		)
		SELECT count(*) FROM u`, after).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("re-embed: %w", err)
	}
	return n, nil
}

type Dupe struct {
	A   string  `json:"a"`
	B   string  `json:"b"`
	Sim float64 `json:"sim"`
}

func (s *Store) Dupes(ctx context.Context) ([]Dupe, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT a.note_id::text, b.note_id::text, (1 - (a.embedding <=> b.embedding))::float8
		FROM note_versions a
		JOIN note_current ca ON ca.note_id = a.note_id AND ca.version = a.version
		JOIN note_versions b ON a.note_id < b.note_id
		JOIN note_current cb ON cb.note_id = b.note_id AND cb.version = b.version
		WHERE a.embedding IS NOT NULL AND b.embedding IS NOT NULL
		  AND (1 - (a.embedding <=> b.embedding)) > 0.95
		LIMIT 50`)
	if err != nil {
		return nil, fmt.Errorf("dupes: %w", err)
	}
	defer rows.Close()
	var out []Dupe
	for rows.Next() {
		var d Dupe
		if err := rows.Scan(&d.A, &d.B, &d.Sim); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) Orphans(ctx context.Context, olderThan time.Time) ([]Version, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+versionColsNV+`
		FROM note_versions nv
		JOIN note_current nc ON nc.note_id = nv.note_id AND nc.version = nv.version
		WHERE nv.created_at < $1
		  AND NOT EXISTS (SELECT 1 FROM note_reads r WHERE r.note_id = nv.note_id)
		  AND NOT EXISTS (
		    SELECT 1 FROM note_versions o
		    WHERE o.note_id <> nv.note_id AND position(nv.note_id::text in o.body) > 0
		  )
		ORDER BY nv.created_at
		LIMIT 100`, olderThan)
	if err != nil {
		return nil, fmt.Errorf("orphans: %w", err)
	}
	return collectVersions(rows)
}
