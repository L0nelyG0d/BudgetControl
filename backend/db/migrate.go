package db

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var embeddedMigrations embed.FS

// schemaLockID is the advisory lock that serializes schema application, so
// several processes (e.g. parallel test packages) can apply it at once.
const schemaLockID = 727001

var migrationName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

type migration struct {
	version  int64
	name     string // file name
	sql      string
	checksum string // hex SHA-256 of the exact file bytes
}

// ApplySchema applies the embedded migrations that the database has not seen
// yet. It is safe to call concurrently from several processes.
func ApplySchema(ctx context.Context, pool *pgxpool.Pool) error {
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return err
	}
	return ApplyMigrations(ctx, pool, sub)
}

// loadMigrations reads and validates the migration files in fsys (flat
// directory of NNNN_description.sql). The result is sorted by version and is
// contiguous from 1.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var out []migration
	byVersion := map[int64]string{}
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			return nil, fmt.Errorf("migration file %q: name must match NNNN_description.sql", e.Name())
		}
		v, _ := strconv.ParseInt(m[1], 10, 64)
		if other, dup := byVersion[v]; dup {
			return nil, fmt.Errorf("migration files %q and %q: duplicate version %d", other, e.Name(), v)
		}
		byVersion[v] = e.Name()
		body, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		sum := sha256.Sum256(body)
		out = append(out, migration{version: v, name: e.Name(), sql: string(body), checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i, m := range out {
		if want := int64(i + 1); m.version != want {
			return nil, fmt.Errorf("migration file %q: gap in versions, expected version %d", m.name, want)
		}
	}
	return out, nil
}

// ApplyMigrations runs every pending migration from fsys in one transaction
// under the schema advisory lock. Any failure rolls the whole run back.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	migs, err := loadMigrations(fsys)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", schemaLockID); err != nil {
		return err
	}

	// Decide on adoption before the table exists: a database built from the
	// old schema.sql has users but no schema_migrations.
	var hasTracking, hasUsers bool
	if err := tx.QueryRow(ctx,
		`SELECT to_regclass('schema_migrations') IS NOT NULL, to_regclass('users') IS NOT NULL`,
	).Scan(&hasTracking, &hasUsers); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    BIGINT PRIMARY KEY,
    name       TEXT        NOT NULL,
    checksum   TEXT        NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return err
	}
	applied := map[int64]string{} // version -> recorded checksum
	if !hasTracking && hasUsers && len(migs) > 0 {
		if err := record(ctx, tx, migs[0]); err != nil {
			return err
		}
		applied[migs[0].version] = migs[0].checksum
	} else {
		rows, err := tx.Query(ctx, `SELECT version, checksum FROM schema_migrations`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var v int64
			var sum string
			if err := rows.Scan(&v, &sum); err != nil {
				rows.Close()
				return err
			}
			applied[v] = sum
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}

	if err := checkApplied(migs, applied); err != nil {
		return err
	}
	for _, m := range migs {
		if _, done := applied[m.version]; done {
			continue
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if err := record(ctx, tx, m); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
	}
	return tx.Commit(ctx)
}

// checkApplied rejects a database that is ahead of, or inconsistent with, the
// embedded migration set.
func checkApplied(migs []migration, applied map[int64]string) error {
	known := map[int64]migration{}
	for _, m := range migs {
		known[m.version] = m
	}
	var versions []int64
	var highest int64
	for v := range applied {
		versions = append(versions, v)
		if v > highest {
			highest = v
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	for _, v := range versions {
		m, ok := known[v]
		if !ok {
			return fmt.Errorf("database has migration version %d, which is newer than this binary", v)
		}
		if applied[v] != m.checksum {
			return fmt.Errorf("migration %s was modified after it was applied (checksum mismatch)", m.name)
		}
	}
	for _, m := range migs {
		if _, done := applied[m.version]; !done && m.version < highest {
			return fmt.Errorf("migration %s (version %d) is not applied but version %d is: hole in applied migrations",
				m.name, m.version, highest)
		}
	}
	return nil
}

func record(ctx context.Context, tx pgx.Tx, m migration) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		m.version, strings.TrimSuffix(m.name, ".sql"), m.checksum)
	return err
}
