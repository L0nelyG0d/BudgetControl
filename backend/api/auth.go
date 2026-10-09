package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"budgetcontrol/httpx"
)

const (
	// MinSecretLen is the shortest JWT secret the server accepts.
	MinSecretLen = 32
	// CookieName is the session cookie.
	CookieName = "session"
	// SessionTTL is the lifetime of both the token and the cookie.
	SessionTTL = 7 * 24 * time.Hour
)

// Validate reports a configuration problem; main refuses to start on error.
func (d Deps) Validate() error {
	if len(d.JWTSecret) < MinSecretLen {
		return errors.New("JWT_SECRET must be set to at least 32 characters")
	}
	return nil
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

type ctxKey struct{}

// UserID returns the authenticated user's id stored by requireAuth.
func UserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(ctxKey{}).(int64)
	return id, ok
}

// UserIDFrom returns the authenticated user's id for a request that went
// through requireAuth. It panics if the route is not protected, which is a
// programming error.
func UserIDFrom(r *http.Request) int64 {
	id, ok := UserID(r.Context())
	if !ok {
		panic("api: UserIDFrom called on a route without requireAuth")
	}
	return id
}

// NewSessionCookie signs a session token for userID and wraps it in the
// session cookie. Exported so test helpers can mint valid sessions.
func NewSessionCookie(secret []byte, userID int64, now time.Time, secure bool) (*http.Cookie, error) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(SessionTTL)),
	})
	signed, err := tok.SignedString(secret)
	if err != nil {
		return nil, err
	}
	return sessionCookie(signed, int(SessionTTL/time.Second), secure), nil
}

func sessionCookie(value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// parseToken verifies an HS256 token and returns the user id it carries.
func (h *Handler) parseToken(raw string) (int64, error) {
	if len(h.deps.JWTSecret) < MinSecretLen {
		return 0, errors.New("jwt secret not configured")
	}
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(raw, claims,
		func(*jwt.Token) (any, error) { return h.deps.JWTSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(h.deps.now),
	)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(claims.Subject, 10, 64)
}

// requireAuth wraps a handler: it answers 401 unless the request carries a
// valid session for an existing user, and otherwise stores the user id in the
// request context (read it with UserIDFrom).
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "authentication required")
			return
		}
		id, err := h.parseToken(c.Value)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "authentication required")
			return
		}
		var exists bool
		err = h.deps.DB.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists)
		if err != nil {
			httpx.InternalError(w, r, err)
			return
		}
		if !exists {
			httpx.Error(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	}
}

func (h *Handler) registerAuthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("GET /auth/me", h.requireAuth(h.me))
}

// dummyHash is compared against when the email is unknown, so both failure
// paths cost one bcrypt comparison.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if !httpx.DecodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		httpx.Error(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if len(h.deps.JWTSecret) < MinSecretLen {
		httpx.InternalError(w, r, errors.New("JWT secret is too short"))
		return
	}

	ip, now := clientIP(r, h.deps.TrustedProxy), h.deps.now()
	if wait := h.loginLimit.check(email, ip, now); wait > 0 {
		writeTooManyAttempts(w, wait)
		return
	}

	var id int64
	var hash string
	err := h.deps.DB.QueryRow(r.Context(),
		`SELECT id, password_hash FROM users WHERE lower(email) = $1`, email).Scan(&id, &hash)
	known := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		hash = string(dummyHash)
	} else if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	cmpErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password))
	if !known || cmpErr != nil {
		h.loginLimit.fail(email, ip, now)
		httpx.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	h.loginLimit.succeed(email)
	c, err := NewSessionCookie(h.deps.JWTSecret, id, now, h.deps.Production)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	http.SetCookie(w, c)
	httpx.JSON(w, http.StatusOK, userResponse{ID: id, Email: email})
}

func (h *Handler) logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, sessionCookie("", -1, h.deps.Production))
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	var u userResponse
	err := h.deps.DB.QueryRow(r.Context(),
		`SELECT id, email FROM users WHERE id = $1`, UserIDFrom(r)).Scan(&u.ID, &u.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}
