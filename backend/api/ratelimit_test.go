package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"budgetcontrol/api"
	"budgetcontrol/testutil"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func loginFrom(h http.Handler, ip, email, pw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/auth/login",
		strings.NewReader(`{"email":"`+email+`","password":"`+pw+`"}`))
	req.RemoteAddr = ip + ":4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.99") // must be ignored
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newLimitedRouter(t *testing.T) (http.Handler, *fakeClock, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.Pool(t)
	clock := &fakeClock{t: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
	deps := testutil.Deps(pool)
	deps.Now = clock.Now
	return api.NewRouter(deps), clock, pool
}

func TestLoginRateLimitByEmail(t *testing.T) {
	h, clock, pool := newLimitedRouter(t)
	email := testutil.UniqueEmail("rl")
	testutil.CleanupUser(t, pool, email)
	registerUser(t, h, nil, email, "password123")

	for i := 0; i < 5; i++ {
		// Different IP each time so only the email counter fills up.
		rec := loginFrom(h, "198.51.100."+string(rune('1'+i)), email, "wrong")
		if rec.Code != 401 {
			t.Fatalf("attempt %d: got %d, want 401", i+1, rec.Code)
		}
	}
	// Blocked even with the right password.
	rec := loginFrom(h, "198.51.100.50", email, "password123")
	if rec.Code != 429 {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "900" {
		t.Errorf("Retry-After = %q, want 900", got)
	}

	// Retry-After counts down and rounds up.
	clock.Advance(10*time.Minute + 500*time.Millisecond)
	rec = loginFrom(h, "198.51.100.50", email, "password123")
	if rec.Code != 429 || rec.Header().Get("Retry-After") != "300" {
		t.Errorf("got %d Retry-After %q, want 429 and 300", rec.Code, rec.Header().Get("Retry-After"))
	}

	// The limit expires after the window.
	clock.Advance(5 * time.Minute)
	rec = loginFrom(h, "198.51.100.50", email, "password123")
	if rec.Code != 200 {
		t.Fatalf("after window: got %d, want 200", rec.Code)
	}
}

func TestLoginRateLimitByIP(t *testing.T) {
	h, clock, _ := newLimitedRouter(t)
	for i := 0; i < 5; i++ {
		// A different unknown email each time: only the IP counter fills up.
		rec := loginFrom(h, "192.0.2.77", testutil.UniqueEmail("ip"), "wrong")
		if rec.Code != 401 {
			t.Fatalf("attempt %d: got %d, want 401", i+1, rec.Code)
		}
	}
	if rec := loginFrom(h, "192.0.2.77", testutil.UniqueEmail("ip"), "wrong"); rec.Code != 429 {
		t.Fatalf("same IP: got %d, want 429", rec.Code)
	}
	if rec := loginFrom(h, "192.0.2.78", testutil.UniqueEmail("ip"), "wrong"); rec.Code != 401 {
		t.Fatalf("other IP: got %d, want 401", rec.Code)
	}
	clock.Advance(15 * time.Minute)
	if rec := loginFrom(h, "192.0.2.77", testutil.UniqueEmail("ip"), "wrong"); rec.Code != 401 {
		t.Fatalf("after window: got %d, want 401", rec.Code)
	}
}

func TestLoginSuccessResetsEmailCounter(t *testing.T) {
	h, _, pool := newLimitedRouter(t)
	email := testutil.UniqueEmail("reset")
	testutil.CleanupUser(t, pool, email)
	registerUser(t, h, nil, email, "password123")

	for round := 0; round < 3; round++ {
		for i := 0; i < 4; i++ {
			ip := "198.51.100." + string(rune('1'+round*4+i))
			if rec := loginFrom(h, ip, email, "wrong"); rec.Code != 401 {
				t.Fatalf("round %d attempt %d: got %d, want 401", round, i, rec.Code)
			}
		}
		if rec := loginFrom(h, "203.0.113."+string(rune('1'+round)), email, "password123"); rec.Code != 200 {
			t.Fatalf("round %d success: got %d, want 200", round, rec.Code)
		}
	}
}

func TestLoginRateLimitSameForUnknownEmail(t *testing.T) {
	h, _, pool := newLimitedRouter(t)
	known := testutil.UniqueEmail("known")
	testutil.CleanupUser(t, pool, known)
	registerUser(t, h, nil, known, "password123")
	unknown := testutil.UniqueEmail("unknown")

	var recs []*httptest.ResponseRecorder
	for _, email := range []string{known, unknown} {
		for i := 0; i < 5; i++ {
			loginFrom(h, "198.51.100.1"+string(rune('0'+i)), email, "wrong")
		}
		recs = append(recs, loginFrom(h, "198.51.100.99", email, "wrong"))
	}
	if recs[0].Code != 429 || recs[1].Code != 429 {
		t.Fatalf("got %d and %d, want 429 twice", recs[0].Code, recs[1].Code)
	}
	if recs[0].Body.String() != recs[1].Body.String() {
		t.Errorf("bodies differ: %q vs %q", recs[0].Body, recs[1].Body)
	}
	for _, k := range []string{"Retry-After", "Content-Type"} {
		if a, b := recs[0].Header().Get(k), recs[1].Header().Get(k); a != b {
			t.Errorf("%s differs: %q vs %q", k, a, b)
		}
	}
}
