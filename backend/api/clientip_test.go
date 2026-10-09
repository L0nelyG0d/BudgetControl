package api

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	t.Parallel()
	const remote = "10.0.0.1:5555"
	tests := []struct {
		name  string
		trust bool
		xff   []string
		want  string
	}{
		{"disabled ignores header", false, []string{"1.1.1.1"}, "10.0.0.1"},
		{"disabled no header", false, nil, "10.0.0.1"},
		{"enabled single", true, []string{"1.1.1.1"}, "1.1.1.1"},
		{"enabled last of list", true, []string{"1.1.1.1, 2.2.2.2"}, "2.2.2.2"},
		{"enabled no space", true, []string{"1.1.1.1,2.2.2.2"}, "2.2.2.2"},
		{"enabled trims whitespace", true, []string{"  3.3.3.3  "}, "3.3.3.3"},
		{"enabled several lines", true, []string{"1.1.1.1, 2.2.2.2", "4.4.4.4, 5.5.5.5"}, "5.5.5.5"},
		{"enabled ipv6", true, []string{"1.1.1.1, 2001:db8::1"}, "2001:db8::1"},
		{"enabled missing", true, nil, "10.0.0.1"},
		{"enabled empty", true, []string{""}, "10.0.0.1"},
		{"enabled blank", true, []string{"  "}, "10.0.0.1"},
		{"enabled invalid last", true, []string{"1.1.1.1, nonsense"}, "10.0.0.1"},
		{"enabled trailing comma", true, []string{"1.1.1.1,"}, "10.0.0.1"},
		{"enabled empty last line", true, []string{"1.1.1.1", ""}, "10.0.0.1"},
		{"enabled with port is invalid", true, []string{"1.1.1.1:80"}, "10.0.0.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = remote
			for _, l := range tc.xff {
				r.Header.Add("X-Forwarded-For", l)
			}
			if got := clientIP(r, tc.trust); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
