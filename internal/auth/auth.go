// Package auth implements frictionless, secure sign-in:
//
//   - Passwordless email codes + magic links (default; sign-up and sign-in are one flow)
//   - Optional password (argon2id) for users who want one
//   - Optional phone verification by SMS
//   - Short-lived access JWT (15 min) + rotating refresh token (30 days) with
//     reuse detection: a stolen, already-used refresh token revokes the whole family.
//
// HTTP wiring lives in internal/web (cookies, middleware, handlers).
package auth

import (
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/config"
	"app/internal/jobs"
	"app/internal/security"
)

var (
	ErrInvalidCode   = errors.New("auth: invalid or expired code")
	ErrRateLimited   = errors.New("auth: too many requests, try again later")
	ErrInvalidLogin  = errors.New("auth: invalid email or password")
	ErrSessionEnded  = errors.New("auth: session expired")
	ErrTokenReuse    = errors.New("auth: refresh token reuse detected")
	ErrWeakPassword  = errors.New("auth: password must be at least 10 characters")
	ErrAccountLocked = errors.New("auth: account locked")
)

// Versions of legal documents. Bump when the text changes materially; the
// consents table keeps proof of which version each user accepted.
const (
	TermsVersion   = "2026-01-01"
	PrivacyVersion = "2026-01-01"
)

const (
	otpDigits      = 6
	otpTTL         = 10 * time.Minute
	otpMaxAttempts = 5
	otpMaxPerHour  = 5
	refreshGrace   = 10 * time.Second // tolerate parallel requests racing to refresh
)

type Service struct {
	pool  *pgxpool.Pool
	keys  *security.Keyring
	cfg   *config.Config
	queue *jobs.Queue
	log   *slog.Logger
}

func New(pool *pgxpool.Pool, keys *security.Keyring, cfg *config.Config, q *jobs.Queue, log *slog.Logger) *Service {
	return &Service{pool: pool, keys: keys, cfg: cfg, queue: q, log: log}
}

// Principal is the authenticated identity carried by the access token.
type Principal struct {
	UserID    string
	SessionID string
	Role      string
}

func (p *Principal) IsAdmin() bool { return p != nil && p.Role == "admin" }

// User is the decrypted view of a users row.
type User struct {
	ID            string
	Email         string
	Name          string
	Phone         string
	Role          string
	Status        string
	Locale        string
	EmailVerified bool
	PhoneVerified bool
	HasPassword   bool
	CreatedAt     time.Time
	LastLoginAt   *time.Time
}

func (s *Service) emailIndex(email string) []byte {
	return security.BlindIndex(s.cfg.BlindIndexKey, "users.email", security.NormalizeEmail(email))
}

func (s *Service) phoneIndex(phone string) []byte {
	return security.BlindIndex(s.cfg.BlindIndexKey, "users.phone", security.NormalizePhone(phone))
}
