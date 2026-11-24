// Package items is the EXAMPLE feature module. It shows the full pattern for a
// resource in one folder: SQL access (repo.go), HTTP handlers (handlers.go)
// and templates (views.templ). Copy it for new resources (/add-resource) or
// delete it (also remove migration 0002 and the Routes call in internal/web).
package items

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/listing"
	"app/internal/privacy"
	"app/internal/seo"
)

type Item struct {
	ID        string
	OwnerID   string
	Title     string
	Body      string
	Status    string
	CoverID   *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var Statuses = []string{"draft", "published", "archived"}

// Spec defines what the list page can search, filter and sort by.
// Only these columns ever reach SQL.
var Spec = listing.Spec{
	From:       "items",
	Select:     "id, owner_id, title, body, status, cover_id, created_at, updated_at",
	SearchFTS:  "search",
	SearchLike: []string{"title"},
	Filters: []listing.Filter{
		{Param: "status", Column: "status", Op: listing.In, Kind: listing.Enum, Values: Statuses, Label: "Status"},
		{Param: "from", Column: "created_at", Op: listing.Gte, Kind: listing.Date, Label: "From"},
		{Param: "to", Column: "created_at", Op: listing.Lte, Kind: listing.Date, Label: "To"},
	},
	Sorts: map[string]string{
		"newest":  "created_at DESC, id DESC",
		"oldest":  "created_at ASC, id ASC",
		"title":   "title ASC, id ASC",
		"updated": "updated_at DESC, id DESC",
	},
	DefaultSort:    "newest",
	DefaultPerPage: 20,
	CountLimit:     10_000,
}

// SortOptions drives the sort <select> (key, label).
var SortOptions = [][2]string{{"newest", "Newest"}, {"oldest", "Oldest"}, {"updated", "Recently updated"}, {"title", "Title A–Z"}}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(pool *pgxpool.Pool) *Repo {
	r := &Repo{pool: pool}
	// Data-subject access: everything this module stores about a user.
	privacy.RegisterExporter("items", func(ctx context.Context, userID string) (any, error) {
		rows, err := pool.Query(ctx, "SELECT "+Spec.Select+" FROM items WHERE owner_id = $1 ORDER BY created_at", userID)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowToStructByPos[Item])
	})
	return r
}

// List returns one page for the owner, honouring search/filter/sort params.
func (r *Repo) List(ctx context.Context, ownerID string, p listing.Params) (listing.Page[Item], error) {
	q, args, cq, cargs := Spec.Build(p, "owner_id = $1", ownerID)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return listing.Page[Item]{}, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Item])
	if err != nil {
		return listing.Page[Item]{}, err
	}
	total := -1
	if err := r.pool.QueryRow(ctx, cq, cargs...).Scan(&total); err != nil {
		return listing.Page[Item]{}, err
	}
	return listing.NewPage(items, p, total, Spec.CountLimit), nil
}

func (r *Repo) Get(ctx context.Context, id string) (*Item, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+Spec.Select+" FROM items WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	it, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Item])
	return &it, err
}

func (r *Repo) Create(ctx context.Context, it *Item) error {
	return r.pool.QueryRow(ctx, `INSERT INTO items (owner_id, title, body, status, cover_id) VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`, it.OwnerID, it.Title, it.Body, it.Status, it.CoverID).
		Scan(&it.ID, &it.CreatedAt, &it.UpdatedAt)
}

// Update is scoped by owner_id so users can never modify others' rows (IDOR protection).
func (r *Repo) Update(ctx context.Context, it *Item) error {
	tag, err := r.pool.Exec(ctx, `UPDATE items SET title = $3, body = $4, status = $5, cover_id = $6, updated_at = now()
		WHERE id = $1 AND owner_id = $2`, it.ID, it.OwnerID, it.Title, it.Body, it.Status, it.CoverID)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func (r *Repo) Delete(ctx context.Context, id, ownerID string) error {
	tag, err := r.pool.Exec(ctx, "DELETE FROM items WHERE id = $1 AND owner_id = $2", id, ownerID)
	if err == nil && tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

// PublicSitemap lists published items for sitemap.xml.
func (r *Repo) PublicSitemap(ctx context.Context, limit int) ([]seo.URL, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, updated_at FROM items WHERE status = 'published'
		ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (seo.URL, error) {
		var id string
		var t time.Time
		err := row.Scan(&id, &t)
		return seo.URL{Loc: "/i/" + id, Changed: t}, err
	})
}
