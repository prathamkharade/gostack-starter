package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters follow the OWASP Password Storage Cheat Sheet minimum
// (m=19 MiB, t=2, p=1). This keeps memory low on small servers. Raise Memory
// when you move to bigger machines; NeedsRehash upgrades hashes on next login.
type Argon2Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	KeyLen  uint32
	SaltLen int
}

var DefaultArgon2 = Argon2Params{Memory: 19 * 1024, Time: 2, Threads: 1, KeyLen: 32, SaltLen: 16}

var ErrBadHash = errors.New("security: invalid password hash")

// HashPassword returns a PHC string: $argon2id$v=19$m=19456,t=2,p=1$salt$hash
func HashPassword(pw string) (string, error) {
	p := DefaultArgon2
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(pw), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.Memory, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(h)), nil
}

// VerifyPassword compares in constant time.
func VerifyPassword(pw, encoded string) (bool, error) {
	p, salt, want, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(pw), salt, p.Time, p.Memory, p.Threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NeedsRehash reports whether the hash uses weaker params than DefaultArgon2.
func NeedsRehash(encoded string) bool {
	p, _, _, err := decodeHash(encoded)
	return err != nil || p.Memory < DefaultArgon2.Memory || p.Time < DefaultArgon2.Time
}

// DummyVerify burns the same time as a real verify; call it when the user does
// not exist so response timing does not reveal which emails are registered.
func DummyVerify(pw string) {
	_, _ = VerifyPassword(pw, dummyHash)
}

var dummyHash, _ = HashPassword("dummy-password-for-timing")

func decodeHash(s string) (Argon2Params, []byte, []byte, error) {
	var p Argon2Params
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return p, nil, nil, ErrBadHash
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return p, nil, nil, ErrBadHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return p, nil, nil, ErrBadHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, ErrBadHash
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) < 16 {
		return p, nil, nil, ErrBadHash
	}
	return p, salt, hash, nil
}
