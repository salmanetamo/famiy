package identity

import (
	"context"
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

func TestAttemptLimiter(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	l := newAttemptLimiter(clk.now)

	for i := 0; i < l.maxPerKey; i++ {
		if l.blocked("a@x.com", "1.1.1.1") {
			t.Fatalf("blocked after only %d failures", i)
		}
		l.fail("a@x.com", "1.1.1.1")
	}
	if !l.blocked("a@x.com", "1.1.1.1") {
		t.Fatal("not blocked after reaching the per-key limit")
	}
	if l.blocked("b@x.com", "1.1.1.1") {
		t.Error("a different email from the same address must not be blocked yet")
	}
	if l.blocked("a@x.com", "2.2.2.2") {
		t.Error("the same email from a different address must not be blocked")
	}

	clk.t = clk.t.Add(l.window)
	if l.blocked("a@x.com", "1.1.1.1") {
		t.Error("still blocked after the window elapsed")
	}

	// Per-address ceiling: many different emails from one address.
	for i := 0; i < l.maxPerIP; i++ {
		l.fail(string(rune('a'+i%26))+strings.Repeat("z", i), "9.9.9.9")
	}
	if !l.blocked("fresh@x.com", "9.9.9.9") {
		t.Error("address-wide limit did not trigger")
	}

	// A success clears the (email, address) counter but not the address one.
	l2 := newAttemptLimiter(clk.now)
	l2.fail("c@x.com", "3.3.3.3")
	l2.reset("c@x.com", "3.3.3.3")
	if l2.byKey[key("c@x.com", "3.3.3.3")] != nil {
		t.Error("reset left the per-key counter in place")
	}
	if l2.byIP["3.3.3.3"] == nil {
		t.Error("reset must not clear the per-address counter")
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
