package notify

import (
	"bytes"
	"fmt"
	"html/template"
)

// Email templates live in code: small, type-checked, and easy for an AI to find.
// Keep emails short and plain – they land in inboxes more reliably.

var layout = template.Must(template.New("layout").Parse(`<!doctype html>
<html lang="en"><body style="margin:0;background:#f4f4f5;font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:32px 16px">
<table role="presentation" width="100%" style="max-width:480px;background:#fff;border-radius:12px;padding:32px">
<tr><td style="font-size:20px;font-weight:600;color:#18181b;padding-bottom:16px">{{.App}}</td></tr>
<tr><td style="font-size:16px;line-height:1.5;color:#3f3f46">{{.Body}}</td></tr>
<tr><td style="font-size:12px;color:#a1a1aa;padding-top:24px">{{.Footer}}</td></tr>
</table></td></tr></table></body></html>`))

func render(app, footer string, body template.HTML) string {
	var b bytes.Buffer
	_ = layout.Execute(&b, map[string]any{"App": app, "Body": body, "Footer": footer})
	return b.String()
}

// LoginCode is sent for passwordless sign-in / sign-up and email verification.
func LoginCode(app, to, code, link string, minutes int) Email {
	body := template.HTML(fmt.Sprintf(
		`<p>Your sign-in code is:</p><p style="font-size:32px;font-weight:700;letter-spacing:6px;margin:16px 0">%s</p>`+
			`<p>Or <a href="%s">click here to sign in</a>. The code expires in %d minutes.</p>`,
		template.HTMLEscapeString(code), template.HTMLEscapeString(link), minutes))
	return Email{
		To:      to,
		Subject: fmt.Sprintf("%s is your %s code", code, app),
		Text:    fmt.Sprintf("Your %s sign-in code is %s\n\nOr open: %s\n\nIt expires in %d minutes. If you did not request this, ignore this email.", app, code, link, minutes),
		HTML:    render(app, "If you did not request this, you can ignore this email.", body),
	}
}

// AccountDeleted confirms erasure (GDPR Art. 17) – sent before the address is destroyed.
func AccountDeleted(app, to string) Email {
	return Email{
		To:      to,
		Subject: "Your " + app + " account has been deleted",
		Text:    "Your account and personal data have been deleted. This is the last email you will receive from us.",
		HTML:    render(app, "", "<p>Your account and personal data have been deleted. This is the last email you will receive from us.</p>"),
	}
}

// SecurityNotice tells the user about a sensitive change (new password, refresh-token reuse…).
func SecurityNotice(app, to, what string) Email {
	return Email{
		To:      to,
		Subject: "Security notice for your " + app + " account",
		Text:    what + "\n\nIf this was not you, sign in and use \"Sign out everywhere\", then contact support.",
		HTML:    render(app, "", template.HTML("<p>"+template.HTMLEscapeString(what)+"</p><p>If this was not you, sign in and use “Sign out everywhere”, then contact support.</p>")),
	}
}
