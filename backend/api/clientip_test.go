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
			if got := clientIP(r, tc.trust, nil, 0); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClientIPTrustedProxies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		trust  bool
		list   string
		remote string
		xff    []string
		want   string
	}{
		{"proxy flag off ignores list", false, "10.0.0.0/8", "10.0.0.1:1", []string{"1.1.1.1"}, "10.0.0.1"},
		{"single proxy", true, "10.0.0.0/8", "10.0.0.1:1", []string{"1.1.1.1"}, "1.1.1.1"},
		{"direct client cannot set ip", true, "10.0.0.0/8", "203.0.113.9:1", []string{"1.1.1.1"}, "203.0.113.9"},
		{"forged leading entry", true, "10.0.0.0/8", "10.0.0.1:1", []string{"9.9.9.9, 1.1.1.1"}, "1.1.1.1"},
		{"two proxies", true, "10.0.0.0/8", "10.0.0.2:1", []string{"1.1.1.1, 10.0.0.1"}, "1.1.1.1"},
		{"forged trusted-looking entry", true, "10.0.0.0/8", "10.0.0.1:1", []string{"10.0.0.5, 1.1.1.1"}, "1.1.1.1"},
		{"all trusted", true, "10.0.0.0/8", "10.0.0.1:1", []string{"10.0.0.3, 10.0.0.1"}, "10.0.0.1"},
		{"no header", true, "10.0.0.0/8", "10.0.0.1:1", nil, "10.0.0.1"},
		{"several lines", true, "10.0.0.0/8", "10.0.0.2:1", []string{"1.1.1.1, 2.2.2.2", "10.0.0.1"}, "2.2.2.2"},
		{"several lines same as one", true, "10.0.0.0/8", "10.0.0.2:1", []string{"1.1.1.1, 2.2.2.2, 10.0.0.1"}, "2.2.2.2"},
		{"nonsense", true, "10.0.0.0/8", "10.0.0.1:1", []string{"nonsense"}, "10.0.0.1"},
		{"empty entry", true, "10.0.0.0/8", "10.0.0.1:1", []string{"1.1.1.1,,10.0.0.1"}, "10.0.0.1"},
		{"port entry", true, "10.0.0.0/8", "10.0.0.1:1", []string{"1.1.1.1:80"}, "10.0.0.1"},
		{"trailing comma", true, "10.0.0.0/8", "10.0.0.1:1", []string{"1.1.1.1,"}, "10.0.0.1"},
		{"ipv6 cidr", true, "fd00::/8", "[fd00::1]:5555", []string{"2001:db8::1"}, "2001:db8::1"},
		{"ipv6 bare", true, "fd00::1", "[fd00::1]:5555", []string{"2001:db8::1"}, "2001:db8::1"},
		{"ipv4-mapped remote", true, "10.0.0.0/8", "[::ffff:10.0.0.1]:5555", []string{"1.1.1.1"}, "1.1.1.1"},
		{"ipv4-mapped untrusted remote", true, "10.0.0.0/8", "[::ffff:203.0.113.9]:5555", []string{"1.1.1.1"}, "203.0.113.9"},
		{"ipv4-mapped chain entry", true, "10.0.0.0/8", "10.0.0.2:1", []string{"1.1.1.1, ::ffff:10.0.0.1"}, "1.1.1.1"},
		{"single ip matches exactly", true, "192.168.1.5", "192.168.1.5:1", []string{"1.1.1.1"}, "1.1.1.1"},
		{"single ip does not match neighbour", true, "192.168.1.5", "192.168.1.6:1", []string{"1.1.1.1"}, "192.168.1.6"},
		{"list of several", true, "192.168.1.5, 10.0.0.0/8", "10.0.0.2:1", []string{"1.1.1.1, 192.168.1.5"}, "1.1.1.1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			list, err := ParseTrustedProxies(tc.list)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for _, l := range tc.xff {
				r.Header.Add("X-Forwarded-For", l)
			}
			if got := clientIP(r, tc.trust, list, 0); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"", "  "} {
		if got, err := ParseTrustedProxies(v); err != nil || len(got) != 0 {
			t.Errorf("%q: got %v, %v; want empty", v, got, err)
		}
	}
	if got, err := ParseTrustedProxies(" 10.0.0.0/8 , 192.168.1.5,fd00::/8"); err != nil || len(got) != 3 {
		t.Errorf("valid list: got %v, %v", got, err)
	}
	for _, tc := range []struct{ value, bad string }{
		{"10.0.0.0/33", "10.0.0.0/33"},
		{"nonsense", "nonsense"},
		{"10.0.0.0/8,,", ""},
		{"fe80::1%eth0", "fe80::1%eth0"},
		{"10.0.0.0/8,bad", "bad"},
	} {
		_, err := ParseTrustedProxies(tc.value)
		want := `invalid TRUSTED_PROXIES entry "` + tc.bad + `"`
		if err == nil || err.Error() != want {
			t.Errorf("%q: got %v, want %s", tc.value, err, want)
		}
	}
}

func TestClientIPHops(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		hops  int
		xff   []string
		want  string
		trust bool
	}{
		{"N=1 last", 1, []string{"1.1.1.1, 2.2.2.2"}, "2.2.2.2", true},
		{"N=2", 2, []string{"1.1.1.1, 2.2.2.2"}, "1.1.1.1", true},
		{"N=2 forged left ignored", 2, []string{"9.9.9.9, 1.1.1.1, 2.2.2.2"}, "1.1.1.1", true},
		{"N=2 two lines joined", 2, []string{"1.1.1.1", "2.2.2.2"}, "1.1.1.1", true},
		{"N=2 line with commas then line", 2, []string{"9.9.9.9, 1.1.1.1", "2.2.2.2"}, "1.1.1.1", true},
		{"chain equals N", 3, []string{"1.1.1.1, 2.2.2.2, 3.3.3.3"}, "1.1.1.1", true},
		{"chain shorter than N", 3, []string{"1.1.1.1, 2.2.2.2"}, "10.0.0.1", true},
		{"missing header", 2, nil, "10.0.0.1", true},
		{"empty header", 2, []string{""}, "10.0.0.1", true},
		{"Nth garbage", 2, []string{"1.1.1.1, nonsense, 2.2.2.2"}, "10.0.0.1", true},
		{"Nth empty between commas", 2, []string{"1.1.1.1,,2.2.2.2"}, "10.0.0.1", true},
		{"Nth with port", 1, []string{"1.1.1.1:80"}, "10.0.0.1", true},
		{"Nth with zone", 1, []string{"fe80::1%eth0"}, "10.0.0.1", true},
		{"other entries not inspected", 2, []string{"garbage, 1.1.1.1, 2.2.2.2"}, "1.1.1.1", true},
		{"entry right of N garbage is fine", 2, []string{"1.1.1.1, 2.2.2.2"}, "1.1.1.1", true},
		{"ipv6", 2, []string{"2001:db8::1, 2.2.2.2"}, "2001:db8::1", true},
		{"ipv6 normalized", 1, []string{"2001:DB8:0:0::1"}, "2001:db8::1", true},
		{"mapped shown as ipv4", 1, []string{"::ffff:1.2.3.4"}, "1.2.3.4", true},
		{"trims entries", 2, []string{"  1.1.1.1  ,  2.2.2.2 "}, "1.1.1.1", true},
		{"trust disabled ignores hops", 1, []string{"1.1.1.1"}, "10.0.0.1", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = "10.0.0.1:5555"
			for _, l := range tc.xff {
				r.Header.Add("X-Forwarded-For", l)
			}
			if got := clientIP(r, tc.trust, nil, tc.hops); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClientIPHopsRemoteNotChecked(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.9:1"
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	if got := clientIP(r, true, nil, 1); got != "1.1.1.1" {
		t.Errorf("got %q, want 1.1.1.1", got)
	}
}

func TestParseTrustedProxyHops(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value string
		want  int
	}{{"", 0}, {"   ", 0}, {"1", 1}, {"2", 2}, {" 2 ", 2}, {"64", 64}} {
		if got, err := ParseTrustedProxyHops(tc.value); err != nil || got != tc.want {
			t.Errorf("%q: got %d, %v; want %d", tc.value, got, err, tc.want)
		}
	}
	for _, v := range []string{"0", "-1", "abc", "1.5", "2 3", "+", "65", "1000", "99999999999999999999"} {
		_, err := ParseTrustedProxyHops(v)
		want := `invalid TRUSTED_PROXY_HOPS "` + v + `"`
		if err == nil || err.Error() != want {
			t.Errorf("%q: got %v, want %s", v, err, want)
		}
	}
}
