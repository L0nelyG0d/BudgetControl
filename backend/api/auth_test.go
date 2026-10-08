package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"budgetcontrol/api"
	"budgetcontrol/testutil"
)

func do(h http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func registerUser(t *testing.T, h http.Handler, _ any, email, pw string) int64 {
	t.Helper()
	rec := do(h, "POST", "/auth/register", `{"email":"`+email+`","password":"`+pw+`"}`)
	if rec.Code != 201 {
		t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	var u struct{ ID int64 }
	_ = json.Unmarshal(rec.Body.Bytes(), &u)
	return u.ID
}

func sessionOf(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	return nil
}

func TestLogin(t *testing.T) {
	pool := testutil.Pool(t)
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for _, prod := range []bool{false, true} {
		t.Run("prod="+strconv.FormatBool(prod), func(t *testing.T) {
			deps := testutil.Deps(pool)
			deps.Production, deps.Now = prod, func() time.Time { return now }
			h := api.NewRouter(deps)
			email := testutil.UniqueEmail("login")
			testutil.CleanupUser(t, pool, email)
			id := registerUser(t, h, pool, email, "password123")

			rec := do(h, "POST", "/auth/login", `{"email":" `+strings.ToUpper(email)+` ","password":"password123"}`)
			if rec.Code != 200 {
				t.Fatalf("got %d: %s", rec.Code, rec.Body)
			}
			var u struct {
				ID    int64
				Email string
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &u)
			if u.ID != id || u.Email != email {
				t.Errorf("body %+v, want id %d email %s", u, id, email)
			}
			c := sessionOf(rec)
			if c == nil {
				t.Fatal("no session cookie")
			}
			if strings.Contains(rec.Body.String(), c.Value) {
				t.Error("token in body")
			}
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" ||
				c.MaxAge != 7*24*3600 || c.Secure != prod {
				t.Errorf("bad cookie attributes: %+v", c)
			}
			claims := &jwt.RegisteredClaims{}
			tok, err := jwt.ParseWithClaims(c.Value, claims, func(*jwt.Token) (any, error) { return testutil.TestSecret, nil },
				jwt.WithTimeFunc(func() time.Time { return now }))
			if err != nil || tok.Method.Alg() != "HS256" {
				t.Fatalf("token: %v", err)
			}
			if claims.Subject != strconv.FormatInt(id, 10) || !claims.ExpiresAt.Time.Equal(now.Add(7*24*time.Hour)) {
				t.Errorf("claims %+v", claims)
			}
		})
	}
}

func TestLoginFailures(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	email := testutil.UniqueEmail("fail")
	testutil.CleanupUser(t, pool, email)
	registerUser(t, h, pool, email, "password123")

	tests := []struct {
		name, body string
		want       int
	}{
		{"wrong password", `{"email":"` + email + `","password":"nope-nope"}`, 401},
		{"unknown email", `{"email":"nobody-x@test.example","password":"password123"}`, 401},
		{"missing password", `{"email":"` + email + `"}`, 400},
		{"missing email", `{"password":"password123"}`, 400},
		{"empty", `{}`, 400},
		{"malformed", `{"email":`, 400},
	}
	var msgs []string
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, "POST", "/auth/login", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
			if sessionOf(rec) != nil {
				t.Error("cookie set")
			}
			if tc.want == 401 {
				msgs = append(msgs, rec.Body.String())
			}
		})
	}
	if len(msgs) == 2 && msgs[0] != msgs[1] {
		t.Errorf("401 bodies differ: %q vs %q", msgs[0], msgs[1])
	}
}

func TestLogout(t *testing.T) {
	h := api.NewRouter(api.Deps{JWTSecret: testutil.TestSecret})
	rec := do(h, "POST", "/auth/logout", "")
	if rec.Code != 204 {
		t.Fatalf("got %d, want 204", rec.Code)
	}
	c := sessionOf(rec)
	if c == nil || c.MaxAge >= 0 || c.Value != "" || !c.HttpOnly || c.Path != "/" {
		t.Errorf("cookie not cleared: %+v", c)
	}
}

func signed(t *testing.T, method jwt.SigningMethod, key any, sub string, exp time.Time) string {
	t.Helper()
	s, err := jwt.NewWithClaims(method, jwt.RegisteredClaims{Subject: sub, ExpiresAt: jwt.NewNumericDate(exp)}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMe(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	id, email, cookie := testutil.NewUser(t, pool)
	sub := strconv.FormatInt(id, 10)
	future := time.Now().Add(time.Hour)

	good := signed(t, jwt.SigningMethodHS256, testutil.TestSecret, sub, future)
	tests := []struct {
		name   string
		cookie *http.Cookie
		want   int
	}{
		{"valid", cookie, 200},
		{"no cookie", nil, 401},
		{"expired", &http.Cookie{Name: "session", Value: signed(t, jwt.SigningMethodHS256, testutil.TestSecret, sub, time.Now().Add(-time.Hour))}, 401},
		{"tampered", &http.Cookie{Name: "session", Value: good[:len(good)-2] + "xx"}, 401},
		{"other secret", &http.Cookie{Name: "session", Value: signed(t, jwt.SigningMethodHS256, []byte("another-secret-another-secret-another"), sub, future)}, 401},
		{"alg none", &http.Cookie{Name: "session", Value: signed(t, jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType, sub, future)}, 401},
		{"hs512", &http.Cookie{Name: "session", Value: signed(t, jwt.SigningMethodHS512, testutil.TestSecret, sub, future)}, 401},
		{"no expiry", &http.Cookie{Name: "session", Value: func() string {
			s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: sub}).SignedString(testutil.TestSecret)
			return s
		}()}, 401},
		{"garbage", &http.Cookie{Name: "session", Value: "abc"}, 401},
		{"deleted user", &http.Cookie{Name: "session", Value: signed(t, jwt.SigningMethodHS256, testutil.TestSecret, "999999999999", future)}, 401},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if tc.cookie == nil {
				rec = do(h, "GET", "/auth/me", "")
			} else {
				rec = do(h, "GET", "/auth/me", "", tc.cookie)
			}
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.want, rec.Body)
			}
			if tc.want == 401 {
				var e map[string]string
				if json.Unmarshal(rec.Body.Bytes(), &e) != nil || e["error"] == "" {
					t.Errorf("not JSON error shape: %s", rec.Body)
				}
			} else {
				var u struct {
					ID    int64
					Email string
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &u)
				if u.ID != id || u.Email != email {
					t.Errorf("got %+v", u)
				}
			}
		})
	}
}

func TestDeletedUserToken(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	id, email, cookie := testutil.NewUser(t, pool)
	if rec := do(h, "GET", "/auth/me", "", cookie); rec.Code != 200 {
		t.Fatalf("before delete: %d", rec.Code)
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM users WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_ = email
	if rec := do(h, "GET", "/auth/me", "", cookie); rec.Code != 401 {
		t.Errorf("after delete: got %d, want 401", rec.Code)
	}
}

func TestExpiredViaClock(t *testing.T) {
	pool := testutil.Pool(t)
	deps := testutil.Deps(pool)
	now := time.Now()
	deps.Now = func() time.Time { return now }
	h := api.NewRouter(deps)
	_, _, cookie := testutil.NewUser(t, pool)
	if rec := do(h, "GET", "/auth/me", "", cookie); rec.Code != 200 {
		t.Fatalf("fresh: %d", rec.Code)
	}
	now = now.Add(7*24*time.Hour + time.Minute)
	if rec := do(h, "GET", "/auth/me", "", cookie); rec.Code != 401 {
		t.Errorf("after 7 days: got %d, want 401", rec.Code)
	}
}

func TestDeps_Validate(t *testing.T) {
	tests := []struct {
		name   string
		secret string
		ok     bool
	}{
		{"empty", "", false},
		{"short", strings.Repeat("a", 31), false},
		{"min", strings.Repeat("a", 32), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := api.Deps{JWTSecret: []byte(tc.secret)}.Validate()
			if (err == nil) != tc.ok {
				t.Errorf("err=%v, want ok=%v", err, tc.ok)
			}
		})
	}
}
