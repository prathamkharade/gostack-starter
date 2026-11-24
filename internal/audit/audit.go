// Package audit records security-relevant events (sign-ins, exports, deletions,
// permission changes). Never put personal data in meta: use ids.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"

	"app/internal/security"
)

type Execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// Log writes an audit row. actorID may be "" (anonymous). Failures are logged,
// not returned: auditing must never break the user's action.
func Log(ctx context.Context, db Execer, actorID, action, target string, meta map[string]any, ip string) {
	if meta == nil {
		meta = map[string]any{}
	}
	b, _ := json.Marshal(meta)
	var actor any
	if actorID != "" {
		actor = actorID
	}
	if _, err := db.Exec(ctx,
		"INSERT INTO audit_log (actor_id, action, target, meta, ip_trunc) VALUES ($1, $2, $3, $4, $5)",
		actor, action, target, b, security.TruncateIP(ip)); err != nil {
		slog.Default().Error("audit write failed", "action", action, "err", err)
	}
}
