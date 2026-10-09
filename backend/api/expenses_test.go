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
	for _, d := range []string{"0001-01-01", "2028-02-29", "9999-12-31"} {
		if rec := do(h, "POST", "/expenses", expBody(10, def, d, ""), c); rec.Code != 201 {
			t.Errorf("date %s: %d %s", d, rec.Code, rec.Body)
		}
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
		{"year zero", expBody(5, def, "0000-01-01", "")},
		{"year zero end", expBody(5, def, "0000-12-31", "")},
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
		{"year zero january", "/expenses?month=0000-01", nil, 400},
		{"year zero december", "/expenses?month=0000-12", nil, 400},
		{"year one", "/expenses?month=0001-01", []int64{}, 200},
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

	// Another user's expense and nonexistent numeric ids (including 0 and
	// negative ones, which parse as numbers) are 404.
	for _, p := range []string{path, "/expenses/999999999", "/expenses/0", "/expenses/-1", "/expenses/-5"} {
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

func TestAmountCap(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	uid, _, c := testutil.NewUser(t, pool)
	def := defaultCategory(t, pool)
	own := ownCategory(t, pool, uid)
	const limit, over = 1000000000000, 1000000000001

	t.Run("POST expenses", func(t *testing.T) {
		rec := do(h, "POST", "/expenses", expBody(limit, def, "2026-10-08", ""), c)
		var e expenseJSON
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != 201 || e.Amount != limit {
			t.Fatalf("limit: %d %s", rec.Code, rec.Body)
		}
		before := listIDs(t, h, c, "/expenses")
		rec = do(h, "POST", "/expenses", expBody(over, def, "2026-10-08", ""), c)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "at most 1000000000000 tenge") {
			t.Errorf("limit+1: %d %s", rec.Code, rec.Body)
		}
		if after := listIDs(t, h, c, "/expenses"); !sameIDs(before, after) {
			t.Errorf("rejected expense was stored: %v -> %v", before, after)
		}
	})

	t.Run("PUT expenses", func(t *testing.T) {
		e := mustCreate(t, h, c, expBody(5, def, "2026-10-08", "keep"))
		path := fmt.Sprintf("/expenses/%d", e.ID)
		if rec := do(h, "PUT", path, expBody(over, def, "2026-10-09", "changed"), c); rec.Code != 400 {
			t.Errorf("limit+1: %d %s", rec.Code, rec.Body)
		}
		var got expenseJSON
		rec := do(h, "GET", path, "", c)
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if got.Amount != 5 || got.Date != "2026-10-08" || got.Note == nil || *got.Note != "keep" {
			t.Errorf("expense changed by rejected PUT: %+v", got)
		}
		rec = do(h, "PUT", path, expBody(limit, def, "2026-10-09", ""), c)
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != 200 || got.Amount != limit {
			t.Errorf("limit: %d %s", rec.Code, rec.Body)
		}
	})

	t.Run("PUT budgets", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			cat  string
		}{
			{"category", fmt.Sprintf(`"category_id":%d,`, own)},
			{"overall", ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				month := "2026-" + map[string]string{"category": "03", "overall": "04"}[tc.name]
				body := func(a int64) string { return fmt.Sprintf(`{%s"amount":%d}`, tc.cat, a) }
				if rec := do(h, "PUT", "/budgets?month="+month, body(7), c); rec.Code != 200 {
					t.Fatalf("seed: %d %s", rec.Code, rec.Body)
				}
				rec := do(h, "PUT", "/budgets?month="+month, body(over), c)
				if rec.Code != 400 || !strings.Contains(rec.Body.String(), "at most 1000000000000 tenge") {
					t.Errorf("limit+1: %d %s", rec.Code, rec.Body)
				}
				if got := getBudgets(t, h, c, month); len(got) != 1 || got[0].Amount != 7 {
					t.Errorf("budget changed by rejected PUT: %v", got)
				}
				if rec := do(h, "PUT", "/budgets?month="+month, body(limit), c); rec.Code != 200 {
					t.Errorf("limit: %d %s", rec.Code, rec.Body)
				}
				if got := getBudgets(t, h, c, month); len(got) != 1 || got[0].Amount != limit {
					t.Errorf("budget at limit: %v", got)
				}
			})
		}
	})

	t.Run("overflowing amount", func(t *testing.T) {
		const huge = "99999999999999999999"
		if rec := do(h, "POST", "/expenses", expBody(huge, def, "2026-10-08", ""), c); rec.Code != 400 {
			t.Errorf("POST: %d", rec.Code)
		}
		if rec := do(h, "PUT", "/budgets?month=2026-05", `{"amount":`+huge+`}`, c); rec.Code != 400 {
			t.Errorf("PUT budgets: %d", rec.Code)
		}
	})
}

func TestExpenseMalformedID(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	_, _, c := testutil.NewUser(t, pool)
	def := defaultCategory(t, pool)
	e := mustCreate(t, h, c, expBody(100, def, "2026-10-08", "keep"))

	for _, id := range []string{"abc", "1.5", "12abc", "99999999999999999999", "%20"} {
		for _, m := range []string{"GET", "PUT", "DELETE"} {
			t.Run(m+" "+id, func(t *testing.T) {
				rec := do(h, m, "/expenses/"+id, expBody(1, def, "2026-10-08", ""), c)
				if rec.Code != 400 {
					t.Fatalf("got %d, want 400: %s", rec.Code, rec.Body)
				}
				if got := strings.TrimSpace(rec.Body.String()); got != `{"error":"invalid expense id"}` {
					t.Errorf("body = %s", got)
				}
				// Auth still runs first.
				if rec := do(h, m, "/expenses/"+id, `{}`); rec.Code != 401 {
					t.Errorf("no cookie: got %d, want 401", rec.Code)
				}
			})
		}
	}

	// The id is checked before the body is read: an invalid body changes nothing
	// and the error is about the id.
	for _, body := range []string{`{`, ``, `{"amount":-1}`} {
		rec := do(h, "PUT", "/expenses/abc", body, c)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "invalid expense id") {
			t.Errorf("PUT abc body %q: %d %s", body, rec.Code, rec.Body)
		}
	}
	rec := do(h, "GET", fmt.Sprintf("/expenses/%d", e.ID), "", c)
	var got expenseJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || got.Amount != 100 || got.Note == nil || *got.Note != "keep" {
		t.Errorf("expense changed: %d %s", rec.Code, rec.Body)
	}
}
