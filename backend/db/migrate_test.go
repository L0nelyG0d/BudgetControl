package db_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"

	"budgetcontrol/db"
)

func files(m map[string]string) fstest.MapFS {
	fs := fstest.MapFS{}
	for k, v := range m {
		fs[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return fs
}

// TestApplyMigrationsInvalidSet needs no database: the set is validated
// before anything is sent, so a nil pool is never touched.
func TestApplyMigrationsInvalidSet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fs      fstest.MapFS
		wantErr []string
	}{
		{"gap", files(map[string]string{"0001_a.sql": "SELECT 1", "0003_c.sql": "SELECT 1"}), []string{"0003_c.sql", "gap"}},
		{"does not start at 1", files(map[string]string{"0002_b.sql": "SELECT 1"}), []string{"0002_b.sql", "gap"}},
		{"duplicate", files(map[string]string{"0001_a.sql": "SELECT 1", "0001_b.sql": "SELECT 1"}), []string{"0001_a.sql", "0001_b.sql", "duplicate"}},
		{"uppercase description", files(map[string]string{"0001_Init.sql": "SELECT 1"}), []string{"0001_Init.sql"}},
		{"three digits", files(map[string]string{"001_init.sql": "SELECT 1"}), []string{"001_init.sql"}},
		{"no description", files(map[string]string{"0001.sql": "SELECT 1"}), []string{"0001.sql"}},
		{"wrong extension", files(map[string]string{"0001_init.txt": "SELECT 1"}), []string{"0001_init.txt"}},
		{"directory", files(map[string]string{"0001_init.sql/x": "SELECT 1"}), []string{"0001_init.sql"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := db.ApplyMigrations(context.Background(), (*pgxpool.Pool)(nil), tc.fs)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			for _, w := range tc.wantErr {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestApplyMigrationsOrderAndRecording(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	// Version order, not name order: 0002_a runs after 0001_z.
	fs := files(map[string]string{
		"0002_a.sql": "INSERT INTO t VALUES (2)",
		"0001_z.sql": "CREATE TABLE t (n INT)",
		"0003_m.sql": "INSERT INTO t VALUES (3)",
	})
	if err := db.ApplyMigrations(ctx, pool, fs); err != nil {
		t.Fatal(err)
	}
	var seq string
	if err := pool.QueryRow(ctx, `SELECT string_agg(version::text, ',' ORDER BY version) FROM schema_migrations`).Scan(&seq); err != nil || seq != "1,2,3" {
		t.Fatalf("versions = %q, err %v", seq, err)
	}
	sum := sha256.Sum256([]byte("CREATE TABLE t (n INT)"))
	var name, checksum string
	if err := pool.QueryRow(ctx, `SELECT name, checksum FROM schema_migrations WHERE version = 1`).Scan(&name, &checksum); err != nil {
		t.Fatal(err)
	}
	if name != "0001_z" || checksum != hex.EncodeToString(sum[:]) {
		t.Errorf("recorded name %q checksum %q", name, checksum)
	}

	// Second run executes nothing and changes nothing.
	var before string
	q := `SELECT string_agg(version||name||checksum||applied_at::text, ';' ORDER BY version) FROM schema_migrations`
	pool.QueryRow(ctx, q).Scan(&before)
	if err := db.ApplyMigrations(ctx, pool, fs); err != nil {
		t.Fatal(err)
	}
	var after string
	pool.QueryRow(ctx, q).Scan(&after)
	if before != after {
		t.Errorf("schema_migrations changed on second run:\n%s\n%s", before, after)
	}
	if n := count(t, pool, `SELECT count(*) FROM t`); n != 2 {
		t.Errorf("rows in t = %d, want 2 (migrations re-executed?)", n)
	}
}

func TestApplyMigrationsFailureRollsBackEverything(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	err := db.ApplyMigrations(ctx, pool, files(map[string]string{
		"0001_ok.sql":  "CREATE TABLE first_ok (n INT)",
		"0002_bad.sql": "CREATE TABLE half (n INT); INSERT INTO no_such_table VALUES (1)",
	}))
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "0002_bad.sql") || !strings.Contains(err.Error(), "no_such_table") {
		t.Errorf("error %q lacks file name or database error", err)
	}
	for _, tbl := range []string{"first_ok", "half", "schema_migrations"} {
		if n := count(t, pool, `SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename = $1`, tbl); n != 0 {
			t.Errorf("table %s exists after failed run", tbl)
		}
	}
}

func TestApplyMigrationsRejectsInconsistentDatabase(t *testing.T) {
	base := map[string]string{
		"0001_a.sql": "CREATE TABLE a (n INT)",
		"0002_b.sql": "CREATE TABLE b (n INT)",
		"0003_c.sql": "CREATE TABLE c (n INT)",
	}
	tests := []struct {
		name    string
		prepare func(t *testing.T, pool *pgxpool.Pool)
		fs      map[string]string
		want    string
		absent  string // table that must not have been created
	}{
		{"modified applied migration", nil,
			map[string]string{"0001_a.sql": "CREATE TABLE a (n INT, extra INT)", "0002_b.sql": base["0002_b.sql"], "0003_c.sql": base["0003_c.sql"]},
			"0001_a.sql", "c"},
		{"database newer than binary", func(t *testing.T, pool *pgxpool.Pool) {
			if _, err := pool.Exec(context.Background(), `INSERT INTO schema_migrations (version, name, checksum) VALUES (9, '0009_future', 'x')`); err != nil {
				t.Fatal(err)
			}
		}, nil, "version 9", ""},
		{"hole below highest applied", func(t *testing.T, pool *pgxpool.Pool) {
			if _, err := pool.Exec(context.Background(), `DELETE FROM schema_migrations WHERE version = 2`); err != nil {
				t.Fatal(err)
			}
		}, nil, "0002_b.sql", "b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, _ := schemaPool(t)
			ctx := context.Background()
			if err := db.ApplyMigrations(ctx, pool, files(map[string]string{"0001_a.sql": base["0001_a.sql"], "0002_b.sql": base["0002_b.sql"]})); err != nil {
				t.Fatal(err)
			}
			if tc.prepare != nil {
				tc.prepare(t, pool)
			}
			fs := tc.fs
			if fs == nil {
				fs = base
			}
			err := db.ApplyMigrations(ctx, pool, files(fs))
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err, tc.want)
			}
			switch tc.name {
			case "modified applied migration":
				if !strings.Contains(err.Error(), "modified after it was applied") {
					t.Errorf("error %q", err)
				}
			case "database newer than binary":
				if !strings.Contains(err.Error(), "newer than this binary") {
					t.Errorf("error %q", err)
				}
			}
			// Executes nothing: the pending migration 3 was not run.
			if n := count(t, pool, `SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename = 'c'`); n != 0 {
				t.Error("pending migration was executed despite the error")
			}
		})
	}
}

// oldSchema is the original schema.sql text, kept as a fixture: databases
// created before migrations existed were built from it.
const oldSchema = `
CREATE TABLE IF NOT EXISTS users (
    id            BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_key ON users (lower(email));
CREATE TABLE IF NOT EXISTS categories (
    id         BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id    BIGINT REFERENCES users (id),
    name       TEXT NOT NULL,
    color      TEXT NOT NULL CHECK (color ~ '^#[0-9A-Fa-f]{6}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS categories_user_name_key
    ON categories (user_id, lower(name)) NULLS NOT DISTINCT;
CREATE TABLE IF NOT EXISTS expenses (
    id          BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users (id),
    category_id BIGINT      NOT NULL REFERENCES categories (id),
    amount      BIGINT      NOT NULL CHECK (amount > 0),
    date        DATE        NOT NULL,
    note        TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS expenses_user_date_idx ON expenses (user_id, date);
CREATE TABLE IF NOT EXISTS budgets (
    id          BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users (id),
    category_id BIGINT      REFERENCES categories (id) ON DELETE CASCADE,
    amount      BIGINT      NOT NULL CHECK (amount > 0),
    month       DATE        NOT NULL CHECK (month = date_trunc('month', month)::date),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS budgets_user_category_month_key
    ON budgets (user_id, category_id, month) NULLS NOT DISTINCT;
INSERT INTO categories (user_id, name, color) VALUES
    (NULL, 'Food',          '#F59E0B'),
    (NULL, 'Transport',     '#3B82F6'),
    (NULL, 'Housing',       '#10B981'),
    (NULL, 'Entertainment', '#8B5CF6'),
    (NULL, 'Health',        '#EF4444'),
    (NULL, 'Other',         '#6B7280')
ON CONFLICT DO NOTHING;
`

func TestApplySchemaAdoptsExistingDatabase(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	for _, q := range []string{
		oldSchema,
		`INSERT INTO users (email, password_hash) VALUES ('a@x.example', 'h')`,
		`INSERT INTO categories (user_id, name, color) SELECT id, 'Pets', '#112233' FROM users`,
		`INSERT INTO expenses (user_id, category_id, amount, date, note) SELECT u.id, c.id, 500, '2026-01-02', 'n' FROM users u, categories c WHERE c.name = 'Pets'`,
		`INSERT INTO budgets (user_id, category_id, amount, month) SELECT id, NULL, 9000, '2026-01-01' FROM users`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func() string {
		var s string
		err := pool.QueryRow(ctx, `SELECT
		  (SELECT coalesce(string_agg(u::text, '|' ORDER BY id), '') FROM users u) || '#' ||
		  (SELECT coalesce(string_agg(c::text, '|' ORDER BY id), '') FROM categories c) || '#' ||
		  (SELECT coalesce(string_agg(e::text, '|' ORDER BY id), '') FROM expenses e) || '#' ||
		  (SELECT coalesce(string_agg(b::text, '|' ORDER BY id), '') FROM budgets b)`).Scan(&s)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	before := snapshot()

	var migrated string
	for i := 1; i <= 2; i++ {
		if err := db.ApplySchema(ctx, pool); err != nil {
			t.Fatalf("apply #%d: %v", i, err)
		}
		if got := snapshot(); got != before {
			t.Fatalf("apply #%d changed data:\nbefore %s\nafter  %s", i, before, got)
		}
		if n := count(t, pool, `SELECT count(*) FROM schema_migrations`); n != 1 {
			t.Fatalf("apply #%d: schema_migrations rows = %d, want 1", i, n)
		}
		var v int
		var at string
		pool.QueryRow(ctx, `SELECT version, applied_at::text FROM schema_migrations`).Scan(&v, &at)
		if v != 1 {
			t.Errorf("recorded version %d, want 1", v)
		}
		if i == 1 {
			migrated = at
		} else if at != migrated {
			t.Errorf("applied_at changed on second run")
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM categories WHERE user_id IS NULL`); n != 6 {
		t.Errorf("default categories = %d, want 6", n)
	}
}

func TestApplySchemaConcurrent(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	const callers = 6
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- db.ApplySchema(ctx, pool)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent ApplySchema: %v", err)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM schema_migrations WHERE version = 1`); n != 1 {
		t.Errorf("rows for version 1 = %d, want 1", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM categories WHERE user_id IS NULL`); n != 6 {
		t.Errorf("default categories = %d, want 6", n)
	}
}

func TestApplySchemaFreshMatchesOldSchema(t *testing.T) {
	fresh, _ := schemaPool(t)
	old, _ := schemaPool(t)
	ctx := context.Background()
	if err := db.ApplySchema(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(ctx, oldSchema); err != nil {
		t.Fatal(err)
	}
	q := `SELECT coalesce(string_agg(x, E'\n' ORDER BY x), '') FROM (
	  SELECT 'col ' || table_name || '.' || column_name || ' ' || data_type || ' ' || is_nullable || ' ' || coalesce(column_default, '') || ' ' || is_identity AS x
	    FROM information_schema.columns WHERE table_schema = current_schema() AND table_name <> 'schema_migrations'
	  UNION ALL SELECT 'idx ' || indexdef FROM (SELECT regexp_replace(indexdef, 'ON [a-z_0-9]+\.', 'ON ') AS indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename <> 'schema_migrations') i
	  UNION ALL SELECT 'con ' || conrelid::regclass::text || ' ' || pg_get_constraintdef(oid) FROM pg_constraint WHERE connamespace = current_schema()::regnamespace AND conrelid::regclass::text <> 'schema_migrations'
	  UNION ALL SELECT 'cat ' || name || color FROM categories
	) s`
	var a, b string
	if err := fresh.QueryRow(ctx, q).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := old.QueryRow(ctx, q).Scan(&b); err != nil {
		t.Fatal(err)
	}
	if a != b || a == "" {
		t.Errorf("fresh schema differs from old schema.sql:\n--- fresh\n%s\n--- old\n%s", a, b)
	}
}
