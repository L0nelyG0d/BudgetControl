// Package api builds the HTTP router. Each area has its own handler file and
// its own registerXRoutes function; NewRouter just calls them.
package api

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps are the shared dependencies handlers need. Add new fields here.
type Deps struct {
	DB *pgxpool.Pool
	// JWTSecret signs session tokens; it must be at least MinSecretLen bytes
	// (see Validate). Tests pass their own.
	JWTSecret []byte
	// Production makes the session cookie Secure (APP_ENV=production).
	Production bool
	// Now is the clock; nil means time.Now. Tests inject a fake.
	Now func() time.Time
}

// Handler groups handlers around shared dependencies.
type Handler struct {
	deps Deps
}

// NewRouter returns the application mux with all routes registered.
//
// To add routes for a new area: create registerXRoutes(mux, h) in its own
// file (stubs for categories, expenses and budgets already exist) and wrap
// every protected handler with h.requireAuth.
func NewRouter(deps Deps) *http.ServeMux {
	h := &Handler{deps: deps}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	h.registerAuthRoutes(mux)
	h.registerCategoryRoutes(mux)
	h.registerExpenseRoutes(mux)
	h.registerBudgetRoutes(mux)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
