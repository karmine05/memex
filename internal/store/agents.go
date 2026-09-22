package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateAgent(ctx context.Context, a Agent) error {
	card := a.Card
	if len(card) == 0 {
		card = []byte(`{}`)
	}
	quota := a.Quota
	if len(quota) == 0 {
		quota = []byte(`{}`)
	}
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO agents (agent_id, name, description, card_json, key_hash, quota, status)
		VALUES ($1::uuid, $2, $3, $4::jsonb, $5, $6::jsonb, 'active')`,
		a.ID, a.Name, a.Description, card, a.KeyHash, quota)
	if isUnique(err) {
		return ErrExists
	}
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return nil
}

func (s *Store) AgentByID(ctx context.Context, id string) (Agent, error) {
	return s.oneAgent(ctx, `WHERE agent_id=$1::uuid`, id)
}

func (s *Store) AgentByName(ctx context.Context, name string) (Agent, error) {
	return s.oneAgent(ctx, `WHERE name=$1`, name)
}

func (s *Store) AgentByKeyHash(ctx context.Context, hash string) (Agent, error) {
	return s.oneAgent(ctx, `WHERE key_hash=$1`, hash)
}

func (s *Store) AgentByToken(ctx context.Context, hash string) (Agent, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT a.agent_id::text, a.name, COALESCE(a.description,''), a.card_json, a.key_hash, a.quota,
		       a.status, a.last_active, a.created_at
		FROM agents a
		JOIN agent_tokens t ON t.agent_id = a.agent_id
		WHERE t.token_hash=$1 AND t.expires_at > now()`, hash)
	a, err := scanAgent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, fmt.Errorf("token: %w", err)
	}
	return a, nil
}

func (s *Store) oneAgent(ctx context.Context, where string, arg any) (Agent, error) {
	q := `SELECT agent_id::text, name, COALESCE(description,''), card_json, key_hash, quota, status, last_active, created_at FROM agents ` + where
	a, err := scanAgent(s.Pool.QueryRow(ctx, q, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, fmt.Errorf("agent: %w", err)
	}
	return a, nil
}

func scanAgent(row scanner) (Agent, error) {
	var a Agent
	err := row.Scan(&a.ID, &a.Name, &a.Description, &a.Card, &a.KeyHash, &a.Quota, &a.Status, &a.LastActive, &a.CreatedAt)
	return a, err
}

func (s *Store) ListAgents(ctx context.Context, q string, includeRevoked bool) ([]Agent, error) {
	q = strings.TrimSpace(q)
	pat := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
	statusSQL := `status <> 'revoked'`
	if includeRevoked {
		statusSQL = `TRUE`
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT agent_id::text, name, COALESCE(description,''), card_json, ''::text, quota, status, last_active, created_at
		FROM agents
		WHERE `+statusSQL+`
		  AND ($1 = '' OR name ILIKE $2 ESCAPE '\' OR COALESCE(description,'') ILIKE $2 ESCAPE '\' OR card_json::text ILIKE $2 ESCAPE '\')
		ORDER BY last_active DESC NULLS LAST, name
		LIMIT 50`, q, pat)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) Touch(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE agents SET last_active = now()
		WHERE agent_id=$1::uuid AND status='active'
		  AND (last_active IS NULL OR last_active < now() - interval '5 minutes')`, id)
	if err != nil {
		return fmt.Errorf("touch %s: %w", id, err)
	}
	return nil
}

func (s *Store) InsertToken(ctx context.Context, hash, agentID string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO agent_tokens (token_hash, agent_id, expires_at)
		VALUES ($1, $2::uuid, now() + interval '1 hour')`, hash, agentID)
	if err != nil {
		return fmt.Errorf("token: %w", err)
	}
	return nil
}

func (s *Store) SetStatus(ctx context.Context, id, status string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE agents SET status=$2 WHERE agent_id=$1::uuid`, id, status)
	if err != nil {
		return fmt.Errorf("status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if status != "active" {
		if _, err := tx.Exec(ctx, `DELETE FROM agent_tokens WHERE agent_id=$1::uuid`, id); err != nil {
			return fmt.Errorf("tokens: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Resume(ctx context.Context, id string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE agents SET status='active' WHERE agent_id=$1::uuid AND status='suspended'`, id)
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetQuota(ctx context.Context, id string, quota []byte) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE agents SET quota=$2::jsonb WHERE agent_id=$1::uuid`, id, quota)
	if err != nil {
		return fmt.Errorf("quota: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteExpiredTokens(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM agent_tokens WHERE expires_at < now()`)
	if err != nil {
		return fmt.Errorf("expire tokens: %w", err)
	}
	return nil
}

func (s *Store) AgentStats(ctx context.Context, id string) (AgentStats, error) {
	var st AgentStats
	err := s.Pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM note_versions WHERE agent_id=$1::uuid),
		  (SELECT coalesce(sum(octet_length(body)),0) FROM note_versions WHERE agent_id=$1::uuid),
		  (SELECT count(*) FROM note_reads WHERE agent_id=$1::uuid)`, id).Scan(&st.Notes, &st.Bytes, &st.Reads)
	if err != nil {
		return st, fmt.Errorf("stats: %w", err)
	}
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT space_id FROM note_versions WHERE agent_id=$1::uuid ORDER BY space_id`, id)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var sp string
		if err := rows.Scan(&sp); err != nil {
			return st, err
		}
		st.Spaces = append(st.Spaces, sp)
	}
	if st.Spaces == nil {
		st.Spaces = []string{}
	}
	return st, rows.Err()
}

func (s *Store) Stats(ctx context.Context, space string) (Totals, error) {
	var t Totals
	pat := likePattern(space)
	err := s.Pool.QueryRow(ctx, `
		SELECT count(DISTINCT note_id), count(*), count(DISTINCT agent_id), coalesce(sum(octet_length(body)),0)
		FROM note_versions
		WHERE ($1 = '' OR space_id = $1 OR space_id LIKE $2 ESCAPE '\')`, space, pat).Scan(&t.Notes, &t.Versions, &t.Agents, &t.Bytes)
	if err != nil {
		return t, fmt.Errorf("stats: %w", err)
	}
	err = s.Pool.QueryRow(ctx, `
		SELECT count(*) FROM note_reads r
		JOIN note_versions nv ON nv.note_id = r.note_id AND nv.version = r.version
		WHERE ($1 = '' OR nv.space_id = $1 OR nv.space_id LIKE $2 ESCAPE '\')`, space, pat).Scan(&t.Reads)
	if err != nil {
		return t, fmt.Errorf("stats reads: %w", err)
	}
	return t, nil
}

func (s *Store) Counts(ctx context.Context) (versions int64, pending int64, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM note_versions`).Scan(&versions)
	if err != nil {
		return 0, 0, err
	}
	err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM note_versions WHERE embedding IS NULL`).Scan(&pending)
	return versions, pending, err
}

func (s *Store) TrimStreams(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM stream_events WHERE created_at < now() - interval '24 hours'`)
	if err != nil {
		return fmt.Errorf("trim streams: %w", err)
	}
	return nil
}
