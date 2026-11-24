package web

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"

	"app/internal/auth"
	"app/internal/httpx"
	"app/internal/seo"
	"app/internal/validate"
	"app/internal/views/pages"
)

func (s *Server) loginPage(c fiber.Ctx) error {
	next := safeNext(c.Query("next", "/"))
	if httpx.User(c) != nil {
		return httpx.Redirect(c, next)
	}
	httpx.SetMeta(c, seo.Meta{Title: "Sign in", Description: "Sign in or create an account with just your email.", Path: "/login", NoIndex: true})
	step := "email"
	if c.Query("step") == "password" {
		step = "password"
	}
	return httpx.Render(c, pages.Login(pages.LoginState{Step: step, Next: next}))
}

// renderLogin renders only the login card for htmx, or the full page otherwise.
func (s *Server) renderLogin(c fiber.Ctx, status int, st pages.LoginState) error {
	c.Status(status)
	if httpx.IsPartial(c) {
		return httpx.Render(c, pages.LoginCard(st))
	}
	httpx.SetMeta(c, seoNoIndex("Sign in"))
	return httpx.Render(c, pages.Login(st))
}

func (s *Server) loginStart(c fiber.Ctx) error {
	st := pages.LoginState{Step: "email", Email: strings.TrimSpace(c.FormValue("email")), Next: safeNext(c.FormValue("next"))}
	if c.FormValue("website") != "" { // honeypot: bots fill every field
		return s.renderLogin(c, fiber.StatusOK, pages.LoginState{Step: "code", Email: st.Email, Next: st.Next})
	}
	v := validate.New().Required("email", st.Email).Email("email", st.Email)
	if !v.OK() {
		st.Errors = v.Errors
		return s.renderLogin(c, fiber.StatusUnprocessableEntity, st)
	}
	if err := s.auth.StartEmailLogin(c.Context(), st.Email, c.IP()); err != nil {
		if errors.Is(err, auth.ErrRateLimited) {
			st.Errors = validate.Errors{"email": "Too many codes requested. Please wait and try again."}
			return s.renderLogin(c, fiber.StatusTooManyRequests, st)
		}
		return err
	}
	st.Step = "code"
	return s.renderLogin(c, fiber.StatusOK, st)
}

func (s *Server) loginVerify(c fiber.Ctx) error {
	st := pages.LoginState{Step: "code", Email: c.FormValue("email"), Next: safeNext(c.FormValue("next"))}
	code := strings.ReplaceAll(strings.TrimSpace(c.FormValue("code")), " ", "")
	u, created, err := s.auth.VerifyEmailCode(c.Context(), st.Email, code, c.IP())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCode) {
			st.Errors = validate.Errors{"code": "That code is wrong or has expired."}
			return s.renderLogin(c, fiber.StatusUnprocessableEntity, st)
		}
		return err
	}
	return s.finishLogin(c, u.ID, created, st.Next)
}

func (s *Server) loginPassword(c fiber.Ctx) error {
	st := pages.LoginState{Step: "password", Email: c.FormValue("email"), Next: safeNext(c.FormValue("next"))}
	u, err := s.auth.LoginPassword(c.Context(), st.Email, c.FormValue("password"), c.IP())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidLogin) || errors.Is(err, auth.ErrAccountLocked) {
			st.Errors = validate.Errors{"password": "Email or password is incorrect."}
			return s.renderLogin(c, fiber.StatusUnprocessableEntity, st)
		}
		return err
	}
	return s.finishLogin(c, u.ID, false, st.Next)
}

// magicLinkPage shows a button instead of signing in on GET: email security
// scanners pre-fetch links, which would otherwise burn the one-time token.
func (s *Server) magicLinkPage(c fiber.Ctx) error {
	httpx.SetMeta(c, seoNoIndex("Continue signing in"))
	return httpx.Render(c, pages.MagicLink(c.Query("t")))
}

func (s *Server) magicLink(c fiber.Ctx) error {
	u, created, err := s.auth.VerifyMagicLink(c.Context(), c.FormValue("t"), c.IP())
	if err != nil {
		httpx.SetFlash(c, "error", "That sign-in link is invalid or has expired. Request a new code.")
		return httpx.Redirect(c, "/login")
	}
	return s.finishLogin(c, u.ID, created, "/")
}

func (s *Server) finishLogin(c fiber.Ctx, userID string, created bool, next string) error {
	t, err := s.auth.StartSession(c.Context(), userID, c.Get(fiber.HeaderUserAgent), c.IP())
	if err != nil {
		return err
	}
	s.setAuthCookies(c, t)
	if created {
		httpx.SetFlash(c, "success", "Welcome! Your account is ready.")
		if next == "/" {
			next = "/account"
		}
	} else {
		httpx.SetFlash(c, "success", "You're signed in.")
	}
	return httpx.Redirect(c, next)
}

func (s *Server) logout(c fiber.Ctx) error {
	if rt := c.Cookies(s.cookieName("rt")); rt != "" {
		s.auth.Logout(c.Context(), rt)
	}
	s.clearAuthCookies(c)
	httpx.SetFlash(c, "info", "You have been signed out.")
	return httpx.Redirect(c, "/")
}
