package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"budgetcontrol/httpx"
)

type budgetResponse struct {
	CategoryID *int64 `json:"category_id"`
	Amount     int64  `json:"amount"`
}

type budgetRequest struct {
	CategoryID json.RawMessage `json:"category_id"`
	Amount     json.RawMessage `json:"amount"`
}

// monthParam parses the required ?month=YYYY-MM and returns the 1st of that
// month as YYYY-MM-DD, writing a 400 on failure.
func monthParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	m := r.URL.Query().Get("month")
	t, err := time.Parse("2006-01", m)
	if err != nil || t.Format("2006-01") != m {
		httpx.Error(w, http.StatusBadRequest, "month is required in YYYY-MM format")
		return "", false
	}
	return t.Format("2006-01-02"), true
}

func (h *Handler) listBudgets(w http.ResponseWriter, r *http.Request) {
	month, ok := monthParam(w, r)
	if !ok {
		return
	}
	rows, err := h.deps.DB.Query(r.Context(),
		`SELECT category_id, amount FROM budgets
		 WHERE user_id = $1 AND month = $2
		 ORDER BY category_id NULLS FIRST`, UserIDFrom(r), month)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	out := []budgetResponse{}
	for rows.Next() {
		var b budgetResponse
		if err := rows.Scan(&b.CategoryID, &b.Amount); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, b)
	}
	if rows.Err() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) setBudget(w http.ResponseWriter, r *http.Request) {
	month, ok := monthParam(w, r)
	if !ok {
		return
	}
	var req budgetRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	amount, ok := parseWholeNumber(req.Amount)
	if !ok || amount <= 0 {
		httpx.Error(w, http.StatusBadRequest, "amount must be a positive whole number of tenge")
		return
	}
	// Omitted or null category_id means the overall budget.
	var categoryID *int64
	if raw := bytes.TrimSpace(req.CategoryID); len(raw) > 0 && string(raw) != "null" {
		id, ok := parseWholeNumber(raw)
		if !ok || id <= 0 {
			httpx.Error(w, http.StatusBadRequest, "category_id must be a positive whole number or null")
			return
		}
		var visible bool
		err := h.deps.DB.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM categories WHERE id = $1 AND (user_id IS NULL OR user_id = $2))`,
			id, UserIDFrom(r)).Scan(&visible)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		if !visible {
			httpx.Error(w, http.StatusBadRequest, "category_id must be a default category or one of your own")
			return
		}
		categoryID = &id
	}
	var out budgetResponse
	err := h.deps.DB.QueryRow(r.Context(),
		`INSERT INTO budgets (user_id, category_id, amount, month)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, category_id, month) DO UPDATE SET amount = EXCLUDED.amount
		 RETURNING category_id, amount`,
		UserIDFrom(r), categoryID, amount, month).Scan(&out.CategoryID, &out.Amount)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
