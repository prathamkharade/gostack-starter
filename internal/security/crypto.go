// Package security holds all cryptographic primitives: field encryption for
// personal data (AES-256-GCM with key rotation), blind indexes for searching
// encrypted values, password hashing (argon2id), random tokens and OTP codes.
//
// Rule: personal data (email, phone, name, address, free-text about a person)
// is stored ONLY as ciphertext (Keyring.Encrypt) plus, when it must be looked
// up, a blind index (BlindIndex). See docs/compliance/data-storage.md.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

var ErrDecrypt = errors.New("security: cannot decrypt value")

// Keyring encrypts with the first (active) key and decrypts with any key, so
// keys can be rotated: add a new key in front, re-encrypt in the background,
// then remove the old key.
type Keyring struct {
	activeID string
	aeads    map[string]cipher.AEAD
}

// NewKeyring parses "v2:base64key,v1:base64key". Each key must be 32 bytes.
func NewKeyring(spec string) (*Keyring, error) {
	kr := &Keyring{aeads: map[string]cipher.AEAD{}}
	for i, part := range strings.Split(spec, ",") {
		id, b64, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok || id == "" || len(id) > 16 {
			return nil, fmt.Errorf("security: bad key spec at position %d (want id:base64)", i)
		}
		key, err := base64.StdEncoding.DecodeString(b64)
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("security: key %q must be 32 bytes base64", id)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		if _, dup := kr.aeads[id]; dup {
			return nil, fmt.Errorf("security: duplicate key id %q", id)
		}
		kr.aeads[id] = aead
		if i == 0 {
			kr.activeID = id
		}
	}
	if kr.activeID == "" {
		return nil, errors.New("security: no encryption keys")
	}
	return kr, nil
}

// Encrypt returns [len(id)][id][nonce][ciphertext|tag]. The aad ("users.email")
// binds the ciphertext to its column so values cannot be swapped between fields.
// Empty plaintext encrypts to nil (store NULL).
func (k *Keyring) Encrypt(plaintext, aad string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	aead := k.aeads[k.activeID]
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(k.activeID)+len(nonce)+len(plaintext)+aead.Overhead())
	out = append(out, byte(len(k.activeID)))
	out = append(out, k.activeID...)
	out = append(out, nonce...)
	return aead.Seal(out, nonce, []byte(plaintext), []byte(aad)), nil
}

// Decrypt reverses Encrypt. nil/empty input returns "".
func (k *Keyring) Decrypt(blob []byte, aad string) (string, error) {
	if len(blob) == 0 {
		return "", nil
	}
	idLen := int(blob[0])
	if len(blob) < 1+idLen {
		return "", ErrDecrypt
	}
	aead, ok := k.aeads[string(blob[1:1+idLen])]
	if !ok {
		return "", ErrDecrypt
	}
	rest := blob[1+idLen:]
	if len(rest) < aead.NonceSize()+aead.Overhead() {
		return "", ErrDecrypt
	}
	pt, err := aead.Open(nil, rest[:aead.NonceSize()], rest[aead.NonceSize():], []byte(aad))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(pt), nil
}

// MustDecrypt is for templates/exports where a decrypt failure should show "".
func (k *Keyring) MustDecrypt(blob []byte, aad string) string {
	s, _ := k.Decrypt(blob, aad)
	return s
}

// KeyID returns the key id used for a blob (for rotation jobs).
func KeyID(blob []byte) string {
	if len(blob) == 0 || len(blob) < 1+int(blob[0]) {
		return ""
	}
	return string(blob[1 : 1+int(blob[0])])
}

// ActiveKeyID is the id new values are encrypted with.
func (k *Keyring) ActiveKeyID() string { return k.activeID }

// BlindIndex is a keyed hash used for equality lookups on encrypted columns
// (e.g. find user by email) without storing the value in clear text.
// "scope" separates indexes of different columns.
func BlindIndex(key []byte, scope, normalized string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(scope))
	m.Write([]byte{0})
	m.Write([]byte(normalized))
	return m.Sum(nil)
}

// NormalizeEmail lower-cases and trims. (No gmail dot-stripping: it is lossy.)
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// NormalizePhone keeps a leading + and digits only (E.164-ish).
func NormalizePhone(s string) string {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		if (r == '+' && i == 0) || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// MaskEmail returns "j***@example.com" for display in logs/UI confirmations.
func MaskEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return "***"
	}
	return local[:1] + "***@" + domain
}

// MaskPhone returns "+44*******89".
func MaskPhone(p string) string {
	if len(p) <= 5 {
		return "***"
	}
	return p[:3] + strings.Repeat("*", len(p)-5) + p[len(p)-2:]
}
