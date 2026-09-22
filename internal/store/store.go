package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"memex/migrations"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
	ErrTooLarge  = errors.New("too large")
	ErrQuota     = errors.New("quota")
	ErrExists    = errors.New("exists")
)

type Store struct {
	Pool *pgxpool.Pool
}

type Limits struct {
	NotesPerHour int
	BytesPerHour int64
	HistoryMax   int64
}

type Agent struct {
	ID          string
	Name        string
	Description string
	Card        []byte
	KeyHash     string
	Quota       []byte
	Status      string
	LastActive  *time.Time
	CreatedAt   time.Time
}

type Version struct {
	NoteID    string
	Version   int64
	AgentID   string
	SpaceID   string
	Body      string
	BodyHash  string
	PrevHash  string
	CreatedAt time.Time
}

type WriteResult struct {
	Version      Version
	ConflictHash string
	PrevHash     string
	RetryAfter   int
}

type Space struct {
	ID  string
	ACL []byte
}

type Reader struct {
	AgentID string
	Version int64
	ReadAt  time.Time
}

type StreamRow struct {
	ID        int64
	SpaceID   string
	NoteID    string
	Version   int64
	BodyHash  string
	AgentID   string
	CreatedAt time.Time
}

type AgentStats struct {
	Notes  int64    `json:"notes"`
	Bytes  int64    `json:"bytes"`
	Reads  int64    `json:"reads"`
	Spaces []string `json:"spaces"`
}

type Totals struct {
	Notes    int64 `json:"notes"`
	Versions int64 `json:"versions"`
	Reads    int64 `json:"reads"`
	Agents   int64 `json:"agents"`
	Bytes    int64 `json:"bytes"`
}

type Hit struct {
	Version Version
	Score   float64
}

type SearchQuery struct {
	Text     string
	Vector   string
	Space    string
	AgentID  string
	Since    *time.Time
	Limit    int
	VectorW  float64
	LexicalW float64
	RecencyW float64
	HalfLife float64
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 1
	cfg.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return &Store{Pool: pool}, nil
}

func (s *Store) Close() {
	s.Pool.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.Pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var have bool
		err := s.Pool.QueryRow(ctx, `SELECT true FROM schema_migrations WHERE version=$1`, name).Scan(&have)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if have {
			continue
		}
		body, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		tx, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}

type scanner interface {
	Scan(dest ...any) error
}

func scanVersion(row scanner) (Version, error) {
	var v Version
	err := row.Scan(&v.NoteID, &v.Version, &v.AgentID, &v.SpaceID, &v.Body, &v.BodyHash, &v.PrevHash, &v.CreatedAt)
	return v, err
}

const versionCols = `note_id::text, version, agent_id::text, space_id, body, body_hash, COALESCE(prev_hash,''), created_at`
const versionColsNV = `nv.note_id::text, nv.version, nv.agent_id::text, nv.space_id, nv.body, nv.body_hash, COALESCE(nv.prev_hash,''), nv.created_at`

func likePattern(space string) string {
	if space == "" {
		return ""
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(space)
	return esc + "/%"
}
