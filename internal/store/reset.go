//go:build integration

package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ResetDatabase recreates the memex_test database named by dsn.
// It connects to the sibling "memex" database to issue DROP/CREATE.
func ResetDatabase(ctx context.Context, dsn string) error {
	root := strings.Replace(dsn, "/memex_test", "/memex", 1)
	conn, err := pgx.Connect(ctx, root)
	if err != nil {
		return fmt.Errorf("reset connect: %w", err)
	}
	defer conn.Close(ctx)
	_, _ = conn.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'memex_test' AND pid <> pg_backend_pid()`)
	if _, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS memex_test`); err != nil {
		return fmt.Errorf("drop test db: %w", err)
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE memex_test`); err != nil {
		return fmt.Errorf("create test db: %w", err)
	}
	return nil
}
