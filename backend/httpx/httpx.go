// Package httpx holds JSON request/response helpers shared by all handlers.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

// MaxBodyBytes is the default request body limit.
const MaxBodyBytes = 1 << 20

// JSON writes v as a JSON response with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes the standard error body {"error": msg}.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// DecodeJSON reads a size-limited JSON body into dst. On failure it writes a
// 400 (413 if too large) error response and returns false.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			Error(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		Error(w, http.StatusBadRequest, "malformed JSON body")
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		Error(w, http.StatusBadRequest, "malformed JSON body")
		return false
	}
	return true
}

// InternalError logs err with the request method and path and writes the
// standard 500 body {"error": "internal error"}. Only the method, the path
// (without query string) and err are logged: never the body, headers,
// cookies or query, so no password, token or secret can reach the log. err
// may be nil when the failure has no underlying error.
func InternalError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("internal error: %s %q: %v", r.Method, r.URL.Path, err)
	Error(w, http.StatusInternalServerError, "internal error")
}
