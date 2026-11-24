package security

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func testKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func TestKeyringRoundTripAndRotation(t *testing.T) {
	oldSpec := "v1:" + testKey()
	old, err := NewKeyring(oldSpec)
	if err != nil {
		t.Fatal(err)
	}
	ct, err := old.Encrypt("jane@example.com", "users.email")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("jane")) {
		t.Fatal("plaintext leaked into ciphertext")
	}
	// rotate: new key first, old key still decrypts
	rotated, err := NewKeyring("v2:" + testKey() + "," + oldSpec)
	if err != nil {
		t.Fatal(err)
	}
	got, err := rotated.Decrypt(ct, "users.email")
	if err != nil || got != "jane@example.com" {
		t.Fatalf("decrypt after rotation: %q %v", got, err)
	}
	if KeyID(ct) != "v1" {
		t.Fatalf("KeyID = %q", KeyID(ct))
	}
	ct2, _ := rotated.Encrypt("x", "a")
	if KeyID(ct2) != "v2" {
		t.Fatal("new values must use the active key")
	}
}

func TestKeyringAADBinding(t *testing.T) {
	k, _ := NewKeyring("v1:" + testKey())
	ct, _ := k.Encrypt("secret", "users.email")
	if _, err := k.Decrypt(ct, "users.phone"); err == nil {
		t.Fatal("ciphertext must not decrypt under another column's AAD")
	}
	ct[len(ct)-1] ^= 1
	if _, err := k.Decrypt(ct, "users.email"); err == nil {
		t.Fatal("tampered ciphertext must fail")
	}
	if v, err := k.Decrypt(nil, "x"); err != nil || v != "" {
		t.Fatal("nil decrypts to empty")
	}
}

func TestKeyringRejectsBadSpecs(t *testing.T) {
	for _, s := range []string{"", "v1", "v1:notbase64!", "v1:" + base64.StdEncoding.EncodeToString([]byte("short"))} {
		if _, err := NewKeyring(s); err == nil {
			t.Errorf("expected error for %q", s)
		}
	}
}

func TestBlindIndexDeterministicAndScoped(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	a := BlindIndex(key, "users.email", NormalizeEmail(" Jane@Example.com "))
	b := BlindIndex(key, "users.email", "jane@example.com")
	c := BlindIndex(key, "users.phone", "jane@example.com")
	if !bytes.Equal(a, b) {
		t.Fatal("same input must give same index")
	}
	if bytes.Equal(a, c) {
		t.Fatal("different scopes must differ")
	}
}

func TestTokensAndOTP(t *testing.T) {
	if len(RandomToken(32)) != 43 {
		t.Fatal("32 bytes -> 43 base64url chars")
	}
	for i := 0; i < 50; i++ {
		c := OTPCode(6)
		if len(c) != 6 || strings.Trim(c, "0123456789") != "" {
			t.Fatalf("bad otp %q", c)
		}
	}
	key := []byte(strings.Repeat("k", 32))
	if bytes.Equal(HashOTP(key, "login", "t1", "123456"), HashOTP(key, "login", "t2", "123456")) {
		t.Fatal("otp hash must bind target")
	}
}

func TestTruncateIPAndMasks(t *testing.T) {
	cases := map[string]string{
		"203.0.113.77":        "203.0.113.0",
		"2001:db8:abcd:12::1": "2001:db8:abcd::",
		"not-an-ip":           "",
	}
	for in, want := range cases {
		if got := TruncateIP(in); got != want {
			t.Errorf("TruncateIP(%q)=%q want %q", in, got, want)
		}
	}
	if MaskEmail("jane@example.com") != "j***@example.com" {
		t.Fatal("MaskEmail")
	}
	if NormalizePhone(" +44 (0)7700-900123 ") != "+4407700900123" {
		t.Fatal("NormalizePhone")
	}
	if MaskPhone("+447700900123") != "+44********23" {
		t.Fatalf("MaskPhone %s", MaskPhone("+447700900123"))
	}
}
