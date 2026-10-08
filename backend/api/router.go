// Package api builds the HTTP router. Each area has its own handler file and
// a method on Handler; add new routes in NewRouter.
package api

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps are the shared dependencies handlers need. Later tasks add fields
// (e.g. JWT secret) here.
type Deps struct {
	DB *pgxpool.Pool
}

// Handler groups handlers around shared dependencies.
type Handler struct {
	deps Deps
}

// NewRouter returns the application mux with all routes registered.
func NewRouter(deps Deps) *http.ServeMux {
	h := &Handler{deps: deps}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("POST /auth/register", h.register)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
