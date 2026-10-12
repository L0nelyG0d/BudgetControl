package db_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"budgetcontrol/db"
)

// render is the stdout of `migrate status` as db produces it.
func render(r db.StatusReport) string {
	var b strings.Builder
	for _, l := range r.Lines {
		b.WriteString(l.String() + "\n")
	}
	b.WriteString(r.Summary() + "\n")
	return b.String()
}

func problemText(r db.StatusReport) string {
	var parts []string
	for _, e := range r.Problems {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, "\n")
}

func TestMigrationStatus(t *testing.T) {
	ctx := context.Background()
	set := map[string]string{
		"0001_a.sql":     "CREATE TABLE a (n INT)",
		"0002_add_b.sql": "CREATE TABLE b (n INT)",
		"0003_c.sql":     "CREATE TABLE c (n INT)",
	}
	two := map[string]string{"0001_a.sql": set["0001_a.sql"], "0002_add_b.sql": set["0002_add_b.sql"]}

	tests := []struct {
		name     string
		applySet map[string]string // applied first (nil = nothing)
		mutate   []string          // SQL run after applying
		binary   map[string]string
		want     string
		problems []string
	}{
		{name: "all applied", applySet: set, binary: set,
			want: "0001\ta\tapplied\tok\n0002\tadd_b\tapplied\tok\n0003\tc\tapplied\tok\n3 applied, 0 pending\n"},
		{name: "some pending", applySet: two, binary: set,
			want: "0001\ta\tapplied\tok\n0002\tadd_b\tapplied\tok\n0003\tc\tpending\t-\n2 applied, 1 pending\n"},
		{name: "table exists with no rows", applySet: map[string]string{"0001_a.sql": "SELECT 1"}, mutate: []string{"DELETE FROM schema_migrations"}, binary: set,
			want: "0001\ta\tpending\t-\n0002\tadd_b\tpending\t-\n0003\tc\tpending\t-\n0 applied, 3 pending\n"},
		{name: "mismatch", applySet: set, binary: map[string]string{"0001_a.sql": "CREATE TABLE a (n INT, x INT)", "0002_add_b.sql": set["0002_add_b.sql"], "0003_c.sql": set["0003_c.sql"]},
			want:     "0001\ta\tapplied\tmismatch\n0002\tadd_b\tapplied\tok\n0003\tc\tapplied\tok\n3 applied, 0 pending\n",
			problems: []string{"migration 0001_a.sql was modified after it was applied"}},
		{name: "newer", applySet: two, mutate: []string{"INSERT INTO schema_migrations (version, name, checksum) VALUES (9, '0009_future_thing', 'x')"}, binary: two,
			want:     "0001\ta\tapplied\tok\n0002\tadd_b\tapplied\tok\n0009\tfuture_thing\tapplied\tnewer\n3 applied, 0 pending\n",
			problems: []string{"database has migration version 9, which is newer than this binary"}},
		{name: "hole", applySet: set, mutate: []string{"DELETE FROM schema_migrations WHERE version = 2"}, binary: set,
			want:     "0001\ta\tapplied\tok\n0002\tadd_b\tpending\thole\n0003\tc\tapplied\tok\n2 applied, 1 pending\n",
			problems: []string{"migration 0002_add_b.sql (version 2) is not applied but version 3 is: hole in applied migrations"}},
		{name: "hole below a newer version", applySet: two, mutate: []string{"DELETE FROM schema_migrations WHERE version = 1", "INSERT INTO schema_migrations (version, name, checksum) VALUES (5, '0005_z', 'x')"}, binary: two,
			want:     "0001\ta\tpending\thole\n0002\tadd_b\tapplied\tok\n0005\tz\tapplied\tnewer\n2 applied, 1 pending\n",
			problems: []string{"hole in applied migrations", "newer than this binary"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, _ := schemaPool(t)
			if err := db.ApplyMigrations(ctx, pool, files(tc.applySet)); err != nil {
				t.Fatal(err)
			}
			for _, q := range tc.mutate {
				if _, err := pool.Exec(ctx, q); err != nil {
					t.Fatal(err)
				}
			}
			before := versions(t, pool)
			rep, err := db.MigrationStatus(ctx, pool, files(tc.binary))
			if err != nil {
				t.Fatal(err)
			}
			if got := render(rep); got != tc.want {
				t.Errorf("status:\n%q\nwant:\n%q", got, tc.want)
			}
			if again, _ := db.MigrationStatus(ctx, pool, files(tc.binary)); render(again) != tc.want {
				t.Error("second run differs")
			}
			if len(rep.Problems) != len(tc.problems) {
				t.Errorf("problems = %q, want %d", problemText(rep), len(tc.problems))
			}
			for _, w := range tc.problems {
				if !strings.Contains(problemText(rep), w) {
					t.Errorf("problems %q lack %q", problemText(rep), w)
				}
			}
			if got := versions(t, pool); got != before {
				t.Errorf("status changed schema_migrations: %q -> %q", before, got)
			}
		})
	}
}

func TestMigrationStatusEmptyDatabaseCreatesNothing(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	set := files(map[string]string{"0001_a.sql": "SELECT 1", "0002_b.sql": "SELECT 1"})
	rep, err := db.MigrationStatus(ctx, pool, set)
	if err != nil {
		t.Fatal(err)
	}
	if want := "0001\ta\tpending\t-\n0002\tb\tpending\t-\n0 applied, 2 pending\n"; render(rep) != want {
		t.Errorf("got %q", render(rep))
	}
	if rep.Legacy || len(rep.Problems) != 0 {
		t.Errorf("legacy %v problems %v", rep.Legacy, rep.Problems)
	}
	if tableExists(t, pool, "schema_migrations") {
		t.Error("status created schema_migrations")
	}
	// Nothing at all in the set: still fine.
	if rep, err := db.MigrationStatus(ctx, pool, files(nil)); err != nil || render(rep) != "0 applied, 0 pending\n" {
		t.Errorf("empty set: %q, %v", render(rep), err)
	}
}

func TestMigrationStatusLegacyDatabaseUntouched(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, oldSchema); err != nil {
		t.Fatal(err)
	}
	rep, err := db.Status(ctx, pool) // embedded migrations
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Legacy || rep.Applied != 0 || rep.Pending < 1 || len(rep.Problems) != 0 {
		t.Errorf("report = %+v", rep)
	}
	if !strings.HasPrefix(render(rep), "0001\tinitial\tpending\t-\n") {
		t.Errorf("got %q", render(rep))
	}
	if tableExists(t, pool, "schema_migrations") {
		t.Error("status adopted the legacy database")
	}
}

// TestMigrationStatusIsReadOnlyAndLockFree holds the advisory lock in another
// transaction: status must neither block nor write.
func TestMigrationStatusIsReadOnlyAndLockFree(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	if err := db.ApplyMigrations(ctx, pool, files(map[string]string{"0001_a.sql": "SELECT 1"})); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(727001)"); err != nil {
		t.Fatal(err)
	}
	rep, err := db.MigrationStatus(ctx, pool, files(map[string]string{"0001_a.sql": "SELECT 1"}))
	if err != nil || rep.Applied != 1 {
		t.Fatalf("status under lock: %+v, %v", rep, err)
	}
}

func TestApplyMigrationsReport(t *testing.T) {
	ctx := context.Background()
	set := map[string]string{"0001_a.sql": "CREATE TABLE a (n INT)", "0002_b.sql": "CREATE TABLE b (n INT)", "0003_c.sql": "CREATE TABLE c (n INT)"}

	t.Run("pending in order then nothing", func(t *testing.T) {
		pool, _ := schemaPool(t)
		res, err := db.ApplyMigrationsReport(ctx, pool, files(set))
		if err != nil {
			t.Fatal(err)
		}
		if res.Adopted || strings.Join(res.Applied, ",") != "0001_a,0002_b,0003_c" {
			t.Errorf("res = %+v", res)
		}
		res, err = db.ApplyMigrationsReport(ctx, pool, files(set))
		if err != nil || res.Adopted || len(res.Applied) != 0 {
			t.Errorf("second run = %+v, %v", res, err)
		}
	})
	t.Run("legacy adopts 0001 then applies the rest", func(t *testing.T) {
		pool, _ := schemaPool(t)
		if _, err := pool.Exec(ctx, "CREATE TABLE users (id INT)"); err != nil {
			t.Fatal(err)
		}
		res, err := db.ApplyMigrationsReport(ctx, pool, files(set))
		if err != nil {
			t.Fatal(err)
		}
		if !res.Adopted || strings.Join(res.Applied, ",") != "0002_b,0003_c" {
			t.Errorf("res = %+v", res)
		}
		if tableExists(t, pool, "a") {
			t.Error("adopted migration was run")
		}
	})
	t.Run("legacy with only 0001", func(t *testing.T) {
		pool, _ := schemaPool(t)
		if _, err := pool.Exec(ctx, "CREATE TABLE users (id INT)"); err != nil {
			t.Fatal(err)
		}
		res, err := db.ApplyMigrationsReport(ctx, pool, files(map[string]string{"0001_a.sql": "CREATE TABLE a (n INT)"}))
		if err != nil || !res.Adopted || len(res.Applied) != 0 {
			t.Errorf("res = %+v, %v", res, err)
		}
	})
	t.Run("failure reports nothing and applies nothing", func(t *testing.T) {
		pool, _ := schemaPool(t)
		res, err := db.ApplyMigrationsReport(ctx, pool, files(map[string]string{
			"0001_a.sql": "CREATE TABLE a (n INT)", "0002_bad.sql": "SELECT * FROM nope"}))
		if err == nil || !strings.Contains(err.Error(), "0002_bad.sql") {
			t.Fatalf("err = %v", err)
		}
		if res.Adopted || len(res.Applied) != 0 {
			t.Errorf("res = %+v", res)
		}
		if tableExists(t, pool, "a") {
			t.Error("first migration survived the failure")
		}
	})
}

func TestApplyMigrationsReportConcurrent(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	fs := files(map[string]string{"0001_a.sql": "CREATE TABLE a (n INT)", "0002_b.sql": "CREATE TABLE b (n INT)"})
	const callers = 4
	results := make(chan db.ApplyResult, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := db.ApplyMigrationsReport(ctx, pool, fs)
			results <- r
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("up: %v", err)
		}
	}
	appliedRuns := 0
	for r := range results {
		if len(r.Applied) > 0 {
			appliedRuns++
			if len(r.Applied) != 2 {
				t.Errorf("applied = %v", r.Applied)
			}
		}
	}
	if appliedRuns != 1 {
		t.Errorf("%d runs applied migrations, want exactly 1", appliedRuns)
	}
}

func TestUpAfterDownThenStatusOk(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	fs := files(downSet())
	if err := db.ApplyMigrations(ctx, pool, fs); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RevertMigration(ctx, pool, fs); err != nil {
		t.Fatal(err)
	}
	res, err := db.ApplyMigrationsReport(ctx, pool, fs)
	if err != nil || strings.Join(res.Applied, ",") != "0003_c" {
		t.Fatalf("res = %+v, %v", res, err)
	}
	rep, err := db.MigrationStatus(ctx, pool, fs)
	if err != nil || len(rep.Problems) != 0 || rep.Lines[2].String() != "0003\tc\tapplied\tok" {
		t.Errorf("status = %q, %v", render(rep), err)
	}
}

func TestStatusIgnoresDownFiles(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	if err := db.ApplyMigrations(ctx, pool, files(downSet())); err != nil {
		t.Fatal(err)
	}
	other := downSet()
	other["0003_c.down.sql"] = "DROP TABLE changed"
	rep, err := db.MigrationStatus(ctx, pool, files(other))
	if err != nil || len(rep.Problems) != 0 || rep.Applied != 3 {
		t.Errorf("status = %q, %v", render(rep), err)
	}
}
