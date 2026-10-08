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

type catJSON struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	IsDefault bool   `json:"is_default"`
}

func catCall(t *testing.T, pool *pgxpool.Pool, method, path, body string, c *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	api.NewRouter(testutil.Deps(pool)).ServeHTTP(rec, req)
	return rec
}

func mustCreateCat(t *testing.T, pool *pgxpool.Pool, c *http.Cookie, name string) catJSON {
	t.Helper()
	rec := catCall(t, pool, "POST", "/categories", fmt.Sprintf(`{"name":%q,"color":"#112233"}`, name), c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %q: status %d body %s", name, rec.Code, rec.Body)
	}
	var cat catJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &cat); err != nil {
		t.Fatal(err)
	}
	return cat
}

func listCats(t *testing.T, pool *pgxpool.Pool, c *http.Cookie) []catJSON {
	t.Helper()
	rec := catCall(t, pool, "GET", "/categories", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status %d", rec.Code)
	}
	var out []catJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCategoriesRequireAuth(t *testing.T) {
	pool := testutil.Pool(t)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/categories", ""},
		{"POST", "/categories", `{"name":"x","color":"#000000"}`},
		{"PUT", "/categories/1", `{"name":"x"}`},
		{"DELETE", "/categories/1", ""},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			if rec := catCall(t, pool, tc.method, tc.path, tc.body, nil); rec.Code != http.StatusUnauthorized {
				t.Errorf("got %d, want 401", rec.Code)
			}
		})
	}
}

func TestListCategories(t *testing.T) {
	pool := testutil.Pool(t)
	_, _, ca := testutil.NewUser(t, pool)
	_, _, cb := testutil.NewUser(t, pool)
	mustCreateCat(t, pool, ca, "Zebra")
	mustCreateCat(t, pool, ca, "Apple")
	other := mustCreateCat(t, pool, cb, "Bs Secret")

	list := listCats(t, pool, ca)
	var defaults, seenCustom int
	var customNames []string
	for i, c := range list {
		if c.IsDefault {
			if seenCustom > 0 {
				t.Errorf("default %q after custom at %d", c.Name, i)
			}
			defaults++
		} else {
			seenCustom++
			customNames = append(customNames, c.Name)
		}
		if c.ID == other.ID {
			t.Errorf("other user's category leaked")
		}
	}
	if defaults != 6 {
		t.Errorf("defaults = %d, want 6", defaults)
	}
	if got := strings.Join(customNames, ","); got != "Apple,Zebra" {
		t.Errorf("custom = %s, want Apple,Zebra", got)
	}
}

func TestCreateCategory(t *testing.T) {
	pool := testutil.Pool(t)
	_, _, ca := testutil.NewUser(t, pool)
	_, _, cb := testutil.NewUser(t, pool)
	mustCreateCat(t, pool, ca, "Pets")

	tests := []struct {
		name   string
		cookie *http.Cookie
		body   string
		want   int
	}{
		{"ok trimmed", ca, `{"name":"  Gym  ","color":"#aBc123"}`, 201},
		{"50 chars", ca, fmt.Sprintf(`{"name":%q,"color":"#000000"}`, strings.Repeat("a", 50)), 201},
		{"51 chars", ca, fmt.Sprintf(`{"name":%q,"color":"#000000"}`, strings.Repeat("a", 51)), 400},
		{"blank name", ca, `{"name":"   ","color":"#000000"}`, 400},
		{"missing name", ca, `{"color":"#000000"}`, 400},
		{"missing color", ca, `{"name":"X1"}`, 400},
		{"short color", ca, `{"name":"X2","color":"#fff"}`, 400},
		{"no hash", ca, `{"name":"X3","color":"000000"}`, 400},
		{"bad hex", ca, `{"name":"X4","color":"#GGGGGG"}`, 400},
		{"malformed", ca, `{`, 400},
		{"dup own", ca, `{"name":"pETS","color":"#000000"}`, 409},
		{"dup default", ca, `{"name":"food","color":"#000000"}`, 409},
		{"dup default trimmed", ca, `{"name":" OTHER ","color":"#000000"}`, 409},
		{"same name other user", cb, `{"name":"Pets","color":"#000000"}`, 201},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := catCall(t, pool, "POST", "/categories", tc.body, tc.cookie)
			if rec.Code != tc.want {
				t.Fatalf("got %d (%s), want %d", rec.Code, rec.Body, tc.want)
			}
			if tc.want == 201 {
				var c catJSON
				_ = json.Unmarshal(rec.Body.Bytes(), &c)
				if c.ID == 0 || c.IsDefault || c.Name != strings.TrimSpace(c.Name) {
					t.Errorf("bad body %+v", c)
				}
			}
		})
	}
}

func TestUpdateCategory(t *testing.T) {
	pool := testutil.Pool(t)
	_, _, ca := testutil.NewUser(t, pool)
	_, _, cb := testutil.NewUser(t, pool)
	mine := mustCreateCat(t, pool, ca, "Mine")
	mustCreateCat(t, pool, ca, "Taken")
	theirs := mustCreateCat(t, pool, cb, "Theirs")
	var food int64
	for _, c := range listCats(t, pool, ca) {
		if c.IsDefault && c.Name == "Food" {
			food = c.ID
		}
	}
	p := func(id int64) string { return fmt.Sprintf("/categories/%d", id) }

	tests := []struct {
		name   string
		path   string
		cookie *http.Cookie
		body   string
		want   int
	}{
		{"rename", p(mine.ID), ca, `{"name":"Renamed"}`, 200},
		{"recolor only", p(mine.ID), ca, `{"color":"#ABCDEF"}`, 200},
		{"own current name", p(mine.ID), ca, `{"name":"renamed","color":"#000000"}`, 200},
		{"dup own", p(mine.ID), ca, `{"name":"TAKEN"}`, 409},
		{"dup default", p(mine.ID), ca, `{"name":"Food"}`, 409},
		{"bad name", p(mine.ID), ca, `{"name":""}`, 400},
		{"bad color", p(mine.ID), ca, `{"color":"red"}`, 400},
		{"empty body", p(mine.ID), ca, `{}`, 400},
		{"default 403", p(food), ca, `{"name":"Meals"}`, 403},
		{"other user 403", p(theirs.ID), ca, `{"name":"Hijack"}`, 403},
		{"missing 404", p(999999999), ca, `{"name":"x"}`, 404},
		{"non-numeric 400", "/categories/abc", ca, `{"name":"x"}`, 400},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := catCall(t, pool, "PUT", tc.path, tc.body, tc.cookie)
			if rec.Code != tc.want {
				t.Fatalf("got %d (%s), want %d", rec.Code, rec.Body, tc.want)
			}
		})
	}
	// Rejected requests left the other user's category untouched.
	for _, c := range listCats(t, pool, cb) {
		if c.ID == theirs.ID && c.Name != "Theirs" {
			t.Errorf("other user's category changed to %q", c.Name)
		}
	}
	// The default is unchanged.
	for _, c := range listCats(t, pool, ca) {
		if c.ID == food && c.Name != "Food" {
			t.Errorf("default changed to %q", c.Name)
		}
	}
}

func TestDeleteCategory(t *testing.T) {
	pool := testutil.Pool(t)
	ctx := context.Background()
	ua, _, ca := testutil.NewUser(t, pool)
	_, _, cb := testutil.NewUser(t, pool)
	var food, other int64
	for _, c := range listCats(t, pool, ca) {
		if c.IsDefault && c.Name == "Food" {
			food = c.ID
		}
		if c.IsDefault && c.Name == "Other" {
			other = c.ID
		}
	}
	p := func(id int64) string { return fmt.Sprintf("/categories/%d", id) }

	t.Run("moves expenses to Other and drops budgets", func(t *testing.T) {
		cat := mustCreateCat(t, pool, ca, "Doomed")
		keep := mustCreateCat(t, pool, ca, "Keeper")
		for i := 0; i < 2; i++ {
			if _, err := pool.Exec(ctx, `INSERT INTO expenses (user_id, category_id, amount, date) VALUES ($1,$2,100,'2026-01-05')`, ua, cat.ID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `INSERT INTO expenses (user_id, category_id, amount, date) VALUES ($1,$2,100,'2026-01-05')`, ua, keep.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO budgets (user_id, category_id, amount, month) VALUES ($1,$2,500,'2026-01-01')`, ua, cat.ID); err != nil {
			t.Fatal(err)
		}
		if rec := catCall(t, pool, "DELETE", p(cat.ID), "", ca); rec.Code != http.StatusNoContent {
			t.Fatalf("got %d (%s), want 204", rec.Code, rec.Body)
		}
		var moved, kept, budgets, cats int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM expenses WHERE user_id=$1 AND category_id=$2`, ua, other).Scan(&moved)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM expenses WHERE user_id=$1 AND category_id=$2`, ua, keep.ID).Scan(&kept)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM budgets WHERE category_id=$1`, cat.ID).Scan(&budgets)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM categories WHERE id=$1`, cat.ID).Scan(&cats)
		if moved != 2 || kept != 1 || budgets != 0 || cats != 0 {
			t.Errorf("moved=%d kept=%d budgets=%d cats=%d, want 2 1 0 0", moved, kept, budgets, cats)
		}
	})
	t.Run("without expenses", func(t *testing.T) {
		cat := mustCreateCat(t, pool, ca, "Empty")
		if rec := catCall(t, pool, "DELETE", p(cat.ID), "", ca); rec.Code != http.StatusNoContent {
			t.Fatalf("got %d, want 204", rec.Code)
		}
	})
	t.Run("default 403", func(t *testing.T) {
		if rec := catCall(t, pool, "DELETE", p(food), "", ca); rec.Code != http.StatusForbidden {
			t.Errorf("got %d, want 403", rec.Code)
		}
	})
	t.Run("other user 403", func(t *testing.T) {
		theirs := mustCreateCat(t, pool, cb, "NotYours")
		if rec := catCall(t, pool, "DELETE", p(theirs.ID), "", ca); rec.Code != http.StatusForbidden {
			t.Errorf("got %d, want 403", rec.Code)
		}
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM categories WHERE id=$1`, theirs.ID).Scan(&n)
		if n != 1 {
			t.Errorf("category was deleted")
		}
	})
	t.Run("missing 404", func(t *testing.T) {
		if rec := catCall(t, pool, "DELETE", p(999999999), "", ca); rec.Code != http.StatusNotFound {
			t.Errorf("got %d, want 404", rec.Code)
		}
	})
	t.Run("non-numeric 400", func(t *testing.T) {
		if rec := catCall(t, pool, "DELETE", "/categories/abc", "", ca); rec.Code != http.StatusBadRequest {
			t.Errorf("got %d, want 400", rec.Code)
		}
	})
}
