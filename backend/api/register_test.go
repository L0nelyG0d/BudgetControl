package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"budgetcontrol/api"
	"budgetcontrol/testutil"
)

func post(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRegisterSuccess(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(api.Deps{DB: pool})
	email := testutil.UniqueEmail("ok")
	testutil.CleanupUser(t, pool, email)

	rec := post(h, `{"email":"  `+strings.ToUpper(email)+` ","password":"password123"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201: %s", rec.Code, rec.Body)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Error("cookie set")
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("response leaks password data: %s", rec.Body)
	}
	var got struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 || got.Email != email {
		t.Errorf("got %+v, want id and email %q", got, email)
	}

	var hash string
	err := pool.QueryRow(context.Background(), `SELECT password_hash FROM users WHERE id=$1`, got.ID).Scan(&hash)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "password123" || bcrypt.CompareHashAndPassword([]byte(hash), []byte("password123")) != nil {
		t.Error("password not stored as bcrypt hash")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(api.Deps{DB: pool})
	email := testutil.UniqueEmail("dup")
	testutil.CleanupUser(t, pool, email)

	if rec := post(h, `{"email":"`+email+`","password":"password123"}`); rec.Code != http.StatusCreated {
		t.Fatalf("first: got %d: %s", rec.Code, rec.Body)
	}
	rec := post(h, `{"email":"`+strings.ToUpper(email)+`","password":"password123"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("got %d, want 409", rec.Code)
	}
	var e struct{ Error string }
	if json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Error == "" {
		t.Errorf("bad error body: %s", rec.Body)
	}
}

func TestRegisterConcurrent(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(api.Deps{DB: pool})
	email := testutil.UniqueEmail("race")
	testutil.CleanupUser(t, pool, email)

	const n = 8
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = post(h, `{"email":"`+email+`","password":"password123"}`).Code
		}()
	}
	wg.Wait()
	created := 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	if created != 1 {
		t.Errorf("created %d users, want 1", created)
	}
}

func TestRegisterInvalid(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(api.Deps{DB: pool})
	tests := []struct {
		name string
		body string
		want int
	}{
		{"short password", `{"email":"a@x.com","password":"short12"}`, 400},
		{"password 73 bytes", `{"email":"a@x.com","password":"` + strings.Repeat("a", 73) + `"}`, 400},
		{"no at sign", `{"email":"ax.com","password":"password123"}`, 400},
		{"empty email", `{"email":"","password":"password123"}`, 400},
		{"missing password", `{"email":"a@x.com"}`, 400},
		{"empty body object", `{}`, 400},
		{"malformed json", `{"email":`, 400},
		{"trailing data", `{"email":"a@x.com","password":"password123"} x`, 400},
		{"oversized body", `{"email":"a@x.com","password":"` + strings.Repeat("a", 2<<20) + `"}`, 413},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := post(h, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
			var e struct{ Error string }
			if json.Unmarshal(rec.Body.Bytes(), &e) != nil || e.Error == "" {
				t.Errorf("bad error body: %s", rec.Body)
			}
		})
	}
}

func TestPasswordBoundaries(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(api.Deps{DB: pool})
	for name, pw := range map[string]string{"8 chars": strings.Repeat("a", 8), "72 bytes": strings.Repeat("a", 72)} {
		t.Run(name, func(t *testing.T) {
			email := testutil.UniqueEmail("bound")
			testutil.CleanupUser(t, pool, email)
			rec := post(h, `{"email":"`+email+`","password":"`+pw+`"}`)
			if rec.Code != http.StatusCreated {
				t.Errorf("got %d: %s", rec.Code, rec.Body)
			}
		})
	}
}
