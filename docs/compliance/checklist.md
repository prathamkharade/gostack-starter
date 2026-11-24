# Launch checklist (privacy, security, accessibility)

## Legal texts & records
- [ ] Privacy notice (`views/pages/legal.templ` → Privacy): controller identity & contact, purposes + lawful
      bases table, recipients/processors, transfers, retention, rights, complaint right, (India: grievance officer).
- [ ] Terms of service; for consumers: withdrawal right info, easy cancellation, price display rules.
- [ ] Record of processing activities (ROPA) – start from `data-catalog.md` §5 / `app-data-map.md`.
- [ ] DPA signed with every processor: hosting (Hetzner/OVH/…), email (SES/Brevo/Resend/Postmark), SMS, storage (R2/B2), error tracking.
- [ ] Non-EU processor? Check EU-US Data Privacy Framework certification or SCCs + transfer assessment.
- [ ] DPIA if: special-category data at scale, systematic monitoring, children, new tech/AI decisions about people.
- [ ] EU/UK representative (GDPR Art. 27) if you have no establishment there but target those users.
- [ ] Bump `TermsVersion` / `PrivacyVersion` in `internal/auth` when texts change materially; notify users.

## Security
- [ ] `APP_ENV=production`, `COOKIE_SECURE=true`, https `BASE_URL`, fresh secrets (not the dev ones).
- [ ] Secrets backed up in a vault/password manager, separately from backups.
- [ ] Server hardened (`deploy/server-setup.sh`): SSH keys only, firewall, unattended upgrades.
- [ ] Backups run daily, are encrypted, copied off-site, and a **restore was tested**.
- [ ] `make lint` (govulncheck) clean; Dependabot enabled; image rebuilt at least monthly.
- [ ] security.txt contact works; incident runbook (`incident-response.md`) owner assigned.
- [ ] Admin accounts use a password + email code; least privilege for staff.
- [ ] Rate limits fit expected traffic; uptime monitor on `/readyz`.

## Accessibility (EAA / WCAG 2.2 AA)
- [ ] `make test-e2e` green (axe: no serious/critical issues).
- [ ] Manual: keyboard-only walkthrough, screen reader smoke test (NVDA/VoiceOver), 200% zoom, mobile.
- [ ] Accessibility statement updated (`/accessibility`) with known limitations and contact.

## Product
- [ ] Only strictly necessary cookies (else: consent banner first – see `cookies-and-tracking.md`).
- [ ] Email deliverability: SPF, DKIM, DMARC set for your sending domain.
- [ ] `AI_CRAWLERS` policy chosen; robots disallows private paths; sitemap lists public pages.
- [ ] Children: if the service could appeal to under-16s, decide on age gate + parental consent (do a DPIA).
