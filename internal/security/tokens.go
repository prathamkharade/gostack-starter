package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
)

// RandomToken returns n random bytes, base64url encoded (no padding).
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is a fast hash for high-entropy secrets (session/refresh tokens,
// magic links). Low-entropy secrets (passwords) use HashPassword instead.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// EqualHash compares two hashes in constant time.
func EqualHash(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }

// OTPCode returns a uniformly random numeric code with the given digits.
func OTPCode(digits int) string {
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%0*d", digits, n)
}

// HashOTP binds a short code to its purpose and target so a code issued for
// one email cannot be used for another. Uses HMAC via BlindIndex.
func HashOTP(key []byte, purpose, targetHash, code string) []byte {
	return BlindIndex(key, "otp:"+purpose+":"+targetHash, code)
}

// TruncateIP pseudonymises an IP (IPv4 /24, IPv6 /48) before storage so logs
// keep security value without storing a full identifier (GDPR data minimisation).
func TruncateIP(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	if v4 := parsed.To4(); v4 != nil {
		return net.IP(v4.Mask(net.CIDRMask(24, 32))).String()
	}
	return parsed.Mask(net.CIDRMask(48, 128)).String()
}
