package auth

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"app/internal/audit"
	"app/internal/database"
	"app/internal/notify"
	"app/internal/security"
)

// StartEmailLogin sends a 6-digit code + magic link. The same flow signs up new
// users, so there is no separate registration form (frictionless onboarding).
// It returns nil for unknown emails too: responses never reveal who has an account.
func (s *Service) StartEmailLogin(ctx context.Context, email, ip string) error {
	email = security.NormalizeEmail(email)
	bidx := s.emailIndex(email)
	if err := s.checkOTPRate(ctx, bidx, "login"); err != nil {
		return err
	}
	code := security.OTPCode(otpDigits)
	link := security.RandomToken(32)
	enc, err := s.keys.Encrypt(email, "otp_codes.target")
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO otp_codes (purpose, target_bidx, target_enc, code_hash, link_hash, expires_at)
		VALUES ('login', $1, $2, $3, $4, $5)`,
		bidx, enc, security.HashOTP(s.cfg.BlindIndexKey, "login", string(bidx), code),
		security.HashToken(link), time.Now().Add(otpTTL)); err != nil {
		return err
	}
	verifyURL := s.cfg.BaseURL + "/auth/link?t=" + url.QueryEscape(link)
	audit.Log(ctx, s.pool, "", "auth.code_sent", "", nil, ip)
	return s.queue.Enqueue(ctx, "email.send", notify.LoginCode(s.cfg.AppName, email, code, verifyURL, int(otpTTL.Minutes())))
}

// VerifyEmailCode checks a code; on success it returns the user, creating the
// account (with consent records) on first sign-in.
func (s *Service) VerifyEmailCode(ctx context.Context, email, code, ip string) (u *User, created bool, err error) {
	bidx := s.emailIndex(email)
	var id string
	var hash []byte
	var attempts int
	err = s.pool.QueryRow(ctx, `
		SELECT id, code_hash, attempts FROM otp_codes
		WHERE target_bidx = $1 AND purpose = 'login' AND used_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC LIMIT 1`, bidx).Scan(&id, &hash, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrInvalidCode
	}
	if err != nil {
		return nil, false, err
	}
	if attempts >= otpMaxAttempts {
		return nil, false, ErrInvalidCode
	}
	if !security.EqualHash(hash, security.HashOTP(s.cfg.BlindIndexKey, "login", string(bidx), code)) {
		_, _ = s.pool.Exec(ctx, "UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1", id)
		return nil, false, ErrInvalidCode
	}
	return s.consumeLogin(ctx, id, security.NormalizeEmail(email), ip)
}

// VerifyMagicLink signs in with the link token from the email.
func (s *Service) VerifyMagicLink(ctx context.Context, token, ip string) (*User, bool, error) {
	var id string
	var enc []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, target_enc FROM otp_codes
		WHERE link_hash = $1 AND purpose = 'login' AND used_at IS NULL AND expires_at > now() AND attempts < $2`,
		security.HashToken(token), otpMaxAttempts).Scan(&id, &enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrInvalidCode
	}
	if err != nil {
		return nil, false, err
	}
	email, err := s.keys.Decrypt(enc, "otp_codes.target")
	if err != nil {
		return nil, false, err
	}
	return s.consumeLogin(ctx, id, email, ip)
}

func (s *Service) consumeLogin(ctx context.Context, otpID, email, ip string) (*User, bool, error) {
	tag, err := s.pool.Exec(ctx, "UPDATE otp_codes SET used_at = now() WHERE id = $1 AND used_at IS NULL", otpID)
	if err != nil {
		return nil, false, err
	}
	if tag.RowsAffected() == 0 {
		return nil, false, ErrInvalidCode // raced with another verify
	}
	// invalidate any other outstanding codes for this target
	_, _ = s.pool.Exec(ctx, "UPDATE otp_codes SET used_at = now() WHERE target_bidx = $1 AND purpose = 'login' AND used_at IS NULL", s.emailIndex(email))

	u, err := s.UserByEmail(ctx, email)
	if err == nil {
		if !u.EmailVerified {
			_, _ = s.pool.Exec(ctx, "UPDATE users SET email_verified_at = now() WHERE id = $1", u.ID)
			u.EmailVerified = true
		}
		return u, false, nil
	}
	if !database.IsNotFound(err) {
		return nil, false, err
	}
	u, err = s.createUser(ctx, email, ip)
	return u, err == nil, err
}

func (s *Service) createUser(ctx context.Context, email, ip string) (*User, error) {
	enc, err := s.keys.Encrypt(email, "users.email")
	if err != nil {
		return nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO users (email_enc, email_bidx, email_verified_at, locale)
		VALUES ($1, $2, now(), $3) RETURNING id`, enc, s.emailIndex(email), s.cfg.DefaultLocale).Scan(&id); err != nil {
		if database.IsUniqueViolation(err) { // concurrent sign-up of the same email
			return s.UserByEmail(ctx, email)
		}
		return nil, err
	}
	// Proof that the user accepted the current Terms and saw the Privacy Notice.
	if _, err := tx.Exec(ctx, `INSERT INTO consents (user_id, purpose, version, granted)
		VALUES ($1, 'terms', $2, true), ($1, 'privacy', $3, true)`, id, TermsVersion, PrivacyVersion); err != nil {
		return nil, err
	}
	audit.Log(ctx, tx, id, "auth.signup", "", nil, ip)
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.GetUser(ctx, id)
}

func (s *Service) checkOTPRate(ctx context.Context, bidx []byte, purpose string) error {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM otp_codes
		WHERE target_bidx = $1 AND purpose = $2 AND created_at > now() - interval '1 hour'`, bidx, purpose).Scan(&n); err != nil {
		return err
	}
	if n >= otpMaxPerHour {
		return ErrRateLimited
	}
	return nil
}

// --- phone verification (optional, costs money per SMS) ----------------------------

func (s *Service) StartPhoneVerify(ctx context.Context, userID, phone string) error {
	phone = security.NormalizePhone(phone)
	bidx := s.phoneIndex(phone)
	if err := s.checkOTPRate(ctx, bidx, "verify_phone"); err != nil {
		return err
	}
	code := security.OTPCode(otpDigits)
	enc, err := s.keys.Encrypt(phone, "otp_codes.target")
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO otp_codes (purpose, target_bidx, target_enc, code_hash, expires_at)
		VALUES ('verify_phone', $1, $2, $3, $4)`,
		bidx, enc, security.HashOTP(s.cfg.BlindIndexKey, "verify_phone", string(bidx)+userID, code), time.Now().Add(otpTTL)); err != nil {
		return err
	}
	return s.queue.Enqueue(ctx, "sms.send", notify.SMS{To: phone, Body: code + " is your " + s.cfg.AppName + " verification code"})
}

func (s *Service) VerifyPhone(ctx context.Context, userID, phone, code, ip string) error {
	phone = security.NormalizePhone(phone)
	bidx := s.phoneIndex(phone)
	var id string
	var hash []byte
	var attempts int
	err := s.pool.QueryRow(ctx, `SELECT id, code_hash, attempts FROM otp_codes
		WHERE target_bidx = $1 AND purpose = 'verify_phone' AND used_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC LIMIT 1`, bidx).Scan(&id, &hash, &attempts)
	if err != nil || attempts >= otpMaxAttempts {
		return ErrInvalidCode
	}
	if !security.EqualHash(hash, security.HashOTP(s.cfg.BlindIndexKey, "verify_phone", string(bidx)+userID, code)) {
		_, _ = s.pool.Exec(ctx, "UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1", id)
		return ErrInvalidCode
	}
	enc, err := s.keys.Encrypt(phone, "users.phone")
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, "UPDATE otp_codes SET used_at = now() WHERE id = $1", id); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx, `UPDATE users SET phone_enc = $2, phone_bidx = $3, phone_verified_at = now(), updated_at = now()
		WHERE id = $1`, userID, enc, bidx); err != nil {
		if database.IsUniqueViolation(err) {
			return errors.New("this phone number is already used by another account")
		}
		return err
	}
	audit.Log(ctx, s.pool, userID, "account.phone_verified", "", nil, ip)
	return nil
}
