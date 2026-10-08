package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"budgetcontrol/httpx"
)

const maxNoteLen = 500

const expenseCols = `id, category_id, amount, to_char(date, 'YYYY-MM-DD'), note, created_at`

type expenseResponse struct {
	ID         int64     `json:"id"`
	CategoryID int64     `json:"category_id"`
	Amount     int64     `json:"amount"`
	Date       string    `json:"date"`
	Note       *string   `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
}

type expenseRequest struct {
	Amount     json.RawMessage `json:"amount"`
	CategoryID json.RawMessage `json:"category_id"`
	Date       *string         `json:"date"`
	Note       *string         `json:"note"`
}

type expenseInput struct {
	amount, categoryID int64
	date               string
	note               *string
}

// parseWholeNumber accepts only a JSON integer literal (no quotes, fraction
// or exponent) that fits in int64.
func parseWholeNumber(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0, false
	}
	for i, c := range raw {
		if c >= '0' && c <= '9' || (i == 0 && c == '-') {
			continue
		}
		return 0, false
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	return n, err == nil
}

// readExpense decodes and validates a POST/PUT body, writing a 400 on failure.
func (h *Handler) readExpense(w http.ResponseWriter, r *http.Request) (expenseInput, bool) {
	var req expenseRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return expenseInput{}, false
	}
	bad := func(msg string) (expenseInput, bool) {
		httpx.Error(w, http.StatusBadRequest, msg)
		return expenseInput{}, false
	}
	var in expenseInput
	var ok bool
	if in.amount, ok = parseWholeNumber(req.Amount); !ok || in.amount <= 0 {
		return bad("amount must be a positive whole number of tenge")
	}
	if in.categoryID, ok = parseWholeNumber(req.CategoryID); !ok || in.categoryID <= 0 {
		return bad("category_id is required")
	}
	if req.Date == nil {
		return bad("date is required (YYYY-MM-DD)")
	}
	if t, err := time.Parse("2006-01-02", *req.Date); err != nil || t.Format("2006-01-02") != *req.Date {
		return bad("date must be a real calendar date in YYYY-MM-DD format")
	}
	in.date = *req.Date
	if req.Note != nil {
		if utf8.RuneCountInString(*req.Note) > maxNoteLen {
			return bad("note must be at most 500 characters")
		}
		in.note = req.Note
	}
	var visible bool
	err := h.deps.DB.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM categories WHERE id = $1 AND (user_id IS NULL OR user_id = $2))`,
		in.categoryID, UserIDFrom(r)).Scan(&visible)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return expenseInput{}, false
	}
	if !visible {
		return bad("category_id must be a default category or one of your own")
	}
	return in, true
}

func scanExpense(row pgx.Row) (expenseResponse, error) {
	var e expenseResponse
	err := row.Scan(&e.ID, &e.CategoryID, &e.Amount, &e.Date, &e.Note, &e.CreatedAt)
	return e, err
}

// expenseID parses the {id} path value; a malformed id is a 404.
func expenseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "expense not found")
		return 0, false
	}
	return id, true
}

// respondExpense writes the scanned row, mapping no-rows to 404.
func respondExpense(w http.ResponseWriter, status int, row pgx.Row) {
	e, err := scanExpense(row)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusNotFound, "expense not found")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.JSON(w, status, e)
}

func (h *Handler) createExpense(w http.ResponseWriter, r *http.Request) {
	in, ok := h.readExpense(w, r)
	if !ok {
		return
	}
	respondExpense(w, http.StatusCreated, h.deps.DB.QueryRow(r.Context(),
		`INSERT INTO expenses (user_id, category_id, amount, date, note)
		 VALUES ($1, $2, $3, $4, $5) RETURNING `+expenseCols,
		UserIDFrom(r), in.categoryID, in.amount, in.date, in.note))
}

func (h *Handler) listExpenses(w http.ResponseWriter, r *http.Request) {
	q := `SELECT ` + expenseCols + ` FROM expenses WHERE user_id = $1`
	args := []any{UserIDFrom(r)}
	if m := r.URL.Query().Get("month"); r.URL.Query().Has("month") {
		t, err := time.Parse("2006-01", m)
		if err != nil || t.Format("2006-01") != m {
			httpx.Error(w, http.StatusBadRequest, "month must be in YYYY-MM format")
			return
		}
		q += ` AND date >= $2 AND date < $2::date + interval '1 month'`
		args = append(args, t.Format("2006-01-02"))
	}
	q += ` ORDER BY date DESC, id DESC`
	rows, err := h.deps.DB.Query(r.Context(), q, args...)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	out := []expenseResponse{}
	for rows.Next() {
		e, err := scanExpense(rows)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal error")
			return
		}
		out = append(out, e)
	}
	if rows.Err() != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) getExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseID(w, r)
	if !ok {
		return
	}
	respondExpense(w, http.StatusOK, h.deps.DB.QueryRow(r.Context(),
		`SELECT `+expenseCols+` FROM expenses WHERE id = $1 AND user_id = $2`, id, UserIDFrom(r)))
}

func (h *Handler) updateExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseID(w, r)
	if !ok {
		return
	}
	in, ok := h.readExpense(w, r)
	if !ok {
		return
	}
	respondExpense(w, http.StatusOK, h.deps.DB.QueryRow(r.Context(),
		`UPDATE expenses SET category_id = $3, amount = $4, date = $5, note = $6
		 WHERE id = $1 AND user_id = $2 RETURNING `+expenseCols,
		id, UserIDFrom(r), in.categoryID, in.amount, in.date, in.note))
}

func (h *Handler) deleteExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := expenseID(w, r)
	if !ok {
		return
	}
	tag, err := h.deps.DB.Exec(r.Context(),
		`DELETE FROM expenses WHERE id = $1 AND user_id = $2`, id, UserIDFrom(r))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "expense not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
