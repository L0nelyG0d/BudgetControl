package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"budgetcontrol/api"
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
			newMux(api.Deps{}).ServeHTTP(rec, httptest.NewRequest(tc.method, "/health", nil))
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

func TestTrustedProxy(t *testing.T) {
	tests := []struct {
		value string
		set   bool
		want  bool
	}{
		{"", false, false},
		{"", true, false},
		{"true", true, true},
		{"false", true, false},
		{"1", true, false},
		{"TRUE", true, false},
		{"True", true, false},
		{" true", true, false},
		{"yes", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			if tc.set {
				t.Setenv("TRUSTED_PROXY", tc.value)
			} else {
				t.Setenv("TRUSTED_PROXY", "")
				os.Unsetenv("TRUSTED_PROXY")
			}
			if got := trustedProxy(); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
