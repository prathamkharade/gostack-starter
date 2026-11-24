// Package jobs is a durable background job queue stored in Postgres
// (SELECT … FOR UPDATE SKIP LOCKED). It replaces Redis/RabbitMQ for most apps:
// one less service to pay for, back up and secure. Payloads are encrypted
// because they may contain personal data (e.g. an email address).
//
//	q.Register("email.send", func(ctx context.Context, p json.RawMessage) error {...})
//	q.Enqueue(ctx, "email.send", payload)            // run ASAP
//	q.EnqueueAt(ctx, "report.build", payload, when)  // run later
//
// Handlers must be idempotent: a job may run more than once after a crash.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/security"
)

type Handler func(ctx context.Context, payload json.RawMessage) error

type Queue struct {
	pool     *pgxpool.Pool
	keys     *security.Keyring
	log      *slog.Logger
	mu       sync.RWMutex
	handlers map[string]Handler
	periodic []periodic
}

type periodic struct {
	every time.Duration
	fn    func(ctx context.Context) error
	name  string
}

func New(pool *pgxpool.Pool, keys *security.Keyring, log *slog.Logger) *Queue {
	return &Queue{pool: pool, keys: keys, log: log, handlers: map[string]Handler{}}
}

func (q *Queue) Register(kind string, h Handler) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.handlers[kind] = h
}

// Every registers an in-process periodic task (cleanup, retention…). With
// several workers each runs it; tasks must therefore be idempotent.
func (q *Queue) Every(name string, d time.Duration, fn func(ctx context.Context) error) {
	q.periodic = append(q.periodic, periodic{every: d, fn: fn, name: name})
}

func (q *Queue) Enqueue(ctx context.Context, kind string, payload any) error {
	return q.EnqueueAt(ctx, kind, payload, time.Now())
}

func (q *Queue) EnqueueAt(ctx context.Context, kind string, payload any, at time.Time) error {
	return q.EnqueueTx(ctx, q.pool, kind, payload, at)
}

// Execer is implemented by *pgxpool.Pool and pgx.Tx, so jobs can be enqueued
// inside the same transaction as the data change that triggers them.
type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// EnqueueTx enqueues using the given pool or transaction.
func (q *Queue) EnqueueTx(ctx context.Context, db Execer, kind string, payload any, at time.Time) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	enc, err := q.keys.Encrypt(string(b), "jobs.payload")
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, "INSERT INTO jobs (kind, payload, run_at) VALUES ($1, $2, $3)", kind, enc, at)
	return err
}

// Run processes jobs until ctx is cancelled. concurrency goroutines poll the
// table; an idle poll costs one cheap indexed query.
func (q *Queue) Run(ctx context.Context, concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.loop(ctx)
		}()
	}
	for _, p := range q.periodic {
		wg.Add(1)
		go func(p periodic) {
			defer wg.Done()
			t := time.NewTicker(p.every)
			defer t.Stop()
			for {
				if err := p.fn(ctx); err != nil && ctx.Err() == nil {
					q.log.Error("periodic task failed", "task", p.name, "err", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}(p)
	}
	q.log.Info("job worker started", "concurrency", concurrency)
	wg.Wait()
}

func (q *Queue) loop(ctx context.Context) {
	idle := 500 * time.Millisecond
	for ctx.Err() == nil {
		worked, err := q.runOne(ctx)
		if err != nil && ctx.Err() == nil {
			q.log.Error("job poll failed", "err", err)
		}
		if worked {
			idle = 50 * time.Millisecond
			continue
		}
		select {
		case <-ctx.Done():
		case <-time.After(idle + time.Duration(rand.Int64N(int64(250*time.Millisecond)))):
		}
		if idle < 5*time.Second {
			idle *= 2 // back off when idle: near-zero DB load at night
		}
	}
}

// runOne claims one due job by leasing it (locked_until), runs it outside the
// claiming transaction, then records the outcome.
func (q *Queue) runOne(ctx context.Context) (bool, error) {
	var (
		id      int64
		kind    string
		payload []byte
		attempt int
		max     int
	)
	err := q.pool.QueryRow(ctx, `
		UPDATE jobs SET attempts = attempts + 1, locked_until = now() + interval '15 minutes'
		WHERE id = (
			SELECT id FROM jobs
			WHERE done_at IS NULL AND run_at <= now() AND attempts < max_attempts
			  AND (locked_until IS NULL OR locked_until < now())
			ORDER BY run_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1)
		RETURNING id, kind, payload, attempts, max_attempts`).Scan(&id, &kind, &payload, &attempt, &max)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	q.mu.RLock()
	h := q.handlers[kind]
	q.mu.RUnlock()

	runErr := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic: %v", r)
			}
		}()
		if h == nil {
			return fmt.Errorf("no handler for job kind %q", kind)
		}
		plain, err := q.keys.Decrypt(payload, "jobs.payload")
		if err != nil {
			return err
		}
		return h(ctx, json.RawMessage(plain))
	}()

	if runErr == nil {
		// Wipe the payload once done: no PII lingers in the queue.
		_, err = q.pool.Exec(ctx, "UPDATE jobs SET done_at = now(), locked_until = NULL, payload = '\\x'::bytea, last_error = NULL WHERE id = $1", id)
		return true, err
	}
	// exponential backoff: 30s, 1m, 2m, 4m … capped at 6h
	delay := time.Duration(math.Min(float64(30*time.Second)*math.Pow(2, float64(attempt-1)), float64(6*time.Hour)))
	q.log.Warn("job failed", "id", id, "kind", kind, "attempt", attempt, "max", max, "err", runErr)
	_, err = q.pool.Exec(ctx, "UPDATE jobs SET run_at = now() + make_interval(secs => $2), locked_until = NULL, last_error = $3 WHERE id = $1",
		id, delay.Seconds(), truncate(runErr.Error(), 2000))
	return true, err
}

// Purge deletes finished jobs older than days (called by the retention task).
func (q *Queue) Purge(ctx context.Context, days int) (int64, error) {
	tag, err := q.pool.Exec(ctx, "DELETE FROM jobs WHERE done_at < now() - make_interval(days => $1)", days)
	return tag.RowsAffected(), err
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
