package api

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"budgetcontrol/db"
	"budgetcontrol/httpx"
)

const (
	maxCategoryNameLen = 50
	otherCategoryName  = "Other"
)

var categoryColorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type categoryResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	IsDefault bool   `json:"is_default"`
}

type categoryRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

// validName trims name and checks its length (1-50 characters).
func validName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	return name, n >= 1 && n <= maxCategoryNameLen
}

// nameTaken reports whether name (case-insensitive) is a default category or
// one of the user's own, ignoring the category with id exceptID.
func (h *Handler) nameTaken(ctx context.Context, userID int64, name string, exceptID int64) (bool, error) {
	var taken bool
	err := h.deps.DB.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM categories
		   WHERE (user_id IS NULL OR user_id = $1) AND lower(name) = lower($2) AND id <> $3)`,
		userID, name, exceptID).Scan(&taken)
	return taken, err
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := h.deps.DB.Query(r.Context(),
		`SELECT id, name, color, user_id IS NULL FROM categories
		 WHERE user_id IS NULL OR user_id = $1
		 ORDER BY (user_id IS NULL) DESC, name, id`, UserIDFrom(r))
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	defer rows.Close()
	out := []categoryResponse{}
	for rows.Next() {
		var c categoryResponse
		if err := rows.Scan(&c.ID, &c.Name, &c.Color, &c.IsDefault); err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == nil || req.Color == nil {
		httpx.Error(w, http.StatusBadRequest, "name and color are required")
		return
	}
	name, ok := validName(*req.Name)
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "name must be 1-50 characters")
		return
	}
	if !categoryColorRE.MatchString(*req.Color) {
		httpx.Error(w, http.StatusBadRequest, "color must be in #RRGGBB format")
		return
	}
	userID := UserIDFrom(r)
	taken, err := h.nameTaken(r.Context(), userID, name, 0)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if taken {
		httpx.Error(w, http.StatusConflict, "category name is already in use")
		return
	}
	c := categoryResponse{Name: name, Color: *req.Color}
	err = h.deps.DB.QueryRow(r.Context(),
		`INSERT INTO categories (user_id, name, color) VALUES ($1, $2, $3) RETURNING id`,
		userID, name, c.Color).Scan(&c.ID)
	if db.IsUniqueViolation(err) {
		httpx.Error(w, http.StatusConflict, "category name is already in use")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

// categoryID parses the {id} path value, answering 400 when it is not numeric.
func categoryID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid category id")
		return 0, false
	}
	return id, true
}

// ownCategory answers 404 for a missing id and 403 for a default or another
// user's category. It returns the current name and color of the user's own one.
func ownCategory(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, w http.ResponseWriter, r *http.Request, userID, id int64, lock bool) (name, color string, ok bool) {
	query := `SELECT user_id, name, color FROM categories WHERE id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	var owner *int64
	err := q.QueryRow(ctx, query, id).Scan(&owner, &name, &color)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		httpx.Error(w, http.StatusNotFound, "category not found")
	case err != nil:
		httpx.InternalError(w, r, err)
	case owner == nil:
		httpx.Error(w, http.StatusForbidden, "default categories cannot be changed")
	case *owner != userID:
		httpx.Error(w, http.StatusForbidden, "not your category")
	default:
		return name, color, true
	}
	return "", "", false
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := categoryID(w, r)
	if !ok {
		return
	}
	var req categoryRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	if req.Name == nil && req.Color == nil {
		httpx.Error(w, http.StatusBadRequest, "name or color is required")
		return
	}
	userID := UserIDFrom(r)
	name, color, ok := ownCategory(r.Context(), h.deps.DB, w, r, userID, id, false)
	if !ok {
		return
	}
	if req.Name != nil {
		if name, ok = validName(*req.Name); !ok {
			httpx.Error(w, http.StatusBadRequest, "name must be 1-50 characters")
			return
		}
	}
	if req.Color != nil {
		if !categoryColorRE.MatchString(*req.Color) {
			httpx.Error(w, http.StatusBadRequest, "color must be in #RRGGBB format")
			return
		}
		color = *req.Color
	}
	taken, err := h.nameTaken(r.Context(), userID, name, id)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if taken {
		httpx.Error(w, http.StatusConflict, "category name is already in use")
		return
	}
	_, err = h.deps.DB.Exec(r.Context(),
		`UPDATE categories SET name = $1, color = $2 WHERE id = $3 AND user_id = $4`,
		name, color, id, userID)
	if db.IsUniqueViolation(err) {
		httpx.Error(w, http.StatusConflict, "category name is already in use")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, categoryResponse{ID: id, Name: name, Color: color})
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := categoryID(w, r)
	if !ok {
		return
	}
	userID := UserIDFrom(r)
	ctx := r.Context()
	tx, err := h.deps.DB.Begin(ctx)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, _, ok := ownCategory(ctx, tx, w, r, userID, id, true); !ok {
		return
	}
	var otherID int64
	err = tx.QueryRow(ctx,
		`SELECT id FROM categories WHERE user_id IS NULL AND lower(name) = lower($1)`,
		otherCategoryName).Scan(&otherID)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE expenses SET category_id = $1 WHERE category_id = $2`, otherID, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM budgets WHERE category_id = $1`, id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM categories WHERE id = $1 AND user_id = $2`, id, userID)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
