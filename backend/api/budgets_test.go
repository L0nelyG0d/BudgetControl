package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"budgetcontrol/api"
	"budgetcontrol/testutil"
)

type budgetJSON struct {
	CategoryID *int64 `json:"category_id"`
	Amount     int64  `json:"amount"`
}

func getBudgets(t *testing.T, h http.Handler, c *http.Cookie, month string) []budgetJSON {
	t.Helper()
	rec := do(h, "GET", "/budgets?month="+month, "", c)
	if rec.Code != 200 {
		t.Fatalf("GET budgets: %d %s", rec.Code, rec.Body)
	}
	var out []budgetJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out == nil {
		t.Fatalf("body is not a JSON array: %s", rec.Body)
	}
	return out
}

func budgetRows(t *testing.T, pool *pgxpool.Pool, userID int64) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM budgets WHERE user_id = $1`, userID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBudgetsRequireAuth(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/budgets?month=2026-10", ""},
		{"PUT", "/budgets?month=2026-10", `{"amount":1000}`},
	} {
		t.Run(tc.method, func(t *testing.T) {
			if rec := do(h, tc.method, tc.path, tc.body); rec.Code != 401 {
				t.Errorf("no cookie: got %d, want 401", rec.Code)
			}
			bad := &http.Cookie{Name: "session", Value: "garbage"}
			if rec := do(h, tc.method, tc.path, tc.body, bad); rec.Code != 401 {
				t.Errorf("garbage cookie: got %d, want 401", rec.Code)
			}
		})
	}
}

func TestBudgetsEmptyAndBadMonth(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	_, _, c := testutil.NewUser(t, pool)

	if got := getBudgets(t, h, c, "2026-10"); len(got) != 0 {
		t.Errorf("empty month: got %v", got)
	}
	for _, m := range []string{"", "2026", "2026-13", "2026-1", "2026-10-01", "abc", "2026-00", "0000-01", "0000-12"} {
		t.Run("month="+m, func(t *testing.T) {
			if rec := do(h, "GET", "/budgets?month="+m, "", c); rec.Code != 400 {
				t.Errorf("GET: got %d, want 400", rec.Code)
			}
			if rec := do(h, "PUT", "/budgets?month="+m, `{"amount":1000}`, c); rec.Code != 400 {
				t.Errorf("PUT: got %d, want 400", rec.Code)
			}
		})
	}
	if rec := do(h, "PUT", "/budgets?month=0001-01", `{"amount":1000}`, c); rec.Code != 200 {
		t.Errorf("year 1 PUT: got %d, want 200", rec.Code)
	}
	if got := getBudgets(t, h, c, "0001-01"); len(got) != 1 {
		t.Errorf("year 1 GET: %v", got)
	}
	if rec := do(h, "GET", "/budgets", "", c); rec.Code != 400 {
		t.Errorf("missing month: got %d, want 400", rec.Code)
	}
}

func TestBudgetsSetUpdateOverall(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	uid, _, c := testutil.NewUser(t, pool)
	food := defaultCategory(t, pool)
	own := ownCategory(t, pool, uid)

	put := func(month, body string) {
		t.Helper()
		if rec := do(h, "PUT", "/budgets?month="+month, body, c); rec.Code != 200 {
			t.Fatalf("PUT %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	put("2026-10", fmt.Sprintf(`{"category_id":%d,"amount":5000}`, food))
	put("2026-10", fmt.Sprintf(`{"category_id":%d,"amount":700}`, own))
	put("2026-10", `{"amount":90000}`)
	put("2026-11", `{"category_id":null,"amount":1}`)

	// Update every one of them: no duplicates, amounts replaced.
	put("2026-10", fmt.Sprintf(`{"category_id":%d,"amount":6000}`, food))
	put("2026-10", `{"amount":95000}`)
	put("2026-10", `{"category_id":null,"amount":96000}`)

	got := getBudgets(t, h, c, "2026-10")
	if len(got) != 3 {
		t.Fatalf("got %d budgets, want 3: %v", len(got), got)
	}
	want := map[int64]int64{0: 96000, food: 6000, own: 700} // 0 = overall
	for _, b := range got {
		var key int64
		if b.CategoryID != nil {
			key = *b.CategoryID
		}
		if want[key] != b.Amount {
			t.Errorf("category %d: amount %d, want %d", key, b.Amount, want[key])
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("missing budgets: %v", want)
	}
	// Other month is independent.
	if nov := getBudgets(t, h, c, "2026-11"); len(nov) != 1 || nov[0].CategoryID != nil || nov[0].Amount != 1 {
		t.Errorf("november: %v", nov)
	}
	if n := budgetRows(t, pool, uid); n != 4 {
		t.Errorf("rows: %d, want 4", n)
	}
	// Stored as the 1st of the month.
	var d string
	if err := pool.QueryRow(context.Background(),
		`SELECT to_char(min(month), 'YYYY-MM-DD') FROM budgets WHERE user_id = $1`, uid).Scan(&d); err != nil {
		t.Fatal(err)
	}
	if d != "2026-10-01" {
		t.Errorf("stored month %s", d)
	}
}

func TestBudgetsInvalidBody(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	uid, _, c := testutil.NewUser(t, pool)
	otherID, _, otherC := testutil.NewUser(t, pool)
	otherCat := ownCategory(t, pool, otherID)
	food := defaultCategory(t, pool)

	tests := []struct{ name, body string }{
		{"missing amount", `{}`},
		{"zero", `{"amount":0}`},
		{"negative", `{"amount":-5}`},
		{"fraction", `{"amount":10.5}`},
		{"float whole", `{"amount":10.0}`},
		{"exponent", `{"amount":1e3}`},
		{"string", `{"amount":"100"}`},
		{"null", `{"amount":null}`},
		{"malformed", `{"amount":`},
		{"other user's category", fmt.Sprintf(`{"category_id":%d,"amount":100}`, otherCat)},
		{"nonexistent category", `{"category_id":999999999,"amount":100}`},
		{"zero category", `{"category_id":0,"amount":100}`},
		{"string category", `{"category_id":"1","amount":100}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := do(h, "PUT", "/budgets?month=2026-10", tc.body, c); rec.Code != 400 {
				t.Errorf("got %d, want 400 (%s)", rec.Code, rec.Body)
			}
		})
	}
	if n := budgetRows(t, pool, uid); n != 0 {
		t.Errorf("invalid requests created %d rows", n)
	}

	// Users are independent: same category and month, different users.
	for _, cc := range []*http.Cookie{c, otherC} {
		body := fmt.Sprintf(`{"category_id":%d,"amount":100}`, food)
		if rec := do(h, "PUT", "/budgets?month=2026-10", body, cc); rec.Code != 200 {
			t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
		}
	}
	do(h, "PUT", "/budgets?month=2026-10", fmt.Sprintf(`{"category_id":%d,"amount":777}`, food), otherC)
	if got := getBudgets(t, h, c, "2026-10"); len(got) != 1 || got[0].Amount != 100 {
		t.Errorf("first user's budgets changed: %v", got)
	}
	if got := getBudgets(t, h, otherC, "2026-10"); len(got) != 1 || got[0].Amount != 777 {
		t.Errorf("second user's budgets: %v", got)
	}
}

func TestBudgetsDelete(t *testing.T) {
	pool := testutil.Pool(t)
	h := api.NewRouter(testutil.Deps(pool))
	uid, _, c := testutil.NewUser(t, pool)
	otherID, _, otherC := testutil.NewUser(t, pool)
	food := defaultCategory(t, pool)
	own := ownCategory(t, pool, uid)
	otherCat := ownCategory(t, pool, otherID)

	put := func(cc *http.Cookie, month, body string) {
		t.Helper()
		if rec := do(h, "PUT", "/budgets?month="+month, body, cc); rec.Code != 200 {
			t.Fatalf("PUT %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	catBody := fmt.Sprintf(`{"category_id":%d,"amount":100}`, food)
	put(c, "2026-10", catBody)
	put(c, "2026-10", fmt.Sprintf(`{"category_id":%d,"amount":200}`, own))
	put(c, "2026-10", `{"amount":300}`)
	put(c, "2026-11", catBody)
	put(otherC, "2026-10", catBody)
	put(otherC, "2026-10", `{"amount":999}`)

	del := func(cc *http.Cookie, path string) int {
		return do(h, "DELETE", path, "", cc).Code
	}
	// Success, then not found the second time.
	if got := del(c, fmt.Sprintf("/budgets?month=2026-10&category_id=%d", food)); got != 204 {
		t.Fatalf("delete category budget: %d", got)
	}
	if got := del(c, fmt.Sprintf("/budgets?month=2026-10&category_id=%d", food)); got != 404 {
		t.Errorf("delete again: %d, want 404", got)
	}
	// Overall: omitted category_id.
	if got := del(c, "/budgets?month=2026-10"); got != 204 {
		t.Fatalf("delete overall: %d", got)
	}
	if got := del(c, "/budgets?month=2026-10"); got != 404 {
		t.Errorf("delete overall again: %d, want 404", got)
	}
	// Remaining: own category in Oct, food in Nov.
	got := getBudgets(t, h, c, "2026-10")
	if len(got) != 1 || got[0].CategoryID == nil || *got[0].CategoryID != own {
		t.Errorf("october after delete: %v", got)
	}
	if nov := getBudgets(t, h, c, "2026-11"); len(nov) != 1 {
		t.Errorf("november changed: %v", nov)
	}
	// Month with nothing, valid category without a budget.
	if got := del(c, fmt.Sprintf("/budgets?month=2026-12&category_id=%d", food)); got != 404 {
		t.Errorf("nothing to delete: %d, want 404", got)
	}
	// Another user's data is untouched and not deletable.
	if got := getBudgets(t, h, otherC, "2026-10"); len(got) != 2 {
		t.Errorf("other user's budgets changed: %v", got)
	}
	if got := del(c, fmt.Sprintf("/budgets?month=2026-10&category_id=%d", otherCat)); got != 400 {
		t.Errorf("other user's category: %d, want 400", got)
	}
	// Overall of one user does not delete the other's.
	if got := del(otherC, "/budgets?month=2026-10"); got != 204 {
		t.Errorf("other user's own overall: %d", got)
	}
	if got := getBudgets(t, h, otherC, "2026-10"); len(got) != 1 {
		t.Errorf("other user's remaining: %v", got)
	}
	// Bad input and auth.
	for _, p := range []string{
		"/budgets", "/budgets?month=", "/budgets?month=2026-13", "/budgets?month=0000-01",
		"/budgets?month=2026-10&category_id=", "/budgets?month=2026-10&category_id=abc",
		"/budgets?month=2026-10&category_id=0", "/budgets?month=2026-10&category_id=-1",
		"/budgets?month=2026-10&category_id=1.5", "/budgets?month=2026-10&category_id=999999999",
	} {
		if got := del(c, p); got != 400 {
			t.Errorf("DELETE %s: %d, want 400", p, got)
		}
	}
	if rec := do(h, "DELETE", "/budgets?month=2026-10", ""); rec.Code != 401 {
		t.Errorf("no cookie: %d, want 401", rec.Code)
	}
	if n := budgetRows(t, pool, uid); n != 2 {
		t.Errorf("rows: %d, want 2", n)
	}
}
