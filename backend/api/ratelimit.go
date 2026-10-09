package api

import (
	"budgetcontrol/httpx"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// loginMaxFailures is how many failed logins are allowed per window; the
	// next attempt is answered with 429.
	loginMaxFailures = 5
	// loginWindow is how long a failure counts against its email and IP.
	loginWindow = 15 * time.Minute
)

// failureLimiter counts recent failures per key (in memory, single server).
// A key is blocked once it has loginMaxFailures failures in the last window.
type failureLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time // oldest first
}

func newFailureLimiter() *failureLimiter {
	return &failureLimiter{failures: map[string][]time.Time{}}
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
	if len(times) < loginMaxFailures {
		return 0
	}
	// Unblocked when the oldest counted failure leaves the window.
	return times[len(times)-loginMaxFailures].Add(loginWindow).Sub(now)
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
	return &loginLimiter{byEmail: newFailureLimiter(), byIP: newFailureLimiter()}
}

// clientIP is the client address used for the per-IP login limit.
//
// By default it is the host of the connection's remote address and
// X-Forwarded-For is ignored, because clients can forge it. With trustProxy
// (Deps.TrustedProxy, only behind a proxy that appends the real client IP)
// it is the last entry of the last X-Forwarded-For line, falling back to the
// remote address when the header is missing, empty or not a valid IP.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
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
		return r.RemoteAddr
	}
	return host
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
