package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Allow decides a write from the ACL JSON read inside the transaction.
// The store does not import the note package. The API passes the policy.
type Allow func(acl []byte) error

func (s *Store) Space(ctx context.Context, id string) (Space, error) {
	var sp Space
	err := s.Pool.QueryRow(ctx, `SELECT space_id, acl_json FROM spaces WHERE space_id=$1`, id).Scan(&sp.ID, &sp.ACL)
	if errors.Is(err, pgx.ErrNoRows) {
		return Space{}, ErrNotFound
	}
	if err != nil {
		return Space{}, fmt.Errorf("space %s: %w", id, err)
	}
	return sp, nil
}

func (s *Store) SetSpaceACL(ctx context.Context, id string, acl []byte) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE spaces SET acl_json=$2::jsonb WHERE space_id=$1`, id, acl)
	if err != nil {
		return fmt.Errorf("acl %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		_, err = s.Pool.Exec(ctx, `INSERT INTO spaces (space_id, acl_json) VALUES ($1, $2::jsonb)`, id, acl)
		if err != nil {
			return fmt.Errorf("acl %s: %w", id, err)
		}
	}
	return nil
}

func (s *Store) CreateNote(ctx context.Context, v Version, acl []byte, lim Limits, allow Allow) (WriteResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return WriteResult{}, err
	}
	defer tx.Rollback(ctx)
	retry, err := s.prepare(ctx, tx, v.SpaceID, acl, v.AgentID, len(v.Body), lim, allow)
	if err != nil {
		return WriteResult{RetryAfter: retry}, err
	}
	var exists bool
	err = tx.QueryRow(ctx, `SELECT true FROM note_current WHERE note_id=$1::uuid`, v.NoteID).Scan(&exists)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return WriteResult{}, fmt.Errorf("note %s: %w", v.NoteID, err)
	}
	if exists {
		return WriteResult{}, ErrExists
	}
	out, err := insertVersion(ctx, tx, v, 1, nil)
	if err != nil {
		return WriteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Version: out}, nil
}

func (s *Store) UpdateNote(ctx context.Context, noteID, agentID, body, bodyHash, baseHash string, lim Limits, allow Allow) (WriteResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return WriteResult{}, err
	}
	defer tx.Rollback(ctx)
	var curVer int64
	var curHash, prev, space string
	err = tx.QueryRow(ctx, `
		SELECT nc.version, nv.body_hash, COALESCE(nv.prev_hash,''), nv.space_id
		FROM note_current nc
		JOIN note_versions nv ON nv.note_id = nc.note_id AND nv.version = nc.version
		WHERE nc.note_id=$1::uuid
		FOR UPDATE OF nc`, noteID).Scan(&curVer, &curHash, &prev, &space)
	if errors.Is(err, pgx.ErrNoRows) {
		return WriteResult{}, ErrNotFound
	}
	if err != nil {
		return WriteResult{}, fmt.Errorf("note %s: %w", noteID, err)
	}
	if baseHash != curHash {
		return WriteResult{ConflictHash: curHash, PrevHash: prev}, ErrConflict
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT acl_json FROM spaces WHERE space_id=$1`, space).Scan(&raw); err != nil {
		return WriteResult{}, fmt.Errorf("space %s: %w", space, err)
	}
	if allow != nil {
		if err := allow(raw); err != nil {
			return WriteResult{}, err
		}
	}
	var hist int64
	if err := tx.QueryRow(ctx, `SELECT coalesce(sum(octet_length(body)),0) FROM note_versions WHERE note_id=$1::uuid`, noteID).Scan(&hist); err != nil {
		return WriteResult{}, err
	}
	if lim.HistoryMax > 0 && hist+int64(len(body)) > lim.HistoryMax {
		return WriteResult{}, ErrTooLarge
	}
	retry, err := s.enforceQuota(ctx, tx, agentID, len(body), lim)
	if err != nil {
		return WriteResult{RetryAfter: retry}, err
	}
	out, err := insertVersion(ctx, tx, Version{
		NoteID: noteID, AgentID: agentID, SpaceID: space, Body: body, BodyHash: bodyHash,
	}, curVer+1, &curHash)
	if err != nil {
		return WriteResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Version: out}, nil
}

func (s *Store) prepare(ctx context.Context, tx pgx.Tx, space string, acl []byte, agentID string, add int, lim Limits, allow Allow) (int, error) {
	if len(acl) == 0 {
		acl = []byte(`{}`)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO spaces (space_id, acl_json) VALUES ($1, $2::jsonb) ON CONFLICT DO NOTHING`, space, acl); err != nil {
		return 0, fmt.Errorf("space %s: %w", space, err)
	}
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT acl_json FROM spaces WHERE space_id=$1`, space).Scan(&raw); err != nil {
		return 0, fmt.Errorf("space %s: %w", space, err)
	}
	if allow != nil {
		if err := allow(raw); err != nil {
			return 0, err
		}
	}
	return s.enforceQuota(ctx, tx, agentID, add, lim)
}

func (s *Store) enforceQuota(ctx context.Context, tx pgx.Tx, agentID string, add int, lim Limits) (int, error) {
	var n int
	var bytes int64
	err := tx.QueryRow(ctx, `
		SELECT count(*), coalesce(sum(octet_length(body)),0)
		FROM note_versions
		WHERE agent_id=$1::uuid AND created_at > now() - interval '1 hour'`, agentID).Scan(&n, &bytes)
	if err != nil {
		return 0, fmt.Errorf("quota: %w", err)
	}
	overNotes := lim.NotesPerHour > 0 && n+1 > lim.NotesPerHour
	overBytes := lim.BytesPerHour > 0 && bytes+int64(add) > lim.BytesPerHour
	if !overNotes && !overBytes {
		return 0, nil
	}
	var oldest time.Time
	err = tx.QueryRow(ctx, `
		SELECT created_at FROM note_versions
		WHERE agent_id=$1::uuid AND created_at > now() - interval '1 hour'
		ORDER BY created_at ASC LIMIT 1`, agentID).Scan(&oldest)
	retry := 60
	if err == nil {
		retry = int(time.Until(oldest.Add(time.Hour)).Seconds()) + 1
		if retry < 1 {
			retry = 1
		}
	}
	return retry, ErrQuota
}

func insertVersion(ctx context.Context, tx pgx.Tx, v Version, version int64, prev *string) (Version, error) {
	row := tx.QueryRow(ctx, `
		INSERT INTO note_versions (note_id, version, agent_id, space_id, body, body_hash, prev_hash)
		VALUES ($1::uuid, $2, $3::uuid, $4, $5, $6, $7)
		RETURNING `+versionCols, v.NoteID, version, v.AgentID, v.SpaceID, v.Body, v.BodyHash, prev)
	out, err := scanVersion(row)
	if err != nil {
		return Version{}, fmt.Errorf("insert note %s: %w", v.NoteID, err)
	}
	return out, nil
}

func (s *Store) Latest(ctx context.Context, noteID string) (Version, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+versionColsNV+`
		FROM note_versions nv
		JOIN note_current nc ON nc.note_id = nv.note_id AND nc.version = nv.version
		WHERE nv.note_id=$1::uuid`, noteID)
	v, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, fmt.Errorf("note %s: %w", noteID, err)
	}
	return v, nil
}

func (s *Store) Versions(ctx context.Context, noteID string) ([]Version, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+versionCols+` FROM note_versions WHERE note_id=$1::uuid ORDER BY version`, noteID)
	if err != nil {
		return nil, fmt.Errorf("versions %s: %w", noteID, err)
	}
	return collectVersions(rows)
}

func (s *Store) VersionByHash(ctx context.Context, noteID, hash string) (Version, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+versionCols+` FROM note_versions WHERE note_id=$1::uuid AND body_hash=$2`, noteID, hash)
	v, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, ErrNotFound
	}
	if err != nil {
		return Version{}, fmt.Errorf("version %s: %w", noteID, err)
	}
	return v, nil
}

func collectVersions(rows pgx.Rows) ([]Version, error) {
	defer rows.Close()
	var out []Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) RecordRead(ctx context.Context, noteID string, version int64, agentID string) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO note_reads (note_id, version, agent_id)
		VALUES ($1::uuid, $2, $3::uuid)
		ON CONFLICT DO NOTHING`, noteID, version, agentID)
	if err != nil {
		return fmt.Errorf("read %s: %w", noteID, err)
	}
	return nil
}

func (s *Store) Readers(ctx context.Context, noteID string) ([]Reader, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT agent_id::text, version, read_at FROM note_reads
		WHERE note_id=$1::uuid ORDER BY version, read_at`, noteID)
	if err != nil {
		return nil, fmt.Errorf("readers %s: %w", noteID, err)
	}
	defer rows.Close()
	var out []Reader
	for rows.Next() {
		var r Reader
		if err := rows.Scan(&r.AgentID, &r.Version, &r.ReadAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ReaderCount(ctx context.Context, noteID string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(DISTINCT agent_id) FROM note_reads WHERE note_id=$1::uuid`, noteID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("readers %s: %w", noteID, err)
	}
	return n, nil
}

func (s *Store) PurgeNote(ctx context.Context, noteID string) ([]Version, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT `+versionCols+` FROM note_versions WHERE note_id=$1::uuid ORDER BY version`, noteID)
	if err != nil {
		return nil, err
	}
	chain, err := collectVersions(rows)
	if err != nil {
		return nil, err
	}
	if len(chain) == 0 {
		return nil, ErrNotFound
	}
	for _, q := range []string{
		`DELETE FROM note_reads WHERE note_id=$1::uuid`,
		`DELETE FROM stream_events WHERE note_id=$1::uuid`,
		`DELETE FROM note_current WHERE note_id=$1::uuid`,
		`DELETE FROM note_versions WHERE note_id=$1::uuid`,
	} {
		if _, err := tx.Exec(ctx, q, noteID); err != nil {
			return nil, fmt.Errorf("purge %s: %w", noteID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return chain, nil
}
