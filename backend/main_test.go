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
	"budgetcontrol/db"
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
	const noURL = "DATABASE_URL is not set; set it to the PostgreSQL connection string"
	const legacyNote = "note: legacy database; migrate up will record 0001_initial as applied without running it"
	line := func(v int64, d, state, sum string) db.StatusLine {
		return db.StatusLine{Version: v, Description: d, State: state, Checksum: sum}
	}
	okRep := db.StatusReport{Lines: []db.StatusLine{line(1, "initial", "applied", "ok"), line(2, "b", "pending", "-")}, Applied: 1, Pending: 1}
	badRep := db.StatusReport{
		Lines:    []db.StatusLine{line(1, "initial", "applied", "mismatch"), line(9, "future", "applied", "newer")},
		Applied:  2,
		Problems: []error{errors.New("migration 0001_initial.sql was modified after it was applied"), errors.New("database has migration version 9, which is newer than this binary")},
	}
	tests := []struct {
		name       string
		args       []string
		getenv     func(string) string
		revertName string
		revertErr  error
		statusRep  db.StatusReport
		statusErr  error
		upRes      db.ApplyResult
		upErr      error
		wantCode   int
		wantOut    string
		wantErr    string // substring; "" means stderr must be empty
		wantCalled string // which op ran: "", "revert", "status", "up"
	}{
		{name: "no verb", getenv: withURL, wantCode: 2, wantErr: "status|up|down"},
		{name: "unknown verb", args: []string{"sideways"}, getenv: withURL, wantCode: 2, wantErr: "status|up|down"},
		{name: "extra args down", args: []string{"down", "2"}, getenv: withURL, wantCode: 2, wantErr: "status|up|down"},
		{name: "extra args status", args: []string{"status", "--json"}, getenv: withURL, wantCode: 2, wantErr: "usage: budgetcontrol migrate status|up|down"},
		{name: "extra args up", args: []string{"up", "1"}, getenv: withURL, wantCode: 2, wantErr: "usage: budgetcontrol migrate status|up|down"},
		{name: "usage before DATABASE_URL", args: []string{"x"}, getenv: env(nil), wantCode: 2, wantErr: "status|up|down"},

		{name: "down without DATABASE_URL", args: []string{"down"}, getenv: env(nil), wantCode: 1, wantErr: noURL},
		{name: "down reverted", args: []string{"down"}, getenv: withURL, revertName: "0002_b", wantOut: "reverted 0002_b\n", wantCalled: "revert"},
		{name: "down nothing", args: []string{"down"}, getenv: withURL, wantOut: "nothing to revert\n", wantCalled: "revert"},
		{name: "down failure", args: []string{"down"}, getenv: withURL, revertErr: errors.New("no down migration for 0002_b"), wantCode: 1, wantErr: "no down migration for 0002_b", wantCalled: "revert"},

		{name: "status without DATABASE_URL", args: []string{"status"}, getenv: env(nil), wantCode: 1, wantErr: noURL},
		{name: "status ok", args: []string{"status"}, getenv: withURL, statusRep: okRep,
			wantOut: "0001\tinitial\tapplied\tok\n0002\tb\tpending\t-\n1 applied, 1 pending\n", wantCalled: "status"},
		{name: "status nothing known", args: []string{"status"}, getenv: withURL, wantOut: "0 applied, 0 pending\n", wantCalled: "status"},
		{name: "status problems list and exit 1", args: []string{"status"}, getenv: withURL, statusRep: badRep, wantCode: 1,
			wantOut: "0001\tinitial\tapplied\tmismatch\n0009\tfuture\tapplied\tnewer\n2 applied, 0 pending\n",
			wantErr: "modified after it was applied", wantCalled: "status"},
		{name: "status legacy note", args: []string{"status"}, getenv: withURL,
			statusRep: db.StatusReport{Lines: []db.StatusLine{line(1, "initial", "pending", "-")}, Pending: 1, Legacy: true},
			wantOut:   "0001\tinitial\tpending\t-\n0 applied, 1 pending\n", wantErr: legacyNote, wantCalled: "status"},
		{name: "status connection error", args: []string{"status"}, getenv: withURL, statusErr: errors.New("ping database: refused"), wantCode: 1, wantErr: "ping database: refused", wantCalled: "status"},

		{name: "up without DATABASE_URL", args: []string{"up"}, getenv: env(nil), wantCode: 1, wantErr: noURL},
		{name: "up applied", args: []string{"up"}, getenv: withURL, upRes: db.ApplyResult{Applied: []string{"0002_b", "0003_c"}},
			wantOut: "applied 0002_b\napplied 0003_c\n", wantCalled: "up"},
		{name: "up nothing", args: []string{"up"}, getenv: withURL, wantOut: "nothing to apply\n", wantCalled: "up"},
		{name: "up legacy only 0001", args: []string{"up"}, getenv: withURL, upRes: db.ApplyResult{Adopted: true},
			wantOut: "adopted 0001_initial\n", wantCalled: "up"},
		{name: "up legacy and more", args: []string{"up"}, getenv: withURL, upRes: db.ApplyResult{Adopted: true, Applied: []string{"0002_b"}},
			wantOut: "adopted 0001_initial\napplied 0002_b\n", wantCalled: "up"},
		{name: "up failure prints nothing on stdout", args: []string{"up"}, getenv: withURL, upErr: errors.New("migration 0002_b.sql: boom"), wantCode: 1, wantErr: "0002_b.sql", wantCalled: "up"},
		{name: "up partial failure prints committed migrations", args: []string{"up"}, getenv: withURL,
			upRes: db.ApplyResult{Applied: []string{"0002_b"}}, upErr: errors.New("migration 0003_c.sql: boom"),
			wantCode: 1, wantOut: "applied 0002_b\n", wantErr: "migrate up: migration 0003_c.sql: boom", wantCalled: "up"},
		{name: "up partial failure after adoption", args: []string{"up"}, getenv: withURL,
			upRes: db.ApplyResult{Adopted: true}, upErr: errors.New("migration 0002_b.sql did not finish"),
			wantCode: 1, wantOut: "adopted 0001_initial\n", wantErr: "did not finish", wantCalled: "up"},
		{name: "status interrupted", args: []string{"status"}, getenv: withURL,
			statusRep: db.StatusReport{Lines: []db.StatusLine{line(1, "initial", "applied", "ok"), line(2, "idx", "pending", "interrupted")}, Applied: 1, Pending: 1,
				Problems: []error{errors.New("migration 0002_idx.sql did not finish (up)")}},
			wantCode: 1, wantOut: "0001\tinitial\tapplied\tok\n0002\tidx\tpending\tinterrupted\n1 applied, 1 pending\n",
			wantErr: "did not finish", wantCalled: "status"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errOut strings.Builder
			called := ""
			check := func(op, url string) {
				if called != "" {
					t.Errorf("%s called after %s", op, called)
				}
				called = op
				if url != "postgres://x" {
					t.Errorf("url = %q", url)
				}
			}
			ops := migrateOps{
				revert: func(ctx context.Context, url string) (string, error) {
					check("revert", url)
					return tc.revertName, tc.revertErr
				},
				status: func(ctx context.Context, url string) (db.StatusReport, error) {
					check("status", url)
					return tc.statusRep, tc.statusErr
				},
				up: func(ctx context.Context, url string) (db.ApplyResult, error) {
					check("up", url)
					return tc.upRes, tc.upErr
				},
			}
			code := runMigrate(context.Background(), tc.args, &out, &errOut, tc.getenv, ops)
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
				t.Errorf("operation called = %q, want %q", called, tc.wantCalled)
			}
		})
	}
}
