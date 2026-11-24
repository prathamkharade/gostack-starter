// Package privacy implements data-subject rights and retention:
//
//   - Export  (GDPR Art. 15 access + Art. 20 portability): one JSON document.
//   - Delete  (Art. 17 erasure): removes the account, cascades to all owned
//     rows, deletes stored files, pseudonymises the audit trail.
//   - Retention: purges expired codes, sessions, finished jobs, old audit rows.
//
// Every resource that stores user data MUST register an exporter (and use
// ON DELETE CASCADE on owner_id) — see docs/compliance/data-storage.md.
package privacy

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/audit"
	"app/internal/auth"
	"app/internal/jobs"
	"app/internal/notify"
)

// Exporter returns everything a resource stores about a user.
type Exporter func(ctx context.Context, userID string) (any, error)

var (
	mu        sync.RWMutex
	exporters = map[string]Exporter{}
)

// RegisterExporter is called from each resource package's init/wiring.
func RegisterExporter(name string, fn Exporter) {
	mu.Lock()
	defer mu.Unlock()
	exporters[name] = fn
}

type Service struct {
	pool  *pgxpool.Pool
	auth  *auth.Service
	queue *jobs.Queue
	app   string
	log   *slog.Logger
}

func New(pool *pgxpool.Pool, a *auth.Service, q *jobs.Queue, appName string, log *slog.Logger) *Service {
	return &Service{pool: pool, auth: a, queue: q, app: appName, log: log}
}

// Export builds the user's data package.
func (s *Service) Export(ctx context.Context, userID, ip string) ([]byte, error) {
	u, err := s.auth.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	consents, err := s.rows(ctx, `SELECT purpose, version, granted, created_at FROM consents WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	sessions, err := s.auth.ActiveSessions(ctx, userID)
	if err != nil {
		return nil, err
	}
	auditRows, err := s.rows(ctx, `SELECT action, target, ip_trunc, created_at FROM audit_log WHERE actor_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	doc := map[string]any{
		"exported_at": time.Now().UTC(),
		"format":      "Personal data export – JSON. Fields are documented in our privacy notice.",
		"account": map[string]any{
			"id": u.ID, "email": u.Email, "name": u.Name, "phone": u.Phone, "locale": u.Locale,
			"role": u.Role, "email_verified": u.EmailVerified, "phone_verified": u.PhoneVerified,
			"has_password": u.HasPassword, "created_at": u.CreatedAt, "last_login_at": u.LastLoginAt,
		},
		"consents":        consents,
		"active_sessions": sessions,
		"security_log":    auditRows,
	}
	mu.RLock()
	names := make([]string, 0, len(exporters))
	for n := range exporters {
		names = append(names, n)
	}
	mu.RUnlock()
	sort.Strings(names)
	for _, n := range names {
		data, err := exporters[n](ctx, userID)
		if err != nil {
			return nil, err
		}
		doc[n] = data
	}
	audit.Log(ctx, s.pool, userID, "privacy.export", "", nil, ip)
	return json.MarshalIndent(doc, "", "  ")
}

// Delete erases the account immediately. Files are removed asynchronously by
// "storage.delete" jobs (keys are not personal data).
func (s *Service) Delete(ctx context.Context, userID, ip string) error {
	u, err := s.auth.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	rows, err := tx.Query(ctx, `SELECT v->>'key' FROM media m, jsonb_array_elements(m.variants) v WHERE m.owner_id = $1`, userID)
	if err != nil {
		return err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := s.queue.EnqueueTx(ctx, tx, "storage.delete", map[string]string{"key": k}, time.Now()); err != nil {
			return err
		}
	}
	// final confirmation email: payload is encrypted and wiped once sent
	if err := s.queue.EnqueueTx(ctx, tx, "email.send", notify.AccountDeleted(s.app, u.Email), time.Now()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM otp_codes WHERE target_bidx = (SELECT email_bidx FROM users WHERE id = $1)", userID); err != nil {
		return err
	}
	// ON DELETE CASCADE removes sessions, consents, media rows and every owned resource;
	// audit_log.actor_id is set NULL (kept for security, no longer linked to a person).
	if _, err := tx.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
		return err
	}
	audit.Log(ctx, tx, "", "privacy.account_deleted", "", map[string]any{"files": len(keys)}, ip)
	return tx.Commit(ctx)
}

// Retention runs periodically (see cmd/app wiring).
func (s *Service) Retention(auditDays, jobDays int) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if err := s.auth.Cleanup(ctx); err != nil {
			return err
		}
		if _, err := s.queue.Purge(ctx, jobDays); err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, "DELETE FROM audit_log WHERE created_at < now() - make_interval(days => $1)", auditDays); err != nil {
			return err
		}
		// uploads stuck in processing for a day are marked failed
		_, err := s.pool.Exec(ctx, "UPDATE media SET status = 'failed', error = 'timeout' WHERE status = 'pending' AND created_at < now() - interval '1 day'")
		return err
	}
}

func (s *Service) rows(ctx context.Context, sql string, args ...any) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToMap)
}
