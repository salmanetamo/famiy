package identity

import (
	"sync"
	"time"
)

// attemptLimiter caps attempts per (email, address) and per address within a
// fixed window. It is in-memory, which is correct for the single instance this
// app runs as (ADR-004); scale-out would need shared state.
//
// An attempt is RESERVED before the slow password check, not recorded after it.
// Recording afterwards lets a burst of parallel requests all see "no attempts
// yet" and each get a guess.
type attemptLimiter struct {
	mu        sync.Mutex
	now       func() time.Time
	window    time.Duration
	maxPerKey int
	maxPerIP  int
	byKey     map[string]*bucket
	byIP      map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	count int
	start time.Time
}

// maxTracked bounds each map. When it is full an arbitrary entry is evicted, so
// memory and per-call work stay constant however many addresses an attacker uses.
const maxTracked = 10000

func newAttemptLimiter(now func() time.Time, window time.Duration, maxPerKey, maxPerIP int) *attemptLimiter {
	return &attemptLimiter{
		now:       now,
		window:    window,
		maxPerKey: maxPerKey,
		maxPerIP:  maxPerIP,
		byKey:     map[string]*bucket{},
		byIP:      map[string]*bucket{},
	}
}

func key(email, ip string) string { return email + "|" + ip }

// reserve counts one attempt and reports whether it may proceed. A refused
// attempt is not counted, so a blocked client does not extend its own lockout.
func (l *attemptLimiter) reserve(email, ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep()
	k := key(email, ip)
	if l.over(l.byKey, k, l.maxPerKey) || l.over(l.byIP, ip, l.maxPerIP) {
		return false
	}
	l.bump(l.byKey, k)
	l.bump(l.byIP, ip)
	return true
}

// release is called after a successful login: it forgets the (email, address)
// counter and hands back this attempt's slot in the per-address counter. Earlier
// failures from the address stay counted, so one valid account cannot be used to
// launder guesses against others.
func (l *attemptLimiter) release(email, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.byKey, key(email, ip))
	if b := l.byIP[ip]; b != nil && b.count > 0 {
		b.count--
	}
}

func (l *attemptLimiter) over(m map[string]*bucket, k string, max int) bool {
	b, ok := m[k]
	if !ok || l.now().Sub(b.start) >= l.window {
		return false
	}
	return b.count >= max
}

func (l *attemptLimiter) bump(m map[string]*bucket, k string) {
	now := l.now()
	b, ok := m[k]
	if ok && now.Sub(b.start) < l.window {
		b.count++
		return
	}
	if !ok && len(m) >= maxTracked {
		for evict := range m { // map iteration order is arbitrary: O(1) eviction
			delete(m, evict)
			break
		}
	}
	m[k] = &bucket{count: 1, start: now}
}

// sweep drops expired buckets, at most once per quarter-window.
func (l *attemptLimiter) sweep() {
	now := l.now()
	if now.Sub(l.lastSweep) < l.window/4 {
		return
	}
	l.lastSweep = now
	for _, m := range []map[string]*bucket{l.byKey, l.byIP} {
		for k, b := range m {
			if now.Sub(b.start) >= l.window {
				delete(m, k)
			}
		}
	}
}
