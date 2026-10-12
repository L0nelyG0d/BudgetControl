package db_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"budgetcontrol/db"
)

const (
	marker  = "-- migrate:no-transaction\n"
	cicStmt = "CREATE INDEX CONCURRENTLY idx_t_n ON t (n)"
)

// cicSet is a table, an index built concurrently, and a down file for it.
func cicSet() map[string]string {
	return map[string]string{
		"0001_a.sql":          "CREATE TABLE t (n INT)",
		"0002_idx.sql":        marker + cicStmt,
		"0002_idx.down.sql":   marker + "DROP INDEX CONCURRENTLY IF EXISTS idx_t_n",
		"0003_after.sql":      "CREATE TABLE after (n INT)",
		"0003_after.down.sql": "DROP TABLE after",
	}
}

func without(m map[string]string, keys ...string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

func with(m map[string]string, kv ...string) map[string]string {
	out := without(m)
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func mustApply(t *testing.T, pool *pgxpool.Pool, set map[string]string) {
	t.Helper()
	if err := db.ApplyMigrations(context.Background(), pool, files(set)); err != nil {
		t.Fatal(err)
	}
}

func indexValid(t *testing.T, pool *pgxpool.Pool, name string) (exists, valid bool) {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*), coalesce(bool_and(i.indisvalid), false) FROM pg_class c JOIN pg_index i ON i.indexrelid = c.oid
		 WHERE c.relname = $1 AND c.relnamespace = current_schema::regnamespace`, name).Scan(&n, &valid); err != nil {
		t.Fatal(err)
	}
	return n > 0, valid
}

func TestNoTransactionBaselineRejected(t *testing.T) {
	t.Parallel()
	err := db.ApplyMigrations(context.Background(), (*pgxpool.Pool)(nil), files(map[string]string{"0001_initial.sql": marker + "SELECT 1"}))
	if err == nil || !strings.Contains(err.Error(), "0001_initial.sql") {
		t.Fatalf("err = %v, want one naming 0001_initial.sql", err)
	}
}

func TestNoTransactionApplied(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	res, err := db.ApplyMigrationsReport(ctx, pool, files(cicSet()))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Applied, ","); got != "0001_a,0002_idx,0003_after" {
		t.Errorf("applied = %s", got)
	}
	if exists, valid := indexValid(t, pool, "idx_t_n"); !exists || !valid {
		t.Errorf("index exists %v valid %v", exists, valid)
	}
	if got := versions(t, pool); got != "1,2,3" {
		t.Errorf("versions = %s", got)
	}
	if n := count(t, pool, `SELECT count(*) FROM schema_migrations_in_progress`); n != 0 {
		t.Errorf("in-progress rows = %d", n)
	}
	// Second run: nothing to do.
	if res, err := db.ApplyMigrationsReport(ctx, pool, files(cicSet())); err != nil || len(res.Applied) != 0 {
		t.Errorf("second run = %+v, %v", res, err)
	}
}

func TestNoInProgressTableWithoutNonTransactional(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	mustApply(t, pool, downSet())
	if _, err := db.RevertMigration(ctx, pool, files(downSet())); err != nil {
		t.Fatal(err)
	}
	if rep, err := db.MigrationStatus(ctx, pool, files(downSet())); err != nil || len(rep.Problems) != 0 {
		t.Fatalf("status %v %v", rep.Problems, err)
	}
	if tableExists(t, pool, "schema_migrations_in_progress") {
		t.Error("in-progress table created without a non-transactional migration")
	}
}

func TestNoTransactionMarkerDetection(t *testing.T) {
	ctx := context.Background()
	t.Run("CRLF first line opts out", func(t *testing.T) {
		pool, _ := schemaPool(t)
		set := with(cicSet(), "0002_idx.sql", "-- migrate:no-transaction\r\n"+cicStmt)
		if err := db.ApplyMigrations(ctx, pool, files(set)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("marker on another line is a comment", func(t *testing.T) {
		pool, _ := schemaPool(t)
		set := with(cicSet(), "0002_idx.sql", "-- note\n"+marker+cicStmt)
		err := db.ApplyMigrations(ctx, pool, files(set))
		if err == nil || !strings.Contains(err.Error(), "0002_idx.sql") {
			t.Fatalf("err = %v, want failure (ran inside a transaction)", err)
		}
		if tableExists(t, pool, "schema_migrations_in_progress") {
			t.Error("in-progress table created")
		}
	})
	t.Run("marker with trailing text is a comment", func(t *testing.T) {
		pool, _ := schemaPool(t)
		set := with(cicSet(), "0002_idx.sql", "-- migrate:no-transaction please\n"+cicStmt)
		if err := db.ApplyMigrations(ctx, pool, files(set)); err == nil {
			t.Fatal("want error")
		}
	})
}

func TestNoTransactionChecksumCoversMarker(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	mustApply(t, pool, cicSet())
	for name, body := range map[string]string{"marker removed": cicStmt, "marker added to tx file": ""} {
		set := with(cicSet(), "0002_idx.sql", body)
		want := "0002_idx.sql"
		if body == "" {
			set = with(cicSet(), "0003_after.sql", marker+"CREATE TABLE after (n INT)")
			want = "0003_after.sql"
		}
		err := db.ApplyMigrations(ctx, pool, files(set))
		if err == nil || !strings.Contains(err.Error(), "modified after it was applied") || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestNoTransactionTwoStatementsFail(t *testing.T) {
	pool, _ := schemaPool(t)
	set := with(cicSet(), "0002_idx.sql", marker+cicStmt+"; SELECT 1")
	err := db.ApplyMigrations(context.Background(), pool, files(set))
	if err == nil || !strings.Contains(err.Error(), "0002_idx.sql") || !strings.Contains(err.Error(), "CONCURRENTLY") {
		t.Fatalf("err = %v", err)
	}
	if n := count(t, pool, `SELECT count(*) FROM schema_migrations_in_progress`); n != 1 {
		t.Errorf("in-progress rows = %d, want 1", n)
	}
}

func TestNoTransactionGroupsCommitSeparately(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	ins := "INSERT INTO log (tx) VALUES (txid_current())"
	set := map[string]string{
		"0001_a.sql": "CREATE TABLE log (id SERIAL, tx BIGINT)",
		"0002_b.sql": ins,
		"0003_c.sql": marker + ins,
		"0004_d.sql": ins,
	}
	res, err := db.ApplyMigrationsReport(ctx, pool, files(set))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Applied, ","); got != "0001_a,0002_b,0003_c,0004_d" {
		t.Errorf("applied order = %s", got)
	}
	rows, err := pool.Query(ctx, `SELECT tx FROM log ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var txs []int64
	for rows.Next() {
		var x int64
		rows.Scan(&x)
		txs = append(txs, x)
	}
	if len(txs) != 3 || txs[0] == txs[1] || txs[1] == txs[2] || txs[0] == txs[2] {
		t.Fatalf("transaction ids = %v, want three different ones in insert order", txs)
	}
	if !(txs[0] < txs[1] && txs[1] < txs[2]) {
		t.Errorf("transactions not ascending: %v", txs)
	}
}

func TestNoTransactionGroupsStayAppliedOnFailure(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	set := map[string]string{
		"0001_a.sql": "CREATE TABLE t (n INT)",
		"0002_b.sql": "INSERT INTO t VALUES (1), (1)",
		"0003_c.sql": marker + "CREATE UNIQUE INDEX CONCURRENTLY idx_t_n ON t (n)",
		"0004_d.sql": "CREATE TABLE d (n INT)",
	}
	res, err := db.ApplyMigrationsReport(ctx, pool, files(set))
	if err == nil {
		t.Fatal("want error")
	}
	if got := strings.Join(res.Applied, ","); got != "0001_a,0002_b" {
		t.Errorf("partial result = %q, want the committed group", got)
	}
	if !strings.Contains(err.Error(), "0003_c.sql") || !strings.Contains(err.Error(), "could not create unique index") {
		t.Errorf("err = %v", err)
	}
	if got := versions(t, pool); got != "1,2" {
		t.Errorf("versions = %s, want 1,2", got)
	}
	if exists, valid := indexValid(t, pool, "idx_t_n"); !exists || valid {
		t.Errorf("index exists %v valid %v, want an invalid leftover", exists, valid)
	}
	var ver int64
	var dir string
	if err := pool.QueryRow(ctx, `SELECT version, direction FROM schema_migrations_in_progress`).Scan(&ver, &dir); err != nil || ver != 3 || dir != "up" {
		t.Fatalf("in-progress row = %d %s, %v", ver, dir, err)
	}
	if tableExists(t, pool, "d") {
		t.Error("migration after the failed one ran")
	}

	// Every later entry point refuses, with the recovery statement.
	const del = "DELETE FROM schema_migrations_in_progress WHERE version = 3"
	check := func(what string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "did not finish") || !strings.Contains(err.Error(), del) || !strings.Contains(err.Error(), "0003_c.sql") {
			t.Errorf("%s: err = %v", what, err)
		}
	}
	res, err = db.ApplyMigrationsReport(ctx, pool, files(set))
	check("apply", err)
	if len(res.Applied) != 0 {
		t.Errorf("second run applied %v", res.Applied)
	}
	_, err = db.RevertMigration(ctx, pool, files(set))
	check("revert", err)
	rep, err := db.MigrationStatus(ctx, pool, files(set))
	if err != nil {
		t.Fatal(err)
	}
	if want := "0001\ta\tapplied\tok\n0002\tb\tapplied\tok\n0003\tc\tpending\tinterrupted\n0004\td\tpending\t-\n2 applied, 2 pending\n"; render(rep) != want {
		t.Errorf("status = %q, want %q", render(rep), want)
	}
	if len(rep.Problems) != 1 {
		t.Fatalf("problems = %v", rep.Problems)
	}
	check("status", rep.Problems[0])

	// Manual recovery, then the migration is attempted again from the start.
	for _, q := range []string{`DROP INDEX idx_t_n`, `DELETE FROM t`, del} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	res, err = db.ApplyMigrationsReport(ctx, pool, files(set))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Applied, ","); got != "0003_c,0004_d" {
		t.Errorf("after recovery applied = %s", got)
	}
	if exists, valid := indexValid(t, pool, "idx_t_n"); !exists || !valid {
		t.Errorf("index exists %v valid %v", exists, valid)
	}
	if n := count(t, pool, `SELECT count(*) FROM schema_migrations_in_progress`); n != 0 {
		t.Errorf("in-progress rows = %d", n)
	}
}

func TestNoTransactionInterruptedBlocksEarlierPending(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	set := map[string]string{"0001_a.sql": "CREATE TABLE a (n INT)", "0002_b.sql": "CREATE TABLE b (n INT)", "0003_c.sql": marker + "SELECT 1"}
	mustApply(t, pool, without(set, "0002_b.sql", "0003_c.sql"))
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations_in_progress (version BIGINT PRIMARY KEY, name TEXT NOT NULL, direction TEXT NOT NULL, started_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations_in_progress (version, name, direction) VALUES (3, '0003_c.sql', 'up')`); err != nil {
		t.Fatal(err)
	}
	err := db.ApplyMigrations(ctx, pool, files(set))
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("err = %v", err)
	}
	if tableExists(t, pool, "b") {
		t.Error("earlier pending transactional migration ran")
	}
}

func TestNoTransactionLegacyAdoptionInFirstGroup(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, oldSchema); err != nil {
		t.Fatal(err)
	}
	set := map[string]string{"0001_initial.sql": "SELECT 1", "0002_idx.sql": marker + "CREATE INDEX CONCURRENTLY idx_users_x ON users (id)"}
	res, err := db.ApplyMigrationsReport(ctx, pool, files(set))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Adopted || strings.Join(res.Applied, ",") != "0002_idx" {
		t.Errorf("res = %+v", res)
	}
	if got := versions(t, pool); got != "1,2" {
		t.Errorf("versions = %s", got)
	}
}

// waitFor polls q (which must return a bool) on pool until it is true.
func waitFor(t *testing.T, pool *pgxpool.Pool, q string) {
	t.Helper()
	for i := 0; i < 1000; i++ {
		var ok bool
		if err := pool.QueryRow(context.Background(), q).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		if ok {
			return
		}
		pool.Exec(context.Background(), "SELECT pg_sleep(0.02)")
	}
	t.Fatalf("condition never became true: %s", q)
}

func TestNoTransactionHoldsSessionLock(t *testing.T) {
	pool, admin := schemaPool(t)
	ctx := context.Background()
	const gate = 990033
	// The test holds the gate; the migration blocks on it while running.
	gc, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer gc.Release()
	if _, err := gc.Exec(ctx, `SELECT pg_advisory_lock($1)`, gate); err != nil {
		t.Fatal(err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			gc.Exec(ctx, `SELECT pg_advisory_unlock($1)`, gate)
		}
	}
	defer release()
	set := map[string]string{
		"0001_a.sql":    "CREATE TABLE runs (n INT)",
		"0002_wait.sql": marker + "DO $$ BEGIN PERFORM pg_advisory_lock(990033); PERFORM pg_advisory_unlock(990033); INSERT INTO runs VALUES (1); END $$",
	}
	errs := make(chan error, 2)
	go func() { errs <- db.ApplyMigrations(ctx, pool, files(set)) }()
	// The session lock is granted to the running migration (its statement is
	// waiting on the gate, and the transaction-level lock is long gone).
	waitFor(t, admin, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND objid = 727001 AND granted)
	  AND EXISTS (SELECT 1 FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE '%990033%' AND query NOT LIKE '%pg_stat_activity%')`)
	go func() { errs <- db.ApplyMigrations(ctx, pool, files(set)) }()
	// The second caller is polling for the lock, not holding a transaction.
	waitFor(t, admin, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE query LIKE '%pg_try_advisory_lock%' AND query NOT LIKE '%pg_stat_activity%')`)
	select {
	case err := <-errs:
		t.Fatalf("a call returned while the lock was held: %v", err)
	default:
	}
	release()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("apply: %v", err)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM runs`); n != 1 {
		t.Errorf("migration ran %d times, want 1", n)
	}
	if got := versions(t, pool); got != "1,2" {
		t.Errorf("versions = %s", got)
	}
	// Both calls finished: the lock is free again.
	if n := count(t, admin, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND objid = 727001`); n != 0 {
		t.Errorf("%d advisory lock entries left", n)
	}
}

func TestNoTransactionLockReleasedOnFailure(t *testing.T) {
	pool, admin := schemaPool(t)
	ctx := context.Background()
	set := map[string]string{"0001_a.sql": "SELECT 1", "0002_bad.sql": marker + "SELECT * FROM no_such_table"}
	if err := db.ApplyMigrations(ctx, pool, files(set)); err == nil {
		t.Fatal("want error")
	}
	if n := count(t, admin, `SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND objid = 727001`); n != 0 {
		t.Errorf("%d advisory lock entries left after failure", n)
	}
}

func TestNoTransactionConcurrent(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	set := files(cicSet())
	const callers = 6
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- db.ApplyMigrations(ctx, pool, set)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("apply: %v", err)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM schema_migrations`); n != 3 {
		t.Errorf("schema_migrations rows = %d, want 3", n)
	}
	if got := versions(t, pool); got != "1,2,3" {
		t.Errorf("versions = %s", got)
	}
}

func TestNoTransactionRevert(t *testing.T) {
	ctx := context.Background()
	t.Run("non-transactional down", func(t *testing.T) {
		pool, _ := schemaPool(t)
		set := without(cicSet(), "0003_after.sql", "0003_after.down.sql")
		mustApply(t, pool, set)
		name, err := db.RevertMigration(ctx, pool, files(set))
		if err != nil || name != "0002_idx" {
			t.Fatalf("got %q, %v", name, err)
		}
		if exists, _ := indexValid(t, pool, "idx_t_n"); exists {
			t.Error("index still exists")
		}
		if got := versions(t, pool); got != "1" {
			t.Errorf("versions = %s", got)
		}
		if n := count(t, pool, `SELECT count(*) FROM schema_migrations_in_progress`); n != 0 {
			t.Errorf("in-progress rows = %d", n)
		}
		// Up again after the revert.
		mustApply(t, pool, set)
		if exists, valid := indexValid(t, pool, "idx_t_n"); !exists || !valid {
			t.Errorf("index exists %v valid %v", exists, valid)
		}
	})
	t.Run("down file decides: transactional down for non-transactional up", func(t *testing.T) {
		pool, _ := schemaPool(t)
		set := with(without(cicSet(), "0003_after.sql", "0003_after.down.sql"), "0002_idx.down.sql", "DROP INDEX idx_t_n")
		mustApply(t, pool, set)
		if name, err := db.RevertMigration(ctx, pool, files(set)); err != nil || name != "0002_idx" {
			t.Fatalf("got %q, %v", name, err)
		}
		if exists, _ := indexValid(t, pool, "idx_t_n"); exists {
			t.Error("index still exists")
		}
	})
	t.Run("down file decides: non-transactional down for transactional up", func(t *testing.T) {
		pool, _ := schemaPool(t)
		set := map[string]string{
			"0001_a.sql": "CREATE TABLE t (n INT)", "0002_idx.sql": "CREATE INDEX idx_t_n ON t (n)",
			"0002_idx.down.sql": marker + "DROP INDEX CONCURRENTLY idx_t_n",
		}
		mustApply(t, pool, set)
		if name, err := db.RevertMigration(ctx, pool, files(set)); err != nil || name != "0002_idx" {
			t.Fatalf("got %q, %v", name, err)
		}
		if exists, _ := indexValid(t, pool, "idx_t_n"); exists {
			t.Error("index still exists")
		}
	})
	t.Run("failure leaves rows and blocks the next run", func(t *testing.T) {
		pool, _ := schemaPool(t)
		set := with(without(cicSet(), "0003_after.sql", "0003_after.down.sql"), "0002_idx.down.sql", marker+"DROP INDEX CONCURRENTLY no_such_index")
		mustApply(t, pool, set)
		name, err := db.RevertMigration(ctx, pool, files(set))
		if err == nil || name != "" || !strings.Contains(err.Error(), "0002_idx.down.sql") {
			t.Fatalf("got %q, %v", name, err)
		}
		if got := versions(t, pool); got != "1,2" {
			t.Errorf("versions = %s, want the row kept", got)
		}
		var dir string
		if err := pool.QueryRow(ctx, `SELECT direction FROM schema_migrations_in_progress WHERE version = 2`).Scan(&dir); err != nil || dir != "down" {
			t.Fatalf("in-progress = %q, %v", dir, err)
		}
		_, err = db.RevertMigration(ctx, pool, files(set))
		if err == nil || !strings.Contains(err.Error(), "did not finish") || !strings.Contains(err.Error(), "WHERE version = 2") {
			t.Errorf("revert again: %v", err)
		}
		if err := db.ApplyMigrations(ctx, pool, files(set)); err == nil || !strings.Contains(err.Error(), "did not finish") {
			t.Errorf("apply: %v", err)
		}
		rep, err := db.MigrationStatus(ctx, pool, files(set))
		if err != nil || len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0].Error(), "did not finish") {
			t.Errorf("status problems = %v, %v", rep.Problems, err)
		}
	})
	t.Run("refusals happen before anything runs", func(t *testing.T) {
		pool, _ := schemaPool(t)
		set := without(cicSet(), "0002_idx.down.sql", "0003_after.sql", "0003_after.down.sql")
		mustApply(t, pool, set)
		if _, err := db.RevertMigration(ctx, pool, files(set)); err == nil || !strings.Contains(err.Error(), "no down migration") {
			t.Errorf("err = %v", err)
		}
		if n := count(t, pool, `SELECT count(*) FROM schema_migrations_in_progress`); n != 0 {
			t.Errorf("in-progress rows = %d", n)
		}
	})
}

func TestStatusWithoutInProgressTableCreatesNothing(t *testing.T) {
	pool, _ := schemaPool(t)
	ctx := context.Background()
	mustApply(t, pool, downSet())
	if rep, err := db.MigrationStatus(ctx, pool, files(downSet())); err != nil || len(rep.Problems) != 0 {
		t.Fatalf("%v %v", rep.Problems, err)
	}
	if tableExists(t, pool, "schema_migrations_in_progress") {
		t.Error("status created the in-progress table")
	}
}
