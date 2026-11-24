// Package web wires HTTP: Fiber app, middleware (security headers, CSRF,
// rate limits, auth), error pages and the core routes (home, auth, account,
// media, SEO files, health). Feature packages register their own routes.
package web

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/csrf"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/jackc/pgx/v5/pgxpool"

	"app/internal/auth"
	"app/internal/config"
	"app/internal/httpx"
	"app/internal/items"
	"app/internal/media"
	"app/internal/privacy"
	"app/internal/views/pages"
	staticfs "app/static"
)

type Server struct {
	App     *fiber.App
	cfg     *config.Config
	log     *slog.Logger
	pool    *pgxpool.Pool
	auth    *auth.Service
	media   *media.Service
	privacy *privacy.Service
	items   *items.Repo
}

type Deps struct {
	Cfg     *config.Config
	Log     *slog.Logger
	Pool    *pgxpool.Pool
	Auth    *auth.Service
	Media   *media.Service
	Privacy *privacy.Service
	Items   *items.Repo
}

func New(d Deps) *Server {
	s := &Server{cfg: d.Cfg, log: d.Log, pool: d.Pool, auth: d.Auth, media: d.Media, privacy: d.Privacy, items: d.Items}
	trust := len(d.Cfg.TrustedProxies) > 0
	s.App = fiber.New(fiber.Config{
		AppName:      d.Cfg.AppName,
		BodyLimit:    (d.Cfg.MediaMaxUploadMB + 1) << 20, // tighter per-route limits are set in Caddy
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
		TrustProxy:   trust,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies:  d.Cfg.TrustedProxies,
			Loopback: true,
			Private:  true, // Caddy reaches the app over the Docker network
		},
		ProxyHeader:  proxyHeader(trust),
		ErrorHandler: s.errorHandler,
	})
	s.routes()
	return s
}

func proxyHeader(trust bool) string {
	if trust {
		return fiber.HeaderXForwardedFor
	}
	return ""
}

func (s *Server) routes() {
	app := s.App
	app.Use(recover.New())

	// Static assets first: no auth, no DB, cached for a year (URLs are fingerprinted).
	app.Get("/static*", static.New("", static.Config{FS: staticfs.FS, MaxAge: 31536000}))
	app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("ok") })
	app.Get("/readyz", s.ready)

	app.Use(requestid.New())
	app.Use(s.securityHeaders)
	app.Use(s.accessLog)
	// Tokenless CSRF protection (Sec-Fetch-Site / Origin), same model as Go's
	// net/http CrossOriginProtection. Works with htmx, forms and fetch without tokens.
	app.Use(csrf.NewCrossOriginProtection(csrf.CrossOriginConfig{TrustedOrigins: []string{s.cfg.BaseURL}}))
	// Global per-IP limit: cheap protection against scraping and floods.
	app.Use(limiter.New(limiter.Config{Max: 300, Expiration: time.Minute, LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c fiber.Ctx) string { return c.IP() }, LimitReached: tooMany}))
	app.Use(s.viewData)
	app.Use(s.authenticate)

	// SEO / GEO / well-known
	app.Get("/robots.txt", s.robots)
	app.Get("/sitemap.xml", s.sitemap)
	app.Get("/llms.txt", s.llms)
	app.Get("/.well-known/security.txt", s.securityTxt)
	app.Get("/manifest.webmanifest", s.manifest)
	app.Get("/favicon.ico", func(c fiber.Ctx) error { return c.Redirect().Status(301).To(staticfs.Path("img/favicon.svg")) })

	// Public pages
	app.Get("/", s.home)
	app.Get("/privacy", s.page(pages.Privacy, "Privacy notice", "How we collect, use and protect your personal data."))
	app.Get("/terms", s.page(pages.Terms, "Terms of service", "The rules for using this service."))
	app.Get("/cookies", s.page(pages.Cookies, "Cookies", "We only use strictly necessary cookies."))
	app.Get("/accessibility", s.page(pages.Accessibility, "Accessibility statement", "Our commitment to accessibility."))

	// Auth (strict rate limits: 10 attempts / 5 min per IP)
	authLimit := limiter.New(limiter.Config{Max: 10, Expiration: 5 * time.Minute, LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c fiber.Ctx) string { return "auth:" + c.IP() }, LimitReached: tooMany})
	app.Get("/login", s.loginPage)
	app.Post("/login", authLimit, s.loginStart)
	app.Post("/login/verify", authLimit, s.loginVerify)
	app.Post("/login/password", authLimit, s.loginPassword)
	app.Get("/auth/link", s.magicLinkPage)
	app.Post("/auth/link", authLimit, s.magicLink)
	app.Post("/logout", s.logout)

	// Account & privacy self-service (GDPR rights)
	acc := app.Group("/account", s.requireAuth)
	acc.Get("/", s.accountPage)
	acc.Post("/profile", s.updateProfile)
	acc.Post("/password", s.requireFresh, s.setPassword)
	acc.Post("/phone", authLimit, s.phoneStart)
	acc.Post("/phone/verify", authLimit, s.phoneVerify)
	acc.Post("/consents", s.updateConsents)
	acc.Post("/sessions/revoke-all", s.logoutAll)
	acc.Get("/export", s.requireFresh, s.exportData)
	acc.Post("/delete", s.requireFresh, s.deleteAccount)

	// Media
	app.Get("/media/:id/:variant", s.serveMedia)
	up := app.Group("/uploads", s.requireAuth)
	up.Get("/", s.uploadsPage)
	up.Post("/", s.upload)
	up.Get("/:id/status", s.uploadStatus)
	up.Post("/:id/delete", s.deleteUpload)

	// Feature modules
	items.Routes(app, s.items, s.media, s.requireAuth)

	app.Use(func(c fiber.Ctx) error { return fiber.ErrNotFound })
}

// --- middleware ---------------------------------------------------------------------

func (s *Server) securityHeaders(c fiber.Ctx) error {
	h := &c.Response().Header
	connect := "'self'"
	img := "'self' data: blob:"
	if s.cfg.S3PublicURL != "" {
		img += " " + s.cfg.S3PublicURL
		connect += " " + s.cfg.S3PublicURL
	}
	if s.cfg.StorageDriver == "s3" && s.cfg.S3Endpoint != "" {
		img += " https:" // presigned URLs live on the bucket host
	}
	// Strict CSP: only our own scripts (no inline JS, no eval – Alpine runs its CSP build).
	h.Set("Content-Security-Policy", strings.Join([]string{
		"default-src 'self'", "script-src 'self'", "style-src 'self'", "img-src " + img,
		"media-src " + img, "connect-src " + connect, "font-src 'self'", "object-src 'none'",
		"base-uri 'self'", "form-action 'self'", "frame-ancestors 'none'", "upgrade-insecure-requests",
	}, "; "))
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()")
	if s.cfg.IsProd() {
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
	}
	return c.Next()
}

func (s *Server) accessLog(c fiber.Ctx) error {
	start := time.Now()
	err := c.Next()
	status := c.Response().StatusCode()
	if err != nil {
		var fe *fiber.Error
		if errors.As(err, &fe) {
			status = fe.Code
		} else {
			status = 500
		}
	}
	lvl := slog.LevelInfo
	if status >= 500 {
		lvl = slog.LevelError
	}
	// No query strings, IPs or user ids in access logs: they can be personal data.
	s.log.Log(c.Context(), lvl, "http", "method", c.Method(), "path", c.Path(), "status", status,
		"ms", time.Since(start).Milliseconds(), "rid", requestid.FromContext(c))
	return err
}

func (s *Server) viewData(c fiber.Ctx) error {
	vd := &httpx.ViewData{
		AppName: s.cfg.AppName, OrgName: s.cfg.OrgName, OrgEmail: s.cfg.OrgEmail, BaseURL: s.cfg.BaseURL, Path: c.Path(),
		RequestID: requestid.FromContext(c), Dev: s.cfg.IsDev(), Year: time.Now().Year(),
	}
	if !httpx.IsPartial(c) {
		vd.Flash = httpx.PopFlash(c)
	}
	c.SetContext(httpx.WithVD(c.Context(), vd))
	return c.Next()
}

func tooMany(c fiber.Ctx) error {
	return fiber.NewError(fiber.StatusTooManyRequests, "Too many requests. Please wait a moment and try again.")
}

func (s *Server) ready(c fiber.Ctx) error {
	if err := s.pool.Ping(c.Context()); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("db unavailable")
	}
	return c.SendString("ready")
}

// errorHandler renders friendly pages; htmx requests get a toast and no swap.
func (s *Server) errorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	msg := "Something went wrong. Please try again."
	var fe *fiber.Error
	if errors.As(err, &fe) {
		code = fe.Code
		msg = fe.Message
	}
	if errors.Is(err, csrf.ErrCrossOriginRequest) {
		code, msg = fiber.StatusForbidden, "Request blocked for your security. Reload the page and try again."
	}
	if code >= 500 {
		s.log.Error("request failed", "path", c.Path(), "err", err, "rid", requestid.FromContext(c))
		msg = "Something went wrong on our side. Please try again."
	}
	if httpx.IsHTMX(c) {
		c.Set("HX-Reswap", "none")
		httpx.Toast(c, "error", msg)
		return c.SendStatus(code)
	}
	if strings.HasPrefix(c.Get(fiber.HeaderAccept), "application/json") {
		return c.Status(code).JSON(fiber.Map{"error": msg})
	}
	httpx.SetMeta(c, seoNoIndex(msg))
	return httpx.RenderStatus(c, code, pages.Error(code, msg))
}
