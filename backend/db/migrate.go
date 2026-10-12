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

// downName matches NNNN_description.down.sql. It cannot match migrationName
// (the description has no dot), so the two kinds of file are told apart.
var downName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.down\.sql$`)

type migration struct {
	version  int64
	name     string // file name
	sql      string
	checksum string // hex SHA-256 of the exact file bytes
	downFile string // name of the matching NNNN_description.down.sql, "" if none
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
// directory of NNNN_description.sql, optionally with a matching
// NNNN_description.down.sql). Down files are only matched to their up file
// here; they are not read or checksummed. The result is sorted by version and is
// contiguous from 1.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	var out []migration
	byVersion := map[int64]string{}
	var downs []string
	for _, e := range entries {
		if downName.MatchString(e.Name()) && !e.IsDir() {
			downs = append(downs, e.Name())
			continue
		}
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
	for _, d := range downs {
		up := strings.TrimSuffix(d, ".down.sql") + ".sql"
		if d[:4] == "0001" {
			return nil, fmt.Errorf("down file %q: 0001 is the baseline and cannot be reverted", d)
		}
		found := false
		for i := range out {
			if out[i].name == up {
				out[i].downFile = d
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("down file %q has no matching up file %q", d, up)
		}
	}
	return out, nil
}

// ApplyMigrations runs every pending migration from fsys in one transaction
// under the schema advisory lock. Any failure rolls the whole run back.
func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	_, err := ApplyMigrationsReport(ctx, pool, fsys)
	return err
}

// ApplyResult says what a successful ApplyMigrationsReport did.
type ApplyResult struct {
	Adopted bool     // the first migration was recorded without running (legacy database)
	Applied []string // names (e.g. "0002_add_tags") of migrations run, in version order
}

// ApplyEmbedded is ApplyMigrationsReport for the embedded migrations.
func ApplyEmbedded(ctx context.Context, pool *pgxpool.Pool) (ApplyResult, error) {
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyMigrationsReport(ctx, pool, sub)
}

// ApplyMigrationsReport is ApplyMigrations that also reports what it did. The
// result is only meaningful (and non-zero) after a successful commit.
func ApplyMigrationsReport(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) (ApplyResult, error) {
	var res ApplyResult
	migs, err := loadMigrations(fsys)
	if err != nil {
		return ApplyResult{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", schemaLockID); err != nil {
		return ApplyResult{}, err
	}

	// Decide on adoption before the table exists: a database built from the
	// old schema.sql has users but no schema_migrations.
	var hasTracking, hasUsers bool
	if err := tx.QueryRow(ctx,
		`SELECT to_regclass('schema_migrations') IS NOT NULL, to_regclass('users') IS NOT NULL`,
	).Scan(&hasTracking, &hasUsers); err != nil {
		return ApplyResult{}, err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
    version    BIGINT PRIMARY KEY,
    name       TEXT        NOT NULL,
    checksum   TEXT        NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`); err != nil {
		return ApplyResult{}, err
	}
	applied := map[int64]string{} // version -> recorded checksum
	if !hasTracking && hasUsers && len(migs) > 0 {
		if err := record(ctx, tx, migs[0]); err != nil {
			return ApplyResult{}, err
		}
		applied[migs[0].version] = migs[0].checksum
		res.Adopted = true
	} else {
		var err error
		if applied, err = readApplied(ctx, tx); err != nil {
			return ApplyResult{}, err
		}
	}

	if err := checkApplied(migs, applied); err != nil {
		return ApplyResult{}, err
	}
	for _, m := range migs {
		if _, done := applied[m.version]; done {
			continue
		}
		if _, err := tx.Exec(ctx, m.sql); err != nil {
			return ApplyResult{}, fmt.Errorf("migration %s: %w", m.name, err)
		}
		if err := record(ctx, tx, m); err != nil {
			return ApplyResult{}, fmt.Errorf("migration %s: %w", m.name, err)
		}
		res.Applied = append(res.Applied, strings.TrimSuffix(m.name, ".sql"))
	}
	if err := tx.Commit(ctx); err != nil {
		return ApplyResult{}, err
	}
	return res, nil
}

// checkApplied rejects a database that is ahead of, or inconsistent with, the
// embedded migration set. It reports the first problem.
func checkApplied(migs []migration, applied map[int64]string) error {
	if errs := appliedProblems(migs, applied); len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// appliedProblems returns every inconsistency between applied and migs:
// newer-than-binary and checksum mismatches by version, then holes.
func appliedProblems(migs []migration, applied map[int64]string) []error {
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
	var errs []error
	for _, v := range versions {
		m, ok := known[v]
		if !ok {
			errs = append(errs, fmt.Errorf("database has migration version %d, which is newer than this binary", v))
			continue
		}
		if applied[v] != m.checksum {
			errs = append(errs, fmt.Errorf("migration %s was modified after it was applied (checksum mismatch)", m.name))
		}
	}
	for _, m := range migs {
		if _, done := applied[m.version]; !done && m.version < highest {
			errs = append(errs, fmt.Errorf("migration %s (version %d) is not applied but version %d is: hole in applied migrations",
				m.name, m.version, highest))
		}
	}
	return errs
}

func record(ctx context.Context, tx pgx.Tx, m migration) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`,
		m.version, strings.TrimSuffix(m.name, ".sql"), m.checksum)
	return err
}

// readApplied returns version -> recorded checksum from schema_migrations,
// which must exist.
func readApplied(ctx context.Context, tx pgx.Tx) (map[int64]string, error) {
	rows, err := tx.Query(ctx, `SELECT version, checksum FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	applied := map[int64]string{}
	for rows.Next() {
		var v int64
		var sum string
		if err := rows.Scan(&v, &sum); err != nil {
			return nil, err
		}
		applied[v] = sum
	}
	return applied, rows.Err()
}

// RevertLatest reverts the latest applied embedded migration; see
// RevertMigration. It is only called by the `migrate down` command, never at
// server startup.
func RevertLatest(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return "", err
	}
	return RevertMigration(ctx, pool, sub)
}

// RevertMigration reverts only the single latest applied migration of fsys:
// it runs its down file and deletes its schema_migrations row in one
// transaction under the schema advisory lock, and returns the migration name
// (e.g. "0002_add_tags"). It returns "" and no error when nothing is applied;
// in that case it creates nothing and does not adopt a legacy database. It
// refuses, changing nothing, on the same inconsistencies as ApplyMigrations,
// when the latest migration is 0001, or when it has no down file.
func RevertMigration(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) (string, error) {
	migs, err := loadMigrations(fsys)
	if err != nil {
		return "", err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", schemaLockID); err != nil {
		return "", err
	}
	var hasTracking bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&hasTracking); err != nil {
		return "", err
	}
	if !hasTracking {
		return "", nil
	}
	applied, err := readApplied(ctx, tx)
	if err != nil {
		return "", err
	}
	if len(applied) == 0 {
		return "", nil
	}
	if err := checkApplied(migs, applied); err != nil {
		return "", err
	}
	// checkApplied guarantees every applied version is known, contiguous
	// from 1, so the latest is migs[len(applied)-1].
	latest := migs[len(applied)-1]
	base := strings.TrimSuffix(latest.name, ".sql")
	if latest.version == 1 {
		return "", fmt.Errorf("%s cannot be reverted", base)
	}
	if latest.downFile == "" {
		return "", fmt.Errorf("no down migration for %s", base)
	}
	body, err := fs.ReadFile(fsys, latest.downFile)
	if err != nil {
		return "", fmt.Errorf("read migration %s: %w", latest.downFile, err)
	}
	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return "", fmt.Errorf("migration %s: %w", latest.downFile, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, latest.version); err != nil {
		return "", fmt.Errorf("migration %s: %w", latest.downFile, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return base, nil
}

// StatusLine is one row of `migrate status`.
type StatusLine struct {
	Version     int64
	Description string
	State       string // "applied" or "pending"
	Checksum    string // "ok", "mismatch", "-", "hole" or "newer"
}

// StatusReport is the read-only view of applied and pending migrations.
type StatusReport struct {
	Lines    []StatusLine
	Applied  int // includes "newer" rows
	Pending  int
	Problems []error // mismatch, newer and hole, worded as ApplyMigrations does
	Legacy   bool    // users exists but schema_migrations does not
}

// Status reports the migration state of the database against the embedded
// migrations; see MigrationStatus.
func Status(ctx context.Context, pool *pgxpool.Pool) (StatusReport, error) {
	sub, err := fs.Sub(embeddedMigrations, "migrations")
	if err != nil {
		return StatusReport{}, err
	}
	return MigrationStatus(ctx, pool, sub)
}

var versionPrefix = regexp.MustCompile(`^\d{4}_`)

// MigrationStatus compares fsys with the database in a read-only transaction.
// It creates nothing, adopts nothing and takes no advisory lock. Down files
// are not read.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) (StatusReport, error) {
	var rep StatusReport
	migs, err := loadMigrations(fsys)
	if err != nil {
		return rep, err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return rep, err
	}
	defer tx.Rollback(ctx)
	var hasTracking, hasUsers bool
	if err := tx.QueryRow(ctx,
		`SELECT to_regclass('schema_migrations') IS NOT NULL, to_regclass('users') IS NOT NULL`,
	).Scan(&hasTracking, &hasUsers); err != nil {
		return rep, err
	}
	applied := map[int64]string{}
	names := map[int64]string{}
	if hasTracking {
		rows, err := tx.Query(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
		if err != nil {
			return rep, err
		}
		defer rows.Close()
		for rows.Next() {
			var v int64
			var name, sum string
			if err := rows.Scan(&v, &name, &sum); err != nil {
				return rep, err
			}
			applied[v], names[v] = sum, name
		}
		if err := rows.Err(); err != nil {
			return rep, err
		}
	} else if hasUsers && len(migs) > 0 {
		rep.Legacy = true
	}
	rep.Problems = appliedProblems(migs, applied)

	var highest int64
	for v := range applied {
		if v > highest {
			highest = v
		}
	}
	known := map[int64]bool{}
	for _, m := range migs {
		known[m.version] = true
		l := StatusLine{Version: m.version, Description: strings.TrimSuffix(m.name[5:], ".sql")}
		if sum, ok := applied[m.version]; ok {
			l.State = "applied"
			l.Checksum = "ok"
			if sum != m.checksum {
				l.Checksum = "mismatch"
			}
			rep.Applied++
		} else {
			l.State = "pending"
			l.Checksum = "-"
			if m.version < highest {
				l.Checksum = "hole"
			}
			rep.Pending++
		}
		rep.Lines = append(rep.Lines, l)
	}
	var extra []int64
	for v := range applied {
		if !known[v] {
			extra = append(extra, v)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	for _, v := range extra {
		rep.Lines = append(rep.Lines, StatusLine{Version: v, Description: versionPrefix.ReplaceAllString(names[v], ""), State: "applied", Checksum: "newer"})
		rep.Applied++
	}
	return rep, nil
}

// String is the stable, tab-separated `migrate status` line:
// version (4 digits), description, state, checksum state.
func (l StatusLine) String() string {
	return fmt.Sprintf("%04d\t%s\t%s\t%s", l.Version, l.Description, l.State, l.Checksum)
}

// Summary is the last line of `migrate status`.
func (r StatusReport) Summary() string {
	return fmt.Sprintf("%d applied, %d pending", r.Applied, r.Pending)
}
