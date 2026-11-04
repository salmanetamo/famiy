package identity

import (
	"sync"
	"time"
)

// attemptLimiter counts FAILED logins in a fixed window, per (email, address)
// and per address alone. It is in-memory, which is correct for the single
// instance this app runs as (ADR-004); scale-out would need shared state.
type attemptLimiter struct {
	mu        sync.Mutex
	now       func() time.Time
	window    time.Duration
	maxPerKey int
	maxPerIP  int
	byKey     map[string]*bucket
	byIP      map[string]*bucket
}

type bucket struct {
	count int
	start time.Time
}

const pruneAbove = 1000 // prune expired buckets once a map grows past this

func newAttemptLimiter(now func() time.Time) *attemptLimiter {
	return &attemptLimiter{
		now:       now,
		window:    15 * time.Minute,
		maxPerKey: 5,
		maxPerIP:  30,
		byKey:     map[string]*bucket{},
		byIP:      map[string]*bucket{},
	}
}

func key(email, ip string) string { return email + "|" + ip }

// blocked reports whether a further attempt should be refused.
func (l *attemptLimiter) blocked(email, ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.over(l.byKey, key(email, ip), l.maxPerKey) || l.over(l.byIP, ip, l.maxPerIP)
}

func (l *attemptLimiter) over(m map[string]*bucket, k string, max int) bool {
	b, ok := m[k]
	if !ok || l.now().Sub(b.start) >= l.window {
		return false
	}
	return b.count >= max
}

// fail records a failed attempt.
func (l *attemptLimiter) fail(email, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bump(l.byKey, key(email, ip))
	l.bump(l.byIP, ip)
}

func (l *attemptLimiter) bump(m map[string]*bucket, k string) {
	now := l.now()
	if len(m) > pruneAbove {
		for kk, b := range m {
			if now.Sub(b.start) >= l.window {
				delete(m, kk)
			}
		}
	}
	b, ok := m[k]
	if !ok || now.Sub(b.start) >= l.window {
		m[k] = &bucket{count: 1, start: now}
		return
	}
	b.count++
}

// reset clears the (email, address) counter after a successful login. The
// per-address counter is left alone so one valid account cannot be used to
// launder failures against others.
func (l *attemptLimiter) reset(email, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byKey, key(email, ip))
}
