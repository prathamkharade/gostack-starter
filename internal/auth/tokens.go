package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"

	"app/internal/audit"
	"app/internal/notify"
	"app/internal/security"
)

type claims struct {
	SID  string `json:"sid"`
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Tokens is what the web layer stores in cookies (or returns to API clients).
type Tokens struct {
	Access         string
	Refresh        string
	AccessExpires  time.Time
	RefreshExpires time.Time
}

func (s *Service) signAccess(userID, sessionID, role string, now time.Time) (string, time.Time, error) {
	exp := now.Add(s.cfg.AccessTTL)
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		SID: sessionID, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    s.cfg.BaseURL,
			Audience:  jwt.ClaimStrings{"app"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	})
	signed, err := t.SignedString(s.cfg.JWTSecret)
	return signed, exp, err
}

// ParseAccess validates signature, algorithm, issuer, audience and expiry.
func (s *Service) ParseAccess(token string) (*Principal, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return s.cfg.JWTSecret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.cfg.BaseURL),
		jwt.WithAudience("app"),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, err
	}
	return &Principal{UserID: c.Subject, SessionID: c.SID, Role: c.Role}, nil
}

// StartSession creates a new session family after a successful sign-in.
func (s *Service) StartSession(ctx context.Context, userID, userAgent, ip string) (*Tokens, error) {
	var role, status string
	if err := s.pool.QueryRow(ctx, "SELECT role, status FROM users WHERE id = $1", userID).Scan(&role, &status); err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, ErrAccountLocked
	}
	tok, err := s.insertSession(ctx, s.pool, userID, role, "", userAgent, ip)
	if err != nil {
		return nil, err
	}
	_, _ = s.pool.Exec(ctx, "UPDATE users SET last_login_at = now() WHERE id = $1", userID)
	audit.Log(ctx, s.pool, userID, "auth.login", "", nil, ip)
	return tok, nil
}

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Service) insertSession(ctx context.Context, db queryRower, userID, role, familyID, ua, ip string) (*Tokens, error) {
	refresh := security.RandomToken(32)
	now := time.Now()
	exp := now.Add(s.cfg.RefreshTTL)
	if len(ua) > 200 {
		ua = ua[:200]
	}
	var fam any
	if familyID != "" {
		fam = familyID
	}
	var sid string
	err := db.QueryRow(ctx, `
		INSERT INTO sessions (user_id, family_id, token_hash, user_agent, ip_trunc, expires_at)
		VALUES ($1, COALESCE($2::uuid, uuidv7()), $3, $4, $5, $6) RETURNING id`,
		userID, fam, security.HashToken(refresh), ua, security.TruncateIP(ip), exp).Scan(&sid)
	if err != nil {
		return nil, err
	}
	access, aexp, err := s.signAccess(userID, sid, role, now)
	if err != nil {
		return nil, err
	}
	return &Tokens{Access: access, Refresh: refresh, AccessExpires: aexp, RefreshExpires: exp}, nil
}

// Refresh rotates a refresh token. Presenting a token that was already rotated
// (outside a short grace window) means it was stolen: the whole family is revoked.
func (s *Service) Refresh(ctx context.Context, refresh, ua, ip string) (*Tokens, *Principal, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var (
		sid, userID, familyID, role, status string
		expires                             time.Time
		rotated, revoked                    *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT s.id, s.user_id, s.family_id, s.expires_at, s.rotated_at, s.revoked_at, u.role, u.status
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 FOR UPDATE OF s`, security.HashToken(refresh)).
		Scan(&sid, &userID, &familyID, &expires, &rotated, &revoked, &role, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrSessionEnded
	}
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	if revoked != nil || now.After(expires) || status != "active" {
		return nil, nil, ErrSessionEnded
	}
	if rotated != nil && now.Sub(*rotated) > refreshGrace {
		if _, err := tx.Exec(ctx, "UPDATE sessions SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL", familyID); err != nil {
			return nil, nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, nil, err
		}
		audit.Log(ctx, s.pool, userID, "auth.refresh_reuse", familyID, nil, ip)
		s.notifySecurity(ctx, userID, "Someone tried to reuse an old sign-in token for your account, so we signed out that device for safety.")
		return nil, nil, ErrTokenReuse
	}
	if _, err := tx.Exec(ctx, "UPDATE sessions SET rotated_at = COALESCE(rotated_at, now()), last_used_at = now() WHERE id = $1", sid); err != nil {
		return nil, nil, err
	}
	tok, err := s.insertSession(ctx, tx, userID, role, familyID, ua, ip)
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	p, err := s.ParseAccess(tok.Access)
	return tok, p, err
}

// SessionActive is checked for sensitive actions (exports, deletion, password
// change) so a revoked session cannot act during its remaining access-token life.
func (s *Service) SessionActive(ctx context.Context, sessionID string) bool {
	var ok bool
	_ = s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM sessions WHERE id = $1 AND revoked_at IS NULL AND expires_at > now())", sessionID).Scan(&ok)
	return ok
}

// Logout revokes the current session family (this device).
func (s *Service) Logout(ctx context.Context, refresh string) {
	_, _ = s.pool.Exec(ctx, `UPDATE sessions SET revoked_at = now()
		WHERE family_id = (SELECT family_id FROM sessions WHERE token_hash = $1) AND revoked_at IS NULL`, security.HashToken(refresh))
}

// LogoutEverywhere revokes all sessions of a user.
func (s *Service) LogoutEverywhere(ctx context.Context, userID, ip string) error {
	_, err := s.pool.Exec(ctx, "UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL", userID)
	audit.Log(ctx, s.pool, userID, "auth.logout_all", "", nil, ip)
	return err
}

// ActiveSession is shown on the account page ("where you're signed in").
type ActiveSession struct {
	ID         string
	FamilyID   string
	UserAgent  string
	IP         string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

func (s *Service) ActiveSessions(ctx context.Context, userID string) ([]ActiveSession, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (family_id) id, family_id, user_agent, ip_trunc, created_at, last_used_at
		FROM sessions WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY family_id, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ActiveSession, error) {
		var a ActiveSession
		err := r.Scan(&a.ID, &a.FamilyID, &a.UserAgent, &a.IP, &a.CreatedAt, &a.LastUsedAt)
		return a, err
	})
}

func (s *Service) notifySecurity(ctx context.Context, userID, what string) {
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return
	}
	_ = s.queue.Enqueue(ctx, "email.send", notify.SecurityNotice(s.cfg.AppName, u.Email, what))
}
