package web

import (
	"time"

	"github.com/a-h/templ"
	"github.com/gofiber/fiber/v3"

	"app/internal/httpx"
	"app/internal/seo"
	"app/internal/views/pages"
	staticfs "app/static"
)

func (s *Server) home(c fiber.Ctx) error {
	httpx.SetMeta(c, seo.Meta{
		Title:       s.cfg.AppName,
		Description: s.cfg.SiteDescription,
		Path:        "/",
		JSONLD: []any{
			seo.Organization(s.cfg.OrgName, s.cfg.BaseURL, s.cfg.BaseURL+staticfs.Path("img/logo.svg")),
			seo.WebSite(s.cfg.AppName, s.cfg.BaseURL),
		},
	})
	// public pages can be cached briefly by browsers/CDNs when signed out
	if httpx.User(c) == nil {
		c.Set(fiber.HeaderCacheControl, "public, max-age=300")
	}
	return httpx.Render(c, pages.Home())
}

// page renders a static content page with SEO metadata.
func (s *Server) page(comp func() templ.Component, title, desc string) fiber.Handler {
	return func(c fiber.Ctx) error {
		httpx.SetMeta(c, seo.Meta{Title: title, Description: desc, Path: c.Path()})
		return httpx.Render(c, comp())
	}
}

func (s *Server) robots(c fiber.Ctx) error {
	if !s.cfg.IsProd() { // never let staging/dev get indexed
		return c.SendString("User-agent: *\nDisallow: /\n")
	}
	return c.SendString(seo.Robots(s.cfg.BaseURL, s.cfg.AICrawlers, []string{"/account", "/uploads", "/login", "/auth/", "/items"}))
}

func (s *Server) sitemap(c fiber.Ctx) error {
	urls := []seo.URL{{Loc: "/"}, {Loc: "/privacy"}, {Loc: "/terms"}, {Loc: "/accessibility"}}
	pub, err := s.items.PublicSitemap(c.Context(), 45000)
	if err != nil {
		return err
	}
	urls = append(urls, pub...)
	b, err := seo.Sitemap(s.cfg.BaseURL, urls)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "application/xml; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "public, max-age=3600")
	return c.Send(b)
}

func (s *Server) llms(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "text/markdown; charset=utf-8")
	return c.SendString(seo.LLMSTxt(s.cfg.AppName, s.cfg.SiteDescription, s.cfg.BaseURL, map[string][][2]string{
		"Pages":    {{"Home", "/"}, {"Sitemap", "/sitemap.xml"}},
		"Policies": {{"Privacy notice", "/privacy"}, {"Terms of service", "/terms"}, {"Accessibility", "/accessibility"}},
	}))
}

func (s *Server) securityTxt(c fiber.Ctx) error {
	// RFC 9116. Expires must be < 1 year ahead; computed so it never goes stale.
	exp := time.Now().AddDate(0, 11, 0).UTC().Format(time.RFC3339)
	return c.SendString("Contact: " + s.cfg.SecurityContact + "\nExpires: " + exp + "\nPreferred-Languages: en\nCanonical: " +
		s.cfg.BaseURL + "/.well-known/security.txt\n")
}

func (s *Server) manifest(c fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "application/manifest+json")
	return c.JSON(fiber.Map{
		"name": s.cfg.AppName, "short_name": s.cfg.AppName, "start_url": "/", "display": "standalone",
		"background_color": "#ffffff", "theme_color": "#4f46e5",
		"icons": []fiber.Map{{"src": staticfs.Path("img/favicon.svg"), "sizes": "any", "type": "image/svg+xml"}},
	})
}
