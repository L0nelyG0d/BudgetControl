// Package db opens the PostgreSQL connection pool and carries the schema.
package db

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema is the idempotent schema (schema.sql), safe to apply repeatedly.
//
//go:embed schema.sql
var Schema string

// Connect opens a pool for url and verifies it with a ping.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// schemaLockID is the advisory lock that serializes schema application, so
// several processes (e.g. parallel test packages) can apply it at once.
const schemaLockID = 727001

// ApplySchema runs schema.sql against the pool. It is safe to call
// concurrently from several processes.
func ApplySchema(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", schemaLockID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, Schema); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IsUniqueViolation reports whether err is a PostgreSQL unique_violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
