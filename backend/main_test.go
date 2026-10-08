package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		method string
		want   int
	}{
		{"get", http.MethodGet, http.StatusOK},
		{"post", http.MethodPost, http.StatusMethodNotAllowed},
		{"delete", http.MethodDelete, http.StatusMethodNotAllowed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			newMux().ServeHTTP(rec, httptest.NewRequest(tc.method, "/health", nil))
			if rec.Code != tc.want {
				t.Errorf("got %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestAddr(t *testing.T) {
	t.Setenv("PORT", "")
	if got := addr(); got != ":8080" {
		t.Errorf("got %q, want :8080", got)
	}
	t.Setenv("PORT", "9000")
	if got := addr(); got != ":9000" {
		t.Errorf("got %q, want :9000", got)
	}
}
