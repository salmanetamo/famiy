package identity

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

var fastParams = Params{Memory: 8, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"Ada@Example.COM", "ada@example.com", true},
		{"  ada@example.com  ", "ada@example.com", true},
		{"a@b", "a@b", true},
		{"", "", false},
		{"jürgen@example.com", "", false}, // non-ASCII local part
		{"ada@bücher.example", "", false}, // non-ASCII domain
		{"no-at-sign", "", false},
		{"Ada <ada@example.com>", "", false}, // display names are not allowed
		{"ada@example.com, bob@example.com", "", false},
		{"ada @example.com", "", false},
		{strings.Repeat("a", 250) + "@example.com", "", false}, // > 254
	}
	for _, tc := range tests {
		got, err := NormalizeEmail(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q ok=%v", tc.in, got, err, tc.want, tc.ok)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name string
		pw   string
		ok   bool
	}{
		{"too short", strings.Repeat("a", MinPasswordLen-1), false},
		{"minimum", strings.Repeat("a", MinPasswordLen), true},
		{"maximum", strings.Repeat("a", MaxPasswordLen), true},
		{"too long", strings.Repeat("a", MaxPasswordLen+1), false},
		{"counts characters not bytes", strings.Repeat("é", MinPasswordLen), true},
		{"no composition rules", "aaaaaaaaaaaa", true},
	}
	for _, tc := range tests {
		if err := ValidatePassword(tc.pw); (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}

func TestHasher(t *testing.T) {
	ctx := context.Background()
	h, err := NewHasher(fastParams, 2)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := h.Hash(ctx, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "$argon2id$v=19$m=8,t=1,p=1$") {
		t.Fatalf("unexpected encoding %q", enc)
	}
	if strings.Contains(enc, "correct horse") {
		t.Fatal("hash contains the password")
	}

	if ok, err := h.Verify(ctx, "correct horse battery", enc); err != nil || !ok {
		t.Fatalf("right password: ok=%v err=%v", ok, err)
	}
	if ok, err := h.Verify(ctx, "wrong horse battery", enc); err != nil || ok {
		t.Fatalf("wrong password: ok=%v err=%v", ok, err)
	}

	enc2, _ := h.Hash(ctx, "correct horse battery")
	if enc == enc2 {
		t.Fatal("two hashes of the same password are identical: salt is not random")
	}
	if err := h.VerifyDummy(ctx, "anything"); err != nil {
		t.Fatalf("dummy verify: %v", err)
	}
}

func TestVerifyRejectsMalformedAndHostileHashes(t *testing.T) {
	h, _ := NewHasher(fastParams, 1)
	for name, enc := range map[string]string{
		"empty":            "",
		"wrong algorithm":  "$argon2i$v=19$m=8,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5a2V5",
		"wrong version":    "$argon2id$v=18$m=8,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5a2V5",
		"bad params":       "$argon2id$v=19$m=x,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5a2V5",
		"zero memory":      "$argon2id$v=19$m=0,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5a2V5",
		"absurd memory":    "$argon2id$v=19$m=4294967295,t=1,p=1$c2FsdHNhbHQ$a2V5a2V5a2V5",
		"bad salt base64":  "$argon2id$v=19$m=8,t=1,p=1$!!!$a2V5a2V5a2V5",
		"missing sections": "$argon2id$v=19$m=8,t=1,p=1",
	} {
		if ok, err := h.Verify(context.Background(), "pw", enc); err == nil || ok {
			t.Errorf("%s: ok=%v err=%v, want an error", name, ok, err)
		}
	}
}

func TestHasherHonoursContextWhenSaturated(t *testing.T) {
	h, _ := NewHasher(fastParams, 1)
	h.sem <- struct{}{} // occupy the only slot
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := h.Hash(ctx, "x"); err == nil {
		t.Fatal("expected context error while the semaphore is full")
	}
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func testLimiter(clk *fakeClock) *attemptLimiter {
	return newAttemptLimiter(clk.now, loginWindow, maxLoginAttemptsPerKey, maxLoginAttemptsPerIP)
}

func TestAttemptLimiter(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	l := testLimiter(clk)

	for i := 0; i < maxLoginAttemptsPerKey; i++ {
		if !l.reserve("a@x.com", "1.1.1.1") {
			t.Fatalf("attempt %d refused before the limit", i+1)
		}
	}
	if l.reserve("a@x.com", "1.1.1.1") {
		t.Fatal("attempt past the per-key limit was allowed")
	}
	if !l.reserve("b@x.com", "1.1.1.1") {
		t.Error("a different email from the same address must still be allowed")
	}
	if !l.reserve("a@x.com", "2.2.2.2") {
		t.Error("the same email from a different address must still be allowed")
	}

	clk.t = clk.t.Add(loginWindow)
	if !l.reserve("a@x.com", "1.1.1.1") {
		t.Error("still refused after the window elapsed")
	}
}

func TestAttemptLimiterPerAddressCeiling(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	l := testLimiter(clk)
	for i := 0; i < maxLoginAttemptsPerIP; i++ {
		if !l.reserve(fmt.Sprintf("user%d@x.com", i), "9.9.9.9") {
			t.Fatalf("attempt %d refused before the address limit", i+1)
		}
	}
	if l.reserve("fresh@x.com", "9.9.9.9") {
		t.Error("address-wide limit did not trigger")
	}
}

func TestAttemptLimiterRefusedAttemptsAreNotCounted(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	l := testLimiter(clk)
	for i := 0; i < maxLoginAttemptsPerKey; i++ {
		l.reserve("a@x.com", "1.1.1.1")
	}
	for i := 0; i < 100; i++ {
		l.reserve("a@x.com", "1.1.1.1") // hammering while locked out
	}
	// The lockout must end one window after it began, not be extended by the hammering.
	clk.t = clk.t.Add(loginWindow)
	if !l.reserve("a@x.com", "1.1.1.1") {
		t.Fatal("refused attempts extended the lockout")
	}
}

func TestAttemptLimiterRelease(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	l := testLimiter(clk)
	for i := 0; i < maxLoginAttemptsPerKey-1; i++ {
		l.reserve("a@x.com", "3.3.3.3") // four failures
	}
	l.reserve("a@x.com", "3.3.3.3") // the successful login...
	l.release("a@x.com", "3.3.3.3") // ...clears the pair counter

	if l.byKey[key("a@x.com", "3.3.3.3")] != nil {
		t.Error("release left the per-key counter in place")
	}
	// The address still remembers the four failures, but not the success.
	if got := l.byIP["3.3.3.3"].count; got != maxLoginAttemptsPerKey-1 {
		t.Errorf("per-address count = %d, want %d", got, maxLoginAttemptsPerKey-1)
	}
}

func TestAttemptLimiterMemoryIsBounded(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	l := testLimiter(clk)
	for i := 0; i < maxTracked*3; i++ {
		l.reserve(fmt.Sprintf("u%d@x.com", i), fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255))
	}
	if len(l.byKey) > maxTracked || len(l.byIP) > maxTracked {
		t.Fatalf("maps grew to %d and %d entries, cap is %d", len(l.byKey), len(l.byIP), maxTracked)
	}

	// Expired buckets are swept even when the maps are not full.
	clk.t = clk.t.Add(2 * loginWindow)
	l.reserve("late@x.com", "8.8.8.8")
	if len(l.byKey) != 1 || len(l.byIP) != 1 {
		t.Errorf("after the window: %d and %d entries remain, want 1 and 1", len(l.byKey), len(l.byIP))
	}
}

func TestClientAddr(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	tests := []struct {
		name, remote, xff string
		trusted           []netip.Prefix
		want              string
	}{
		{"direct client", "203.0.113.5:4444", "", nil, "203.0.113.5"},
		{"untrusted peer cannot spoof", "203.0.113.5:4444", "1.2.3.4", trusted, "203.0.113.5"},
		{"trusted proxy: use forwarded client", "10.0.0.2:5555", "198.51.100.7", trusted, "198.51.100.7"},
		{"client-supplied prefix is ignored (right-most wins)", "10.0.0.2:5555", "6.6.6.6, 198.51.100.7", trusted, "198.51.100.7"},
		{"chain of trusted proxies is skipped", "10.0.0.2:5555", "198.51.100.7, 10.0.0.9", trusted, "198.51.100.7"},
		{"garbage header falls back to peer", "10.0.0.2:5555", "not-an-ip", trusted, "10.0.0.2"},
		{"trusted peer without header", "10.0.0.2:5555", "", trusted, "10.0.0.2"},
		{"ipv6 collapses to its /64", "[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:4444", "", nil, "2001:db8:1:2::"},
		{"another host in the same /64 shares the bucket", "[2001:db8:1:2:1111:2222:3333:4444]:4444", "", nil, "2001:db8:1:2::"},
		{"ipv4-mapped ipv6 is the ipv4 address", "[::ffff:203.0.113.5]:4444", "", nil, "203.0.113.5"},
		{"zone id is dropped", "[fe80::1%eth0]:4444", "", nil, "fe80::"},
		{"forwarded ipv6 client is canonicalised", "10.0.0.2:5555", "2001:db8:9:9:1:2:3:4", trusted, "2001:db8:9:9::"},
	}
	for _, tc := range tests {
		r := &http.Request{RemoteAddr: tc.remote, Header: http.Header{}}
		if tc.xff != "" {
			r.Header.Set("X-Forwarded-For", tc.xff)
		}
		if got := ClientAddr(r, tc.trusted); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
