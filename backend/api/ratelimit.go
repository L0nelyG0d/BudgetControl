package api

import (
	"budgetcontrol/httpx"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// loginMaxFailuresPerEmail and loginMaxFailuresPerIP are how many failed
	// logins are allowed per window for one email and for one client IP; the
	// next attempt is answered with 429. The IP limit is higher so shared
	// networks (office, campus, mobile) do not lock each other out.
	loginMaxFailuresPerEmail = 5
	loginMaxFailuresPerIP    = 20
	// loginWindow is how long a failure counts against its email and IP.
	loginWindow = 15 * time.Minute
)

// failureLimiter counts recent failures per key (in memory, single server).
// A key is blocked once it has limit failures in the last window.
type failureLimiter struct {
	limit    int
	mu       sync.Mutex
	failures map[string][]time.Time // oldest first
}

func newFailureLimiter(limit int) *failureLimiter {
	return &failureLimiter{limit: limit, failures: map[string][]time.Time{}}
}

// prune drops expired failures for key; callers hold mu.
func (l *failureLimiter) prune(key string, now time.Time) []time.Time {
	times := l.failures[key]
	i := 0
	for i < len(times) && !times[i].Add(loginWindow).After(now) {
		i++
	}
	times = times[i:]
	if len(times) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = times
	return times
}

// retryAfter returns how long key stays blocked, or 0 if it is not blocked.
func (l *failureLimiter) retryAfter(key string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	times := l.prune(key, now)
	if len(times) < l.limit {
		return 0
	}
	// Unblocked when the oldest counted failure leaves the window.
	return times[len(times)-l.limit].Add(loginWindow).Sub(now)
}

func (l *failureLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(key, now)
	l.failures[key] = append(l.failures[key], now)
}

func (l *failureLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// sweep removes every expired entry so the maps cannot grow without bound.
func (l *failureLimiter) sweep(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k := range l.failures {
		l.prune(k, now)
	}
}

// loginLimiter limits failed logins per lowercased email and per client IP.
type loginLimiter struct {
	byEmail, byIP *failureLimiter
	mu            sync.Mutex
	lastSweep     time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		byEmail: newFailureLimiter(loginMaxFailuresPerEmail),
		byIP:    newFailureLimiter(loginMaxFailuresPerIP),
	}
}

// clientIP is the client address used for the per-IP login limit.
//
// By default it is the host of the connection's remote address and
// X-Forwarded-For is ignored, because clients can forge it. With trustProxy
// (Deps.TrustedProxy, only behind a proxy that appends the real client IP)
// and no proxies list it is the last entry of the last X-Forwarded-For line,
// falling back to the remote address when the header is missing, empty or not
// a valid IP.
//
// With trustProxy and a non-empty proxies list (Deps.TrustedProxies) the
// header is only believed when the remote address is in the list. The lines
// are joined into one chain and walked from the right, skipping listed
// proxies; the first unlisted entry is the client. A malformed entry, an
// empty chain or an all-trusted chain falls back to the remote address.
func clientIP(r *http.Request, trustProxy bool, proxies []netip.Prefix) string {
	if trustProxy && len(proxies) == 0 {
		if lines := r.Header.Values("X-Forwarded-For"); len(lines) > 0 {
			last := lines[len(lines)-1]
			last = strings.TrimSpace(last[strings.LastIndex(last, ",")+1:])
			if addr, err := netip.ParseAddr(last); err == nil {
				return addr.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !trustProxy || len(proxies) == 0 {
		return host
	}
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	remote = remote.Unmap()
	if !inProxies(remote, proxies) {
		return remote.String()
	}
	var chain []string
	for _, line := range r.Header.Values("X-Forwarded-For") {
		chain = append(chain, strings.Split(line, ",")...)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			break
		}
		addr = addr.Unmap()
		if !inProxies(addr, proxies) {
			return addr.String()
		}
	}
	return remote.String()
}

func inProxies(addr netip.Addr, proxies []netip.Prefix) bool {
	for _, p := range proxies {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ParseTrustedProxies parses a comma-separated list of IPs and CIDRs
// (TRUSTED_PROXIES). An empty value gives an empty list. A bad entry
// returns an error naming it. IPv4-mapped IPv6 entries are stored as IPv4.
func ParseTrustedProxies(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var out []netip.Prefix
	for _, raw := range strings.Split(value, ",") {
		entry := strings.TrimSpace(raw)
		var p netip.Prefix
		var err error
		if strings.Contains(entry, "/") {
			p, err = netip.ParsePrefix(entry)
			if err == nil && p.Addr().Is4In6() {
				// Mapped prefix: shift to IPv4 (bits >= 96 only).
				if p.Bits() < 96 {
					err = errors.New("prefix too short")
				} else {
					p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
				}
			}
		} else {
			var a netip.Addr
			a, err = netip.ParseAddr(entry)
			if err == nil && a.Zone() != "" {
				err = errors.New("zone not allowed")
			}
			if err == nil {
				a = a.Unmap()
				p = netip.PrefixFrom(a, a.BitLen())
			}
		}
		if err != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry %q", entry)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// maybeSweep cleans expired entries at most once per window.
func (l *loginLimiter) maybeSweep(now time.Time) {
	l.mu.Lock()
	due := now.Sub(l.lastSweep) >= loginWindow
	if due {
		l.lastSweep = now
	}
	l.mu.Unlock()
	if due {
		l.byEmail.sweep(now)
		l.byIP.sweep(now)
	}
}

// check returns how long the email or IP is blocked (0 if neither is).
func (l *loginLimiter) check(email, ip string, now time.Time) time.Duration {
	l.maybeSweep(now)
	return max(l.byEmail.retryAfter(email, now), l.byIP.retryAfter(ip, now))
}

func (l *loginLimiter) fail(email, ip string, now time.Time) {
	l.byEmail.fail(email, now)
	l.byIP.fail(ip, now)
}

// succeed resets the email's counter; the IP's failures keep counting so one
// valid account cannot be used to wash away guessing against others.
func (l *loginLimiter) succeed(email string) { l.byEmail.reset(email) }

// writeTooManyAttempts answers 429 with Retry-After in whole seconds, rounded
// up. The response never depends on whether the email exists.
func writeTooManyAttempts(w http.ResponseWriter, wait time.Duration) {
	secs := int((wait + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	httpx.Error(w, http.StatusTooManyRequests, "too many login attempts, try again later")
}
