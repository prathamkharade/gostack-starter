package web

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"app/internal/auth"
	"app/internal/database"
	"app/internal/httpx"
	"app/internal/validate"
	"app/internal/views/pages"
)

func (s *Server) accountData(c fiber.Ctx) (pages.AccountData, error) {
	p := httpx.User(c)
	u, err := s.auth.GetUser(c.Context(), p.UserID)
	if err != nil {
		return pages.AccountData{}, err
	}
	sessions, err := s.auth.ActiveSessions(c.Context(), p.UserID)
	if err != nil {
		return pages.AccountData{}, err
	}
	consents, err := s.auth.ConsentState(c.Context(), p.UserID)
	if err != nil {
		return pages.AccountData{}, err
	}
	return pages.AccountData{User: u, Sessions: sessions, Consents: consents, CurrentSession: p.SessionID, Errors: validate.Errors{}}, nil
}

func (s *Server) accountPage(c fiber.Ctx) error {
	d, err := s.accountData(c)
	if database.IsNotFound(err) { // account deleted on another device
		s.clearAuthCookies(c)
		return httpx.Redirect(c, "/login")
	}
	if err != nil {
		return err
	}
	httpx.SetMeta(c, seoNoIndex("Your account"))
	return httpx.Render(c, pages.Account(d))
}

func (s *Server) updateProfile(c fiber.Ctx) error {
	name := c.FormValue("name")
	locale := c.FormValue("locale", "en")
	v := validate.New().MaxLen("name", name, 100).OneOf("locale", locale, "en", "de", "fr", "es", "it", "nl", "pl", "pt")
	if !v.OK() {
		d, err := s.accountData(c)
		if err != nil {
			return err
		}
		d.Errors = v.Errors
		c.Status(fiber.StatusUnprocessableEntity)
		return httpx.Render(c, pages.ProfileForm(d))
	}
	if err := s.auth.UpdateProfile(c.Context(), httpx.UserID(c), name, locale); err != nil {
		return err
	}
	httpx.Toast(c, "success", "Profile saved.")
	if !httpx.IsHTMX(c) {
		return httpx.Redirect(c, "/account")
	}
	d, err := s.accountData(c)
	if err != nil {
		return err
	}
	return httpx.Render(c, pages.ProfileForm(d))
}

func (s *Server) setPassword(c fiber.Ctx) error {
	pw, confirm := c.FormValue("password"), c.FormValue("confirm")
	if pw != confirm {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "Passwords do not match.")
	}
	if err := s.auth.SetPassword(c.Context(), httpx.UserID(c), pw, c.IP()); err != nil {
		if errors.Is(err, auth.ErrWeakPassword) {
			return fiber.NewError(fiber.StatusUnprocessableEntity, "Use at least 10 characters.")
		}
		return err
	}
	httpx.Toast(c, "success", "Password saved.")
	return httpx.Redirect(c, "/account")
}

func (s *Server) phoneStart(c fiber.Ctx) error {
	phone := c.FormValue("phone")
	if !validate.New().Phone("phone", phone).OK() {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "Enter your number in international format, e.g. +447700900123.")
	}
	if err := s.auth.StartPhoneVerify(c.Context(), httpx.UserID(c), phone); err != nil {
		if errors.Is(err, auth.ErrRateLimited) {
			return fiber.NewError(fiber.StatusTooManyRequests, err.Error())
		}
		return err
	}
	return httpx.Render(c, pages.PhoneCodeForm(phone))
}

func (s *Server) phoneVerify(c fiber.Ctx) error {
	if err := s.auth.VerifyPhone(c.Context(), httpx.UserID(c), c.FormValue("phone"), c.FormValue("code"), c.IP()); err != nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "That code is wrong or has expired.")
	}
	httpx.Toast(c, "success", "Phone number verified.")
	return httpx.Redirect(c, "/account")
}

func (s *Server) updateConsents(c fiber.Ctx) error {
	granted := c.FormValue("marketing_email") == "on"
	if err := s.auth.Consent(c.Context(), httpx.UserID(c), "marketing_email", "v1", granted, c.IP()); err != nil {
		return err
	}
	httpx.Toast(c, "success", "Preferences saved.")
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *Server) logoutAll(c fiber.Ctx) error {
	if err := s.auth.LogoutEverywhere(c.Context(), httpx.UserID(c), c.IP()); err != nil {
		return err
	}
	s.clearAuthCookies(c)
	httpx.SetFlash(c, "info", "You have been signed out on all devices.")
	return httpx.Redirect(c, "/login")
}

// exportData implements the right of access / portability (GDPR Art. 15 & 20).
func (s *Server) exportData(c fiber.Ctx) error {
	b, err := s.privacy.Export(c.Context(), httpx.UserID(c), c.IP())
	if err != nil {
		return err
	}
	c.Attachment("my-data-" + time.Now().Format("2006-01-02") + ".json")
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
	return c.Send(b)
}

// deleteAccount implements the right to erasure (GDPR Art. 17).
func (s *Server) deleteAccount(c fiber.Ctx) error {
	if c.FormValue("confirm") != "DELETE" {
		return fiber.NewError(fiber.StatusUnprocessableEntity, `Type DELETE to confirm.`)
	}
	if err := s.privacy.Delete(c.Context(), httpx.UserID(c), c.IP()); err != nil {
		return err
	}
	s.clearAuthCookies(c)
	httpx.SetFlash(c, "info", "Your account and personal data have been deleted.")
	return httpx.Redirect(c, "/")
}
