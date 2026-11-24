// Package httpx holds the small helpers every handler uses: rendering templ
// components, htmx awareness, redirects, flash messages, toasts, the current
// user and per-request view data. Feature packages import this, never internal/web.
package httpx

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"

	"app/internal/auth"
	"app/internal/seo"
)

// ViewData is available to every template via httpx.VD(ctx).
type ViewData struct {
	AppName   string
	OrgName   string
	OrgEmail  string
	BaseURL   string
	Path      string
	User      *auth.Principal // nil when signed out
	Flash     *Flash
	Meta      seo.Meta
	RequestID string
	Dev       bool
	Year      int
}

type ctxKey struct{}

func WithVD(ctx context.Context, vd *ViewData) context.Context {
	return context.WithValue(ctx, ctxKey{}, vd)
}

// VD never returns nil, so templates can use it freely.
func VD(ctx context.Context) *ViewData {
	if vd, ok := ctx.Value(ctxKey{}).(*ViewData); ok && vd != nil {
		return vd
	}
	return &ViewData{AppName: "App", Year: time.Now().Year()}
}

// SetMeta sets page metadata (title, description, canonical, JSON-LD) for the layout.
func SetMeta(c fiber.Ctx, m seo.Meta) {
	VD(c.Context()).Meta = m
}

// Render writes a templ component as HTML.
func Render(c fiber.Ctx, comp templ.Component) error {
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	c.Vary("HX-Request", "HX-Request-Type") // partial vs full responses must not share a cache entry
	return comp.Render(c.Context(), c.Response().BodyWriter())
}

func RenderStatus(c fiber.Ctx, status int, comp templ.Component) error {
	c.Status(status)
	return Render(c, comp)
}

// IsHTMX reports a request made by htmx.
func IsHTMX(c fiber.Ctx) bool { return c.Get("HX-Request") == "true" }

// IsPartial reports an htmx request that targets part of the page (render a
// fragment). Boosted navigations and history restores want the full page.
func IsPartial(c fiber.Ctx) bool {
	return IsHTMX(c) && c.Get("HX-Request-Type") == "partial" && c.Get("HX-History-Restore-Request") != "true"
}

// Redirect works for normal and htmx requests (htmx follows HX-Location without a full reload).
func Redirect(c fiber.Ctx, to string) error {
	if IsHTMX(c) {
		// the next page replaces the DOM, so carry a pending toast over as a flash
		if f, ok := c.Locals("toast").(Flash); ok {
			c.Response().Header.Del("HX-Trigger")
			SetFlash(c, f.Kind, f.Msg)
		}
		c.Set("HX-Location", to)
		return c.SendStatus(fiber.StatusNoContent)
	}
	return c.Redirect().Status(fiber.StatusSeeOther).To(to)
}

// User returns the signed-in principal or nil.
func User(c fiber.Ctx) *auth.Principal {
	p, _ := c.Locals("principal").(*auth.Principal)
	return p
}

func UserID(c fiber.Ctx) string {
	if p := User(c); p != nil {
		return p.UserID
	}
	return ""
}

// ClientIP is the real client IP (Fiber honours TRUSTED_PROXIES).
func ClientIP(c fiber.Ctx) string { return c.IP() }

// Query returns all query parameters as url.Values (for listing.Spec.Parse).
func Query(c fiber.Ctx) url.Values {
	v, _ := url.ParseQuery(string(c.Request().URI().QueryString()))
	return v
}

// --- flash & toast ---------------------------------------------------------------------

type Flash struct {
	Kind   string `json:"kind"` // success | error | info
	Msg    string `json:"msg"`
	Target string `json:"target,omitempty"`
}

const flashCookie = "flash"

// SetFlash shows a message on the next full page load (after a redirect).
func SetFlash(c fiber.Ctx, kind, msg string) {
	b, _ := json.Marshal(Flash{Kind: kind, Msg: msg})
	c.Cookie(&fiber.Cookie{Name: flashCookie, Value: url.QueryEscape(string(b)), Path: "/", HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode, Secure: c.Secure(), MaxAge: 60})
}

// PopFlash reads and clears the flash cookie (called by middleware).
func PopFlash(c fiber.Ctx) *Flash {
	raw := c.Cookies(flashCookie)
	if raw == "" {
		return nil
	}
	ExpireCookie(c, flashCookie)
	s, err := url.QueryUnescape(raw)
	if err != nil {
		return nil
	}
	var f Flash
	if json.Unmarshal([]byte(s), &f) != nil || f.Msg == "" || len(f.Msg) > 300 {
		return nil
	}
	return &f
}

// ExpireCookie deletes a cookie set with Path=/ (attributes must match to delete).
func ExpireCookie(c fiber.Ctx, name string) {
	c.Cookie(&fiber.Cookie{Name: name, Value: "", Path: "/", HTTPOnly: true, Secure: c.Secure() || len(name) > 7 && name[:7] == "__Host-",
		SameSite: fiber.CookieSameSiteLaxMode, MaxAge: -1, Expires: time.Unix(0, 0)})
}

// Toast shows a message after an htmx request (handled in static/js/app.js).
// For non-htmx requests it falls back to a flash message.
func Toast(c fiber.Ctx, kind, msg string) {
	if !IsHTMX(c) {
		SetFlash(c, kind, msg)
		return
	}
	// target body: the source element may have been swapped out of the DOM
	b, _ := json.Marshal(map[string]Flash{"toast": {Kind: kind, Msg: msg, Target: "body"}})
	c.Set("HX-Trigger", string(b))
	c.Locals("toast", Flash{Kind: kind, Msg: msg})
}
