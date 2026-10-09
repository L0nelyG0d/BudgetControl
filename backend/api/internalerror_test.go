package api_test

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"budgetcontrol/api"
	"budgetcontrol/db"
	"budgetcontrol/testutil"
)

// TestInternalErrorIsLogged breaks the database (closed pool) and checks that
// each 500 logs the error, method and path, keeps the generic response body,
// and never logs the request body, query string, cookie or password.
func TestInternalErrorIsLogged(t *testing.T) {
	testutil.Pool(t) // skips when TEST_DATABASE_URL is unset
	pool, err := db.Connect(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	h := api.NewRouter(testutil.Deps(pool))
	cookie := testutil.SessionCookie(t, 1)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	const secret = "hunter2-very-secret"
	tests := []struct {
		name, method, path, body string
		withCookie               bool
		wantPath                 string
	}{
		{"register", "POST", "/auth/register", `{"email":"a@b.example","password":"` + secret + `"}`, false, "/auth/register"},
		{"login", "POST", "/auth/login", `{"email":"a@b.example","password":"` + secret + `"}`, false, "/auth/login"},
		{"list categories", "GET", "/categories?token=" + secret, "", true, "/categories"},
		{"create expense", "POST", "/expenses", `{"amount":5,"note":"` + secret + `"}`, true, "/expenses"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf.Reset()
			var rec = do(h, tc.method, tc.path, tc.body)
			if tc.withCookie {
				rec = do(h, tc.method, tc.path, tc.body, cookie)
			}
			if rec.Code != 500 {
				t.Fatalf("got %d, want 500: %s", rec.Code, rec.Body)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"internal error"}` {
				t.Errorf("body = %s", got)
			}
			out := buf.String()
			for _, want := range []string{tc.method, tc.wantPath, "closed pool"} {
				if !strings.Contains(out, want) {
					t.Errorf("log %q does not contain %q", out, want)
				}
			}
			for _, bad := range []string{secret, "a@b.example", cookie.Value, "token="} {
				if strings.Contains(out, bad) {
					t.Errorf("log %q leaks %q", out, bad)
				}
			}
		})
	}
}
