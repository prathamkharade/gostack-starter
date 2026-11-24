# Email, OTP and SMS

All sending goes through jobs (`email.send`, `sms.send`) – requests never wait for providers; failures retry
with backoff. Configure by filling `.env` only.

## Email (SMTP – works with every major provider)
| Provider | Price (Nov 2026) | Settings |
|---|---|---|
| Amazon SES | $0.10 / 1,000 – cheapest at volume | `email-smtp.<region>.amazonaws.com:587 starttls` |
| Brevo | 300/day free | `smtp-relay.brevo.com:587 starttls` |
| Resend | 3,000/month free (100/day) | `smtp.resend.com:465 tls`, user `resend` |
| Postmark | 100/month free; best deliverability | `smtp.postmarkapp.com:587 starttls` |
| Mailpit (dev) | local | `localhost:1025 none` – inbox at http://localhost:8025 |
Set SPF, DKIM and DMARC DNS records from your provider before launch or codes land in spam.
Templates: `internal/notify/templates.go` (plain text + minimal HTML, short subject with the code first).

## OTP / sign-in
Email code (6 digits) + magic link, 10-minute expiry, 5 attempts, 5 sends/hour/address, codes stored as HMAC.
This is free and phishing-resistant enough for most apps. Add TOTP or passkeys for admins.

## SMS (optional)
`SMS_PROVIDER=twilio|vonage|log`. Prices vary a lot by country (cents to >€0.10 per message) and SMS is
vulnerable to SIM-swap and "SMS pumping" fraud: keep the per-target rate limit, consider geo restrictions in
the provider console, and only use SMS where a verified phone is a product requirement.

## Adding a channel (WhatsApp, push, Telegram…)
Implement a sender like `notify.Twilio`, register a job kind in `cmd/app/main.go`, enqueue from services.
