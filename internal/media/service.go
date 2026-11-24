package media

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/jobs"
	"app/internal/security"
	"app/internal/storage"
)

var (
	ErrTooLarge  = errors.New("media: file too large")
	ErrBlocked   = errors.New("media: this file type is not allowed")
	ErrForbidden = errors.New("media: not allowed")
)

// Variant is one stored rendition (JSON in media.variants).
type Variant struct {
	Name     string `json:"name"`
	Key      string `json:"key"`
	MIME     string `json:"mime"`
	Encoding string `json:"encoding,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Size     int64  `json:"size"`
}

// Media is a row of the media table, ready for templates.
type Media struct {
	ID         string
	OwnerID    *string
	Kind       Kind
	MIME       string
	Name       string
	Visibility string
	Status     string
	Width      int
	Height     int
	Duration   float64
	Size       int64
	Stored     int64
	Variants   []Variant
	publicBase string
}

// URL returns where the browser should fetch a variant: a CDN URL for public
// objects when S3_PUBLIC_URL is set, otherwise the app's access-checked route.
func (m *Media) URL(variant string) string {
	if m.Visibility == "public" && m.publicBase != "" {
		for _, v := range m.Variants {
			if v.Name == variant {
				return m.publicBase + "/" + v.Key
			}
		}
	}
	return "/media/" + m.ID + "/" + variant
}

// SrcSet renders "url 320w, url 640w" for responsive <img>.
func (m *Media) SrcSet() string {
	var parts []string
	for _, v := range m.Variants {
		if strings.HasPrefix(v.Name, "w") && v.Width > 0 {
			parts = append(parts, m.URL(v.Name)+" "+strconv.Itoa(v.Width)+"w")
		}
	}
	return strings.Join(parts, ", ")
}

// Best returns the largest image variant name (fallback src).
func (m *Media) Best() string {
	best := "file"
	w := 0
	for _, v := range m.Variants {
		if strings.HasPrefix(v.Name, "w") && v.Width > w {
			best, w = v.Name, v.Width
		}
		if v.Name == "video" || v.Name == "audio" {
			return v.Name
		}
	}
	return best
}

func (m *Media) Variant(name string) (Variant, bool) {
	for _, v := range m.Variants {
		if v.Name == name {
			return v, true
		}
	}
	return Variant{}, false
}

// SavedPercent is shown in the UI ("saved 82%").
func (m *Media) SavedPercent() int {
	if m.Size == 0 || m.Stored == 0 || m.Stored >= m.Size {
		return 0
	}
	return int(100 - m.Stored*100/m.Size)
}

type Service struct {
	pool       *pgxpool.Pool
	store      storage.Storage
	proc       *Processor
	queue      *jobs.Queue
	keys       *security.Keyring
	log        *slog.Logger
	maxBytes   int64
	keepOrig   bool
	publicBase string
}

func NewService(pool *pgxpool.Pool, st storage.Storage, proc *Processor, q *jobs.Queue, keys *security.Keyring,
	log *slog.Logger, maxMB int, keepOriginal bool, publicBase string) *Service {
	s := &Service{pool: pool, store: st, proc: proc, queue: q, keys: keys, log: log,
		maxBytes: int64(maxMB) << 20, keepOrig: keepOriginal, publicBase: publicBase}
	q.Register("media.process", s.handleProcess)
	q.Register("storage.delete", func(ctx context.Context, p json.RawMessage) error {
		var in struct{ Key string }
		if err := json.Unmarshal(p, &in); err != nil {
			return err
		}
		return st.Delete(ctx, in.Key)
	})
	return s
}

func (s *Service) Storage() storage.Storage { return s.store }

// Upload streams r to a temp file (hashing as it goes), sniffs the real type,
// de-duplicates per owner, stores the original and queues processing.
func (s *Service) Upload(ctx context.Context, ownerID, filename string, r io.Reader, visibility string) (*Media, error) {
	if visibility != "public" {
		visibility = "private"
	}
	tmp, err := os.CreateTemp("", "upload-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, s.maxBytes+1))
	if err != nil {
		return nil, err
	}
	if n > s.maxBytes {
		return nil, ErrTooLarge
	}
	head := make([]byte, 512)
	hn, _ := tmp.ReadAt(head, 0)
	mime, kind := Detect(head[:hn], filename)
	if kind == Blocked {
		return nil, ErrBlocked
	}
	sum := h.Sum(nil)

	if existing, err := s.byOwnerHash(ctx, ownerID, sum); err == nil {
		return existing, nil // same file uploaded again: no extra storage
	}

	nameEnc, err := s.keys.Encrypt(sanitizeName(filename), "media.name")
	if err != nil {
		return nil, err
	}
	var id string
	if err := s.pool.QueryRow(ctx, `INSERT INTO media (owner_id, sha256, kind, mime, name_enc, visibility, size_original)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`, nullable(ownerID), sum, string(kind), mime, nameEnc, visibility, n).Scan(&id); err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err := s.store.Put(ctx, originalKey(id), tmp, n, storage.Meta{ContentType: mime}); err != nil {
		return nil, err
	}
	if err := s.queue.Enqueue(ctx, "media.process", map[string]string{"id": id}); err != nil {
		return nil, err
	}
	return s.Get(ctx, id)
}

func (s *Service) handleProcess(ctx context.Context, p json.RawMessage) error {
	var in struct{ ID string }
	if err := json.Unmarshal(p, &in); err != nil {
		return err
	}
	m, err := s.Get(ctx, in.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // deleted meanwhile
		}
		return err
	}
	if m.Status == "ready" {
		return nil // idempotent
	}
	dir, err := os.MkdirTemp("", "media-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "original")
	if err := s.download(ctx, originalKey(m.ID), src); err != nil {
		return err
	}
	res, err := s.proc.Process(ctx, src, dir, m.MIME, m.Kind)
	if err != nil {
		_, _ = s.pool.Exec(ctx, "UPDATE media SET status = 'failed', error = $2 WHERE id = $1", m.ID, truncate(err.Error(), 500))
		return nil // permanent failure: do not retry forever
	}

	cache := "private, max-age=3600"
	if m.Visibility == "public" {
		cache = "public, max-age=31536000, immutable"
	}
	var variants []Variant
	var stored int64
	for _, o := range res.Outputs {
		key := fmt.Sprintf("m/%s/%s%s", m.ID, o.Name, extFor(o.MIME, o.Encoding))
		f, err := os.Open(o.Path)
		if err != nil {
			return err
		}
		disp := "inline"
		if !Inline(o.MIME) {
			disp = "attachment"
		}
		err = s.store.Put(ctx, key, f, o.Size, storage.Meta{ContentType: o.MIME, ContentEncoding: o.Encoding,
			CacheControl: cache, ContentDisposition: disp})
		f.Close()
		if err != nil {
			return err
		}
		variants = append(variants, Variant{Name: o.Name, Key: key, MIME: o.MIME, Encoding: o.Encoding, Width: o.Width, Height: o.Height, Size: o.Size})
		stored += o.Size
	}
	if s.keepOrig {
		variants = append(variants, Variant{Name: "original", Key: originalKey(m.ID), MIME: m.MIME, Size: m.Size})
		stored += m.Size
	}
	vj, _ := json.Marshal(variants)
	if _, err := s.pool.Exec(ctx, `UPDATE media SET status = 'ready', variants = $2, size_stored = $3,
		width = NULLIF($4, 0), height = NULLIF($5, 0), duration_s = NULLIF($6, 0), error = NULL WHERE id = $1`,
		m.ID, vj, stored, res.Width, res.Height, res.Duration); err != nil {
		return err
	}
	if !s.keepOrig {
		_ = s.store.Delete(ctx, originalKey(m.ID))
	}
	s.log.Info("media processed", "id", m.ID, "kind", m.Kind, "original", m.Size, "stored", stored)
	return nil
}

func (s *Service) download(ctx context.Context, key, dst string) error {
	rc, err := s.store.Get(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, rc); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

const mediaCols = `id, owner_id, kind, mime, name_enc, visibility, status, COALESCE(width, 0), COALESCE(height, 0),
	COALESCE(duration_s, 0), size_original, size_stored, variants`

func (s *Service) scan(row pgx.Row) (*Media, error) {
	var m Media
	var name []byte
	var kind string
	var vj []byte
	var dur float32
	if err := row.Scan(&m.ID, &m.OwnerID, &kind, &m.MIME, &name, &m.Visibility, &m.Status, &m.Width, &m.Height,
		&dur, &m.Size, &m.Stored, &vj); err != nil {
		return nil, err
	}
	m.Kind, m.Duration, m.publicBase = Kind(kind), float64(dur), s.publicBase
	m.Name = s.keys.MustDecrypt(name, "media.name")
	_ = json.Unmarshal(vj, &m.Variants)
	return &m, nil
}

func (s *Service) Get(ctx context.Context, id string) (*Media, error) {
	return s.scan(s.pool.QueryRow(ctx, "SELECT "+mediaCols+" FROM media WHERE id = $1", id))
}

func (s *Service) byOwnerHash(ctx context.Context, ownerID string, sum []byte) (*Media, error) {
	return s.scan(s.pool.QueryRow(ctx, "SELECT "+mediaCols+" FROM media WHERE owner_id = $1 AND sha256 = $2 AND status <> 'failed'", ownerID, sum))
}

// ListByOwner returns a user's uploads (newest first).
func (s *Service) ListByOwner(ctx context.Context, ownerID string, limit int) ([]*Media, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+mediaCols+" FROM media WHERE owner_id = $1 ORDER BY created_at DESC LIMIT $2", ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Media
	for rows.Next() {
		m, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Delete removes the row and queues file deletion.
func (s *Service) Delete(ctx context.Context, m *Media) error {
	for _, v := range m.Variants {
		if err := s.queue.Enqueue(ctx, "storage.delete", map[string]string{"key": v.Key}); err != nil {
			return err
		}
	}
	_ = s.queue.Enqueue(ctx, "storage.delete", map[string]string{"key": originalKey(m.ID)})
	_, err := s.pool.Exec(ctx, "DELETE FROM media WHERE id = $1", m.ID)
	return err
}

// CanView enforces access: public media for everyone, private for owner/admin.
func (m *Media) CanView(userID string, admin bool) bool {
	return m.Visibility == "public" || admin || (m.OwnerID != nil && *m.OwnerID == userID)
}

func originalKey(id string) string { return "u/" + id + "/original" }

func extFor(mime, enc string) string {
	ext := map[string]string{
		"image/webp": ".webp", "image/avif": ".avif", "image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif",
		"video/mp4": ".mp4", "audio/mp4": ".m4a", "application/pdf": ".pdf", "application/zip": ".zip",
		"text/csv": ".csv", "text/plain": ".txt", "application/json": ".json",
	}[mime]
	if ext == "" {
		ext = ".bin"
	}
	if enc == "gzip" {
		ext += ".gz"
	}
	return ext
}

func sanitizeName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	n = strings.Map(func(r rune) rune {
		if r < 32 || r == '"' || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, n)
	if len(n) > 200 {
		n = n[len(n)-200:]
	}
	if n == "" || n == "." {
		n = "file"
	}
	return n
}

// SafeFilename is used for Content-Disposition.
func SafeFilename(n string) string { return sanitizeName(n) }

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
