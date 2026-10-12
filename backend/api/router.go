// Package api builds the HTTP router. Each area has its own handler file and
// its own registerXRoutes function; NewRouter just calls them.
package api

import (
	"net/http"
	"net/netip"
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
	// TrustedProxy makes the login limiter use X-Forwarded-For as the client
	// IP (TRUSTED_PROXY=true): the last entry, or see TrustedProxies. Enable
	// only behind a proxy that appends the real client IP; otherwise clients
	// can forge the header.
	TrustedProxy bool
	// TrustedProxies (TRUSTED_PROXIES) lists the proxy IPs/CIDRs whose
	// X-Forwarded-For is believed; only used when TrustedProxy is true.
	TrustedProxies []netip.Prefix
	// TrustedProxyHops (TRUSTED_PROXY_HOPS) is the number of proxies in front
	// of the server: the client IP is that many entries from the right of
	// X-Forwarded-For. 0 means unset; exclusive with TrustedProxies; only
	// used when TrustedProxy is true.
	TrustedProxyHops int
	// TrustedProxyHeader (TRUSTED_PROXY_HEADER) names a single-IP header to
	// read instead of X-Forwarded-For, in canonical form. "" means unset (or
	// X-Forwarded-For); exclusive with TrustedProxyHops; only used when
	// TrustedProxy is true. The proxy must overwrite it, not append.
	TrustedProxyHeader string
	// Now is the clock; nil means time.Now. Tests inject a fake.
	Now func() time.Time
}

// Handler groups handlers around shared dependencies.
type Handler struct {
	deps       Deps
	loginLimit *loginLimiter
}

// NewRouter returns the application mux with all routes registered.
//
// To add routes for a new area: create registerXRoutes(mux, h) in its own
// file (stubs for categories, expenses and budgets already exist) and wrap
// every protected handler with h.requireAuth.
func NewRouter(deps Deps) *http.ServeMux {
	h := &Handler{deps: deps, loginLimit: newLoginLimiter()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	h.registerConfigRoutes(mux)
	h.registerAuthRoutes(mux)
	h.registerCategoryRoutes(mux)
	h.registerExpenseRoutes(mux)
	h.registerBudgetRoutes(mux)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
