package auth

import (
	"context"
	"time"

	"app/internal/audit"
	"app/internal/notify"
	"app/internal/security"
)

const userCols = `id, email_enc, name_enc, phone_enc, role, status, locale,
	email_verified_at IS NOT NULL, phone_verified_at IS NOT NULL, password_hash IS NOT NULL, created_at, last_login_at`

func (s *Service) scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var email, name, phone []byte
	if err := row.Scan(&u.ID, &email, &name, &phone, &u.Role, &u.Status, &u.Locale,
		&u.EmailVerified, &u.PhoneVerified, &u.HasPassword, &u.CreatedAt, &u.LastLoginAt); err != nil {
		return nil, err
	}
	u.Email = s.keys.MustDecrypt(email, "users.email")
	u.Name = s.keys.MustDecrypt(name, "users.name")
	u.Phone = s.keys.MustDecrypt(phone, "users.phone")
	return &u, nil
}

func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
	return s.scanUser(s.pool.QueryRow(ctx, "SELECT "+userCols+" FROM users WHERE id = $1", id))
}

func (s *Service) UserByEmail(ctx context.Context, email string) (*User, error) {
	return s.scanUser(s.pool.QueryRow(ctx, "SELECT "+userCols+" FROM users WHERE email_bidx = $1", s.emailIndex(email)))
}

// UpdateProfile changes non-identifying profile fields.
func (s *Service) UpdateProfile(ctx context.Context, userID, name, locale string) error {
	enc, err := s.keys.Encrypt(name, "users.name")
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, "UPDATE users SET name_enc = $2, locale = $3, updated_at = now() WHERE id = $1", userID, enc, locale)
	return err
}

// SetPassword lets passwordless users add a password (optional).
func (s *Service) SetPassword(ctx context.Context, userID, pw, ip string) error {
	if len(pw) < 10 || len(pw) > 256 {
		return ErrWeakPassword
	}
	h, err := security.HashPassword(pw)
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, "UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1", userID, h); err != nil {
		return err
	}
	audit.Log(ctx, s.pool, userID, "account.password_set", "", nil, ip)
	s.notifySecurity(ctx, userID, "The password for your account was changed.")
	return nil
}

// LoginPassword verifies email+password in constant-ish time.
func (s *Service) LoginPassword(ctx context.Context, email, pw, ip string) (*User, error) {
	var id, hash, status string
	err := s.pool.QueryRow(ctx, "SELECT id, COALESCE(password_hash, ''), status FROM users WHERE email_bidx = $1",
		s.emailIndex(email)).Scan(&id, &hash, &status)
	if err != nil || hash == "" {
		security.DummyVerify(pw) // same timing whether or not the user exists
		audit.Log(ctx, s.pool, "", "auth.login_failed", "", nil, ip)
		return nil, ErrInvalidLogin
	}
	ok, err := security.VerifyPassword(pw, hash)
	if err != nil || !ok {
		audit.Log(ctx, s.pool, id, "auth.login_failed", "", nil, ip)
		return nil, ErrInvalidLogin
	}
	if status != "active" {
		return nil, ErrAccountLocked
	}
	if security.NeedsRehash(hash) {
		if nh, err := security.HashPassword(pw); err == nil {
			_, _ = s.pool.Exec(ctx, "UPDATE users SET password_hash = $2 WHERE id = $1", id, nh)
		}
	}
	return s.GetUser(ctx, id)
}

// Consent records a granular consent decision (e.g. marketing_email).
func (s *Service) Consent(ctx context.Context, userID, purpose, version string, granted bool, ip string) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO consents (user_id, purpose, version, granted) VALUES ($1, $2, $3, $4)",
		userID, purpose, version, granted)
	audit.Log(ctx, s.pool, userID, "consent.update", purpose, map[string]any{"granted": granted}, ip)
	return err
}

// ConsentState returns the latest decision per purpose.
func (s *Service) ConsentState(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (purpose) purpose, granted FROM consents
		WHERE user_id = $1 ORDER BY purpose, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		var g bool
		if err := rows.Scan(&p, &g); err != nil {
			return nil, err
		}
		out[p] = g
	}
	return out, rows.Err()
}

// Mailer helper so other packages can send to a user without decrypting themselves.
func (s *Service) EmailUser(ctx context.Context, userID string, build func(to string) notify.Email) error {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	return s.queue.Enqueue(ctx, "email.send", build(u.Email))
}

// Cleanup removes expired OTPs and sessions (called by the retention task).
func (s *Service) Cleanup(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, "DELETE FROM otp_codes WHERE expires_at < now() - interval '1 day'"); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM sessions WHERE expires_at < $1 OR revoked_at < $1", time.Now().Add(-7*24*time.Hour))
	return err
}
