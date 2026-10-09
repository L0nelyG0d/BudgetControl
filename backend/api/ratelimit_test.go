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

const ipLimit = 20 // failed logins per client IP per window

func TestLoginRateLimitByIP(t *testing.T) {
	h, clock, pool := newLimitedRouter(t)
	good := testutil.UniqueEmail("ipok")
	testutil.CleanupUser(t, pool, good)
	registerUser(t, h, nil, good, "password123")

	for i := 0; i < ipLimit; i++ {
		// A different unknown email each time: only the IP counter fills up.
		rec := loginFrom(h, "192.0.2.77", testutil.UniqueEmail("ip"), "wrong")
		if rec.Code != 401 {
			t.Fatalf("attempt %d: got %d, want 401", i+1, rec.Code)
		}
	}
	if rec := loginFrom(h, "192.0.2.77", testutil.UniqueEmail("ip"), "wrong"); rec.Code != 429 {
		t.Fatalf("21st from same IP: got %d, want 429", rec.Code)
	} else if got := rec.Header().Get("Retry-After"); got != "900" {
		t.Errorf("Retry-After = %q, want 900", got)
	}
	// Blocked even for a correct password of an existing account.
	if rec := loginFrom(h, "192.0.2.77", good, "password123"); rec.Code != 429 {
		t.Fatalf("correct password from blocked IP: got %d, want 429", rec.Code)
	}
	if rec := loginFrom(h, "192.0.2.78", testutil.UniqueEmail("ip"), "wrong"); rec.Code != 401 {
		t.Fatalf("other IP: got %d, want 401", rec.Code)
	}
	clock.Advance(15 * time.Minute)
	if rec := loginFrom(h, "192.0.2.77", good, "password123"); rec.Code != 200 {
		t.Fatalf("after window: got %d, want 200", rec.Code)
	}
}

func TestLoginRateLimitEmailAndIPAreIndependent(t *testing.T) {
	h, _, pool := newLimitedRouter(t)
	a, b := testutil.UniqueEmail("indA"), testutil.UniqueEmail("indB")
	for _, e := range []string{a, b} {
		testutil.CleanupUser(t, pool, e)
		registerUser(t, h, nil, e, "password123")
	}

	// 5 failures for A from one IP block A, but not B from the same IP.
	for i := 0; i < 5; i++ {
		if rec := loginFrom(h, "192.0.2.10", a, "wrong"); rec.Code != 401 {
			t.Fatalf("attempt %d: got %d, want 401", i+1, rec.Code)
		}
	}
	if rec := loginFrom(h, "192.0.2.10", a, "password123"); rec.Code != 429 {
		t.Fatalf("A: got %d, want 429", rec.Code)
	}
	if rec := loginFrom(h, "192.0.2.10", b, "wrong"); rec.Code != 401 {
		t.Fatalf("B wrong password: got %d, want 401", rec.Code)
	}
	if rec := loginFrom(h, "192.0.2.10", b, "password123"); rec.Code != 200 {
		t.Fatalf("B right password: got %d, want 200", rec.Code)
	}

	// 5 failures for another email spread over 5 IPs do not block those IPs.
	c := testutil.UniqueEmail("indC")
	testutil.CleanupUser(t, pool, c)
	registerUser(t, h, nil, c, "password123")
	for i := 0; i < 5; i++ {
		loginFrom(h, "192.0.2.2"+string(rune('0'+i)), c, "wrong")
	}
	for i := 0; i < 5; i++ {
		if rec := loginFrom(h, "192.0.2.2"+string(rune('0'+i)), b, "password123"); rec.Code != 200 {
			t.Fatalf("IP %d with other email: got %d, want 200", i, rec.Code)
		}
	}
}

func TestLoginSuccessDoesNotResetIPCounter(t *testing.T) {
	h, _, pool := newLimitedRouter(t)
	good := testutil.UniqueEmail("ipreset")
	testutil.CleanupUser(t, pool, good)
	registerUser(t, h, nil, good, "password123")

	for i := 0; i < ipLimit-1; i++ {
		if rec := loginFrom(h, "192.0.2.40", testutil.UniqueEmail("ip"), "wrong"); rec.Code != 401 {
			t.Fatalf("attempt %d: got %d, want 401", i+1, rec.Code)
		}
	}
	if rec := loginFrom(h, "192.0.2.40", good, "password123"); rec.Code != 200 {
		t.Fatalf("success: got %d, want 200", rec.Code)
	}
	if rec := loginFrom(h, "192.0.2.40", testutil.UniqueEmail("ip"), "wrong"); rec.Code != 401 {
		t.Fatalf("20th failure: got %d, want 401", rec.Code)
	}
	if rec := loginFrom(h, "192.0.2.40", testutil.UniqueEmail("ip"), "wrong"); rec.Code != 429 {
		t.Fatalf("after 20 failures with a success between: got %d, want 429", rec.Code)
	}
}

func TestLoginRateLimitBothBlocked(t *testing.T) {
	t.Run("IP value is larger", func(t *testing.T) {
		h, clock, pool := newLimitedRouter(t)
		e := testutil.UniqueEmail("both")
		testutil.CleanupUser(t, pool, e)
		registerUser(t, h, nil, e, "password123")

		// Email E is blocked at t0 (failures from other IPs).
		for i := 0; i < 5; i++ {
			loginFrom(h, "198.51.100."+string(rune('1'+i)), e, "wrong")
		}
		clock.Advance(10 * time.Minute)
		// IP X reaches its limit now, with other emails.
		for i := 0; i < ipLimit; i++ {
			loginFrom(h, "192.0.2.50", testutil.UniqueEmail("x"), "wrong")
		}
		rec := loginFrom(h, "192.0.2.50", e, "password123")
		if rec.Code != 429 || rec.Header().Get("Retry-After") != "900" {
			t.Errorf("got %d Retry-After %q, want 429 and 900", rec.Code, rec.Header().Get("Retry-After"))
		}
	})
	t.Run("email value is larger", func(t *testing.T) {
		h, clock, pool := newLimitedRouter(t)
		e := testutil.UniqueEmail("both")
		testutil.CleanupUser(t, pool, e)
		registerUser(t, h, nil, e, "password123")

		// The IP's 15 oldest failures are at t0, with other emails.
		for i := 0; i < ipLimit-5; i++ {
			loginFrom(h, "192.0.2.51", testutil.UniqueEmail("x"), "wrong")
		}
		clock.Advance(10 * time.Minute)
		// Its last 5 are against E, which blocks both.
		for i := 0; i < 5; i++ {
			loginFrom(h, "192.0.2.51", e, "wrong")
		}
		rec := loginFrom(h, "192.0.2.51", e, "password123")
		if rec.Code != 429 || rec.Header().Get("Retry-After") != "900" {
			t.Errorf("got %d Retry-After %q, want 429 and 900", rec.Code, rec.Header().Get("Retry-After"))
		}
		// An unknown email from the same IP only sees the IP value.
		rec = loginFrom(h, "192.0.2.51", testutil.UniqueEmail("x"), "wrong")
		if rec.Code != 429 || rec.Header().Get("Retry-After") != "300" {
			t.Errorf("got %d Retry-After %q, want 429 and 300", rec.Code, rec.Header().Get("Retry-After"))
		}
	})
}

func TestLoginRateLimitIPBlockIsIdenticalAndNotExtended(t *testing.T) {
	h, clock, pool := newLimitedRouter(t)
	good := testutil.UniqueEmail("ipid")
	testutil.CleanupUser(t, pool, good)
	registerUser(t, h, nil, good, "password123")

	for i := 0; i < ipLimit; i++ {
		loginFrom(h, "192.0.2.60", testutil.UniqueEmail("x"), "wrong")
	}
	clock.Advance(10 * time.Minute)
	recs := []*httptest.ResponseRecorder{
		loginFrom(h, "192.0.2.60", good, "password123"),
		loginFrom(h, "192.0.2.60", good, "wrong"),
		loginFrom(h, "192.0.2.60", testutil.UniqueEmail("x"), "wrong"),
	}
	for i, rec := range recs {
		if rec.Code != 429 {
			t.Fatalf("response %d: got %d, want 429", i, rec.Code)
		}
		if got := rec.Header().Get("Retry-After"); got != "300" {
			t.Errorf("response %d: Retry-After = %q, want 300", i, got)
		}
		if rec.Body.String() != recs[0].Body.String() ||
			rec.Header().Get("Content-Type") != recs[0].Header().Get("Content-Type") {
			t.Errorf("response %d differs from the first", i)
		}
	}
	// 400s and the 429s above were not counted as failures: the block still
	// ends when the oldest real failures leave the window.
	for i := 0; i < ipLimit; i++ {
		req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(`{`))
		req.RemoteAddr = "192.0.2.61:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 400 {
			t.Fatalf("invalid body: got %d, want 400", rec.Code)
		}
	}
	if rec := loginFrom(h, "192.0.2.61", good, "password123"); rec.Code != 200 {
		t.Fatalf("IP after 400s: got %d, want 200", rec.Code)
	}
	clock.Advance(5 * time.Minute)
	if rec := loginFrom(h, "192.0.2.60", good, "password123"); rec.Code != 200 {
		t.Fatalf("after window: got %d, want 200", rec.Code)
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

func loginViaProxy(h http.Handler, forwarded, email, pw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/auth/login",
		strings.NewReader(`{"email":"`+email+`","password":"`+pw+`"}`))
	req.RemoteAddr = "10.0.0.1:4321" // always the proxy
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLoginRateLimitTrustedProxy(t *testing.T) {
	pool := testutil.Pool(t)
	for _, tc := range []struct {
		name         string
		trusted      bool
		sameCounter  bool // do clients A and B share one IP counter
		forwardedFor func(ip string) string
	}{
		{"disabled", false, true, func(ip string) string { return ip }},
		{"enabled", true, false, func(ip string) string { return "9.9.9.9, " + ip }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := testutil.Deps(pool)
			deps.TrustedProxy = tc.trusted
			h := api.NewRouter(deps)
			good := testutil.UniqueEmail("proxyok")
			testutil.CleanupUser(t, pool, good)
			registerUser(t, h, nil, good, "password123")

			a, b := tc.forwardedFor("198.51.100.1"), tc.forwardedFor("198.51.100.2")
			for i := 0; i < ipLimit; i++ {
				// Distinct unknown emails: only the IP counter fills up.
				if rec := loginViaProxy(h, a, testutil.UniqueEmail("p"), "wrong"); rec.Code != 401 {
					t.Fatalf("attempt %d: got %d, want 401", i+1, rec.Code)
				}
			}
			if rec := loginViaProxy(h, a, testutil.UniqueEmail("p"), "wrong"); rec.Code != 429 {
				t.Fatalf("21st from A: got %d, want 429", rec.Code)
			}
			wantWrong, wantRight := 401, 200
			if tc.sameCounter {
				wantWrong, wantRight = 429, 429
			}
			if rec := loginViaProxy(h, b, testutil.UniqueEmail("p"), "wrong"); rec.Code != wantWrong {
				t.Errorf("B wrong password: got %d, want %d", rec.Code, wantWrong)
			}
			if rec := loginViaProxy(h, b, good, "password123"); rec.Code != wantRight {
				t.Errorf("B right password: got %d, want %d", rec.Code, wantRight)
			}
		})
	}
}

func TestLoginRateLimitTrustedProxyFallback(t *testing.T) {
	pool := testutil.Pool(t)
	deps := testutil.Deps(pool)
	deps.TrustedProxy = true
	h := api.NewRouter(deps)
	// Missing or invalid forwarded IP: everyone falls back to the proxy's
	// remote address and shares one counter.
	for i := 0; i < ipLimit; i++ {
		fwd := []string{"", "not-an-ip"}[i%2]
		if rec := loginViaProxy(h, fwd, testutil.UniqueEmail("fb"), "wrong"); rec.Code != 401 {
			t.Fatalf("attempt %d: got %d, want 401", i+1, rec.Code)
		}
	}
	if rec := loginViaProxy(h, "", testutil.UniqueEmail("fb"), "wrong"); rec.Code != 429 {
		t.Fatalf("got %d, want 429", rec.Code)
	}
}
