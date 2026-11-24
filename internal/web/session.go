package web

import (
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"app/internal/auth"
	"app/internal/httpx"
	"app/internal/seo"
)

// Cookie names use the __Host- prefix in production: the browser then enforces
// Secure, Path=/ and no Domain (cannot be set or overwritten by subdomains).
func (s *Server) cookieName(base string) string {
	if s.cfg.CookieSecure {
		return "__Host-" + base
	}
	return base
}

func (s *Server) setAuthCookies(c fiber.Ctx, t *auth.Tokens) {
	set := func(name, val string, exp time.Time) {
		c.Cookie(&fiber.Cookie{Name: name, Value: val, Path: "/", HTTPOnly: true, Secure: s.cfg.CookieSecure,
			SameSite: fiber.CookieSameSiteLaxMode, MaxAge: int(time.Until(exp).Seconds())})
	}
	set(s.cookieName("at"), t.Access, t.AccessExpires)
	set(s.cookieName("rt"), t.Refresh, t.RefreshExpires)
}

func (s *Server) clearAuthCookies(c fiber.Ctx) {
	httpx.ExpireCookie(c, s.cookieName("at"))
	httpx.ExpireCookie(c, s.cookieName("rt"))
}

// authenticate resolves the principal from the access cookie (or a Bearer
// token for API/mobile clients), transparently rotating the refresh token when
// the access token has expired.
func (s *Server) authenticate(c fiber.Ctx) error {
	if h := c.Get(fiber.HeaderAuthorization); strings.HasPrefix(h, "Bearer ") {
		if p, err := s.auth.ParseAccess(strings.TrimPrefix(h, "Bearer ")); err == nil {
			s.setPrincipal(c, p)
		}
		return c.Next()
	}
	if at := c.Cookies(s.cookieName("at")); at != "" {
		if p, err := s.auth.ParseAccess(at); err == nil {
			s.setPrincipal(c, p)
			return c.Next()
		}
	}
	if rt := c.Cookies(s.cookieName("rt")); rt != "" {
		t, p, err := s.auth.Refresh(c.Context(), rt, c.Get(fiber.HeaderUserAgent), c.IP())
		if err != nil {
			s.clearAuthCookies(c)
		} else {
			s.setAuthCookies(c, t)
			s.setPrincipal(c, p)
		}
	}
	return c.Next()
}

func (s *Server) setPrincipal(c fiber.Ctx, p *auth.Principal) {
	c.Locals("principal", p)
	httpx.VD(c.Context()).User = p
	// signed-in pages are personal: never cache in shared caches
	c.Set(fiber.HeaderCacheControl, "private, no-store")
}

// requireAuth redirects anonymous visitors to /login?next=<current path>.
func (s *Server) requireAuth(c fiber.Ctx) error {
	if httpx.User(c) != nil {
		return c.Next()
	}
	next := c.OriginalURL()
	if httpx.IsHTMX(c) {
		if u, err := url.Parse(c.Get("HX-Current-URL")); err == nil {
			next = u.RequestURI()
		}
	}
	return httpx.Redirect(c, "/login?next="+url.QueryEscape(safeNext(next)))
}

// requireFresh re-checks the session in the DB for sensitive actions.
func (s *Server) requireFresh(c fiber.Ctx) error {
	p := httpx.User(c)
	if p == nil || !s.auth.SessionActive(c.Context(), p.SessionID) {
		s.clearAuthCookies(c)
		return httpx.Redirect(c, "/login")
	}
	return c.Next()
}

// requireAdmin guards admin-only routes.
func (s *Server) requireAdmin(c fiber.Ctx) error {
	if !httpx.User(c).IsAdmin() {
		return fiber.ErrForbidden
	}
	return c.Next()
}

// safeNext prevents open redirects: only local paths are allowed.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

func seoNoIndex(title string) seo.Meta { return seo.Meta{Title: title, NoIndex: true} }
