package web

import (
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"app/internal/httpx"
	"app/internal/media"
	"app/internal/storage"
	"app/internal/views/pages"
)

func (s *Server) uploadsPage(c fiber.Ctx) error {
	list, err := s.media.ListByOwner(c.Context(), httpx.UserID(c), 100)
	if err != nil {
		return err
	}
	httpx.SetMeta(c, seoNoIndex("Uploads"))
	return httpx.Render(c, pages.Uploads(list, s.cfg.MediaMaxUploadMB))
}

func (s *Server) upload(c fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "Choose a file to upload.")
	}
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	m, err := s.media.Upload(c.Context(), httpx.UserID(c), fh.Filename, f, c.FormValue("visibility"))
	switch {
	case errors.Is(err, media.ErrTooLarge):
		return fiber.NewError(fiber.StatusRequestEntityTooLarge, "That file is too large.")
	case errors.Is(err, media.ErrBlocked):
		return fiber.NewError(fiber.StatusUnsupportedMediaType, "That file type is not allowed.")
	case err != nil:
		return err
	}
	if httpx.IsHTMX(c) {
		httpx.Toast(c, "success", "Uploaded – optimising in the background.")
		return httpx.Render(c, pages.UploadCard(m))
	}
	return httpx.Redirect(c, "/uploads")
}

// uploadStatus is polled by pending cards (hx-trigger="every 2s") until ready.
func (s *Server) uploadStatus(c fiber.Ctx) error {
	m, err := s.ownMedia(c)
	if err != nil {
		return err
	}
	return httpx.Render(c, pages.UploadCard(m))
}

func (s *Server) deleteUpload(c fiber.Ctx) error {
	m, err := s.ownMedia(c)
	if err != nil {
		return err
	}
	if err := s.media.Delete(c.Context(), m); err != nil {
		return err
	}
	httpx.Toast(c, "success", "Deleted.")
	return c.SendString("") // htmx swaps the card with nothing
}

func (s *Server) ownMedia(c fiber.Ctx) (*media.Media, error) {
	m, err := s.media.Get(c.Context(), c.Params("id"))
	if err != nil || m.OwnerID == nil || *m.OwnerID != httpx.UserID(c) {
		return nil, fiber.ErrNotFound // 404, not 403: do not reveal existence
	}
	return m, nil
}

// serveMedia enforces access control, then redirects to the object store
// (signed or CDN URL) or streams the local file with safe headers.
func (s *Server) serveMedia(c fiber.Ctx) error {
	m, err := s.media.Get(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.ErrNotFound
	}
	p := httpx.User(c)
	uid := ""
	if p != nil {
		uid = p.UserID
	}
	if !m.CanView(uid, p.IsAdmin()) {
		return fiber.ErrNotFound
	}
	v, ok := m.Variant(c.Params("variant"))
	if !ok {
		return fiber.ErrNotFound
	}
	public := m.Visibility == "public"

	if u, err := s.media.Storage().URL(c.Context(), v.Key, public, 15*time.Minute); err == nil && u != "" {
		c.Set(fiber.HeaderCacheControl, "private, max-age=600")
		return c.Redirect().Status(fiber.StatusFound).To(u)
	}

	// Defence in depth for user content served from our origin.
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'; sandbox")
	c.Set("Cross-Origin-Resource-Policy", "same-origin")
	if public {
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	} else {
		c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	}
	disp := "inline"
	if !media.Inline(v.MIME) {
		disp = "attachment"
	}
	name := media.SafeFilename(m.Name)
	if v.Name != "file" && v.Name != "original" {
		name = strings.TrimSuffix(name, extOf(name)) + extOf(v.Key)
	}
	c.Set(fiber.HeaderContentDisposition, disp+`; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)

	local, ok := s.media.Storage().(*storage.Local)
	if !ok {
		return fiber.ErrNotFound
	}
	if v.Encoding == "gzip" {
		return s.serveGzip(c, local, v)
	}
	path, err := local.FilePath(v.Key)
	if err != nil {
		return fiber.ErrNotFound
	}
	return c.SendFile(path, fiber.SendFile{ByteRange: true}) // Range support: video seeking
}

// serveGzip sends gzip-at-rest files as-is to browsers that accept gzip
// (all of them) and decompresses for the rare client that does not.
func (s *Server) serveGzip(c fiber.Ctx, local *storage.Local, v media.Variant) error {
	rc, err := local.Get(c.Context(), v.Key)
	if err != nil {
		return fiber.ErrNotFound
	}
	c.Set(fiber.HeaderContentType, v.MIME)
	c.Vary(fiber.HeaderAcceptEncoding)
	if strings.Contains(c.Get(fiber.HeaderAcceptEncoding), "gzip") {
		c.Set(fiber.HeaderContentEncoding, "gzip")
		return c.SendStream(rc)
	}
	zr, err := gzip.NewReader(rc)
	if err != nil {
		rc.Close()
		return err
	}
	return c.SendStream(struct {
		io.Reader
		io.Closer
	}{zr, rc})
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		ext := name[i:]
		if ext == ".gz" {
			return extOf(name[:i]) + ext
		}
		return ext
	}
	return ""
}
