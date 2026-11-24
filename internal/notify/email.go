// Package notify sends email (any SMTP provider) and SMS (Twilio, Vonage, or a
// log driver for development). Providers are chosen purely by config: a
// developer only fills in .env values. Sending happens in background jobs
// (internal/jobs) so requests never wait on a provider.
package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Email is a fully rendered message.
type Email struct {
	To      string
	Subject string
	Text    string
	HTML    string // optional
}

type Mailer interface {
	Send(ctx context.Context, e Email) error
}

// SMTPMailer works with Amazon SES, Brevo, Resend, Postmark, Mailgun, Mailpit…
type SMTPMailer struct {
	Host, User, Pass string
	Port             int
	TLS              string // "starttls" (587), "tls" (465), "none" (dev only)
	From             string // "Name <addr@domain>"
}

func (m *SMTPMailer) Send(ctx context.Context, e Email) error {
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("notify: bad MAIL_FROM: %w", err)
	}
	to, err := mail.ParseAddress(e.To)
	if err != nil {
		return fmt.Errorf("notify: bad recipient: %w", err)
	}
	msg, err := BuildMIME(from, to, e, time.Now())
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
	d := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	if m.TLS == "tls" {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	}
	c, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if m.TLS == "starttls" {
		if err := c.StartTLS(&tls.Config{ServerName: m.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if m.User != "" {
		if err := c.Auth(smtp.PlainAuth("", m.User, m.Pass, m.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(to.Address); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// BuildMIME renders a standards-compliant multipart/alternative message.
func BuildMIME(from, to *mail.Address, e Email, now time.Time) ([]byte, error) {
	if strings.ContainsAny(e.Subject, "\r\n") {
		return nil, errors.New("notify: header injection in subject")
	}
	var b bytes.Buffer
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", from.String())
	h("To", to.String())
	h("Subject", mime.QEncoding.Encode("utf-8", e.Subject))
	h("Date", now.Format(time.RFC1123Z))
	h("Message-ID", "<"+randomID()+"@"+domainOf(from.Address)+">")
	h("MIME-Version", "1.0")
	h("Auto-Submitted", "auto-generated") // transactional: suppress auto-replies

	if e.HTML == "" {
		h("Content-Type", `text/plain; charset="utf-8"`)
		h("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		return append(b.Bytes(), qp(e.Text)...), nil
	}
	boundary := "b_" + randomID()
	h("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	for _, part := range []struct{ ct, body string }{{"text/plain", e.Text}, {"text/html", e.HTML}} {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString(`Content-Type: ` + part.ct + `; charset="utf-8"` + "\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.Write(qp(part.body))
		b.WriteString("\r\n")
	}
	b.WriteString("--" + boundary + "--\r\n")
	return b.Bytes(), nil
}

func qp(s string) []byte {
	var b bytes.Buffer
	w := quotedprintable.NewWriter(&b)
	_, _ = w.Write([]byte(strings.ReplaceAll(s, "\n", "\r\n")))
	_ = w.Close()
	return b.Bytes()
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func domainOf(addr string) string {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return addr[i+1:]
	}
	return "localhost"
}
