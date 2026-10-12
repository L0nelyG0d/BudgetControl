package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestTrustedProxies(t *testing.T) {
	t.Parallel()
	if got, err := trustedProxies("10.0.0.0/8, fd00::1"); err != nil || len(got) != 2 {
		t.Errorf("valid: got %v, %v", got, err)
	}
	if got, err := trustedProxies(""); err != nil || len(got) != 0 {
		t.Errorf("empty: got %v, %v", got, err)
	}
	for _, v := range []string{"10.0.0.0/33", "nonsense", "10.0.0.0/8,,", "fe80::1%eth0"} {
		if _, err := trustedProxies(v); err == nil {
			t.Errorf("%q: expected error", v)
		}
	}
}

func TestProxyConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, proxies, hops string
		trusted             bool
		wantHops, wantList  int
		wantWarn            string
		wantErr             string
	}{
		{name: "both unset", trusted: true},
		{name: "hops with trust", hops: "2", trusted: true, wantHops: 2},
		{name: "hops with spaces", hops: " 2 ", trusted: true, wantHops: 2},
		{name: "empty hops", hops: "", proxies: "10.0.0.0/8", trusted: true, wantList: 1},
		{name: "list only unchanged", proxies: "10.0.0.1", trusted: true, wantList: 1},
		{name: "hops without trust warns", hops: "2", wantHops: 2, wantWarn: "TRUSTED_PROXY_HOPS is set but TRUSTED_PROXY is not true; the hop count is ignored"},
		{name: "list without trust warns", proxies: "10.0.0.1", wantList: 1, wantWarn: "TRUSTED_PROXIES is set but TRUSTED_PROXY is not true"},
		{name: "above max", hops: "65", trusted: true, wantErr: `invalid TRUSTED_PROXY_HOPS "65"`},
		{name: "zero", hops: "0", trusted: true, wantErr: `invalid TRUSTED_PROXY_HOPS "0"`},
		{name: "invalid without trust still fails", hops: "abc", wantErr: `invalid TRUSTED_PROXY_HOPS "abc"`},
		{name: "both set", hops: "2", proxies: "10.0.0.1", trusted: true, wantErr: "only one of"},
		{name: "both set without trust", hops: "2", proxies: "10.0.0.1", wantErr: "only one of"},
		{name: "blank list and hops ok", hops: "1", proxies: "  ", trusted: true, wantHops: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			list, hops, warns, err := proxyConfig(tc.proxies, tc.hops, tc.trusted)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got err %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hops != tc.wantHops || len(list) != tc.wantList {
				t.Errorf("got hops %d, list %d; want %d, %d", hops, len(list), tc.wantHops, tc.wantList)
			}
			if tc.wantWarn == "" && len(warns) != 0 {
				t.Errorf("unexpected warnings %v", warns)
			}
			if tc.wantWarn != "" && (len(warns) != 1 || !strings.Contains(warns[0], tc.wantWarn)) {
				t.Errorf("got warnings %v, want one containing %q", warns, tc.wantWarn)
			}
		})
	}
}

func TestRunMigrate(t *testing.T) {
	t.Parallel()
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	withURL := env(map[string]string{"DATABASE_URL": "postgres://x"})
	tests := []struct {
		name       string
		args       []string
		getenv     func(string) string
		revertName string
		revertErr  error
		wantCode   int
		wantOut    string
		wantErr    string
		wantCalled bool
	}{
		{"no verb", nil, withURL, "", nil, 2, "", "status|up|down", false},
		{"unknown verb", []string{"sideways"}, withURL, "", nil, 2, "", "status|up|down", false},
		{"extra args", []string{"down", "2"}, withURL, "", nil, 2, "", "status|up|down", false},
		{"usage before DATABASE_URL", []string{"x"}, env(nil), "", nil, 2, "", "status|up|down", false},
		{"status not implemented", []string{"status"}, withURL, "", nil, 2, "", "not implemented", false},
		{"up not implemented", []string{"up"}, withURL, "", nil, 2, "", "not implemented", false},
		{"down without DATABASE_URL", []string{"down"}, env(nil), "", nil, 1, "", "DATABASE_URL is not set; set it to the PostgreSQL connection string", false},
		{"down reverted", []string{"down"}, withURL, "0002_b", nil, 0, "reverted 0002_b\n", "", true},
		{"down nothing", []string{"down"}, withURL, "", nil, 0, "nothing to revert\n", "", true},
		{"down failure", []string{"down"}, withURL, "", errors.New("no down migration for 0002_b"), 1, "", "no down migration for 0002_b", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errOut strings.Builder
			called := false
			code := runMigrate(context.Background(), tc.args, &out, &errOut, tc.getenv,
				func(ctx context.Context, url string) (string, error) {
					called = true
					if url != "postgres://x" {
						t.Errorf("url = %q", url)
					}
					return tc.revertName, tc.revertErr
				})
			if code != tc.wantCode {
				t.Errorf("code = %d, want %d", code, tc.wantCode)
			}
			if out.String() != tc.wantOut {
				t.Errorf("stdout = %q, want %q", out.String(), tc.wantOut)
			}
			if !strings.Contains(errOut.String(), tc.wantErr) || (tc.wantErr == "" && errOut.Len() != 0) {
				t.Errorf("stderr = %q, want containing %q", errOut.String(), tc.wantErr)
			}
			if called != tc.wantCalled {
				t.Errorf("revert called = %v, want %v", called, tc.wantCalled)
			}
		})
	}
}
