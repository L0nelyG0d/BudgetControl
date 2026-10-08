package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"budgetcontrol/api"
	"budgetcontrol/testutil"
)

type expenseJSON struct {
	ID         int64   `json:"id"`
	CategoryID int64   `json:"category_id"`
	Amount     int64   `json:"amount"`
	Date       string  `json:"date"`
	Note       *string `json:"note"`
	CreatedAt  string  `json:"created_at"`
}

func defaultCategory(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`SELECT id FROM categories WHERE user_id IS NULL AND name = 'Food'`).Scan(&id); err != nil {
		t.Fatalf("default category: %v", err)
	}
	return id
}

func ownCategory(t *testing.T, pool *pgxpool.Pool, userID int64) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO categories (user_id, name, color) VALUES ($1, 'exp-test', '#112233') RETURNING id`,
		userID).Scan(&id)
	if err != nil {
		t.Fatalf("own category: %v", err)
	}
	return id // removed by testutil's user cleanup
}

func expBody(amount, cat any, date, note string) string {
	s := fmt.Sprintf(`{"amount":%v,"category_id":%v,"date":%q`, amount, cat, date)
	if note != "" {
		s += fmt.Sprintf(`,"note":%q`, note)
	}
	return s + "}"
}

func mustCreate(t *testing.T, h http.Handler, c *http.Cookie, body string) expenseJSON {
	t.Helper()
	rec := do(h, "POST", "/expenses", body, c)
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	var e expenseJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func listIDs(t *testing.T, h http.Handler, c *http.Cookie, path string) []int64 {
	t.Helper()
	rec := do(h, "GET", path, "", c)
	if rec.Code != 200 {
		t.Fatalf("list %s: %d %s", path, rec.Code, rec.Body)
	}
	var es []expenseJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &es); err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	return ids
}

func sameIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestExpensesNoSession(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	for _, rt := range []struct{ method, path string }{
		{"GET", "/expenses"}, {"POST", "/expenses"},
		{"GET", "/expenses/1"}, {"PUT", "/expenses/1"}, {"DELETE", "/expenses/1"},
	} {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			rec := do(h, rt.method, rt.path, `{}`)
			if rec.Code != 401 {
				t.Errorf("got %d, want 401", rec.Code)
			}
		})
	}
}

func TestExpenseCreate(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	uid, _, c := testutil.NewUser(t, pool)
	def := defaultCategory(t, pool)
	own := ownCategory(t, pool, uid)

	e := mustCreate(t, h, c, expBody(1500, def, "2026-10-08", "lunch"))
	if e.ID == 0 || e.CategoryID != def || e.Amount != 1500 || e.Date != "2026-10-08" ||
		e.Note == nil || *e.Note != "lunch" || e.CreatedAt == "" {
		t.Errorf("unexpected expense %+v", e)
	}
	e = mustCreate(t, h, c, expBody(10, own, "2099-12-31", "")) // future date, own category, no note
	if e.Note != nil || e.Date != "2099-12-31" || e.CategoryID != own {
		t.Errorf("unexpected expense %+v", e)
	}
	rec := do(h, "POST", "/expenses", expBody(10, def, "2024-02-29", strings.Repeat("я", 500)), c)
	if rec.Code != 201 {
		t.Errorf("500-char note: %d %s", rec.Code, rec.Body)
	}
}

func TestExpenseValidation(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	_, _, c := testutil.NewUser(t, pool)
	otherID, _, _ := testutil.NewUser(t, pool)
	def := defaultCategory(t, pool)
	foreign := ownCategory(t, pool, otherID)

	cases := []struct{ name, body string }{
		{"zero amount", expBody(0, def, "2026-10-08", "")},
		{"negative amount", expBody(-5, def, "2026-10-08", "")},
		{"decimal amount", expBody("1500.5", def, "2026-10-08", "")},
		{"decimal point zero", expBody("1500.0", def, "2026-10-08", "")},
		{"exponent amount", expBody("1e3", def, "2026-10-08", "")},
		{"string amount", expBody(`"1500"`, def, "2026-10-08", "")},
		{"huge amount", expBody("99999999999999999999", def, "2026-10-08", "")},
		{"missing amount", fmt.Sprintf(`{"category_id":%d,"date":"2026-10-08"}`, def)},
		{"missing category", `{"amount":5,"date":"2026-10-08"}`},
		{"string category", expBody(5, `"x"`, "2026-10-08", "")},
		{"nonexistent category", expBody(5, 999999999, "2026-10-08", "")},
		{"other user's category", expBody(5, foreign, "2026-10-08", "")},
		{"missing date", fmt.Sprintf(`{"amount":5,"category_id":%d}`, def)},
		{"not a date", expBody(5, def, "yesterday", "")},
		{"impossible date", expBody(5, def, "2026-02-30", "")},
		{"non-leap Feb 29", expBody(5, def, "2025-02-29", "")},
		{"wrong format", expBody(5, def, "2026-1-5", "")},
		{"note too long", expBody(5, def, "2026-10-08", strings.Repeat("a", 501))},
		{"malformed JSON", `{`},
		{"empty body", ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, "POST", "/expenses", tc.body, c)
			if rec.Code != 400 {
				t.Errorf("POST got %d, want 400: %s", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), `"error"`) {
				t.Errorf("body not an error object: %s", rec.Body)
			}
		})
	}
	if ids := listIDs(t, h, c, "/expenses"); len(ids) != 0 {
		t.Errorf("invalid requests created %v", ids)
	}

	// PUT uses the same validation.
	e := mustCreate(t, h, c, expBody(5, def, "2026-10-08", ""))
	for _, tc := range cases {
		t.Run("PUT "+tc.name, func(t *testing.T) {
			rec := do(h, "PUT", fmt.Sprintf("/expenses/%d", e.ID), tc.body, c)
			if rec.Code != 400 {
				t.Errorf("PUT got %d, want 400: %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestExpenseList(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	_, _, c := testutil.NewUser(t, pool)
	_, _, c2 := testutil.NewUser(t, pool)
	def := defaultCategory(t, pool)

	if rec := do(h, "GET", "/expenses", "", c); rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty list: %d %q", rec.Code, rec.Body)
	}

	a := mustCreate(t, h, c, expBody(1, def, "2026-09-30", ""))
	b := mustCreate(t, h, c, expBody(2, def, "2026-10-01", ""))
	d := mustCreate(t, h, c, expBody(3, def, "2026-10-31", ""))
	e := mustCreate(t, h, c, expBody(4, def, "2026-11-01", ""))
	f := mustCreate(t, h, c, expBody(5, def, "2026-10-31", "")) // same date, newer id first
	mustCreate(t, h, c2, expBody(6, def, "2026-10-15", ""))

	tests := []struct {
		name, path string
		want       []int64
		code       int
	}{
		{"all", "/expenses", []int64{e.ID, f.ID, d.ID, b.ID, a.ID}, 200},
		{"october", "/expenses?month=2026-10", []int64{f.ID, d.ID, b.ID}, 200},
		{"september", "/expenses?month=2026-09", []int64{a.ID}, 200},
		{"empty month", "/expenses?month=2020-01", []int64{}, 200},
		{"invalid month", "/expenses?month=2026-13", nil, 400},
		{"bad format", "/expenses?month=2026-1", nil, 400},
		{"full date", "/expenses?month=2026-10-01", nil, 400},
		{"empty month param", "/expenses?month=", nil, 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(h, "GET", tc.path, "", c)
			if rec.Code != tc.code {
				t.Fatalf("got %d, want %d: %s", rec.Code, tc.code, rec.Body)
			}
			if tc.code == 200 {
				if got := listIDs(t, h, c, tc.path); !sameIDs(got, tc.want) {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestExpenseGetUpdateDelete(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	uid, _, c := testutil.NewUser(t, pool)
	_, _, other := testutil.NewUser(t, pool)
	def := defaultCategory(t, pool)
	own := ownCategory(t, pool, uid)

	e := mustCreate(t, h, c, expBody(100, def, "2026-10-08", "old"))
	path := fmt.Sprintf("/expenses/%d", e.ID)

	rec := do(h, "GET", path, "", c)
	var got expenseJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || got.ID != e.ID || got.Amount != 100 || got.CreatedAt != e.CreatedAt {
		t.Errorf("get: %d %s", rec.Code, rec.Body)
	}

	// Another user's expense and nonexistent / malformed ids are 404.
	for _, p := range []string{path, "/expenses/999999999", "/expenses/abc", "/expenses/-1"} {
		for _, m := range []string{"GET", "PUT", "DELETE"} {
			t.Run(m+" "+p, func(t *testing.T) {
				who := other
				if p != path {
					who = c
				}
				if rec := do(h, m, p, expBody(1, def, "2026-10-08", ""), who); rec.Code != 404 {
					t.Errorf("got %d, want 404", rec.Code)
				}
			})
		}
	}
	// The other user's attempts changed nothing.
	rec = do(h, "GET", path, "", c)
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || got.Amount != 100 {
		t.Fatalf("expense modified by another user: %d %s", rec.Code, rec.Body)
	}

	// PUT replaces all fields, including clearing the note.
	rec = do(h, "PUT", path, expBody(250, own, "2027-01-02", ""), c)
	if rec.Code != 200 {
		t.Fatalf("put: %d %s", rec.Code, rec.Body)
	}
	var upd expenseJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &upd)
	if upd.ID != e.ID || upd.Amount != 250 || upd.CategoryID != own || upd.Date != "2027-01-02" ||
		upd.Note != nil || upd.CreatedAt != e.CreatedAt {
		t.Errorf("put result %+v", upd)
	}

	// DELETE
	if rec := do(h, "DELETE", path, "", c); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "DELETE", path, "", c); rec.Code != 404 {
		t.Errorf("second delete: %d, want 404", rec.Code)
	}
	if rec := do(h, "GET", path, "", c); rec.Code != 404 {
		t.Errorf("get after delete: %d, want 404", rec.Code)
	}
}

// Ensure the unauthenticated check also holds for a bogus cookie.
func TestExpensesBadCookie(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	req := httptest.NewRequest("GET", "/expenses", nil)
	req.AddCookie(&http.Cookie{Name: api.CookieName, Value: "garbage"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("got %d, want 401", rec.Code)
	}
}
