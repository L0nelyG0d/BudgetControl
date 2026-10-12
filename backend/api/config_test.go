package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"budgetcontrol/api"
	"budgetcontrol/testutil"
)

func TestConfig(t *testing.T) {
	t.Parallel()
	// No pool and no cookie: the endpoint is public and must not read the database.
	h := api.NewRouter(api.Deps{JWTSecret: testutil.TestSecret})
	rec := do(h, "GET", "/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type = %q", ct)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body %q: %v", rec.Body, err)
	}
	if len(got) != 1 || string(got["maxAmount"]) != "1000000000000" {
		t.Errorf("body = %s, want exactly {\"maxAmount\":1000000000000}", rec.Body)
	}
}

func TestConfigOtherMethods(t *testing.T) {
	t.Parallel()
	h := api.NewRouter(api.Deps{JWTSecret: testutil.TestSecret})
	for _, m := range []string{"POST", "PUT", "DELETE"} {
		t.Run(m, func(t *testing.T) {
			if rec := do(h, m, "/config", ""); rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", rec.Code)
			}
		})
	}
}

// The value /config publishes is the limit POST /expenses enforces.
func TestConfigMatchesEnforcedLimit(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	_, _, c := testutil.NewUser(t, pool)
	def := defaultCategory(t, pool)

	var cfg struct {
		MaxAmount int64 `json:"maxAmount"`
	}
	if err := json.Unmarshal(do(h, "GET", "/config", "").Body.Bytes(), &cfg); err != nil || cfg.MaxAmount <= 0 {
		t.Fatalf("config: %+v, %v", cfg, err)
	}
	if rec := do(h, "POST", "/expenses", expBody(cfg.MaxAmount, def, "2026-10-08", ""), c); rec.Code != 201 {
		t.Errorf("amount = maxAmount: %d %s", rec.Code, rec.Body)
	}
	rec := do(h, "POST", "/expenses", expBody(cfg.MaxAmount+1, def, "2026-10-08", ""), c)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "at most") {
		t.Errorf("amount = maxAmount+1: %d %s", rec.Code, rec.Body)
	}
}
