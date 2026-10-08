// Package testutil provides shared helpers for database-backed tests.
package testutil

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"budgetcontrol/db"
)

// Pool connects to TEST_DATABASE_URL and applies the schema. It skips the
// test when the variable is unset. The pool is closed on cleanup.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.ApplySchema(ctx, pool); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return pool
}

var seq atomic.Int64

// UniqueEmail returns an email unique across tests and runs.
func UniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d-%d@test.example", prefix, time.Now().UnixNano(), seq.Add(1))
}

// CleanupUser deletes the user (and rows depending on it) with the given
// email, case-insensitively, when the test ends.
func CleanupUser(t *testing.T, pool *pgxpool.Pool, email string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, q := range []string{
			`DELETE FROM expenses WHERE user_id IN (SELECT id FROM users WHERE lower(email)=lower($1))`,
			`DELETE FROM budgets WHERE user_id IN (SELECT id FROM users WHERE lower(email)=lower($1))`,
			`DELETE FROM categories WHERE user_id IN (SELECT id FROM users WHERE lower(email)=lower($1))`,
			`DELETE FROM users WHERE lower(email)=lower($1)`,
		} {
			if _, err := pool.Exec(ctx, q, email); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
	})
}

// CreateUser inserts a user with the given email and returns its id; the row
// is removed on cleanup.
func CreateUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	CleanupUser(t, pool, email)
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id`, email).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}
