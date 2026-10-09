package db_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"budgetcontrol/api"
	"budgetcontrol/db"
	"budgetcontrol/testutil"
)

// TestApplySchemaFreshDatabase applies the schema twice to an empty Postgres
// schema (a stand-in for a fresh database) and registers a user through the
// API, which used to fail with 500 when the tables were missing.
func TestApplySchemaFreshDatabase(t *testing.T) {
	admin := testutil.Pool(t) // skips when TEST_DATABASE_URL is unset
	ctx := context.Background()
	name := fmt.Sprintf("fresh_%d", os.Getpid())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+name+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
	})

	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	for i := 1; i <= 2; i++ { // second run: idempotent
		if err := db.ApplySchema(ctx, pool); err != nil {
			t.Fatalf("apply #%d: %v", i, err)
		}
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM categories WHERE user_id IS NULL`).Scan(&n); err != nil || n != 6 {
			t.Fatalf("apply #%d: default categories = %d, err %v; want 6", i, n, err)
		}
	}

	h := api.NewRouter(testutil.Deps(pool))
	req := httptest.NewRequest("POST", "/auth/register",
		strings.NewReader(`{"email":"fresh@test.example","password":"password123"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("register on fresh schema: %d %s", rec.Code, rec.Body)
	}
}
