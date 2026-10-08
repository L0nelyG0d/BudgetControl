package api

import (
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"budgetcontrol/db"
	"budgetcontrol/httpx"
)

const (
	minPasswordLen = 8
	maxPasswordLen = 72 // bcrypt's limit, in bytes
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	switch {
	case email == "" || req.Password == "":
		httpx.Error(w, http.StatusBadRequest, "email and password are required")
		return
	case !strings.Contains(email, "@"):
		httpx.Error(w, http.StatusBadRequest, "email is invalid")
		return
	case len(req.Password) < minPasswordLen:
		httpx.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	case len(req.Password) > maxPasswordLen:
		httpx.Error(w, http.StatusBadRequest, "password must be at most 72 bytes")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}

	var id int64
	err = h.deps.DB.QueryRow(r.Context(),
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		email, string(hash)).Scan(&id)
	if db.IsUniqueViolation(err) {
		httpx.Error(w, http.StatusConflict, "email is already registered")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal error")
		return
	}
	httpx.JSON(w, http.StatusCreated, userResponse{ID: id, Email: email})
}
