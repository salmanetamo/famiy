package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params are the argon2id cost parameters. Memory is in KiB.
type Params struct {
	Memory  uint32
	Time    uint32
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

// DefaultParams follows OWASP's Password Storage Cheat Sheet minimum
// (m=19456 KiB, t=2, p=1), checked 2026-11-03. Re-check before changing.
var DefaultParams = Params{Memory: 19456, Time: 2, Threads: 1, KeyLen: 32, SaltLen: 16}

const maxStoredMemory = 1 << 20 // 1 GiB: refuse absurd parameters read from a stored hash

// Hasher hashes and verifies passwords. A semaphore caps concurrent hashes so a
// burst of logins cannot exhaust memory on a small VM.
type Hasher struct {
	params Params
	sem    chan struct{}
	dummy  string // a valid hash of a random password, for unknown-user timing
}

func NewHasher(p Params, maxConcurrent int) (*Hasher, error) {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	h := &Hasher{params: p, sem: make(chan struct{}, maxConcurrent)}
	junk := make([]byte, 16)
	if _, err := rand.Read(junk); err != nil {
		return nil, fmt.Errorf("random bytes: %w", err)
	}
	d, err := h.Hash(context.Background(), base64.RawStdEncoding.EncodeToString(junk))
	if err != nil {
		return nil, fmt.Errorf("precompute dummy hash: %w", err)
	}
	h.dummy = d
	return h, nil
}

func (h *Hasher) acquire(ctx context.Context) (release func(), err error) {
	select {
	case h.sem <- struct{}{}:
		return func() { <-h.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Hash returns a PHC-format argon2id string with a fresh random salt.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	release, err := h.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	salt := make([]byte, h.params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, h.params.Time, h.params.Memory, h.params.Threads, h.params.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.params.Memory, h.params.Time, h.params.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// Verify checks password against an encoded hash using the parameters stored in
// the hash itself, in constant time.
func (h *Hasher) Verify(ctx context.Context, password, encoded string) (bool, error) {
	p, salt, want, err := decode(encoded)
	if err != nil {
		return false, err
	}
	release, err := h.acquire(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	got := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// VerifyDummy spends the same time as a real verification, so a login for an
// unknown email is not distinguishable by latency.
func (h *Hasher) VerifyDummy(ctx context.Context, password string) error {
	_, err := h.Verify(ctx, password, h.dummy)
	return err
}

var errBadHash = errors.New("malformed password hash")

func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, errBadHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Params{}, nil, nil, errBadHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return Params{}, nil, nil, errBadHash
	}
	if p.Memory == 0 || p.Memory > maxStoredMemory || p.Time == 0 || p.Threads == 0 {
		return Params{}, nil, nil, errBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return Params{}, nil, nil, errBadHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return Params{}, nil, nil, errBadHash
	}
	return p, salt, key, nil
}
